package extract

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// errTestDBDown 是打桩里模拟数据库不可用的哨兵。
var errTestDBDown = errors.New("database is down")

// sqlmockResult 包一层 sqlmock.NewResult，让用例里的期望读起来是意图而非 API 细节。
func sqlmockResult(rows int) driver.Result { return sqlmock.NewResult(0, int64(rows)) }

// --- ParsePostgresTarget ---

// TestParsePostgresTargetURLForm 覆盖 URL 形态的 DATABASE_URL
// （部署手册与 compose 里最常见的写法）。
func TestParsePostgresTargetURLForm(t *testing.T) {
	host, port, dbname, user, password, err := ParsePostgresTarget(
		"postgres://insights:s3cr3t@db.internal:5433/analytics?sslmode=disable")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "db.internal" {
		t.Errorf("host = %q, want db.internal", host)
	}
	if port != 5433 {
		t.Errorf("port = %d, want 5433", port)
	}
	if dbname != "analytics" {
		t.Errorf("dbname = %q, want analytics", dbname)
	}
	if user != "insights" {
		t.Errorf("user = %q, want insights", user)
	}
	if password != "s3cr3t" {
		t.Errorf("password = %q, want s3cr3t", password)
	}
}

// TestParsePostgresTargetDefaultsPortTo5432 URL 不带端口时必须回落 5432，
// 否则自举出来的数据源会连不上（port=0 的连接串无人能连）。
func TestParsePostgresTargetDefaultsPortTo5432(t *testing.T) {
	_, port, _, _, _, err := ParsePostgresTarget("postgres://u:p@localhost/insights")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 5432 {
		t.Errorf("port = %d, want the 5432 default", port)
	}
}

// TestParsePostgresTargetKeywordForm 覆盖 DSN 形态（`host=... port=...`）。
// 两种写法在部署文档里都出现过，任何一种解析不了都会让自举在真实环境里失效。
func TestParsePostgresTargetKeywordForm(t *testing.T) {
	host, port, dbname, user, password, err := ParsePostgresTarget(
		"host=127.0.0.1 port=5432 user=insights password=insights123 dbname=insights sslmode=disable")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "127.0.0.1" || port != 5432 || dbname != "insights" ||
		user != "insights" || password != "insights123" {
		t.Errorf("keyword DSN parsed wrong: %q %d %q %q %q", host, port, dbname, user, password)
	}
}

// TestParsePostgresTargetPercentEncodedPassword 密码里的特殊字符在 URL 形态下是
// 百分号编码的；不解码就会存进数据源行一个连不上的错口令。
func TestParsePostgresTargetPercentEncodedPassword(t *testing.T) {
	_, _, _, _, password, err := ParsePostgresTarget(
		"postgres://u:p%40ss%3Aword@localhost:5432/db")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if password != "p@ss:word" {
		t.Errorf("password = %q, want the decoded p@ss:word", password)
	}
}

// TestParsePostgresTargetRejectsBadInput 坏输入必须报错而不是静默返回空 host——
// 空 host 会让自举落一条永远连不上的数据源行，比启动失败更难排查。
func TestParsePostgresTargetRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"empty":       "",
		"blank":       "   ",
		"no host":     "postgres:///insights",
		"garbage":     "://not a url at all",
		"no database": "postgres://u:p@localhost:5432/",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, _, _, err := ParsePostgresTarget(in); err == nil {
				t.Fatalf("%s must be rejected, got no error", name)
			}
		})
	}
}

// --- Resolve ---

// TestResolveKeepsExplicitConfiguration 显式配置优先：绝不碰数据库，
// 也就绝不覆盖用户显式指定的数据源。
func TestResolveKeepsExplicitConfiguration(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	// 没有排任何期望：一旦Resolve 碰了数据库，sqlmock 会让用例失败。
	g := Resolve(context.Background(), SelfProvisioner{
		DB: sqlDB, DatabaseURL: "postgres://u:p@localhost:5432/db", ConfiguredID: 7,
	})
	if !g.Enabled() || g.DatasourceID != 7 || g.SelfProvisioned {
		t.Fatalf("explicit config must win verbatim, got %+v", g)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("explicit config must not touch the database: %v", err)
	}
}

