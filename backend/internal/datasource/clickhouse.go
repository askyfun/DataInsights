package datasource

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type clickhouseDriver struct{}

func NewClickHouseDriver() Driver {
	return &clickhouseDriver{}
}

func (d *clickhouseDriver) Type() DriverType {
	return DriverClickHouse
}

func (d *clickhouseDriver) Connect(ctx context.Context, config ConnectionConfig) (Connection, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", config.Host, config.Port)},
		Auth: clickhouse.Auth{
			Database: config.DatabaseName,
			Username: config.Username,
			Password: config.Password,
		},
	})
	if err != nil {
		return nil, err
	}
	return &clickhouseConnection{conn: conn}, nil
}

func (d *clickhouseDriver) TestConnection(ctx context.Context, config ConnectionConfig) error {
	conn, err := d.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Ping(ctx)
}

type clickhouseConnection struct {
	conn driver.Conn
}

func (c *clickhouseConnection) Close() error {
	return c.conn.Close()
}

func (c *clickhouseConnection) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}

func (c *clickhouseConnection) GetTables(ctx context.Context) ([]TableInfo, error) {
	rows, err := c.conn.Query(ctx, "SELECT name, comment FROM system.tables WHERE database = currentDatabase() ORDER BY name")
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

func (c *clickhouseConnection) GetColumns(ctx context.Context, tableName string) ([]ColumnInfo, error) {
	if !IsValidIdentifier(tableName) {
		return nil, fmt.Errorf("invalid table name: %q", tableName)
	}
	query := fmt.Sprintf("SELECT name, type, comment FROM system.columns WHERE table = '%s' AND database = currentDatabase() ORDER BY position", tableName)
	rows, err := c.conn.Query(ctx, query)
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
func (c *clickhouseConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	if !IsValidIdentifier(tableName) {
		return nil, fmt.Errorf("invalid table name: %s", tableName)
	}
	query := fmt.Sprintf("SELECT name FROM system.columns WHERE table = '%s' AND database = currentDatabase() AND is_in_primary_key = 1 ORDER BY position", tableName)
	rows, err := c.conn.Query(ctx, query)
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

func (c *clickhouseConnection) Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error) {
	rows, err := c.conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := rows.Columns()
	var results []map[string]interface{}

	rowScan := make([]interface{}, len(columns))
	for i := range rowScan {
		rowScan[i] = new(interface{})
	}

	for rows.Next() {
		if err := rows.Scan(rowScan...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{})
		for i, col := range columns {
			v := *rowScan[i].(*interface{})
			// ClickHouse: 转换为 string（保守方案，ClickHouse 主要用于 OLAP）
			if bv, ok := v.([]byte); ok {
				row[col] = string(bv)
			} else {
				row[col] = v
			}
		}
		results = append(results, row)
	}

	return &QueryResult{
		Columns: columns,
		Rows:    results,
	}, nil
}

// Capabilities 返回 ClickHouse 的静态能力值（本会话无 CH 实例，未运行探针）。
// 按失败模式非对称处理（Task 3-0，Q3 ruling）：
//   - GROUPING SETS / 窗口函数保持 true：CH 21.x+ 支持是文档化事实，若与实际不符，
//     失败模式是显式 SQL 报错（loud），不会静默返回错误数据。注意：这两项是
//     "文档支持、未运行验证"，有实例后应补探针实测。
//   - percentile_cont 保持 false：CH 没有 percentile_cont 函数，是事实而非保守。
//   - PercentileStrategy 保守 "unsupported"：quantilesExactInclusive 的插值语义若与
//     预期 Type-7 不符会静默返回错误的 Q1/Q3 数值（silent wrong data），故未实测前
//     不声明。TODO(Task 3-x)：有真实 CH 实例后用探针 SQL 实测再翻转：
//     SELECT quantilesExactInclusive(0.25, 0.5, 0.75)(number) FROM (SELECT 1 AS number) t
func (c *clickhouseConnection) Capabilities(ctx context.Context) (*DialectCapabilities, error) {
	return &DialectCapabilities{
		SupportsGroupingSets:    true,
		SupportsPercentileCont:  false,
		SupportsWindowFunctions: true,
		PercentileStrategy:      "unsupported",
	}, nil
}
