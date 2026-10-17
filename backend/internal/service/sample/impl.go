// Package sample implements the built-in sample dataset (R-79 / issue #174):
// a one-click seed that gives a fresh deployment something to analyze before
// the user has any database to connect.
//
// The data itself is ordinary PostgreSQL tables (di_sample_*, seeded by
// migration 00012) inside the application database. Loading registers a hidden
// datasource (`is_sample`) pointing at that database plus two datasets with
// pre-typed columns, so the chart builder works without any further setup.
// No new driver is involved.
package sample

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"data-insights/internal/crypto"
	"data-insights/internal/database"
	"data-insights/internal/domain/entity"
	"data-insights/internal/idgen"
	"data-insights/internal/model"

	"github.com/uptrace/bun"
)

const (
	// SettingDismissed persists the onboarding-card "ignore" choice. The
	// bi_setting key-value store already exists (keystore); no new table.
	SettingDismissed = "sample_dataset_dismissed"

	datasourceName      = "示例数据源（内置）"
	ordersDatasetName   = "示例：电商订单"
	productsDatasetName = "示例：商品目录"
	sampleDescription   = "内置示例数据，可一键移除"
)

// sampleTag marks both sample datasets in the list (visible badge).
var sampleTag = `["sample"]`

// Status is the onboarding card state. Loaded and Dismissed are independent:
// dismissing before loading hides the card until the data is loaded, and
// removing loaded sample data brings the card back unless it was dismissed.
type Status struct {
	Loaded    bool `json:"loaded"`
	Dismissed bool `json:"dismissed"`
}

type Service interface {
	Status(ctx context.Context) (*Status, error)
	Load(ctx context.Context) (*Status, error)
	Dismiss(ctx context.Context) (*Status, error)
	Remove(ctx context.Context) error

	// SetSecurityKey injects the 32-byte AES key used to encrypt the sample
	// datasource password at rest (nil = plaintext passthrough).
	SetSecurityKey(key []byte)
}

type service struct {
	db          *bun.DB
	key         []byte
	databaseURL string
}

// NewService creates the sample service. databaseURL is the application's own
// DATABASE_URL: the sample datasource points back at the same PostgreSQL.
func NewService(db *bun.DB, databaseURL string) Service {
	return &service{db: db, databaseURL: databaseURL}
}

func (s *service) SetSecurityKey(key []byte) {
	s.key = key
}

func (s *service) Status(ctx context.Context) (*Status, error) {
	var loaded bool
	if err := s.db.NewRaw(
		`SELECT EXISTS (SELECT 1 FROM bi_datasource WHERE is_sample = TRUE AND deleted_at IS NULL)`,
	).Scan(ctx, &loaded); err != nil {
		return nil, fmt.Errorf("check sample datasource: %w", err)
	}

	var value string
	err := s.db.NewRaw(`SELECT value FROM bi_setting WHERE key = ?`, SettingDismissed).Scan(ctx, &value)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read %s: %w", SettingDismissed, err)
	}
	return &Status{Loaded: loaded, Dismissed: value == "true"}, nil
}

func (s *service) Load(ctx context.Context) (*Status, error) {
	conn, err := parseDatabaseURL(s.databaseURL)
	if err != nil {
		return nil, err
	}

	err = database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		var exists bool
		if err := tx.NewRaw(
			`SELECT EXISTS (SELECT 1 FROM bi_datasource WHERE is_sample = TRUE AND deleted_at IS NULL)`,
		).Scan(ctx, &exists); err != nil {
			return fmt.Errorf("check sample datasource: %w", err)
		}
		if exists {
			return nil // idempotent
		}

		password := conn.password
		if s.key != nil && password != "" {
			encrypted, err := crypto.Encrypt(s.key, password)
			if err != nil {
				return fmt.Errorf("encrypt sample datasource password: %w", err)
			}
			password = encrypted
		}

		now := time.Now()
		ds := &model.Datasource{
			Name:         datasourceName,
			Type:         "postgresql",
			Host:         conn.host,
			Port:         conn.port,
			DatabaseName: conn.database,
			Username:     conn.username,
			Password:     password,
			IsSample:     true,
			CreatedAt:    sql.NullTime{Time: now, Valid: true},
			UpdatedAt:    sql.NullTime{Time: now, Valid: true},
			OwnerID:      sql.NullInt32{Int32: model.SystemOwnerID, Valid: true},
		}
		if _, err := tx.NewInsert().Model(ds).Returning("id").Exec(ctx); err != nil {
			return fmt.Errorf("create sample datasource: %w", err)
		}

		for _, def := range sampleDatasetDefinitions(ds.ID) {
			m := toDatasetModel(def, now)
			if _, err := tx.NewInsert().Model(m).Exec(ctx); err != nil {
				return fmt.Errorf("create sample dataset %q: %w", def.Name, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Status(ctx)
}

func (s *service) Dismiss(ctx context.Context) (*Status, error) {
	if _, err := s.db.NewRaw(
		`INSERT INTO bi_setting (key, value) VALUES (?, 'true')
		 ON CONFLICT (key) DO UPDATE SET value = 'true', updated_at = now()`,
		SettingDismissed,
	).Exec(ctx); err != nil {
		return nil, fmt.Errorf("persist %s: %w", SettingDismissed, err)
	}
	return s.Status(ctx)
}

// Remove soft-deletes the sample datasets (cascading their charts) and the
// sample datasource in one transaction. The seeded tables stay untouched:
// they are tiny, idempotent, and loading again simply reuses them.
func (s *service) Remove(ctx context.Context) error {
	return database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		var id int
		err := tx.NewRaw(
			`SELECT id FROM bi_datasource WHERE is_sample = TRUE AND deleted_at IS NULL ORDER BY id LIMIT 1`,
		).Scan(ctx, &id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // already removed
		}
		if err != nil {
			return fmt.Errorf("find sample datasource: %w", err)
		}

		if _, err := tx.NewUpdate().
			Model((*model.Chart)(nil)).
			Set("deleted_at = now()").
			Where("dataset_id IN (SELECT id FROM bi_dataset WHERE datasource_id = ?)", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("delete sample charts: %w", err)
		}
		if _, err := tx.NewUpdate().
			Model((*model.Dataset)(nil)).
			Set("deleted_at = now()").
			Where("datasource_id = ?", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("delete sample datasets: %w", err)
		}
		if _, err := tx.NewUpdate().
			Model((*model.Datasource)(nil)).
			Set("deleted_at = now()").
			Where("id = ?", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("delete sample datasource: %w", err)
		}
		return nil
	})
}

type connectionInfo struct {
	host     string
	port     int
	database string
	username string
	password string
}

// parseDatabaseURL extracts the pieces the datasource model can store from
// DATABASE_URL. Scheme/sslmode options are not representable in the datasource
// model (the PostgreSQL driver always dials with sslmode=disable), which is
// fine for the same-host deployments the sample data targets.
func parseDatabaseURL(raw string) (connectionInfo, error) {
	if strings.TrimSpace(raw) == "" {
		return connectionInfo{}, errors.New("DATABASE_URL is not configured")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return connectionInfo{}, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return connectionInfo{}, fmt.Errorf("unsupported DATABASE_URL scheme %q", u.Scheme)
	}

	info := connectionInfo{host: u.Hostname(), port: 5432, database: strings.TrimPrefix(u.Path, "/")}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			return connectionInfo{}, fmt.Errorf("parse DATABASE_URL port: %w", err)
		}
		info.port = port
	}
	if u.User != nil {
		info.username = u.User.Username()
		info.password, _ = u.User.Password()
	}
	if info.host == "" || info.database == "" {
		return connectionInfo{}, errors.New("DATABASE_URL must include a host and database name")
	}
	return info, nil
}

