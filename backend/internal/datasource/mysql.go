package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

type mysqlDriver struct{}

func NewMySQLDriver() Driver {
	return &mysqlDriver{}
}

func (d *mysqlDriver) Type() DriverType {
	return DriverMySQL
}

func (d *mysqlDriver) Connect(ctx context.Context, config ConnectionConfig) (Connection, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		config.Username, config.Password, config.Host, config.Port, config.DatabaseName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return &mysqlConnection{db: db}, nil
}

func (d *mysqlDriver) TestConnection(ctx context.Context, config ConnectionConfig) error {
	conn, err := d.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Ping(ctx)
}

type mysqlConnection struct {
	db *sql.DB
	// capsOnce/caps：方言能力懒探针缓存，连接生命周期内只探测一次（见 probeCapabilities）。
	capsOnce sync.Once
	caps     *DialectCapabilities
}

func (c *mysqlConnection) Close() error {
	return c.db.Close()
}

func (c *mysqlConnection) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

func (c *mysqlConnection) GetTables(ctx context.Context) ([]TableInfo, error) {
	query := `
		SELECT table_name, table_comment 
		FROM information_schema.tables 
		WHERE table_schema = DATABASE() 
		ORDER BY table_name`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			return nil, err
		}
		tables = append(tables, TableInfo{Name: name, Comment: comment})
	}
	return tables, nil
}

func (c *mysqlConnection) GetColumns(ctx context.Context, tableName string) ([]ColumnInfo, error) {
	query := `
		SELECT column_name, column_type, column_comment 
		FROM information_schema.columns 
		WHERE table_schema = DATABASE() AND table_name = ? 
		ORDER BY ordinal_position`

	rows, err := c.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var name, colType, comment string
		if err := rows.Scan(&name, &colType, &comment); err != nil {
			return nil, err
		}
		columns = append(columns, ColumnInfo{Name: name, Type: colType, Comment: comment})
	}
	return columns, nil
}

// GetPrimaryKeys returns the primary key columns for a table
func (c *mysqlConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	query := `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE() AND table_name = ? AND constraint_name = 'PRIMARY'
		ORDER BY ordinal_position`

	rows, err := c.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		keys = append(keys, name)
	}
	return keys, nil
}

func (c *mysqlConnection) Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error) {
	rows, err := c.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			// 将 []byte 转换为 string，避免 JSON 序列化为 base64
			if v, ok := values[i].([]byte); ok {
				row[col] = string(v)
			} else {
				row[col] = values[i]
			}
		}
		results = append(results, row)
	}

	return &QueryResult{
		Columns: columns,
		Rows:    results,
	}, nil
}

// Capabilities 懒探针 + 缓存（复用 PG 模式）：首次调用探测、sync.Once 缓存。
func (c *mysqlConnection) Capabilities(ctx context.Context) (*DialectCapabilities, error) {
	c.capsOnce.Do(func() {
		c.caps = c.probeCapabilities(ctx)
	})
	return c.caps, nil
}

// probeCapabilities 从**保守基线**出发，仅对能"响亮失败"的布尔能力跑只读探针，
// 且**只升不降**（探针失败保留基线，绝不降级），所以未接实例时行为与旧静态值完全一致。
//   - GROUPING SETS：基线 false 是文档事实（MySQL 无 GROUPING SETS，仅 WITH ROLLUP），
//     恒 false、无需探针。
//   - 窗口函数：MySQL 8+ 支持、5.7 不支持——跑探针区分，成功才升 true。
//   - PercentileStrategy 恒 "unsupported"：MySQL 无 percentile_cont，唯一候选
//     window_ntile 是窗口函数、无法作标量聚合表达式（见 query/percentile.go 注释），
//     故 boxplot 在 MySQL 上暂不支持。
func (c *mysqlConnection) probeCapabilities(ctx context.Context) *DialectCapabilities {
	caps := &DialectCapabilities{
		SupportsGroupingSets:    false,
		SupportsPercentileCont:  false,
		SupportsWindowFunctions: false,
		PercentileStrategy:      "unsupported",
	}
	// nil-pool 兜底：zero-value 连接（无实例）直接返回基线，与升级前的静态值一致。
	if c.db == nil {
		return caps
	}
	// 窗口函数（MySQL 8+）：失败模式 loud，安全只升不降。
	if _, err := c.Execute(ctx, `SELECT ROW_NUMBER() OVER (ORDER BY 1) FROM (SELECT 1 AS x) t`); err != nil {
		slog.Warn("mysql capabilities probe: window functions unavailable", "error", err)
	} else {
		caps.SupportsWindowFunctions = true
	}
	return caps
}
