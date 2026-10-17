package extract

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// SelfProvisionedDatasourceName 是自举出来的抽取存储数据源名。
//
// 它会出现在 bi_datasource 里（因此能被 GET /api/datasources/:id 单独取到），
// 但列表接口会把它藏掉（extract.Guard 的既有口径）。名字带括号是为了让任何
// 直接查库的人都一眼看出它是平台自建的，而不是用户建的数据源。
const SelfProvisionedDatasourceName = "data-insights internal storage (upload files)"

// selfProvisionKey 是 bi_setting 里记住自举结果的键。
//
// 为什么不每次启动都重新推导：这个数据源行的 id 会写进每一个 extract 模式的
// bi_dataset.datasource_id。重启时若换了一个新行，存量上传数据集就会指向一个
// 空表——图表能打开、数据全空，是最难排查的一类故障。所以 id 必须稳定。
const selfProvisionKey = "extract.self_provisioned_datasource_id"

// SelfProvisioner 装配抽取存储：显式配置优先，否则自举。
type SelfProvisioner struct {
	// DB 是应用自身的数据库连接（迁移已跑完）。
	DB *sql.DB
	// DatabaseURL 是应用自身的连接串，自举时从中解析连接参数。
	DatabaseURL string
	// ConfiguredID 是 EXTRACT_DATASOURCE_ID 的值；>0 表示用户显式指定。
	ConfiguredID int
}

// Resolve 返回本次进程应当使用的抽取存储 Guard。
//
// 三种情形：
//  1. 用户显式配了 EXTRACT_DATASOURCE_ID → 原样采用，不碰数据库；
//  2. 未配但能自举 → 落/复用一条隐藏数据源行，返回 SelfProvisioned 的 Guard；
//  3. 未配且自举失败 → 返回零值 Guard（Enabled() 为 false），上传入口据此
//     置灰并给出引导，而不是让用户在 500 里猜。
//
// 自举失败不 fail 整个进程：抽取存储是旁路能力，平台其余部分（已有数据源、
// 仪表盘、分享）不依赖它，一个数据库权限问题不该让整个服务起不来。
func Resolve(ctx context.Context, p SelfProvisioner) Guard {
	if p.ConfiguredID > 0 {
		return Guard{DatasourceID: p.ConfiguredID}
	}

	id, err := p.provision(ctx)
	if err != nil {
		slog.Error("Local file upload will be unavailable: could not prepare the internal upload storage",
			"error", err,
			"hint", "upload storage falls back to this app's own database (DATABASE_URL); "+
				"fix that connection or set EXTRACT_DATASOURCE_ID")
		return Guard{}
	}
	slog.Info("Local file upload storage ready (using this app's own database)",
		"datasource_id", id, "name", SelfProvisionedDatasourceName)
	return Guard{DatasourceID: id, SelfProvisioned: true}
}

// provision 落一条（或复用已存在的）抽取存储数据源行，返回它的 id。
func (p SelfProvisioner) provision(ctx context.Context) (int, error) {
	if p.DB == nil {
		return 0, fmt.Errorf("no database handle")
	}
	host, port, dbname, user, password, err := ParsePostgresTarget(p.DatabaseURL)
	if err != nil {
		return 0, err
	}

	// 先问 bi_setting：自举过一次就直接复用，保证存量数据集的指向不漂。
	if id, ok := p.loadPersistedID(ctx); ok {
		if existing, err := p.datasourceExists(ctx, id); err == nil && existing {
			// SECURITY_KEY 可能已被换掉（既有运维动作）。自举存储的口令是从
			// DATABASE_URL 推导出来的、**永远可再生**，所以这里顺手把它与当前
			// 连接串对齐：解不开就用新解析出的明文重写。
			//
			// 不做这一步的代价：管理员换 key 后，上传功能静默失效，而这条数据源
			// 在列表里是隐藏的、用户没有界面能看到它、更没有界面能修它——只会看到
			// 「上传坏了」。用户自建的数据源连不通可以自己排查，平台自建的这条不行。
			if err := p.reconcilePassword(ctx, id, password); err != nil {
				slog.Warn("Could not reconcile the upload storage password after a key change",
					"datasource_id", id, "error", err,
					"hint", "local file upload will fail until DATABASE_URL is reachable and matches the stored host")
			}
			return id, nil
		}
		// 行没了（例如有人清理了数据源）：落回自举路径重建，
		// 但旧数据集已经指向那个 id，这里不做迁移——文档会写明
		// 「不要删除内部存储数据源」。
		slog.Warn("Persisted upload storage datasource is gone; recreating it",
			"datasource_id", id)
	}

	id, err := p.insertDatasource(ctx, host, port, dbname, user, password)
	if err != nil {
		return 0, err
	}
	if err := p.persistID(ctx, id); err != nil {
		// 记不住 id 不致命：本次进程已拿到 id，抽取守卫正常生效；
		// 只是下次启动会再建一行。上传本身不受影响。
		slog.Warn("Could not record the upload storage datasource id", "datasource_id", id, "error", err)
	}
	return id, nil
}

