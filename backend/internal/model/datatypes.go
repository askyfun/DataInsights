package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ============================================================
// 标准化数据类型定义
// ============================================================

// StandardDataType 定义标准化数据类型枚举
type StandardDataType string

const (
	// 数值类型
	TypeFloat   StandardDataType = "float"   // 浮点数（含小数/精度数值）
	TypeInteger StandardDataType = "integer" // 整数
	TypeBoolean StandardDataType = "boolean" // 布尔值

	// 文本类型
	TypeString StandardDataType = "string" // 字符串

	// 日期时间类型
	TypeDate     StandardDataType = "date"     // 日期
	TypeDateTime StandardDataType = "datetime" // 日期时间

	// 复杂类型
	TypeArray StandardDataType = "array" // 数组
	TypeMap   StandardDataType = "map"   // 字典（JSON 对象等）

	// 兼容占位：历史词表中的 unknown/json，经 NormalizeStandardType 折叠后不再产生
	TypeUnknown StandardDataType = "unknown"
	TypeJSON    StandardDataType = "json"
)

// StandardDataTypes 所有支持的标准化类型（对外规范词表，不含兼容占位）
var StandardDataTypes = []StandardDataType{
	TypeFloat,
	TypeInteger,
	TypeBoolean,
	TypeString,
	TypeDate,
	TypeDateTime,
	TypeArray,
	TypeMap,
}

// NormalizeStandardType 将任意存量/输入类型值归一为规范词表：
// number→float、json→map，unknown/空/未识别一律折叠为 string。
func NormalizeStandardType(s string) StandardDataType {
	switch StandardDataType(strings.TrimSpace(strings.ToLower(s))) {
	case TypeFloat, StandardDataType("number"):
		return TypeFloat
	case TypeInteger:
		return TypeInteger
	case TypeBoolean:
		return TypeBoolean
	case TypeDate:
		return TypeDate
	case TypeDateTime:
		return TypeDateTime
	case TypeArray:
		return TypeArray
	case TypeMap, TypeJSON:
		return TypeMap
	default:
		return TypeString
	}
}

// TypeCategory 类型的分类
var TypeCategory = map[StandardDataType]string{
	TypeFloat:     "数值",
	TypeInteger:   "数值",
	TypeBoolean:   "数值",
	TypeString:    "文本",
	TypeDate:      "日期时间",
	TypeDateTime:  "日期时间",
	TypeArray:     "复杂类型",
	TypeMap:       "复杂类型",
}

// ============================================================
// 类型配置
// ============================================================

// TypeConfig 类型配置（用于精度控制）
type TypeConfig struct {
	Precision int              `json:"precision,omitempty"`  // 总位数（如 10）
	Scale     int              `json:"scale,omitempty"`      // 小数位数（如 2）
	ItemType  StandardDataType `json:"item_type,omitempty"`  // 数组元素类型
	KeyType   StandardDataType `json:"key_type,omitempty"`   // 字典键类型
	ValueType StandardDataType `json:"value_type,omitempty"` // 字典值类型
}

// GetDisplayName 获取类型的显示名称
func (t StandardDataType) GetDisplayName() string {
	names := map[StandardDataType]string{
		TypeFloat:   "浮点数",
		TypeInteger: "整数",
		TypeBoolean:  "布尔值",
		TypeString:   "字符串",
		TypeDate:     "日期",
		TypeDateTime: "日期时间",
		TypeArray:    "数组",
		TypeMap:      "字典",
	}
	if name, ok := names[t]; ok {
		return name
	}
	return string(t)
}

// IsNumeric 判断是否为数值类型
func (t StandardDataType) IsNumeric() bool {
	return t == TypeFloat || t == TypeInteger
}

// IsDateTime 判断是否为日期时间类型
func (t StandardDataType) IsDateTime() bool {
	return t == TypeDate || t == TypeDateTime
}

// IsComplex 判断是否为复杂类型
func (t StandardDataType) IsComplex() bool {
	return t == TypeArray || t == TypeMap
}

// ============================================================
// 数据源类型映射器接口
// ============================================================

