package datasource

import (
	"context"
	"database/sql"
	"fmt"

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

func (c *mysqlConnection) Capabilities(ctx context.Context) (*DialectCapabilities, error) {
	// TODO(Task 3-0): 未经真实实例探针验证，当前返回保守默认值（不支持），
	// 待有可用 MySQL 实例时按 plan §4.1 的探针 SQL 实测后翻转。
	return &DialectCapabilities{
		SupportsGroupingSets:    false,
		SupportsPercentileCont:  false,
		SupportsWindowFunctions: false,
		PercentileStrategy:      "unsupported",
	}, nil
}
