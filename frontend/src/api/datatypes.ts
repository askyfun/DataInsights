// ============================================================
// 标准化数据类型定义
// ============================================================

export type StandardDataType =
  | 'number'
  | 'integer'
  | 'boolean'
  | 'string'
  | 'date'
  | 'datetime'
  | 'array'
  | 'map'
  | 'json'
  | 'unknown';

export interface TypeConfig {
  precision?: number;
  scale?: number;
  itemType?: StandardDataType;
  keyType?: StandardDataType;
  valueType?: StandardDataType;
}

// ============================================================
// 类型映射配置
// ============================================================

export interface TypeMappingRule {
  standard: StandardDataType;
  config?: TypeConfig;
}

export type TypeMapping = Record<string, TypeMappingRule>;

export const starRocksMapping: TypeMapping = {
  tinyint: { standard: 'integer' },
  smallint: { standard: 'integer' },
  int: { standard: 'integer' },
  bigint: { standard: 'integer' },
  largeint: { standard: 'integer' },
  float: { standard: 'number' },
  double: { standard: 'number' },
  decimal: { standard: 'number' },
  bool: { standard: 'boolean' },
  boolean: { standard: 'boolean' },
  varchar: { standard: 'string' },
  string: { standard: 'string' },
  char: { standard: 'string' },
  date: { standard: 'date' },
  datetime: { standard: 'datetime' },
  timestamp: { standard: 'datetime' },
  array: { standard: 'array' },
  map: { standard: 'map' },
  json: { standard: 'json' },
};

export const postgresqlMapping: TypeMapping = {
  smallint: { standard: 'integer' },
  integer: { standard: 'integer' },
  bigint: { standard: 'integer' },
  real: { standard: 'number' },
  double: { standard: 'number' },
  'double precision': { standard: 'number' },
  numeric: { standard: 'number' },
  decimal: { standard: 'number' },
  boolean: { standard: 'boolean' },
  bool: { standard: 'boolean' },
  varchar: { standard: 'string' },
  text: { standard: 'string' },
  char: { standard: 'string' },
  date: { standard: 'date' },
  timestamp: { standard: 'datetime' },
  timestamptz: { standard: 'datetime' },
  json: { standard: 'json' },
  jsonb: { standard: 'json' },
};

export const mysqlMapping: TypeMapping = {
  tinyint: { standard: 'integer' },
  smallint: { standard: 'integer' },
  mediumint: { standard: 'integer' },
  int: { standard: 'integer' },
  integer: { standard: 'integer' },
  bigint: { standard: 'integer' },
  float: { standard: 'number' },
  double: { standard: 'number' },
  real: { standard: 'number' },
  decimal: { standard: 'number' },
  dec: { standard: 'number' },
  numeric: { standard: 'number' },
  fixed: { standard: 'number' },
  bool: { standard: 'boolean' },
  boolean: { standard: 'boolean' },
  'tinyint(1)': { standard: 'boolean' },
  char: { standard: 'string' },
  varchar: { standard: 'string' },
  tinytext: { standard: 'string' },
  text: { standard: 'string' },
  mediumtext: { standard: 'string' },
  longtext: { standard: 'string' },
  date: { standard: 'date' },
  datetime: { standard: 'datetime' },
  timestamp: { standard: 'datetime' },
  json: { standard: 'json' },
};

export const clickhouseMapping: TypeMapping = {
  int8: { standard: 'integer' },
  int16: { standard: 'integer' },
  int32: { standard: 'integer' },
  int64: { standard: 'integer' },
  int128: { standard: 'integer' },
  int256: { standard: 'integer' },
  uint8: { standard: 'integer' },
  uint16: { standard: 'integer' },
  uint32: { standard: 'integer' },
  uint64: { standard: 'integer' },
  uint128: { standard: 'integer' },
  uint256: { standard: 'integer' },
  float32: { standard: 'number' },
  float64: { standard: 'number' },
  decimal: { standard: 'number' },
  bool: { standard: 'boolean' },
  string: { standard: 'string' },
  fixedstring: { standard: 'string' },
  uuid: { standard: 'string' },
  date: { standard: 'date' },
  date32: { standard: 'date' },
  datetime: { standard: 'datetime' },
  datetime64: { standard: 'datetime' },
  array: { standard: 'array' },
  map: { standard: 'map' },
  json: { standard: 'json' },
  object: { standard: 'json' },
};

export const datasourceTypeMappings: Record<string, TypeMapping> = {
  starrocks: starRocksMapping,
  postgresql: postgresqlMapping,
  mysql: mysqlMapping,
  clickhouse: clickhouseMapping,
};

// ============================================================
// 类型转换函数
// ============================================================

