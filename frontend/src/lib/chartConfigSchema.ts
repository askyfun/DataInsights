/**
 * bi_chart.config 的 v1 文档 schema 与迁移函数（纯逻辑，不依赖 UI/store 运行时）。
 *
 * 旧结构（无 version 字段）把字段引用存为位置 id（`field-0`…），
 * 列名只存在于运行时字段列表，且 6 个小节平铺在顶层
 * （queryConfig / dimensionLabels / metricAggregations / metricAliases /
 * metricUnits / metricFormats / chartStyle / chartQueryOptions）。
 *
 * v1 约定：
 * - query.dimensionGroups / metricGroups 的 fields 与 filters/sort 的 field
 *   一律使用稳定列名；
 * - 5 个平铺 Record 收敛为 fieldMeta（键为列名）；
 * - chartStyle → style，chartQueryOptions → queryOptions；
 * - xAxisField / yAxisFields 丢弃。
 *
 * 迁移是全覆盖函数：任何输入（空串、损坏 JSON、非对象）都返回合法 v1 文档，
 * 绝不抛异常。旧位置 id 只有借助可选 fields 参数才能解析为列名；
 * 解析不到的 id 在组字段中原样保留（有损路径），对应 fieldMeta 条目被丢弃。
 */

export type ChartType = 'bar' | 'line' | 'pie' | 'area' | 'scatter' | 'table' | 'pivot';

export interface ChartMeta {
  label?: string;
  aggregation?: string;
  alias?: string;
  unit?: string;
  format?: string;
}

/** v1 字段组：fields[] 为稳定列名（v1）；迁移输入可能是旧位置 id */
export interface ConfigFieldGroup {
  id: string;
  fields: string[];
  alias?: string;
}

export interface ChartConfigQuery {
  dimensionGroups: ConfigFieldGroup[];
  metricGroups: ConfigFieldGroup[];
  /** 透传既有 FilterCondition 形状；迁移时仅重写 field 键 */
  filters: unknown[];
  sort?: { field: string; order: string };
  limit?: number;
}

export interface ChartConfigDocument {
  version: 1;
  chartType: ChartType;
  title: string;
  query: ChartConfigQuery;
  /** 键为列名；由旧 dimensionLabels 等 5 个平铺 Record 收敛而来 */
  fieldMeta: Record<string, ChartMeta>;
  /** 透传 ChartStyleConfig 形状 */
  style: unknown;
  /** 透传 ChartQueryOptions 形状 */
  queryOptions: unknown;
}

/** 旧配置字段引用可解析出的最大位置 id 形式仅用于文档，不做结构假设 */
const CHART_TYPES: readonly string[] = ['bar', 'line', 'pie', 'area', 'scatter', 'table', 'pivot'];

/** 旧平铺 Record → fieldMeta 键的映射 */
const LEGACY_META_RECORDS: readonly [recordKey: string, metaKey: keyof ChartMeta][] = [
  ['dimensionLabels', 'label'],
  ['metricAggregations', 'aggregation'],
  ['metricAliases', 'alias'],
  ['metricUnits', 'unit'],
  ['metricFormats', 'format'],
];

const META_KEYS: readonly (keyof ChartMeta)[] = ['label', 'aggregation', 'alias', 'unit', 'format'];

/** 把旧位置 id 解析为列名；返回 undefined 表示无法解析 */
type FieldResolver = (id: string) => string | undefined;

