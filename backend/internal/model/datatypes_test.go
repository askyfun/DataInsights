package model

import (
	"testing"
)

func TestNormalizeStandardType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  StandardDataType
	}{
		{"legacy number", "number", TypeFloat},
		{"legacy json", "json", TypeMap},
		{"float", "float", TypeFloat},
		{"integer", "integer", TypeInteger},
		{"boolean", "boolean", TypeBoolean},
		{"string", "string", TypeString},
		{"date", "date", TypeDate},
		{"datetime", "datetime", TypeDateTime},
		{"array", "array", TypeArray},
		{"map", "map", TypeMap},
		{"unknown legacy", "unknown", TypeString},
		{"empty", "", TypeString},
		{"unrecognized", "uuid", TypeString},
		{"case insensitive", "DateTime", TypeDateTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeStandardType(tt.input); got != tt.want {
				t.Errorf("NormalizeStandardType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStarRocksMapper_ToStandard(t *testing.T) {
	m := &StarRocksMapper{}

	tests := []struct {
		name       string
		sourceType string
		want       StandardDataType
	}{
		// 数值类型
		{"int", "int", TypeInteger},
		{"bigint", "bigint", TypeInteger},
		{"tinyint", "tinyint", TypeInteger},
		{"smallint", "smallint", TypeInteger},
		{"largeint", "largeint", TypeInteger},
		{"float", "float", TypeFloat},
		{"double", "double", TypeFloat},
		{"decimal", "decimal", TypeFloat},
		{"decimal with params", "decimal(10,2)", TypeFloat},
		{"decimalv2", "decimalv2", TypeFloat},

		// 布尔类型
		{"bool", "bool", TypeBoolean},
		{"boolean", "boolean", TypeBoolean},

		// 字符串类型
		{"varchar", "varchar", TypeString},
		{"string", "string", TypeString},
		{"char", "char", TypeString},

		// 日期时间类型
		{"date", "date", TypeDate},
		{"datetime", "datetime", TypeDateTime},
		{"timestamp", "timestamp", TypeDateTime},

		// 复杂类型
		{"array", "array", TypeArray},
		{"array with params", "array<int>", TypeArray},
		{"map", "map", TypeMap},
		{"map with params", "map<string,int>", TypeMap},
		{"json", "json", TypeMap},

		// 失配折叠为 string
		{"unknown", "unknown_type", TypeString},
		{"empty", "", TypeString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := m.ToStandard(tt.sourceType)
			if err != nil {
				t.Errorf("ToStandard() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ToStandard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStarRocksMapper_ToSource(t *testing.T) {
	m := &StarRocksMapper{}

	tests := []struct {
		name    string
		stdType StandardDataType
		config  TypeConfig
		want    string
	}{
		{"float default", TypeFloat, TypeConfig{}, "double"},
		{"float with precision", TypeFloat, TypeConfig{Precision: 10, Scale: 2}, "decimal(10,2)"},
		{"integer", TypeInteger, TypeConfig{}, "bigint"},
		{"boolean", TypeBoolean, TypeConfig{}, "boolean"},
		{"string", TypeString, TypeConfig{}, "varchar"},
		{"date", TypeDate, TypeConfig{}, "date"},
		{"datetime", TypeDateTime, TypeConfig{}, "datetime"},
		{"array", TypeArray, TypeConfig{}, "array"},
		{"map", TypeMap, TypeConfig{}, "map"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.ToSource(tt.stdType, tt.config)
			if got != tt.want {
				t.Errorf("ToSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgreSQLMapper_ToStandard(t *testing.T) {
	m := &PostgreSQLMapper{}

	tests := []struct {
		name       string
		sourceType string
		want       StandardDataType
	}{
		// 数值类型
		{"integer", "integer", TypeInteger},
		{"smallint", "smallint", TypeInteger},
		{"bigint", "bigint", TypeInteger},
		{"real", "real", TypeFloat},
		{"double precision", "double precision", TypeFloat},
		{"numeric", "numeric", TypeFloat},
		{"decimal", "decimal", TypeFloat},
		{"numeric with params", "numeric(10,2)", TypeFloat},
		{"money", "money", TypeFloat},

		// 布尔类型
		{"boolean", "boolean", TypeBoolean},
		{"bool", "bool", TypeBoolean},

		// 字符串类型
		{"varchar", "varchar", TypeString},
		{"character varying", "character varying", TypeString},
		{"text", "text", TypeString},
		{"char", "char", TypeString},
		{"uuid", "uuid", TypeString},
		{"time", "time", TypeString},
		{"time without time zone", "time without time zone", TypeString},
		{"interval", "interval", TypeString},

		// 日期时间类型
		{"date", "date", TypeDate},
		{"timestamp", "timestamp", TypeDateTime},
		{"timestamp without time zone", "timestamp without time zone", TypeDateTime},
		{"timestamp with time zone", "timestamp with time zone", TypeDateTime},
		{"timestamptz", "timestamptz", TypeDateTime},

		// 复杂类型
		{"json", "json", TypeMap},
		{"jsonb", "jsonb", TypeMap},
		{"integer array", "integer[]", TypeArray},
		{"text array", "text[]", TypeArray},

		// 失配折叠为 string
		{"unknown", "unknown", TypeString},
		{"empty", "", TypeString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := m.ToStandard(tt.sourceType)
			if err != nil {
				t.Errorf("ToStandard() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ToStandard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgreSQLMapper_ToSource(t *testing.T) {
	m := &PostgreSQLMapper{}

	tests := []struct {
		name    string
		stdType StandardDataType
		config  TypeConfig
		want    string
	}{
		{"float default", TypeFloat, TypeConfig{}, "numeric"},
		{"float with precision", TypeFloat, TypeConfig{Precision: 10, Scale: 2}, "numeric(10,2)"},
		{"integer", TypeInteger, TypeConfig{}, "bigint"},
		{"boolean", TypeBoolean, TypeConfig{}, "boolean"},
		{"string", TypeString, TypeConfig{}, "varchar"},
		{"date", TypeDate, TypeConfig{}, "date"},
		{"datetime", TypeDateTime, TypeConfig{}, "timestamp"},
		{"array", TypeArray, TypeConfig{}, "array"},
		{"map", TypeMap, TypeConfig{}, "jsonb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.ToSource(tt.stdType, tt.config)
			if got != tt.want {
				t.Errorf("ToSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMySQLMapper_ToStandard(t *testing.T) {
	m := &MySQLMapper{}

	tests := []struct {
		name       string
		sourceType string
		want       StandardDataType
	}{
		// 数值类型
		{"int", "int", TypeInteger},
		{"tinyint", "tinyint", TypeInteger},
		{"smallint", "smallint", TypeInteger},
		{"mediumint", "mediumint", TypeInteger},
		{"bigint", "bigint", TypeInteger},
		{"bigint unsigned", "bigint unsigned", TypeInteger},
		{"int with params unsigned", "int(11) unsigned", TypeInteger},
		{"float", "float", TypeFloat},
		{"double", "double", TypeFloat},
		{"decimal", "decimal", TypeFloat},
		{"decimal with params", "decimal(10,2)", TypeFloat},

		// 布尔类型（特殊处理）
		{"bool", "bool", TypeBoolean},
		{"boolean", "boolean", TypeBoolean},
		{"tinyint(1)", "tinyint(1)", TypeBoolean},

		// 字符串类型
		{"varchar", "varchar", TypeString},
		{"varchar with params", "varchar(255)", TypeString},
		{"char", "char", TypeString},
		{"text", "text", TypeString},
		{"enum", "enum('a','b')", TypeString},

		// 日期时间类型
		{"date", "date", TypeDate},
		{"datetime", "datetime", TypeDateTime},
		{"timestamp", "timestamp", TypeDateTime},
		{"year", "year", TypeInteger},

		// 复杂类型
		{"json", "json", TypeMap},

		// 失配折叠为 string
		{"unknown", "unknown", TypeString},
		{"empty", "", TypeString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := m.ToStandard(tt.sourceType)
			if err != nil {
				t.Errorf("ToStandard() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ToStandard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMySQLMapper_ToSource(t *testing.T) {
	m := &MySQLMapper{}

	tests := []struct {
		name    string
		stdType StandardDataType
		config  TypeConfig
		want    string
	}{
		{"float default", TypeFloat, TypeConfig{}, "decimal"},
		{"float with precision", TypeFloat, TypeConfig{Precision: 10, Scale: 2}, "decimal(10,2)"},
		{"integer", TypeInteger, TypeConfig{}, "bigint"},
		{"boolean", TypeBoolean, TypeConfig{}, "tinyint(1)"},
		{"string", TypeString, TypeConfig{}, "varchar"},
		{"date", TypeDate, TypeConfig{}, "date"},
		{"datetime", TypeDateTime, TypeConfig{}, "datetime"},
		{"array", TypeArray, TypeConfig{}, "json"},
		{"map", TypeMap, TypeConfig{}, "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.ToSource(tt.stdType, tt.config)
			if got != tt.want {
				t.Errorf("ToSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClickHouseMapper_ToStandard(t *testing.T) {
	m := &ClickHouseMapper{}

	tests := []struct {
		name       string
		sourceType string
		want       StandardDataType
	}{
		// 数值类型
		{"int8", "int8", TypeInteger},
		{"int16", "int16", TypeInteger},
		{"int32", "int32", TypeInteger},
		{"int64", "int64", TypeInteger},
		{"int256", "int256", TypeInteger},
		{"uint8", "uint8", TypeInteger},
		{"uint64", "uint64", TypeInteger},
		{"float32", "float32", TypeFloat},
		{"float64", "float64", TypeFloat},
		{"decimal", "decimal", TypeFloat},
		{"decimal with params", "decimal(10,2)", TypeFloat},

		// 布尔类型
		{"bool", "bool", TypeBoolean},

		// 字符串类型
		{"string", "string", TypeString},
		{"fixedstring", "FixedString(16)", TypeString},
		{"uuid", "UUID", TypeString},

		// 日期时间类型
		{"date", "date", TypeDate},
		{"date32", "date32", TypeDate},
		{"datetime", "datetime", TypeDateTime},
		{"datetime64", "DateTime64", TypeDateTime},
		{"datetime64 with params", "DateTime64(3)", TypeDateTime},

		// 复杂类型
		{"array", "Array(Int8)", TypeArray},
		{"map", "Map(String, Int32)", TypeMap},
		{"json", "JSON", TypeMap},
		{"object json", "Object('json')", TypeMap},

		// Nullable / LowCardinality 包装
		{"nullable string", "Nullable(String)", TypeString},
		{"nullable int64", "Nullable(Int64)", TypeInteger},
		{"nullable datetime64", "Nullable(DateTime64(3))", TypeDateTime},
		{"lowcardinality string", "LowCardinality(String)", TypeString},
		{"lowcardinality nested nullable", "LowCardinality(Nullable(String))", TypeString},

		// 失配折叠为 string
		{"unknown", "unknown", TypeString},
		{"empty", "", TypeString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := m.ToStandard(tt.sourceType)
			if err != nil {
				t.Errorf("ToStandard() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ToStandard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClickHouseMapper_ToSource(t *testing.T) {
	m := &ClickHouseMapper{}

	tests := []struct {
		name    string
		stdType StandardDataType
		config  TypeConfig
		want    string
	}{
		{"float default", TypeFloat, TypeConfig{}, "float64"},
		{"float with precision", TypeFloat, TypeConfig{Precision: 10, Scale: 2}, "decimal(10,2)"},
		{"integer", TypeInteger, TypeConfig{}, "int64"},
		{"boolean", TypeBoolean, TypeConfig{}, "bool"},
		{"string", TypeString, TypeConfig{}, "string"},
		{"date", TypeDate, TypeConfig{}, "date"},
		{"datetime", TypeDateTime, TypeConfig{}, "datetime"},
		{"array", TypeArray, TypeConfig{}, "array"},
		{"map", TypeMap, TypeConfig{}, "map"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.ToSource(tt.stdType, tt.config)
			if got != tt.want {
				t.Errorf("ToSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewDataTypeMapper(t *testing.T) {
	tests := []struct {
		name       string
		sourceType string
		wantName   string
		wantErr    bool
	}{
		{"starrocks", "starrocks", "starrocks", false},
		{"postgresql", "postgresql", "postgresql", false},
		{"postgres", "postgres", "postgresql", false},
		{"mysql", "mysql", "mysql", false},
		{"clickhouse", "clickhouse", "clickhouse", false},
		{"unknown", "unknown", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewDataTypeMapper(tt.sourceType)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDataTypeMapper() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && mapper.GetSourceName() != tt.wantName {
				t.Errorf("GetSourceName() = %v, want %v", mapper.GetSourceName(), tt.wantName)
			}
		})
	}
}

func TestStandardDataType_Helpers(t *testing.T) {
	tests := []struct {
		name       string
		stdType    StandardDataType
		isNumeric  bool
		isDateTime bool
		isComplex  bool
	}{
		{"float", TypeFloat, true, false, false},
		{"integer", TypeInteger, true, false, false},
		{"boolean", TypeBoolean, false, false, false},
		{"string", TypeString, false, false, false},
		{"date", TypeDate, false, true, false},
		{"datetime", TypeDateTime, false, true, false},
		{"array", TypeArray, false, false, true},
		{"map", TypeMap, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stdType.IsNumeric(); got != tt.isNumeric {
				t.Errorf("IsNumeric() = %v, want %v", got, tt.isNumeric)
			}
			if got := tt.stdType.IsDateTime(); got != tt.isDateTime {
				t.Errorf("IsDateTime() = %v, want %v", got, tt.isDateTime)
			}
			if got := tt.stdType.IsComplex(); got != tt.isComplex {
				t.Errorf("IsComplex() = %v, want %v", got, tt.isComplex)
			}
		})
	}
}

func TestInferExpressionResultType(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want StandardDataType
	}{
		{"SUM", "SUM(price)", TypeFloat},
		{"AVG", "AVG(amount)", TypeFloat},
		{"COUNT", "COUNT(*)", TypeInteger},
		{"MAX", "MAX(score)", TypeFloat},
		{"MIN", "MIN(value)", TypeFloat},
		{"CONCAT", "CONCAT(first_name, last_name)", TypeString},
		{"UPPER", "UPPER(name)", TypeString},
		{"LOWER", "LOWER(title)", TypeString},
		{"LENGTH", "LENGTH(text)", TypeInteger},
		{"YEAR", "YEAR(created_at)", TypeInteger},
		{"MONTH", "MONTH(birth_date)", TypeInteger},
		{"ROUND", "ROUND(price, 2)", TypeFloat},
		{"ABS", "ABS(value)", TypeFloat},
		{"arithmetic multiply", "price * quantity", TypeFloat},
		{"arithmetic add", "amount + tax", TypeFloat},
		{"arithmetic modulo", "count % 10", TypeInteger},
		{"string concat operator", "first_name || last_name", TypeString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferExpressionResultType(tt.expr)
			if got != tt.want {
				t.Errorf("InferExpressionResultType(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestAllDataSources_RoundTrip(t *testing.T) {
	sources := []struct {
		name   string
		mapper DataTypeMapper
		types  []string
	}{
		{
			name:   "StarRocks",
			mapper: &StarRocksMapper{},
			types:  []string{"int", "bigint", "float", "double", "decimal", "varchar", "date", "datetime", "boolean", "array", "map", "json"},
		},
		{
			name:   "PostgreSQL",
			mapper: &PostgreSQLMapper{},
			types:  []string{"integer", "bigint", "numeric", "real", "varchar", "text", "date", "timestamp", "boolean", "json", "jsonb"},
		},
		{
			name:   "MySQL",
			mapper: &MySQLMapper{},
			types:  []string{"int", "bigint", "decimal", "float", "varchar", "text", "date", "datetime", "timestamp", "tinyint(1)", "json"},
		},
		{
			name:   "ClickHouse",
			mapper: &ClickHouseMapper{},
			types:  []string{"int64", "float64", "decimal", "string", "date", "datetime", "bool", "array", "map", "json"},
		},
	}

	for _, source := range sources {
		t.Run(source.name, func(t *testing.T) {
			for _, srcType := range source.types {
				stdType, _, err := source.mapper.ToStandard(srcType)
				if err != nil {
					t.Errorf("ToStandard(%q) failed: %v", srcType, err)
					continue
				}
				if stdType == TypeUnknown {
					t.Errorf("ToStandard(%q) returned unknown type", srcType)
					continue
				}

				// 反向转换
				backType := source.mapper.ToSource(stdType, TypeConfig{})
				if backType == "" {
					t.Errorf("ToSource() returned empty string for %v", stdType)
				}
			}
		})
	}
}
