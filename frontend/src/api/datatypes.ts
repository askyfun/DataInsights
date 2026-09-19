// 标准化数据类型定义
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

// 各数据源类型 → 标准类型映射（toStandardType 唯一消费方）
const datasourceTypeMappings: Record<string, Record<string, StandardDataType>> = {
  starrocks: {
    tinyint: 'integer',
    smallint: 'integer',
    int: 'integer',
    bigint: 'integer',
    largeint: 'integer',
    float: 'number',
    double: 'number',
    decimal: 'number',
    bool: 'boolean',
    boolean: 'boolean',
    varchar: 'string',
    string: 'string',
    char: 'string',
    date: 'date',
    datetime: 'datetime',
    timestamp: 'datetime',
    array: 'array',
    map: 'map',
    json: 'json',
  },
  postgresql: {
    smallint: 'integer',
    integer: 'integer',
    bigint: 'integer',
    real: 'number',
    double: 'number',
    'double precision': 'number',
    numeric: 'number',
    decimal: 'number',
    boolean: 'boolean',
    bool: 'boolean',
    varchar: 'string',
    text: 'string',
    char: 'string',
    date: 'date',
    timestamp: 'datetime',
    timestamptz: 'datetime',
    json: 'json',
    jsonb: 'json',
  },
  mysql: {
    tinyint: 'integer',
    smallint: 'integer',
    mediumint: 'integer',
    int: 'integer',
    integer: 'integer',
    bigint: 'integer',
    float: 'number',
    double: 'number',
    real: 'number',
    decimal: 'number',
    dec: 'number',
    numeric: 'number',
    fixed: 'number',
    bool: 'boolean',
    boolean: 'boolean',
    'tinyint(1)': 'boolean',
    char: 'string',
    varchar: 'string',
    tinytext: 'string',
    text: 'string',
    mediumtext: 'string',
    longtext: 'string',
    date: 'date',
    datetime: 'datetime',
    timestamp: 'datetime',
    json: 'json',
  },
  clickhouse: {
    int8: 'integer',
    int16: 'integer',
    int32: 'integer',
    int64: 'integer',
    int128: 'integer',
    int256: 'integer',
    uint8: 'integer',
    uint16: 'integer',
    uint32: 'integer',
    uint64: 'integer',
    uint128: 'integer',
    uint256: 'integer',
    float32: 'number',
    float64: 'number',
    decimal: 'number',
    bool: 'boolean',
    string: 'string',
    fixedstring: 'string',
    uuid: 'string',
    date: 'date',
    date32: 'date',
    datetime: 'datetime',
    datetime64: 'datetime',
    array: 'array',
    map: 'map',
    json: 'json',
    object: 'json',
  },
};

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

  return mapping[normalizedType] ?? 'unknown';
}
