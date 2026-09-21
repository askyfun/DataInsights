package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

type starRocksDriver struct{}

func NewStarRocksDriver() Driver {
	return &starRocksDriver{}
}

func (d *starRocksDriver) Type() DriverType {
	return DriverStarRocks
}

func (d *starRocksDriver) Connect(ctx context.Context, config ConnectionConfig) (Connection, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		config.Username, config.Password, config.Host, config.Port, config.DatabaseName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return &starRocksConnection{db: db}, nil
}

func (d *starRocksDriver) TestConnection(ctx context.Context, config ConnectionConfig) error {
	conn, err := d.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Ping(ctx)
}

type starRocksConnection struct {
	db *sql.DB
	// capsOnce/caps：方言能力懒探针缓存，连接生命周期内只探测一次（见 probeCapabilities）。
	capsOnce sync.Once
	caps     *DialectCapabilities
}

func (c *starRocksConnection) Close() error {
	return c.db.Close()
}

func (c *starRocksConnection) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

func (c *starRocksConnection) GetTables(ctx context.Context) ([]TableInfo, error) {
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

func (c *starRocksConnection) GetColumns(ctx context.Context, tableName string) ([]ColumnInfo, error) {
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
func (c *starRocksConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
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

func (c *starRocksConnection) Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error) {
	rows, err := c.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// 获取字段类型信息
	columnTypes, err := rows.ColumnTypes()
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
			// 智能类型转换：只有文本类型才转换为 string
			row[col] = convertValue(values[i], columnTypes[i].DatabaseTypeName())
		}
		results = append(results, row)
	}

	return &QueryResult{
		Columns: columns,
		Rows:    results,
	}, nil
}

// Capabilities 懒探针 + 缓存（复用 PG 模式）。
func (c *starRocksConnection) Capabilities(ctx context.Context) (*DialectCapabilities, error) {
	c.capsOnce.Do(func() {
		c.caps = c.probeCapabilities(ctx)
	})
	return c.caps, nil
}

// probeCapabilities 从**保守基线**出发，仅对能"响亮失败"的布尔能力跑只读探针，
// **只升不降**，故未接实例时与升级前的静态值完全一致。
//   - PercentileStrategy "percentile_cont_args_first"：2026-09-19 已在真实
//     StarRocks 实例实测为精确百分位，作为基线保留、**不再重探**（避免探针
//     语法抖动误降级一个已验证能力）。
//   - GROUPING SETS：StarRocks 较新版本支持，基线 false——跑探针，成功才升 true
//     （升 true 后 pivot 走 GROUPING SETS 而非 UNION ALL 回退）。
//   - 窗口函数：跑探针，成功才升 true。
func (c *starRocksConnection) probeCapabilities(ctx context.Context) *DialectCapabilities {
	caps := &DialectCapabilities{
		SupportsGroupingSets:    false,
		SupportsPercentileCont:  true,
		SupportsWindowFunctions: false,
		PercentileStrategy:      "percentile_cont_args_first",
	}
	// nil-pool 兜底：zero-value 连接（无实例）返回基线，与升级前静态值一致。
	if c.db == nil {
		return caps
	}
	if _, err := c.Execute(ctx, `SELECT GROUPING(x) FROM (SELECT 1 AS x) t GROUP BY GROUPING SETS ((x), ())`); err != nil {
		slog.Warn("starrocks capabilities probe: GROUPING SETS unavailable", "error", err)
	} else {
		caps.SupportsGroupingSets = true
	}
	if _, err := c.Execute(ctx, `SELECT ROW_NUMBER() OVER (ORDER BY 1) FROM (SELECT 1 AS x) t`); err != nil {
		slog.Warn("starrocks capabilities probe: window functions unavailable", "error", err)
	} else {
		caps.SupportsWindowFunctions = true
	}
	return caps
}