// loadPersistedID 读回上次自举出的数据源 id。
func (p SelfProvisioner) loadPersistedID(ctx context.Context) (int, bool) {
	var raw string
	err := p.DB.QueryRowContext(ctx,
		`SELECT value FROM bi_setting WHERE key = $1`, selfProvisionKey).Scan(&raw)
	if err != nil {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// persistID 把自举出的 id 记进 bi_setting。
func (p SelfProvisioner) persistID(ctx context.Context, id int) error {
	_, err := p.DB.ExecContext(ctx,
		`INSERT INTO bi_setting (key, value, updated_at) VALUES ($1, $2, now())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		selfProvisionKey, strconv.Itoa(id))
	return err
}

// datasourceExists 判断目标数据源行是否还在（未软删）。
func (p SelfProvisioner) datasourceExists(ctx context.Context, id int) (bool, error) {
	var n int
	err := p.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM bi_datasource WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// insertDatasource 落一条指向应用自身数据库的数据源行。
func (p SelfProvisioner) insertDatasource(
	ctx context.Context, host string, port int, dbname, user, password string,
) (int, error) {
	var id int
	err := p.DB.QueryRowContext(ctx,
		`INSERT INTO bi_datasource
		   (name, type, host, port, database_name, username, password, created_at, updated_at)
		 VALUES ($1, 'postgresql', $2, $3, $4, $5, $6, now(), now())
		 RETURNING id`,
		SelfProvisionedDatasourceName, host, port, dbname, user, password).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create the internal upload storage datasource: %w", err)
	}
	return id, nil
}

// reconcilePassword 让自举存储的口令与当前 DATABASE_URL 对齐。
//
// 它写**明文**：既有的口令解析路径（ResolvePassword）见到明文会用当前
// SECURITY_KEY 自动加密并写回（那是issue #118 就有的 legacy 升级机制），
// 所以这里不需要、也不应该自己持有密钥。
//
// 只在口令确实对不上时才写：解不开或值不同才 UPDATE，避免每次启动都无谓写库。
func (p SelfProvisioner) reconcilePassword(ctx context.Context, id int, password string) error {
	var stored string
	err := p.DB.QueryRowContext(ctx,
		`SELECT password FROM bi_datasource WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&stored)
	if err != nil {
		return fmt.Errorf("failed to read the upload storage password: %w", err)
	}
	if stored == password {
		return nil
	}
	if _, err := p.DB.ExecContext(ctx,
		`UPDATE bi_datasource SET password = $1, updated_at = now() WHERE id = $2`,
		password, id); err != nil {
		return fmt.Errorf("failed to rewrite the upload storage password: %w", err)
	}
	slog.Info("Re-wrote the upload storage password (SECURITY_KEY was likely rotated)",
		"datasource_id", id)
	return nil
}

// ParsePostgresTarget 把一条 PostgreSQL 连接串拆成数据源行需要的字段。
//
// 它不复用 pgconn.ParseConfig 的结果类型，而是自己取字段：数据源行的 password
// 列明文存储（由既有的加密路径负责落密文），这里只需要拿到明文交给上层加密，
// 因此需要保留 password 本身而非 pgconn 那种「已脱敏」的 Config。
//
// url.Parse 失败时退回 pgconn 的 DSN 解析（`host=... port=...` 形式），
// 两种写法都要能读——部署手册里两种都出现过。
func ParsePostgresTarget(rawURL string) (host string, port int, dbname, user, password string, err error) {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return "", 0, "", "", "", fmt.Errorf("DATABASE_URL is empty")
	}

	if u, perr := url.Parse(raw); perr == nil && u.Scheme != "" && strings.Contains(raw, "://") {
		host = u.Hostname()
		port = 5432
		if ps := u.Port(); ps != "" {
			if p, cerr := strconv.Atoi(ps); cerr == nil {
				port = p
			}
		}
		dbname = strings.TrimPrefix(u.Path, "/")
		user = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			password = pw
		}
	} else {
		cfg, cerr := pgconn.ParseConfig(raw)
		if cerr != nil {
			return "", 0, "", "", "", fmt.Errorf("cannot parse DATABASE_URL: %w", cerr)
		}
		host = cfg.Host
		port = int(cfg.Port)
		dbname = cfg.Database
		user = cfg.User
		password = cfg.Password
	}

	if host == "" {
		return "", 0, "", "", "", fmt.Errorf("DATABASE_URL has no host")
	}
	if dbname == "" {
		return "", 0, "", "", "", fmt.Errorf("DATABASE_URL has no database name")
	}
	return host, port, dbname, user, password, nil
}