function makeResolver(fields?: { id: string; name: string }[]): FieldResolver {
  if (!fields) {
    // 无字段列表：尽力保留原 id，不视为"解析失败"
    return (id) => id;
  }
  const map = new Map<string, string>();
  for (const field of fields) {
    if (field && typeof field.id === 'string' && typeof field.name === 'string') {
      map.set(field.id, field.name);
    }
  }
  return (id) => map.get(id);
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function normalizeChartType(value: unknown, fallback: ChartType): ChartType {
  if (typeof value === 'string' && CHART_TYPES.includes(value)) {
    return value as ChartType;
  }
  return fallback;
}

function migrateGroups(input: unknown, resolve: FieldResolver): ConfigFieldGroup[] {
  if (!Array.isArray(input)) {
    return [];
  }
  const groups: ConfigFieldGroup[] = [];
  input.forEach((entry, index) => {
    if (!isPlainObject(entry)) {
      return;
    }
    const fields = Array.isArray(entry.fields)
      ? entry.fields.filter((f): f is string => typeof f === 'string').map((f) => resolve(f) ?? f)
      : [];
    const group: ConfigFieldGroup = {
      id: typeof entry.id === 'string' && entry.id !== '' ? entry.id : `group-${index}`,
      fields,
    };
    if (typeof entry.alias === 'string') {
      group.alias = entry.alias;
    }
    groups.push(group);
  });
  return groups;
}

/** 过滤器透传既有形状，仅在 field 为可解析的字符串时重写为列名 */
function migrateFilters(input: unknown, resolve: FieldResolver): unknown[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input.map((entry) => {
    if (!isPlainObject(entry) || typeof entry.field !== 'string') {
      return entry;
    }
    return { ...entry, field: resolve(entry.field) ?? entry.field };
  });
}

function migrateSort(input: unknown, resolve: FieldResolver): ChartConfigQuery['sort'] | undefined {
  if (!isPlainObject(input) || typeof input.field !== 'string') {
    return undefined;
  }
  return {
    field: resolve(input.field) ?? input.field,
    order: typeof input.order === 'string' ? input.order : 'asc',
  };
}

function migrateLimit(input: unknown): number | undefined {
  return typeof input === 'number' && Number.isFinite(input) ? input : undefined;
}

function migrateQuery(source: Record<string, unknown>, resolve: FieldResolver): ChartConfigQuery {
  const querySource = isPlainObject(source.query)
    ? source.query
    : isPlainObject(source.queryConfig)
      ? source.queryConfig
      : {};
  return {
    dimensionGroups: migrateGroups(querySource.dimensionGroups, resolve),
    metricGroups: migrateGroups(querySource.metricGroups, resolve),
    filters: migrateFilters(querySource.filters, resolve),
    sort: migrateSort(querySource.sort, resolve),
    limit: migrateLimit(querySource.limit),
  };
}

/** 旧 5 个平铺 Record → fieldMeta（键为解析后的列名；解析不到的条目丢弃） */
function mergeLegacyFieldMeta(
  source: Record<string, unknown>,
  resolve: FieldResolver
): Record<string, ChartMeta> {
  const fieldMeta: Record<string, ChartMeta> = {};
  for (const [recordKey, metaKey] of LEGACY_META_RECORDS) {
    const record = source[recordKey];
    if (!isPlainObject(record)) {
      continue;
    }
    for (const [id, value] of Object.entries(record)) {
      if (typeof value !== 'string') {
        continue;
      }
      const name = resolve(id);
      if (name === undefined) {
        continue;
      }
      const prev = fieldMeta[name];
      // 同一列名的同类元数据以先写入者为准（正常情况下不会冲突）
      if (prev && prev[metaKey] !== undefined) {
        continue;
      }
      fieldMeta[name] = { ...prev, [metaKey]: value };
    }
  }
  return fieldMeta;
}

/** v1 直通时的 fieldMeta 校验拷贝：仅保留已知元数据键的字符串值 */
function normalizeFieldMeta(input: unknown): Record<string, ChartMeta> {
  const fieldMeta: Record<string, ChartMeta> = {};
  if (!isPlainObject(input)) {
    return fieldMeta;
  }
  for (const [name, meta] of Object.entries(input)) {
    if (!isPlainObject(meta)) {
      continue;
    }
    const normalized: ChartMeta = {};
    let hasKey = false;
    for (const key of META_KEYS) {
      const value = meta[key];
      if (typeof value === 'string') {
        normalized[key] = value;
        hasKey = true;
      }
    }
    if (hasKey) {
      fieldMeta[name] = normalized;
    }
  }
  return fieldMeta;
}

function passthroughSection(input: unknown): unknown {
  return isPlainObject(input) ? { ...input } : {};
}

function emptyDocument(fallbackType: ChartType): ChartConfigDocument {
  return {
    version: 1,
    chartType: fallbackType,
    title: '',
    query: {
      dimensionGroups: [],
      metricGroups: [],
      filters: [],
      sort: undefined,
      limit: undefined,
    },
    fieldMeta: {},
    style: {},
    queryOptions: {},
  };
}

/**
 * 把任意 bi_chart.config JSON 字符串转为合法 v1 文档。
 *
 * @param raw 图表 config 的 JSON 字符串（可能是旧结构、v1 或损坏内容）
 * @param fallbackType chartType 缺失/非法时的回退（一般传后端 chart_type）
 * @param fields 运行时字段列表（id→name），用于把旧位置 id 解析为稳定列名；
 *               缺省时旧 id 在组字段中原样保留（有损路径）
 */
export function migrateChartConfig(
  raw: string,
  fallbackType: ChartType,
  fields?: { id: string; name: string }[]
): ChartConfigDocument {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return emptyDocument(fallbackType);
  }
  if (!isPlainObject(parsed)) {
    return emptyDocument(fallbackType);
  }

  const resolve = makeResolver(fields);
  const version = typeof parsed.version === 'number' ? parsed.version : undefined;

  if (version !== undefined && version >= 1) {
    // v1（或更高版本的尽力解析）：校验拷贝 + 补默认，绝不改动输入
    return {
      version: 1,
      chartType: normalizeChartType(parsed.chartType, fallbackType),
      title: typeof parsed.title === 'string' ? parsed.title : '',
      query: migrateQuery(parsed, resolve),
      fieldMeta: normalizeFieldMeta(parsed.fieldMeta),
      style: passthroughSection(parsed.style),
      queryOptions: passthroughSection(parsed.queryOptions),
    };
  }

  // 旧结构（无 version 或 version < 1）
  return {
    version: 1,
    chartType: normalizeChartType(parsed.chartType, fallbackType),
    title: typeof parsed.title === 'string' ? parsed.title : '',
    query: migrateQuery(parsed, resolve),
    fieldMeta: mergeLegacyFieldMeta(parsed, resolve),
    style: passthroughSection(parsed.chartStyle),
    queryOptions: passthroughSection(parsed.chartQueryOptions),
  };
}
