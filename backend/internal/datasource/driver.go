package datasource

import (
	"context"
	"fmt"
	"regexp"
)

// DriverType represents the type of database driver
type DriverType string

const (
	DriverPostgreSQL DriverType = "postgresql"
	DriverClickHouse DriverType = "clickhouse"
	DriverMySQL      DriverType = "mysql"
	DriverStarRocks  DriverType = "starrocks"
)

// ConnectionConfig holds the connection configuration for a datasource
type ConnectionConfig struct {
	Host         string
	Port         int
	DatabaseName string
	Username     string
	Password     string
}

// TableInfo represents a table in the database
type TableInfo struct {
	Name    string
	Comment string
}

// ColumnInfo represents a column in a table
type ColumnInfo struct {
	Name    string
	Type    string
	Comment string
}

// QueryResult represents the result of a query
type QueryResult struct {
	Columns []string
	Rows    []map[string]interface{}
}

// Driver is the interface that all database drivers must implement
type Driver interface {
	// Type returns the driver type
	Type() DriverType

	// Connect establishes a connection to the database
	Connect(ctx context.Context, config ConnectionConfig) (Connection, error)

	// TestConnection tests if the connection is valid
	TestConnection(ctx context.Context, config ConnectionConfig) error
}

// Connection represents a database connection
type Connection interface {
	// Close closes the connection
	Close() error

	// Ping pings the database to check connection
	Ping(ctx context.Context) error

	// GetTables returns the list of tables in the database
	GetTables(ctx context.Context) ([]TableInfo, error)

	// GetColumns returns the list of columns for a table
	GetColumns(ctx context.Context, tableName string) ([]ColumnInfo, error)

	// GetPrimaryKeys returns the primary key column names for a table
	GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error)

	// Execute executes a query with positional args (? placeholders).
	Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error)

	// Capabilities 返回该连接的方言查询能力。当前（Task 0-7）各驱动返回静态 stub 值，
	// 真实探针逻辑在 Task 3-0 补充。
	Capabilities(ctx context.Context) (*DialectCapabilities, error)
}

// NewDriver creates a new driver instance based on the driver type
func NewDriver(driverType DriverType) (Driver, error) {
	switch driverType {
	case DriverPostgreSQL:
		return NewPostgreSQLDriver(), nil
	case DriverClickHouse:
		return NewClickHouseDriver(), nil
	case DriverMySQL:
		return NewMySQLDriver(), nil
	case DriverStarRocks:
		return NewStarRocksDriver(), nil
	default:
		return nil, fmt.Errorf("unsupported driver type: %s", driverType)
	}
}


// IsValidIdentifier checks if a SQL identifier contains only safe characters [a-zA-Z0-9_.]
func IsValidIdentifier(name string) bool {
	if name == "" {
		return false
	}
	re := regexp.MustCompile(`^[a-zA-Z0-9_.]+$`)
	return re.MatchString(name)
}

// PostgreSQL OID 常量
const (
	TextOID     int32 = 25  // text
	VarcharOID  int32 = 1043 // varchar
	BPCharOID   int32 = 1042 // char
	ByteaOID    int32 = 17  // bytea
)

// TextTypeNames MySQL/StarRocks 文本类型名称
var TextTypeNames = map[string]bool{
	"CHAR":         true,
	"VARCHAR":      true,
	"TINYTEXT":     true,
	"TEXT":         true,
	"MEDIUMTEXT":   true,
	"LONGTEXT":     true,
	"ENUM":         true,
	"SET":          true,
}

// NonBinaryNamedTypes MySQL/StarRocks 以 []byte 返回文本表示、但**不是**二进制的类型名称。
// 不列进来会被 convertValue 当成二进制，JSON 序列化成 base64（StarRocks DECIMAL 实测踩坑：
// 103.5125 变成 "MTAzLjUxMjUwMDA="）。
var NonBinaryNamedTypes = map[string]bool{
	"DECIMAL":    true,
	"NEWDECIMAL": true,
	"FLOAT":      true,
	"DOUBLE":     true,
	"REAL":       true,
	"TINYINT":    true,
	"SMALLINT":   true,
	"MEDIUMINT":  true,
	"INT":        true,
	"INTEGER":    true,
	"BIGINT":     true,
	"LARGEINT":   true,
	"YEAR":       true,
	"DATE":       true,
	"TIME":       true,
	"DATETIME":   true,
	"TIMESTAMP":  true,
	"JSON":       true,
}

// BinaryTypeNames 二进制类型名称
var BinaryTypeNames = map[string]bool{
	"TINYBLOB":    true,
	"BLOB":        true,
	"MEDIUMBLOB":  true,
	"LONGBLOB":    true,
	"BINARY":      true,
	"VARBINARY":   true,
	"BIT":         true,
	"BYTEA":       true,
}

// convertValue converts []byte to string only for text types
// This prevents JSON serialization of binary data as base64
func convertValue(val interface{}, fieldType interface{}) interface{} {
	byteVal, isByteSlice := val.([]byte)
	if !isByteSlice {
		return val
	}

	// 根据字段类型判断是否为文本类型
	isTextType := false

	if oid, ok := fieldType.(int32); ok {
		// PostgreSQL: 使用 OID 判断
		isTextType = oid == TextOID || oid == VarcharOID || oid == BPCharOID
	} else if typeName, ok := fieldType.(string); ok {
		// MySQL/StarRocks: 使用类型名称判断
		upperType := ""
		for _, c := range typeName {
			if c >= 'A' && c <= 'Z' {
				upperType += string(c)
			}
		}
		isTextType = TextTypeNames[upperType] || NonBinaryNamedTypes[upperType]
	}

	if isTextType {
		return string(byteVal)
	}

	// 二进制类型保持 []byte，JSON 会序列化为 base64
	return val
}