// DataTypeMapper 数据类型映射器接口
type DataTypeMapper interface {
	// ToStandard 将数据源类型转换为标准类型
	ToStandard(sourceType string) (StandardDataType, TypeConfig, error)

	// ToSource 将标准类型转换为数据源类型
	ToSource(stdType StandardDataType, config TypeConfig) string

	// GetSourceName 获取数据源名称
	GetSourceName() string
}

// ============================================================
// StarRocks 类型映射器
// ============================================================

// StarRocksMapper StarRocks 类型映射器
type StarRocksMapper struct{}

var starRocksTypeMap = map[string]StandardDataType{
	// 数值类型
	"tinyint":   TypeInteger,
	"smallint":  TypeInteger,
	"int":       TypeInteger,
	"integer":   TypeInteger,
	"bigint":    TypeInteger,
	"largeint":  TypeInteger,
	"float":     TypeFloat,
	"double":    TypeFloat,
	"decimal":   TypeFloat,
	"decimalv2": TypeFloat,
	"decimal32": TypeFloat,
	"decimal64": TypeFloat,
	"decimal128": TypeFloat,

	// 布尔类型
	"bool":    TypeBoolean,
	"boolean": TypeBoolean,

	// 字符串类型
	"char":      TypeString,
	"varchar":   TypeString,
	"string":    TypeString,
	"binary":    TypeString,
	"varbinary": TypeString,

	// 复杂类型
	"array": TypeArray,
	"map":   TypeMap,
	"json":  TypeMap,

	// 日期时间类型
	"date":      TypeDate,
	"datetime":  TypeDateTime,
	"timestamp": TypeDateTime,
}

// parsePrecisionConfig 从 decimal(10,2) 形式中提取精度配置；无精度时返回零值。
func parsePrecisionConfig(sourceType string) TypeConfig {
	idx := strings.Index(sourceType, "(")
	end := strings.LastIndex(sourceType, ")")
	if idx < 0 || end <= idx {
		return TypeConfig{}
	}
	parts := strings.Split(sourceType[idx+1:end], ",")
	if len(parts) != 2 {
		return TypeConfig{}
	}
	precision, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	scale, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || precision <= 0 || scale < 0 {
		return TypeConfig{}
	}
	return TypeConfig{Precision: precision, Scale: scale}
}

func (m *StarRocksMapper) ToStandard(sourceType string) (StandardDataType, TypeConfig, error) {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))

	// 剥掉 NOT NULL 之类修饰后缀
	if idx := strings.IndexAny(sourceType, " \t"); idx > 0 {
		sourceType = strings.TrimSpace(sourceType[:idx])
	}

	// 处理带参数的数值类型 (如 int(11), tinyint(2))
	if idx := strings.Index(sourceType, "("); idx > 0 {
		baseType := sourceType[:idx]
		if stdType, ok := starRocksTypeMap[baseType]; ok {
			return stdType, parsePrecisionConfig(sourceType), nil
		}
	}

	// 处理带参数的复杂类型
	if strings.HasPrefix(sourceType, "array<") {
		return TypeArray, TypeConfig{}, nil
	}
	if strings.HasPrefix(sourceType, "map<") {
		return TypeMap, TypeConfig{}, nil
	}

	stdType, ok := starRocksTypeMap[sourceType]
	if !ok {
		// 失配折叠为 string（fail-open 到最通用类型）
		return TypeString, TypeConfig{}, nil
	}
	return stdType, TypeConfig{}, nil
}

func (m *StarRocksMapper) ToSource(stdType StandardDataType, config TypeConfig) string {
	switch stdType {
	case TypeFloat:
		if config.Precision > 0 && config.Scale > 0 {
			return fmt.Sprintf("decimal(%d,%d)", config.Precision, config.Scale)
		}
		return "double"
	case TypeInteger:
		return "bigint"
	case TypeBoolean:
		return "boolean"
	case TypeString:
		return "varchar"
	case TypeDate:
		return "date"
	case TypeDateTime:
		return "datetime"
	case TypeArray:
		return "array"
	case TypeMap:
		return "map"
	default:
		return "varchar"
	}
}

