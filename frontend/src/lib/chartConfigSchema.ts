/**
 * bi_chart.config 的 v2 文档 schema 与迁移函数（纯逻辑，不依赖 UI/store 运行时）。
 *
 * 历史结构演进：
 * - 旧结构（无 version 字段）把字段引用存为位置 id（`field-0`…），列名只存在于
 *   运行时字段列表，且 6 个小节平铺在顶层（queryConfig / dimensionLabels /
 *   metricAggregations / metricAliases / metricUnits / metricFormats / chartStyle /
 *   chartQueryOptions）。
 * - v1 把组字段与 filters/sort 的 field 统一为稳定列名，5 个平铺 Record 收敛为
 *   fieldMeta（键为列名），chartStyle → style、chartQueryOptions → queryOptions，
 *   丢弃 xAxisField / yAxisFields。
 *
 * v2 约定（本版本）：
 * - 字段组的 `fields: string[]`（列名数组）升级为 `bindings: BindingInstance[]`，
 *   每个"拖入槽位的字段实例"持有全局唯一 bindingId（形如 b-0/b-1）；
 * - fieldMeta 的键由列名改为 bindingId——同一列被拖入两个不同组会得到两个不同
 *   bindingId 与各自独立的元数据拷贝（修复 D2：按列名共享 aggregation/alias/
 *   unit/format 导致的互相覆盖）；
 * - sort 以 bindingId 引用排序目标绑定（Task 1-7/R-50；v1/legacy 文档的列名键在
 *   迁移时翻译为对应 binding 的 bindingId，翻译不到则丢弃）；filters 的 field 仍为列名；
 * - chartType / title / style / queryOptions 小节形状不变。
 *
 * 迁移是全覆盖函数：任何输入（空串、损坏 JSON、非对象）都返回合法 v2 文档，
 * 绝不抛异常。version===2 走校验拷贝直通；version===1 与旧结构先解析为 v1 中间
 * 表示（fields 列名数组 + 列名键 fieldMeta），再按"先 dimensionGroups 后
 * metricGroups、组内按 fields 顺序"跨所有组连续分配 bindingId（b-0/b-1/…）转为 v2，
 * 并把原列名键 fieldMeta 的内容复制到每个由该列名生成的 bindingId 上（同名列的多个
 * binding 各得一份独立拷贝），原列名键条目丢弃。旧位置 id 只有借助可选 fields 参数
 * 才能解析为列名；解析不到的 id 在组字段中原样保留（有损路径），对应 fieldMeta 条目
 * 被丢弃。
 */
import type { BindingInstance } from '../store';

export type ChartType =
  | 'bar'
  | 'line'
  | 'pie'
  | 'area'
  | 'scatter'
  | 'table'
  | 'pivot'
  | 'combo'
  | 'kpi'
  | 'histogram'
  | 'funnel';

export interface ChartMeta {
  label?: string;
  aggregation?: string;
  alias?: string;
  unit?: string;
  format?: string;
}

/** v2 字段组：bindings[] 为带全局唯一 bindingId 的字段实例 */
export interface ConfigFieldGroup {
  id: string;
  bindings: BindingInstance[];
  alias?: string;
}

/** v1 中间表示（仅迁移内部使用）：fields[] 为稳定列名；迁移输入可能是旧位置 id */
interface V1FieldGroup {
  id: string;
  fields: string[];
  alias?: string;
}

/** v1 中间查询表示（仅迁移内部使用）；sort.field 是列名，转 v2 时翻译为 bindingId */
interface V1Query {
  dimensionGroups: V1FieldGroup[];
  metricGroups: V1FieldGroup[];
  filters: unknown[];
  sort?: { field: string; order: string };
  limit?: number;
}

export interface ChartConfigQuery {
  dimensionGroups: ConfigFieldGroup[];
  metricGroups: ConfigFieldGroup[];
  /** 透传既有 FilterCondition 形状；迁移时仅重写 field 键 */
  filters: unknown[];
  /** v2 形态：bindingId 引用排序目标绑定（Task 1-7/R-50） */
  sort?: { bindingId: string; order: string };
  limit?: number;
}

export interface ChartConfigDocument {
  version: 2;
  chartType: ChartType;
  title: string;
  query: ChartConfigQuery;
  /** 键为 bindingId（v2）；由旧 dimensionLabels 等 5 个平铺 Record 收敛而来 */
  fieldMeta: Record<string, ChartMeta>;
  /** 透传 ChartStyleConfig 形状 */
  style: unknown;
  /** 透传 ChartQueryOptions 形状 */
  queryOptions: unknown;
}