func sampleDatasetDefinitions(datasourceID int) []*entity.Dataset {
	ordersTable := "di_sample_orders"
	productsTable := "di_sample_products"
	description := sampleDescription

	return []*entity.Dataset{
		{
			Name:         ordersDatasetName,
			DatasourceID: datasourceID,
			TableName:    &ordersTable,
			QueryType:    "table",
			Mode:         "direct",
			Description:  &description,
			Tags:         sampleTag,
			Columns:      mustColumnsJSON(sampleColumns(ordersDatasetName)),
		},
		{
			Name:         productsDatasetName,
			DatasourceID: datasourceID,
			TableName:    &productsTable,
			QueryType:    "table",
			Mode:         "direct",
			Description:  &description,
			Tags:         sampleTag,
			Columns:      mustColumnsJSON(sampleColumns(productsDatasetName)),
		},
	}
}

func sampleColumns(datasetName string) []entity.DatasetColumn {
	orders := []entity.DatasetColumn{
		column("order_id", "integer", "dimension", "订单号"),
		column("order_date", "date", "dimension", "下单日期"),
		column("region", "string", "dimension", "销售区域"),
		column("category", "string", "dimension", "商品类目"),
		column("quantity", "integer", "metric", "购买数量"),
		column("amount", "float", "metric", "订单金额"),
		column("status", "string", "dimension", "订单状态"),
	}
	products := []entity.DatasetColumn{
		column("product_id", "integer", "dimension", "商品 ID"),
		column("product_name", "string", "dimension", "商品名称"),
		column("category", "string", "dimension", "商品类目"),
		column("price", "float", "metric", "售价"),
		column("stock", "integer", "metric", "库存"),
		column("launched_on", "date", "dimension", "上架日期"),
	}
	if datasetName == productsDatasetName {
		return products
	}
	return orders
}

func column(name, dataType, role, comment string) entity.DatasetColumn {
	return entity.DatasetColumn{
		ID:      idgen.New(),
		Name:    name,
		Expr:    name,
		Type:    dataType,
		Comment: comment,
		Role:    role,
	}
}

func mustColumnsJSON(columns []entity.DatasetColumn) string {
	buf, err := json.Marshal(columns)
	if err != nil {
		// 固定结构体的序列化不可能失败；真失败也应由调用方感知。
		panic(fmt.Sprintf("marshal sample columns: %v", err))
	}
	return string(buf)
}

func toDatasetModel(def *entity.Dataset, now time.Time) *model.Dataset {
	m := &model.Dataset{
		Name:         def.Name,
		DatasourceID: def.DatasourceID,
		QueryType:    def.QueryType,
		Mode:         def.Mode,
		Tags:         def.Tags,
		QualityRules: "[]",
		Columns:      def.Columns,
		ShardKeys:    "[]",
		CreatedAt:    sql.NullTime{Time: now, Valid: true},
		UpdatedAt:    sql.NullTime{Time: now, Valid: true},
		OwnerID:      sql.NullInt32{Int32: model.SystemOwnerID, Valid: true},
	}
	if def.TableName != nil {
		m.TableName = sql.NullString{String: *def.TableName, Valid: true}
	}
	if def.Description != nil {
		m.Description = sql.NullString{String: *def.Description, Valid: true}
	}
	return m
}