func (m *StarRocksMapper) GetSourceName() string {
	return "starrocks"
}

// ============================================================
// PostgreSQL 类型映射器
// ============================================================

// PostgreSQLMapper PostgreSQL 类型映射器
type PostgreSQLMapper struct{}

var postgresTypeMap = map[string]StandardDataType{
	// 数值类型
	"smallint":         TypeInteger,
	"smallserial":      TypeInteger,
	"integer":          TypeInteger,
	"int":              TypeInteger,
	"serial":           TypeInteger,
	"bigint":           TypeInteger,
	"bigserial":        TypeInteger,
	"real":             TypeFloat,
	"double":           TypeFloat,
	"double precision": TypeFloat,
	"numeric":          TypeFloat,
	"decimal":          TypeFloat,
	"money":            TypeFloat,

	// 布尔类型
	"boolean": TypeBoolean,
	"bool":    TypeBoolean,

	// 字符串类型
	"char":              TypeString,
	"character":         TypeString,
	"varchar":           TypeString,
	"character varying": TypeString,
	"text":              TypeString,
	"uuid":              TypeString,
	"bytea":             TypeString,
	"time":              TypeString,
	"time without time zone":      TypeString,

	// 日期时间类型
	"date":     TypeDate,
	"timestamp": TypeDateTime,
	"timestamp without time zone": TypeDateTime,
	"timestamp with time zone":    TypeDateTime,
	"timestamptz":                 TypeDateTime,
	"timetz":                      TypeString,
	"time with time zone":         TypeString,
	"interval":                    TypeString,

	// 复杂类型
	"json":  TypeMap,
	"jsonb": TypeMap,
}

func (m *PostgreSQLMapper) ToStandard(sourceType string) (StandardDataType, TypeConfig, error) {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))

	// 处理 array 形式：integer[] / character varying[] 等
	if strings.HasSuffix(sourceType, "[]") || strings.HasPrefix(sourceType, "array") {
		return TypeArray, TypeConfig{}, nil
	}

	// 处理 numeric(p,s) / decimal(p,s)
	if strings.HasPrefix(sourceType, "numeric") || strings.HasPrefix(sourceType, "decimal") {
		return TypeFloat, parsePrecisionConfig(sourceType), nil
	}

	stdType, ok := postgresTypeMap[sourceType]
	if !ok {
		// 失配折叠为 string（fail-open 到最通用类型）
		return TypeString, TypeConfig{}, nil
	}
	return stdType, TypeConfig{}, nil
}

func (m *PostgreSQLMapper) ToSource(stdType StandardDataType, config TypeConfig) string {
	switch stdType {
	case TypeFloat:
		if config.Precision > 0 {
			return fmt.Sprintf("numeric(%d,%d)", config.Precision, config.Scale)
		}
		return "numeric"
	case TypeInteger:
		return "bigint"
	case TypeBoolean:
		return "boolean"
	case TypeString:
		return "varchar"
	case TypeDate:
		return "date"
	case TypeDateTime:
		return "timestamp"
	case TypeArray:
		return "array"
	case TypeMap:
		return "jsonb"
	default:
		return "varchar"
	}
}

func (m *PostgreSQLMapper) GetSourceName() string {
	return "postgresql"
}

// ============================================================
// MySQL 类型映射器
// ============================================================

// MySQLMapper MySQL 类型映射器
type MySQLMapper struct{}