/** 旧配置字段引用可解析出的最大位置 id 形式仅用于文档，不做结构假设 */
const CHART_TYPES: readonly string[] = [
  'bar',
  'line',
  'pie',
  'area',
  'scatter',
  'table',
  'pivot',
  'combo',
  'kpi',
  'histogram',
  'funnel',
];

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

function migrateGroups(input: unknown, resolve: FieldResolver): V1FieldGroup[] {
  if (!Array.isArray(input)) {
    return [];
  }
  const groups: V1FieldGroup[] = [];
  input.forEach((entry, index) => {
    if (!isPlainObject(entry)) {
      return;
    }
    const fields = Array.isArray(entry.fields)
      ? entry.fields.filter((f): f is string => typeof f === 'string').map((f) => resolve(f) ?? f)
      : [];
    const group: V1FieldGroup = {
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

/**
 * 解析 v1/legacy 文档的 sort 为列名中间表示（旧位置 id 经 resolve 翻译为列名）。
 * 列名 → bindingId 的最终转换在 convertV1ToV2 里做——bindingId 是那里才生成的，
 * 本函数拿不到（先后顺序见 Task 1-7 E 部分）。
 */
function migrateSort(input: unknown, resolve: FieldResolver): V1Query['sort'] {
  if (!isPlainObject(input) || typeof input.field !== 'string') {
    return undefined;
  }
  return {
    field: resolve(input.field) ?? input.field,
    order: typeof input.order === 'string' ? input.order : 'asc',
  };
}

/**
 * 把列名形态的 sort 翻译为 v2 的 bindingId 形态：按 dimensionGroups → metricGroups
 * （即 bindingId 分配顺序）遍历所有 bindings，取 field 与列名匹配的第一个 binding；
 * 找不到匹配的 binding（排序引用的列在任何组里都不存在）时丢弃 sort（返回 undefined）。
 */
function sortFromColumnField(
  field: string,
  order: unknown,
  dimensionGroups: ConfigFieldGroup[],
  metricGroups: ConfigFieldGroup[]
): ChartConfigQuery['sort'] {
  for (const group of [...dimensionGroups, ...metricGroups]) {
    for (const binding of group.bindings) {
      if (binding.field === field) {
        return {
          bindingId: binding.bindingId,
          order: typeof order === 'string' ? order : 'asc',
        };
      }
    }
  }
  return undefined;
}

/**
 * v2 直通的 sort 校验拷贝：bindingId 键（Task 1-7 起的形状）做形状校验后透传；
 * 兼容 Task 0-3~1-7 窗口期保存的 field 键（列名）文档，按 sortFromColumnField
 * 翻译为 bindingId；两种键都没有则丢弃。
 */
function normalizeV2Sort(
  input: unknown,
  dimensionGroups: ConfigFieldGroup[],
  metricGroups: ConfigFieldGroup[]
): ChartConfigQuery['sort'] {
  if (!isPlainObject(input)) {
    return undefined;
  }
  if (typeof input.bindingId === 'string' && input.bindingId !== '') {
    return {
      bindingId: input.bindingId,
      order: typeof input.order === 'string' ? input.order : 'asc',
    };
  }
  if (typeof input.field === 'string') {
    return sortFromColumnField(input.field, input.order, dimensionGroups, metricGroups);
  }
  return undefined;
}

function migrateLimit(input: unknown): number | undefined {
  return typeof input === 'number' && Number.isFinite(input) ? input : undefined;
}

function migrateQuery(source: Record<string, unknown>, resolve: FieldResolver): V1Query {
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

/** fieldMeta 校验拷贝：仅保留已知元数据键的字符串值（键语义无关，v2 为 bindingId） */
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

/** BindingInstance 校验：bindingId 与 field 均为非空字符串 */
function isBindingInstance(value: unknown): value is BindingInstance {
  return (
    isPlainObject(value) &&
    typeof value.bindingId === 'string' &&
    value.bindingId !== '' &&
    typeof value.field === 'string' &&
    value.field !== ''
  );
}

/** v2 直通：校验拷贝 bindings（产出全新对象，绝不与输入共享引用） */
function normalizeV2Groups(input: unknown): ConfigFieldGroup[] {
  if (!Array.isArray(input)) {
    return [];
  }
  const groups: ConfigFieldGroup[] = [];
  input.forEach((entry, index) => {
    if (!isPlainObject(entry)) {
      return;
    }
    const bindings = Array.isArray(entry.bindings)
      ? entry.bindings
          .filter(isBindingInstance)
          .map((b) => ({ bindingId: b.bindingId, field: b.field }))
      : [];
    const group: ConfigFieldGroup = {
      id: typeof entry.id === 'string' && entry.id !== '' ? entry.id : `group-${index}`,
      bindings,
    };
    if (typeof entry.alias === 'string') {
      group.alias = entry.alias;
    }
    groups.push(group);
  });
  return groups;
}

/**
 * v1 中间表示 → v2：给每个字段实例分配全局递增 bindingId（先 dimensionGroups 后
 * metricGroups、组内按 fields 顺序，跨所有组连续编号 b-0/b-1/…），并把原列名键
 * fieldMeta 的内容复制到每个由该列名生成的 bindingId 上（同名列的多个 binding 各得
 * 一份独立拷贝，之后可各自修改互不影响——D2 修复目标）；原列名键条目丢弃。
 * sort 的列名键在 bindings 生成后翻译为对应 bindingId（无匹配 binding 则丢弃）。
 */
function convertV1ToV2(
  v1Query: V1Query,
  v1FieldMeta: Record<string, ChartMeta>
): { query: ChartConfigQuery; fieldMeta: Record<string, ChartMeta> } {
  const fieldMeta: Record<string, ChartMeta> = {};
  let counter = 0;
  const convertGroups = (groups: V1FieldGroup[]): ConfigFieldGroup[] =>
    groups.map((group) => {
      const bindings: BindingInstance[] = group.fields.map((field) => {
        const bindingId = `b-${counter}`;
        counter += 1;
        const meta = v1FieldMeta[field];
        if (meta) {
          fieldMeta[bindingId] = { ...meta };
        }
        return { bindingId, field };
      });
      const converted: ConfigFieldGroup = { id: group.id, bindings };
      if (group.alias !== undefined) {
        converted.alias = group.alias;
      }
      return converted;
    });

  // bindingId 分配顺序：dimensionGroups 先于 metricGroups（与原对象字面量求值顺序一致）
  const dimensionGroups = convertGroups(v1Query.dimensionGroups);
  const metricGroups = convertGroups(v1Query.metricGroups);

  return {
    query: {
      dimensionGroups,
      metricGroups,
      filters: v1Query.filters,
      // sort 的列名 → bindingId 翻译必须在 bindings 生成之后进行（Task 1-7 E 部分）
      sort: v1Query.sort
        ? sortFromColumnField(v1Query.sort.field, v1Query.sort.order, dimensionGroups, metricGroups)
        : undefined,
      limit: v1Query.limit,
    },
    fieldMeta,
  };
}

function passthroughSection(input: unknown): unknown {
  return isPlainObject(input) ? { ...input } : {};
}

function emptyDocument(fallbackType: ChartType): ChartConfigDocument {
  return {
    version: 2,
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

/** v2 的 filters/sort field 已是稳定列名，无需位置 id 解析（恒等） */
const identityResolver = (id: string): string => id;

/**
 * 把任意 bi_chart.config JSON 字符串转为合法 v2 文档。
 *
 * @param raw 图表 config 的 JSON 字符串（可能是旧结构、v1、v2 或损坏内容）
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
  const chartType = normalizeChartType(parsed.chartType, fallbackType);
  const title = typeof parsed.title === 'string' ? parsed.title : '';

  if (version === 2) {
    // v2 直通：校验拷贝 bindings + bindingId 键 fieldMeta，绝不改动输入
    const querySource = isPlainObject(parsed.query) ? parsed.query : {};
    const dimensionGroups = normalizeV2Groups(querySource.dimensionGroups);
    const metricGroups = normalizeV2Groups(querySource.metricGroups);
    return {
      version: 2,
      chartType,
      title,
      query: {
        dimensionGroups,
        metricGroups,
        filters: migrateFilters(querySource.filters, identityResolver),
        sort: normalizeV2Sort(querySource.sort, dimensionGroups, metricGroups),
        limit: migrateLimit(querySource.limit),
      },
      fieldMeta: normalizeFieldMeta(parsed.fieldMeta),
      style: passthroughSection(parsed.style),
      queryOptions: passthroughSection(parsed.queryOptions),
    };
  }

  // version === 1 或旧结构（无 version / version < 1）：先解析为 v1 中间表示，再转 v2
  const isV1 = version === 1;
  const v1Query = migrateQuery(parsed, resolve);
  const v1FieldMeta = isV1
    ? normalizeFieldMeta(parsed.fieldMeta)
    : mergeLegacyFieldMeta(parsed, resolve);
  const { query, fieldMeta } = convertV1ToV2(v1Query, v1FieldMeta);
  return {
    version: 2,
    chartType,
    title,
    query,
    fieldMeta,
    style: passthroughSection(isV1 ? parsed.style : parsed.chartStyle),
    queryOptions: passthroughSection(isV1 ? parsed.queryOptions : parsed.chartQueryOptions),
  };
}
