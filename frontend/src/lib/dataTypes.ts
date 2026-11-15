import type { IntlShape } from 'react-intl';

/**
 * 数据集字段统一数据类型 —— 前端单一事实源。
 *
 * 规范词表与后端 model.StandardDataTypes（api/openapi.yaml DatasetColumn.type enum）对齐：
 * float / integer / boolean / string / date / datetime / array / map。
 * 历史词 number/json/unknown 由后端归一为 float/map/string，前端对旧布局数据兜底同口径。
 */
export const DATA_TYPES = [
  'float',
  'integer',
  'boolean',
  'string',
  'date',
  'datetime',
  'array',
  'map',
] as const;

export type DataType = (typeof DATA_TYPES)[number];

/** 历史词/未知值归一（与后端 model.NormalizeStandardType 同一口径）。 */
export function normalizeDataType(value: string | null | undefined): DataType {
  const t = (value ?? '').trim().toLowerCase();
  // 剥掉参数形式（int(11) → int、decimal(10,2) → decimal）
  const base = t.replace(/\(.*\)\s*$/, '').trim();
  switch (base) {
    case 'float':
    case 'number':
    case 'real':
    case 'double':
    case 'decimal':
    case 'dec':
    case 'numeric':
    case 'fixed':
    case 'money':
    case 'float32':
    case 'float64':
      return 'float';
    case 'integer':
    case 'int':
    case 'tinyint':
    case 'smallint':
    case 'mediumint':
    case 'bigint':
    case 'largeint':
    case 'year':
      return 'integer';
    case 'boolean':
    case 'bool':
      return 'boolean';
    case 'date':
    case 'date32':
      return 'date';
    case 'datetime':
    case 'timestamp':
    case 'datetime64':
      return 'datetime';
    case 'array':
      return 'array';
    case 'map':
    case 'json':
      return 'map';
    default:
      // 带修饰的原始列类型兜底（timestamp with time zone 等）
      if (/timestamp/.test(t)) return 'datetime';
      return 'string';
  }
}

export function isNumericType(t: string | null | undefined): boolean {
  const v = normalizeDataType(t);
  return v === 'float' || v === 'integer';
}

export function isDateTimeType(t: string | null | undefined): boolean {
  const v = normalizeDataType(t);
  return v === 'date' || v === 'datetime';
}

/** 复杂类型（array/map）：展示层按字符串处理。 */
export function isComplexType(t: string | null | undefined): boolean {
  const v = normalizeDataType(t);
  return v === 'array' || v === 'map';
}

/** 数值类型子分类：决定筛选弹窗的输入控件族。 */
export function classifyFieldKind(
  dataType: string | null | undefined
): 'date' | 'number' | 'string' {
  const t = normalizeDataType(dataType);
  if (isDateTimeType(t)) return 'date';
  if (isNumericType(t)) return 'number';
  return 'string';
}

/**
 * 数据类型显示名（单一映射入口，#266）：从 i18n `dataType.<type>` 取中文/英文名，
 * 未知或非法类型回退为原始值本身（不空白、不崩溃）。
 *
 * 所有「类型下拉 / 类型标签」都应经此渲染，禁止再各自硬编码 Option 列表——历史上
 * DatasetDetail / Dataset 两处残缺下拉漏了 array/map，导致 map 只显示英文。
 */
export function dataTypeLabel(type: string, intl: IntlShape): string {
  return intl.formatMessage({ id: `dataType.${type}`, defaultMessage: type });
}