var mysqlTypeMap = map[string]StandardDataType{
	// 数值类型
	"tinyint":   TypeInteger,
	"smallint":  TypeInteger,
	"mediumint": TypeInteger,
	"int":       TypeInteger,
	"integer":   TypeInteger,
	"bigint":    TypeInteger,
	"float":     TypeFloat,
	"double":    TypeFloat,
	"real":      TypeFloat,
	"decimal":   TypeFloat,
	"dec":       TypeFloat,
	"numeric":   TypeFloat,
	"fixed":     TypeFloat,

	// 布尔类型（MySQL 用 tinyint(1) 表示）
	"bool":       TypeBoolean,
	"boolean":    TypeBoolean,
	"tinyint(1)": TypeBoolean,

	// 字符串类型
	"char":       TypeString,
	"varchar":    TypeString,
	"tinytext":   TypeString,
	"text":       TypeString,
	"mediumtext": TypeString,
	"longtext":   TypeString,
	"binary":     TypeString,
	"varbinary":  TypeString,
	"blob":       TypeString,
	"enum":       TypeString,
	"set":        TypeString,
	"json":       TypeMap,

	// 日期时间类型
	"date":      TypeDate,
	"datetime":  TypeDateTime,
	"timestamp": TypeDateTime,
	"time":      TypeString,
	"year":      TypeInteger,
}

func (m *MySQLMapper) ToStandard(sourceType string) (StandardDataType, TypeConfig, error) {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))

	// 剥掉 unsigned/zerofill 等修饰后缀（bigint unsigned / int(11) unsigned）
	if idx := strings.Index(sourceType, " "); idx > 0 {
		sourceType = sourceType[:idx]
	}

	// 处理 tinyint(1) 作为布尔
	if sourceType == "tinyint(1)" {
		return TypeBoolean, TypeConfig{}, nil
	}

	// 处理 decimal(p,s) 或 numeric(p,s)
	if strings.HasPrefix(sourceType, "decimal") || strings.HasPrefix(sourceType, "numeric") || strings.HasPrefix(sourceType, "dec") || strings.HasPrefix(sourceType, "fixed") {
		return TypeFloat, parsePrecisionConfig(sourceType), nil
	}

	// 处理 enum 和 set
	if strings.HasPrefix(sourceType, "enum") || strings.HasPrefix(sourceType, "set") {
		return TypeString, TypeConfig{}, nil
	}

	// 处理 char(n) / varchar(n)
	if strings.HasPrefix(sourceType, "char") || strings.HasPrefix(sourceType, "varchar") {
		return TypeString, TypeConfig{}, nil
	}

	stdType, ok := mysqlTypeMap[sourceType]
	if !ok {
		// 再剥掉参数形式重试（int(11) → int）
		if idx := strings.Index(sourceType, "("); idx > 0 {
			stdType, ok = mysqlTypeMap[sourceType[:idx]]
		}
	}
	if !ok {
		// 失配折叠为 string（fail-open 到最通用类型）
		return TypeString, TypeConfig{}, nil
	}
	return stdType, TypeConfig{}, nil
}

func (m *MySQLMapper) ToSource(stdType StandardDataType, config TypeConfig) string {
	switch stdType {
	case TypeFloat:
		if config.Precision > 0 && config.Scale > 0 {
			return fmt.Sprintf("decimal(%d,%d)", config.Precision, config.Scale)
		}
		return "decimal"
	case TypeInteger:
		return "bigint"
	case TypeBoolean:
		return "tinyint(1)"
	case TypeString:
		return "varchar"
	case TypeDate:
		return "date"
	case TypeDateTime:
		return "datetime"
	case TypeArray:
		return "json"
	case TypeMap:
		return "json"
	default:
		return "varchar"
	}
}

func (m *MySQLMapper) GetSourceName() string {
	return "mysql"
}

// ============================================================
// ClickHouse 类型映射器
// ============================================================

// ClickHouseMapper ClickHouse 类型映射器
type ClickHouseMapper struct{}

var clickHouseTypeMap = map[string]StandardDataType{
	// 数值类型
	"int8":    TypeInteger,
	"int16":   TypeInteger,
	"int32":   TypeInteger,
	"int64":   TypeInteger,
	"int128":  TypeInteger,
	"int256":  TypeInteger,
	"uint8":   TypeInteger,
	"uint16":  TypeInteger,
	"uint32":  TypeInteger,
	"uint64":  TypeInteger,
	"uint128": TypeInteger,
	"uint256": TypeInteger,
	"float32": TypeFloat,
	"float64": TypeFloat,
	"decimal": TypeFloat,

	// 布尔类型
	"bool": TypeBoolean,

	// 字符串类型
	"string":      TypeString,
	"fixedstring": TypeString,
	"uuid":        TypeString,
	"ipv4":        TypeString,
	"ipv6":        TypeString,

	// 日期时间类型
	"date":       TypeDate,
	"date32":     TypeDate,
	"datetime":   TypeDateTime,
	"datetime64": TypeDateTime,

	// 复杂类型
	"array":          TypeArray,
	"map":            TypeMap,
	"tuple":          TypeArray,
	"nested":         TypeMap,
	"json":           TypeMap,
	"object":         TypeMap,
	"object('json')": TypeMap,
}

