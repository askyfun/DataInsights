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
 * 列标识口径（本版本）：`bindings[].fieldId` 与 `filters[].fieldId` 引用的是**列的稳定 id**
 * （`DatasetColumn.id`），不再是列名——列名是可变的展示名，改名不应切断已保存图表的引用。
 * 列 ID 改造**之前**持久化的文档这两个键叫 `field`，里面存的是位置 id（`field-0`…）或列名；
 * 迁移函数在拿到运行时字段列表（id + name）时按 name→id 升级，并统一写成 `fieldId`
 * （旧键由 `filterFieldRef` 兜底读取，否则旧图表的筛选条件会在加载时静默丢掉字段引用）。
 * 拿不到字段列表时原样保留，后端按「列 ID → 列名」两级解析兜底，因此两种形态都能跑。
 *
 * v2 约定（本版本）：
 * - 字段组的 `fields: string[]`（列名数组）升级为 `bindings: BindingInstance[]`，
 *   每个"拖入槽位的字段实例"持有全局唯一 bindingId（形如 b-0/b-1）；
 * - fieldMeta 的键由列名改为 bindingId——同一列被拖入两个不同组会得到两个不同
 *   bindingId 与各自独立的元数据拷贝（修复 D2：按列名共享 aggregation/alias/
 *   unit/format 导致的互相覆盖）；
 * - sort 以 bindingId 引用排序目标绑定（Task 1-7/R-50；v1/legacy 文档的列名键在
 *   迁移时翻译为对应 binding 的 bindingId，翻译不到则丢弃）；filters 的字段引用与
 *   bindings 同一口径（列 ID，键为 `fieldId`）；
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
  | 'funnel'
  | 'radar'
  | 'boxplot';

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
  /** 透传既有 FilterCondition 形状；迁移时把字段引用统一重写为 `fieldId`（读双键、写 fieldId） */
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
  'radar',
  'boxplot',
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

/**
 * 运行时字段列表条目。id 是列的稳定标识，name 是可变展示名，
 * legacyId 是旧结构（无 version 的图）里用的位置 id（`field-0`…）。
 */
export interface ChartFieldRef {
  id: string;
  name: string;
  legacyId?: string;
}

/** 把字段引用解析成另一种标识；返回 undefined 表示无法解析 */
type FieldResolver = (ref: string) => string | undefined;

/** 旧位置 id（`field-0`…）→ 列名；无字段列表时原样保留，不视为"解析失败" */
function makeLegacyResolver(fields?: ChartFieldRef[]): FieldResolver {
  if (!fields) {
    return (legacyId) => legacyId;
  }
  const map = new Map<string, string>();
  for (const field of fields) {
    if (field && typeof field.legacyId === 'string' && typeof field.name === 'string') {
      map.set(field.legacyId, field.name);
    }
  }
  return (legacyId) => map.get(legacyId);
}