export function toStandardType(sourceType: string, datasourceType: string): StandardDataType {
  const mapping = datasourceTypeMappings[datasourceType.toLowerCase()];
  if (!mapping) {
    return 'unknown';
  }

  const normalizedType = sourceType.toLowerCase().trim();

  // 特殊处理带参数的复杂类型
  if (normalizedType.startsWith('decimal')) return 'number';
  if (normalizedType.startsWith('numeric')) return 'number';
  if (normalizedType.startsWith('array')) return 'array';
  if (normalizedType.startsWith('map')) return 'map';
  if (normalizedType.startsWith('varchar')) return 'string';
  if (normalizedType.startsWith('char')) return 'string';
  if (normalizedType.startsWith('enum')) return 'string';
  if (normalizedType.startsWith('set')) return 'string';
  if (normalizedType.startsWith('fixedstring')) return 'string';

  const rule = mapping[normalizedType];
  return rule?.standard ?? 'unknown';
}

export function toSourceType(
  stdType: StandardDataType,
  datasourceType: string,
  config?: TypeConfig
): string {
  const mapping: Record<string, Record<string, string>> = {
    starrocks: {
      number: config?.precision ? `decimal(${config.precision},${config.scale ?? 0})` : 'double',
      integer: 'bigint',
      boolean: 'boolean',
      string: 'varchar',
      date: 'date',
      datetime: 'datetime',
      array: 'array',
      map: 'map',
      json: 'json',
    },
    postgresql: {
      number: config?.precision ? `numeric(${config.precision},${config.scale ?? 0})` : 'numeric',
      integer: 'bigint',
      boolean: 'boolean',
      string: 'varchar',
      date: 'date',
      datetime: 'timestamp',
      array: 'array',
      map: 'jsonb',
      json: 'jsonb',
    },
    mysql: {
      number: config?.precision ? `decimal(${config.precision},${config.scale ?? 0})` : 'decimal',
      integer: 'bigint',
      boolean: 'tinyint(1)',
      string: 'varchar',
      date: 'date',
      datetime: 'datetime',
      array: 'json',
      map: 'json',
      json: 'json',
    },
    clickhouse: {
      number: config?.precision ? `decimal(${config.precision},${config.scale ?? 0})` : 'float64',
      integer: 'int64',
      boolean: 'bool',
      string: 'string',
      date: 'date',
      datetime: 'datetime',
      array: 'array',
      map: 'map',
      json: 'json',
    },
  };

  const sourceMapping = mapping[datasourceType.toLowerCase()];
  if (!sourceMapping) {
    return 'varchar';
  }

  return sourceMapping[stdType] ?? 'varchar';
}

// ============================================================
// 类型辅助函数
// ============================================================

export const dataTypeOptions: Array<{ value: StandardDataType; label: string; category: string }> =
  [
    { value: 'number', label: '数值', category: '数值' },
    { value: 'integer', label: '整数', category: '数值' },
    { value: 'boolean', label: '布尔值', category: '数值' },
    { value: 'string', label: '字符串', category: '文本' },
    { value: 'date', label: '日期', category: '日期时间' },
    { value: 'datetime', label: '日期时间', category: '日期时间' },
    { value: 'array', label: '数组', category: '复杂类型' },
    { value: 'map', label: '字典', category: '复杂类型' },
    { value: 'json', label: 'JSON', category: '复杂类型' },
  ];

export function getTypeCategory(stdType: StandardDataType): string {
  const categories: Record<string, string> = {
    number: '数值',
    integer: '数值',
    boolean: '数值',
    string: '文本',
    date: '日期时间',
    datetime: '日期时间',
    array: '复杂类型',
    map: '复杂类型',
    json: '复杂类型',
  };
  return categories[stdType] ?? '未知';
}

export function isNumericType(stdType: StandardDataType): boolean {
  return stdType === 'number' || stdType === 'integer';
}

export function isDateTimeType(stdType: StandardDataType): boolean {
  return stdType === 'date' || stdType === 'datetime';
}

export function isComplexType(stdType: StandardDataType): boolean {
  return stdType === 'array' || stdType === 'map' || stdType === 'json';
}

// ============================================================
// 表达式推断
// ============================================================

const expressionFunctions: Record<string, StandardDataType> = {
  SUM: 'number',
  AVG: 'number',
  COUNT: 'integer',
  MAX: 'number',
  MIN: 'number',
  CONCAT: 'string',
  UPPER: 'string',
  LOWER: 'string',
  TRIM: 'string',
  SUBSTRING: 'string',
  LENGTH: 'integer',
  YEAR: 'integer',
  MONTH: 'integer',
  DAY: 'integer',
  DATE_ADD: 'datetime',
  DATE_SUB: 'datetime',
  DATEDIFF: 'integer',
  ROUND: 'number',
  ABS: 'number',
  FLOOR: 'integer',
  CEIL: 'integer',
  POWER: 'number',
  MOD: 'integer',
};

export function inferExpressionType(expression: string): StandardDataType {
  const upperExpr = expression.toUpperCase();

  for (const [func, type] of Object.entries(expressionFunctions)) {
    if (upperExpr.includes(func)) {
      return type;
    }
  }

  // 算术运算
  if (/[+\-*/%]/.test(upperExpr)) {
    if (upperExpr.includes('||')) return 'string';
    return 'number';
  }

  return 'unknown';
}