func (m *ClickHouseMapper) ToStandard(sourceType string) (StandardDataType, TypeConfig, error) {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))

	// 剥掉 Nullable(String) / LowCardinality(String) 包装（可循环嵌套）
	for {
		if inner, ok := unwrapWrapper(sourceType, "nullable"); ok {
			sourceType = inner
			continue
		}
		if inner, ok := unwrapWrapper(sourceType, "lowcardinality"); ok {
			sourceType = inner
			continue
		}
		break
	}

	// 处理 decimal(p,s)
	if strings.HasPrefix(sourceType, "decimal") {
		return TypeFloat, parsePrecisionConfig(sourceType), nil
	}

	// 处理 array(T)
	if strings.HasPrefix(sourceType, "array(") || sourceType == "array" {
		return TypeArray, TypeConfig{}, nil
	}

	// 处理 map(K,V)
	if strings.HasPrefix(sourceType, "map(") || sourceType == "map" {
		return TypeMap, TypeConfig{}, nil
	}

	// 处理 Tuple(...)
	if strings.HasPrefix(sourceType, "tuple") {
		return TypeArray, TypeConfig{}, nil
	}

	// 处理 Nested(...) — 结构上等价于对象
	if strings.HasPrefix(sourceType, "nested") {
		return TypeMap, TypeConfig{}, nil
	}

	// 处理 DateTime64(p[, tz])
	if strings.HasPrefix(sourceType, "datetime64") {
		return TypeDateTime, TypeConfig{}, nil
	}

	// 处理 Object('json')
	if strings.HasPrefix(sourceType, "object") {
		return TypeMap, TypeConfig{}, nil
	}

	// 处理 FixedString(n)
	if strings.HasPrefix(sourceType, "fixedstring") {
		return TypeString, TypeConfig{}, nil
	}

	stdType, ok := clickHouseTypeMap[sourceType]
	if !ok {
		// 失配折叠为 string（fail-open 到最通用类型）
		return TypeString, TypeConfig{}, nil
	}
	return stdType, TypeConfig{}, nil
}

// unwrapWrapper 剥掉 wrapper(inner) 形式的外壳，返回内部类型。
func unwrapWrapper(sourceType, wrapper string) (string, bool) {
	if !strings.HasPrefix(sourceType, wrapper+"(") || !strings.HasSuffix(sourceType, ")") {
		return "", false
	}
	return strings.TrimSpace(sourceType[len(wrapper)+1 : len(sourceType)-1]), true
}

func (m *ClickHouseMapper) ToSource(stdType StandardDataType, config TypeConfig) string {
	switch stdType {
	case TypeFloat:
		if config.Precision > 0 && config.Scale > 0 {
			return fmt.Sprintf("decimal(%d,%d)", config.Precision, config.Scale)
		}
		return "float64"
	case TypeInteger:
		return "int64"
	case TypeBoolean:
		return "bool"
	case TypeString:
		return "string"
	case TypeDate:
		return "date"
	case TypeDateTime:
		return "datetime"
	case TypeArray:
		return "array"
	case TypeMap:
		return "map"
	default:
		return "string"
	}
}

func (m *ClickHouseMapper) GetSourceName() string {
	return "clickhouse"
}

// ============================================================
// 工厂函数
// ============================================================