/** 列名 → 列 ID 与列 ID → 列名的双向查找（都只收合法条目） */
function makeFieldLookup(fields?: ChartFieldRef[]): {
  nameToId: Map<string, string>;
  idToName: Map<string, string>;
} {
  const nameToId = new Map<string, string>();
  const idToName = new Map<string, string>();
  for (const field of fields ?? []) {
    if (!field || typeof field.id !== 'string' || typeof field.name !== 'string') {
      continue;
    }
    // 同名/同 id 取先出现者：数据集内列名与列 ID 都由后端保证唯一
    if (!nameToId.has(field.name)) {
      nameToId.set(field.name, field.id);
    }
    if (!idToName.has(field.id)) {
      idToName.set(field.id, field.name);
    }
  }
  return { nameToId, idToName };
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** 仅认自有属性，避免 `obj['constructor']` 命中 Object.prototype 继承成员造成的假阳性 */
function hasOwn(obj: object, key: string): boolean {
  // ⚠️ 这里**不能**写 `Object.hasOwn`（需 ES2022 lib，本项目 tsc target=ES2020 → tsc 直接报
  // TS2550），也**不能**写 `Object.prototype.hasOwnProperty.call(...)`——biome 的
  // noPrototypeBuiltins 会把它自动改写成 `Object.hasOwn`（`biome check --write` 实测踩过两次，
  // 改完 tsc 立刻红）。用 getOwnPropertyDescriptor：语义就是"自有属性"（含不可枚举），且无规则会动它。
  return Object.getOwnPropertyDescriptor(obj, key) !== undefined;
}

/** 以自有可枚举键写入：`obj['__proto__'] = v` 会被 setter 吞成原型，须绕开 */
function defineOwn<T>(obj: Record<string, T>, key: string, value: T): void {
  Object.defineProperty(obj, key, {
    value,
    enumerable: true,
    writable: true,
    configurable: true,
  });
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
      ? entry.fields
          .filter((f): f is string => typeof f === 'string')
          .map((f) => resolve(f) ?? f)
          .filter((f) => f !== '')
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

/**
 * 读过滤条件的字段引用。
 *
 * 当前文档用 `fieldId`；**列 ID 改造之前**持久化的文档用 `field`（那里存的是位置 id
 * `field-0`… 或列名）。两个键都要读：只认 `fieldId` 会让旧图表的筛选条件在加载时
 * **静默丢掉字段引用**（条件还在、字段没了），这类半截改名最难发现。
 * 统一写出 `fieldId`，旧键保留不动（没有任何消费方再读它）。
 */
function filterFieldRef(entry: unknown): string | null {
  if (!isPlainObject(entry)) {
    return null;
  }
  if (typeof entry.fieldId === 'string') {
    return entry.fieldId;
  }
  return typeof entry.field === 'string' ? entry.field : null;
}

/** 过滤器透传既有形状，仅在字段引用可解析时重写为**列的稳定 id** */
function migrateFilters(input: unknown, resolve: FieldResolver): unknown[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input.map((entry) => {
    // 同 localizeFilters：对象检查必须留在展开之前（`filterFieldRef` 不做类型收窄）。
    if (!isPlainObject(entry)) {
      return entry;
    }
    const ref = filterFieldRef(entry);
    if (ref === null) {
      return entry;
    }
    return { ...entry, fieldId: resolve(ref) ?? ref };
  });
}

/**
 * 解析 v1/legacy 文档的 sort 为列名中间表示（旧位置 id 经 resolve 翻译为列名）。
 * 列名 → bindingId 的最终转换在 convertV1ToV2 里做——bindingId 是那里才生成的，
 * 本函数拿不到（先后顺序见 Task 1-7 E 部分）。
 */
function migrateSort(input: unknown, resolve: FieldResolver): V1Query['sort'] {
  if (!isPlainObject(input)) {
    return undefined;
  }
  // 与 filters 同一套双键读取：当前文档 `fieldId`，列 ID 改造之前是 `field`。
  const ref = typeof input.fieldId === 'string' ? input.fieldId : input.field;
  if (typeof ref !== 'string') {
    return undefined;
  }
  return {
    field: resolve(ref) ?? ref,
    order: typeof input.order === 'string' ? input.order : 'asc',
  };
}

/**
 * 把输出别名形态的 sort 翻译为 v2 的 bindingId 形态：按 dimensionGroups →
 * metricGroups（即 bindingId 分配顺序）遍历所有 bindings，取**输出名**等于别名值的
 * 第一个 binding；找不到匹配的 binding（排序引用的列在任何组里都不存在）时丢弃 sort。
 *
 * 必须比输出名而不是 binding.field：wire/持久化里的排序键是 SQL 输出别名（列名或用户
 * 显式别名），而 binding.field 是列的稳定 id，两者口径不同。
 */
function sortFromColumnField(
  field: string,
  order: unknown,
  dimensionGroups: ConfigFieldGroup[],
  metricGroups: ConfigFieldGroup[],
  outputNameOf: (binding: BindingInstance) => string
): ChartConfigQuery['sort'] {
  for (const group of [...dimensionGroups, ...metricGroups]) {
    for (const binding of group.bindings) {
      if (outputNameOf(binding) === field) {
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
  metricGroups: ConfigFieldGroup[],
  outputNameOf: (binding: BindingInstance) => string
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
    return sortFromColumnField(
      input.field,
      input.order,
      dimensionGroups,
      metricGroups,
      outputNameOf
    );
  }
  return undefined;
}

/**
 * 把字段引用从列名就地升级为列 ID（已是列 ID / 解析不到的引用原样保留）。
 * 只改 `bindings[].fieldId` 与 `filters[].fieldId` 两个键——sort 引用的是 bindingId，不受影响。
 */
function localizeGroups(
  groups: ConfigFieldGroup[],
  nameToId: Map<string, string>
): ConfigFieldGroup[] {
  return groups.map((group) => ({
    ...group,
    bindings: group.bindings.map((binding) => ({
      bindingId: binding.bindingId,
      fieldId: nameToId.get(binding.fieldId) ?? binding.fieldId,
    })),
  }));
}

function localizeFilters(filters: unknown[], nameToId: Map<string, string>): unknown[] {
  if (nameToId.size === 0) {
    return filters;
  }
  return filters.map((entry) => {
    // 对象检查留在展开之前：`filterFieldRef` 只负责取值，不做类型收窄，
    // 否则这里的 `{ ...entry }` 会因为 entry 仍是 unknown 而报 TS2698。
    if (!isPlainObject(entry)) {
      return entry;
    }
    const ref = filterFieldRef(entry);
    if (ref === null) {
      return entry;
    }
    return { ...entry, fieldId: nameToId.get(ref) ?? ref };
  });
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
      const prev = hasOwn(fieldMeta, name) ? fieldMeta[name] : undefined;
      // 同一列名的同类元数据以先写入者为准（正常情况下不会冲突）
      if (prev && prev[metaKey] !== undefined) {
        continue;
      }
      defineOwn(fieldMeta, name, { ...prev, [metaKey]: value });
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
      defineOwn(fieldMeta, name, normalized);
    }
  }
  return fieldMeta;
}

/** BindingInstance 校验：bindingId 与 fieldId 均为非空字符串 */
function isBindingInstance(value: unknown): value is BindingInstance {
  return (
    isPlainObject(value) &&
    typeof value.bindingId === 'string' &&
    value.bindingId !== '' &&
    typeof value.fieldId === 'string' &&
    value.fieldId !== ''
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
          .map((b) => ({ bindingId: b.bindingId, fieldId: b.fieldId }))
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
  v1FieldMeta: Record<string, ChartMeta>,
  outputNameOf: (binding: BindingInstance) => string
): { query: ChartConfigQuery; fieldMeta: Record<string, ChartMeta> } {
  const fieldMeta: Record<string, ChartMeta> = {};
  let counter = 0;
  const convertGroups = (groups: V1FieldGroup[]): ConfigFieldGroup[] =>
    groups.map((group) => {
      const bindings: BindingInstance[] = group.fields.map((field) => {
        const bindingId = `b-${counter}`;
        counter += 1;
        // 仅认自有键：列名恰为 constructor/toString 等继承成员时不得凭空产出 meta 条目
        const meta = hasOwn(v1FieldMeta, field) ? v1FieldMeta[field] : undefined;
        if (meta) {
          defineOwn(fieldMeta, bindingId, { ...meta });
        }
        return { bindingId, fieldId: field };
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
        ? sortFromColumnField(
            v1Query.sort.field,
            v1Query.sort.order,
            dimensionGroups,
            metricGroups,
            outputNameOf
          )
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
 * @param fields 运行时字段列表：`legacyId`（位置 id）用于解析旧结构、
 *               `name → id` 用于把历史文档里的列名引用升级为列 ID。
 *               缺省时两类引用都原样保留（有损路径：旧位置 id 解析不出来、
 *               列名引用保持列名——后端按列 ID → 列名两级解析仍能跑）
 */
export function migrateChartConfig(
  raw: string,
  fallbackType: ChartType,
  fields?: ChartFieldRef[]
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

  const resolve = makeLegacyResolver(fields);
  const { nameToId, idToName } = makeFieldLookup(fields);
  // 绑定的「输出名」：SQL 输出别名 / 响应负载键用的列名。取不到列名（无字段列表，
  // 或持久化值本就是列名）时回落持久化值本身——两种文档形态共用一条匹配规则。
  const outputNameOf = (binding: BindingInstance): string =>
    idToName.get(binding.fieldId) ?? binding.fieldId;
  const version = typeof parsed.version === 'number' ? parsed.version : undefined;
  const chartType = normalizeChartType(parsed.chartType, fallbackType);
  const title = typeof parsed.title === 'string' ? parsed.title : '';

  if (version === 2) {
    // v2 直通：校验拷贝 bindings + bindingId 键 fieldMeta，绝不改动输入
    const querySource = isPlainObject(parsed.query) ? parsed.query : {};
    const rawDimensionGroups = normalizeV2Groups(querySource.dimensionGroups);
    const rawMetricGroups = normalizeV2Groups(querySource.metricGroups);
    return {
      version: 2,
      chartType,
      title,
      query: {
        // 列名引用 → 列 ID 就在此升级（已是列 ID 的原样保留）
        dimensionGroups: localizeGroups(rawDimensionGroups, nameToId),
        metricGroups: localizeGroups(rawMetricGroups, nameToId),
        filters: localizeFilters(migrateFilters(querySource.filters, identityResolver), nameToId),
        // sort 的匹配必须在本地化之前做：排序键是输出别名（列名）
        sort: normalizeV2Sort(querySource.sort, rawDimensionGroups, rawMetricGroups, outputNameOf),
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
  const { query, fieldMeta } = convertV1ToV2(v1Query, v1FieldMeta, outputNameOf);
  return {
    version: 2,
    chartType,
    title,
    query: {
      ...query,
      dimensionGroups: localizeGroups(query.dimensionGroups, nameToId),
      metricGroups: localizeGroups(query.metricGroups, nameToId),
      filters: localizeFilters(query.filters, nameToId),
    },
    fieldMeta,
    style: passthroughSection(isV1 ? parsed.style : parsed.chartStyle),
    queryOptions: passthroughSection(isV1 ? parsed.queryOptions : parsed.chartQueryOptions),
  };
}