// TestResolveSelfProvisionsWhenUnconfigured 是本 issue 的启动期复现用例：
// 未配置时必须落一条数据源行并返回可用的 Guard（修复前直接返回零值 Guard）。
func TestResolveSelfProvisionsWhenUnconfigured(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	// bi_setting 未命中（首次部署）
	mock.ExpectQuery(`SELECT value FROM bi_setting`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}))
	// 落数据源行
	mock.ExpectQuery(`INSERT INTO bi_datasource`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
	// 记住 id
	mock.ExpectExec(`INSERT INTO bi_setting`).
		WillReturnResult(sqlmockResult(1))

	g := Resolve(context.Background(), SelfProvisioner{
		DB: sqlDB, DatabaseURL: "postgres://insights:pw@pg:5432/insights",
	})
	if !g.Enabled() {
		t.Fatal("self-provisioned storage must be enabled")
	}
	if !g.SelfProvisioned {
		t.Error("guard must be marked self-provisioned")
	}
	if g.DatasourceID != 12 {
		t.Errorf("datasource id = %d, want 12", g.DatasourceID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestResolveReusesPersistedDatasource 幂等：第二次启动必须复用同一个 id，
// 否则存量extract 数据集会指向一个新建的空表（图表能开、数据全空）。
func TestResolveReusesPersistedDatasource(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery(`SELECT value FROM bi_setting`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("12"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM bi_datasource`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	g := Resolve(context.Background(), SelfProvisioner{
		DB: sqlDB, DatabaseURL: "postgres://insights:pw@pg:5432/insights",
	})
	if g.DatasourceID != 12 {
		t.Fatalf("must reuse the persisted id 12, got %d", g.DatasourceID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("must not create a second datasource row: %v", err)
	}
}

// TestResolveRecreatesVanishedDatasource 数据源行被删后必须重建而不是卡死：
// 上传入口不能因为一行数据被清理就永久置灰。
func TestResolveRecreatesVanishedDatasource(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery(`SELECT value FROM bi_setting`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("12"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM bi_datasource`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`INSERT INTO bi_datasource`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(13))
	mock.ExpectExec(`INSERT INTO bi_setting`).
		WillReturnResult(sqlmockResult(1))

	g := Resolve(context.Background(), SelfProvisioner{
		DB: sqlDB, DatabaseURL: "postgres://insights:pw@pg:5432/insights",
	})
	if !g.Enabled() || g.DatasourceID != 13 {
		t.Fatalf("must recreate storage and use the new id, got %+v", g)
	}
}

// TestResolveDegradesGracefullyWhenDBFails 自举失败不得让整个进程起不来：
// 抽取存储是旁路能力，返回零值 Guard 让上传入口置灰即可，
// 平台其余部分（已有数据源 / 仪表盘 / 分享）不依赖它。
func TestResolveDegradesGracefullyWhenDBFails(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	mock.ExpectQuery(`SELECT value FROM bi_setting`).
		WillReturnError(errTestDBDown)

	g := Resolve(context.Background(), SelfProvisioner{
		DB: sqlDB, DatabaseURL: "postgres://insights:pw@pg:5432/insights",
	})
	if g.Enabled() {
		t.Fatal("a failed self-provision must yield a disabled guard, not a broken one")
	}
}

// TestResolveWithoutDBHandleNoHandle 缺连接时也必须降级而不是 panic。
func TestResolveWithoutDBHandleNoHandle(t *testing.T) {
	g := Resolve(context.Background(), SelfProvisioner{
		DatabaseURL: "postgres://insights:pw@pg:5432/insights",
	})
	if g.Enabled() {
		t.Fatal("no DB handle must degrade to a disabled guard")
	}
}

// TestResolveRewritesStoragePasswordAfterKeyRotation 钉住换 key 后的自愈：
// SECURITY_KEY 被换掉时，库里的存储口令是旧 key 加密的、必然解不开。
// 平台必须用重新解析出的明文重写它——否则上传功能静默失效，而这条数据源在
// 列表里是隐藏的，用户既看不到也修不了。
func TestResolveRewritesStoragePasswordAfterKeyRotation(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery(`SELECT value FROM bi_setting`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("12"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM bi_datasource`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// 读出库里存的口令（旧 key 加密的密文，与当前连接串推导出的明文不同）
	mock.ExpectQuery(`SELECT password FROM bi_datasource`).
		WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow("v1:9f8e7d旧密文"))
	// 必须重写
	mock.ExpectExec(`UPDATE bi_datasource SET password`).
		WillReturnResult(sqlmockResult(1))

	g := Resolve(context.Background(), SelfProvisioner{
		DB: sqlDB, DatabaseURL: "postgres://insights:pw@pg:5432/insights",
	})
	if !g.Enabled() || g.DatasourceID != 12 {
		t.Fatalf("a key rotation must not disable upload, got %+v", g)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestResolveSkipsPasswordWriteWhenUnchanged 幂等：口令本就一致时不该写库，
// 否则每次启动都产生一次无谓的 UPDATE 与 updated_at 抖动。
func TestResolveSkipsPasswordWriteWhenUnchanged(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery(`SELECT value FROM bi_setting`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("12"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM bi_datasource`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// 口令与连接串推导结果一致（明文尚未被加密升级的形态）
	mock.ExpectQuery(`SELECT password FROM bi_datasource`).
		WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow("pw"))
	// 没有排 UPDATE —— 一旦写了，ExpectationsWereMet 会因未期望的调用而失败。

	g := Resolve(context.Background(), SelfProvisioner{
		DB: sqlDB, DatabaseURL: "postgres://insights:pw@pg:5432/insights",
	})
	if !g.Enabled() {
		t.Fatal("storage must stay enabled")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("must not rewrite an unchanged password: %v", err)
	}
}