// NewDataTypeMapper 创建数据类型映射器
func NewDataTypeMapper(sourceType string) (DataTypeMapper, error) {
	switch strings.ToLower(sourceType) {
	case "starrocks":
		return &StarRocksMapper{}, nil
	case "postgresql", "postgres":
		return &PostgreSQLMapper{}, nil
	case "mysql":
		return &MySQLMapper{}, nil
	case "clickhouse":
		return &ClickHouseMapper{}, nil
	default:
		return nil, fmt.Errorf("unsupported datasource type: %s", sourceType)
	}
}

// ============================================================
// 虚拟字段表达式
// ============================================================

// ExpressionType 表达式类型
type ExpressionType string

const (
	ExprArithmetic  ExpressionType = "arithmetic"  // 算术运算
	ExprString      ExpressionType = "string"      // 字符串操作
	ExprDateTime    ExpressionType = "datetime"    // 日期时间操作
	ExprConditional ExpressionType = "conditional" // 条件判断
	ExprAggregate   ExpressionType = "aggregate"   // 聚合函数
	ExprCustom      ExpressionType = "custom"      // 自定义 SQL
)

// VirtualField 虚拟字段定义
type VirtualField struct {
	Name       string           `json:"name"`        // 字段名称
	Expression string           `json:"expression"`  // 表达式（如：price * quantity）
	ExprType   ExpressionType   `json:"expr_type"`   // 表达式类型
	ResultType StandardDataType `json:"result_type"` // 结果数据类型
	Config     TypeConfig       `json:"config"`      // 类型配置
}

// SupportedExpression 支持的表达式函数
var SupportedExpressions = map[string]StandardDataType{
	// 聚合函数
	"SUM":   TypeFloat,
	"AVG":   TypeFloat,
	"COUNT": TypeInteger,
	"MAX":   TypeFloat,
	"MIN":   TypeFloat,

	// 字符串操作
	"CONCAT":    TypeString,
	"UPPER":     TypeString,
	"LOWER":     TypeString,
	"TRIM":      TypeString,
	"SUBSTRING": TypeString,
	"LENGTH":    TypeInteger,
	"LENGTHB":   TypeInteger,

	// 日期时间操作
	"YEAR":          TypeInteger,
	"MONTH":         TypeInteger,
	"DAY":           TypeInteger,
	"DATE_ADD":      TypeDateTime,
	"DATE_SUB":      TypeDateTime,
	"DATEDIFF":      TypeInteger,
	"TIMESTAMPDIFF": TypeInteger,

	// 数值操作
	"ROUND": TypeFloat,
	"ABS":   TypeFloat,
	"FLOOR": TypeInteger,
	"CEIL":  TypeInteger,
	"POWER": TypeFloat,
	"MOD":   TypeInteger,

	// 条件判断
	"IF":       TypeUnknown, // 取决于返回值
	"CASE":     TypeUnknown,
	"COALESCE": TypeUnknown,
	"NULLIF":   TypeUnknown,
}

// InferExpressionResultType 推断表达式的结果类型
func InferExpressionResultType(expr string) StandardDataType {
	upperExpr := strings.ToUpper(expr)

	// 检查已知函数。必须按排序后的名字遍历：map 迭代序随机，
	// 同一表达式（如 ABS(MONTH(x))）每次调用可能命中不同函数而返回不同类型（非纯函数）。
	funcNames := make([]string, 0, len(SupportedExpressions))
	for name := range SupportedExpressions {
		funcNames = append(funcNames, name)
	}
	sort.Strings(funcNames)
	for _, funcName := range funcNames {
		resultType := SupportedExpressions[funcName]
		if strings.Contains(upperExpr, funcName) {
			if resultType != TypeUnknown {
				return resultType
			}
		}
	}

	// 默认推断
	// 如果包含字符串连接操作符，推断为字符串
	if strings.Contains(expr, "||") {
		return TypeString
	}
	// 如果包含算术运算符，推断为数值
	arithmeticOps := []string{"+", "-", "*", "/", "%"}
	for _, op := range arithmeticOps {
		if strings.Contains(expr, op) {
			return TypeFloat
		}
	}

	// 如果是简单字段引用，无法推断
	return TypeUnknown
}
