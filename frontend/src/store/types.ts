import type { DateFilterIntent } from '@/lib/dateFilter';

/**
 * 按索引补齐字段组数组，避免直接按下标赋值时产生稀疏数组。
 * 调用场景：散点图/透视表会把字段直接写入第 2 个组，若第 1 个组缺失会出现 empty slot。
 * 主要逻辑：复制数组后补足到目标索引，每个缺失位置都写入空字段组。
 */
export const ensureFieldGroupAtIndex = (
  groups: FieldGroup[],
  groupIndex: number,
  prefix: 'dim-group' | 'metric-group'
): FieldGroup[] => {
  const nextGroups = [...groups];
  while (nextGroups.length <= groupIndex) {
    nextGroups.push({
      id: `${prefix}-${nextGroups.length + 1}`,
      bindings: [],
    });
  }
  return nextGroups;
};

/**
 * 返回移除指定键后的 Record 浅拷贝（键不存在时原样返回）。
 * 调用场景：removeDimensionField/removeMetricField 清理按 bindingId 存的五个元数据
 * Record。nextBindingId 取 max+1，删除最大号后新增列会复用该号；不清理会让新列
 * 静默继承被删列的 aggregation/alias/unit/format/label（跨列元数据污染）。
 */
export const omitBindingMeta = (
  record: Record<string, string>,
  bindingId: string
): Record<string, string> => {
  if (!(bindingId in record)) return record;
  const next = { ...record };
  delete next[bindingId];
  return next;
};

/**
 * 生成过滤条件 id。
 * 调用场景：从字段列表把同一个字段连续拖入过滤区（区间筛选会拖两次），
 * 或点「+」快速追加多条条件。
 * 主要逻辑：优先用 crypto.randomUUID（全局唯一）；环境不支持时回退到
 * 时间戳 + 随机后缀——旧实现只用 Date.now()，同一毫秒内连加两条会撞 id，
 * 撞号会让按 id 更新/删除误伤另一条条件。
 */
export const createFilterId = (): string => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `filter-${crypto.randomUUID()}`;
  }
  return `filter-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
};

/** 五个按 bindingId 索引的元数据 Record（整组删除/取消选中路径批量清理时的输入输出形状） */
export interface BindingMetaRecords {
  dimensionLabels: Record<string, string>;
  metricAggregations: Record<string, string>;
  metricAliases: Record<string, string>;
  metricUnits: Record<string, string>;
  metricFormats: Record<string, string>;
}

/**
 * 从五个元数据 Record 中批量移除多个 bindingId 的键（逐个复用 omitBindingMeta）。
 * 调用场景：removeDimensionGroup/removeMetricGroup（整组删除，一次移除该组全部 binding）
 * 与 reconcileGroupFields（QueryPanel 取消选中，一次移除多个 binding）。这些路径同样受
 * nextBindingId(max+1) 复用号影响，不清理会让新列静默继承被删列的元数据（跨列污染）。
 */
export const omitBindingsMeta = (
  records: BindingMetaRecords,
  bindingIds: readonly string[]
): BindingMetaRecords => {
  let { dimensionLabels, metricAggregations, metricAliases, metricUnits, metricFormats } = records;
  for (const bindingId of bindingIds) {
    dimensionLabels = omitBindingMeta(dimensionLabels, bindingId);
    metricAggregations = omitBindingMeta(metricAggregations, bindingId);
    metricAliases = omitBindingMeta(metricAliases, bindingId);
    metricUnits = omitBindingMeta(metricUnits, bindingId);
    metricFormats = omitBindingMeta(metricFormats, bindingId);
  }
  return { dimensionLabels, metricAggregations, metricAliases, metricUnits, metricFormats };
};

// Field types for chart builder
export type FieldType = 'dimension' | 'metric';

// Field interface for chart builder
export interface ChartField {
  /**
   * 字段的**稳定标识** = 数据集列的 id（`DatasetColumn.id`）。
   * 图表配置、shard_keys 与查询请求一律引用它；改名不断链。
   */
  id: string;
  /** 列的**可变展示名**（`DatasetColumn.name`）：只用于展示与 SQL 输出别名。 */
  name: string;
  type: FieldType;
  dataType: string;
  comment?: string;
}

/**
 * 字段绑定实例：一次"把某列拖入某个槽位"的唯一记录。
 * bindingId 全局唯一（形如 b-0/b-1，顺序递增），field 为**列的稳定 id**
 * （ChartField.id == DatasetColumn.id；键名沿用历史叫法，内容已从列名换成列 ID）。
 * 同一列被拖入两个不同组会得到两个不同 bindingId，从而各自持有独立的
 * aggregation/alias/unit/format 元数据（修复 D2：按列名共享导致的互相覆盖）。
 */
export interface BindingInstance {
  bindingId: string;
  /** 数据集列的稳定 id（`DatasetColumn.id`）。列名是可变展示名，不进任何持久化引用。 */
  fieldId: string;
}

// 字段组 - 支持多维度/多指标
export interface FieldGroup {
  id: string;
  bindings: BindingInstance[];
  alias?: string;
}

/** 绑定实例 + 解析后的完整字段对象，供 UI 渲染（key 用 bindingId，展示用 field） */
export interface BoundField {
  binding: BindingInstance;
  field: ChartField;
}

/**
 * 绑定所在位置：拖拽源（从哪里拖起）。
 * 维度组与指标组各有一份组下标空间，kind 用于区分，避免两套下标串味。
 */
export interface BindingLocation {
  kind: 'dimension' | 'metric';
  groupIndex: number;
  bindingId: string;
}

/**
 * moveBinding 的结果：
 * - moved：已完成移动/换序；
 * - rejected：目标组已有同名列（单组内不允许重复列，与 addDimensionField 同口径）；
 * - noop：源绑定/目标组不存在、跨 kind、或落在原位等无需改状态的情况。
 */
export type MoveBindingResult = 'moved' | 'rejected' | 'noop';

/** 拖拽落点：目标字段组 + 目标下标（省略下标 = 追加到该组末尾）。 */
export interface BindingDropTarget {
  kind: 'dimension' | 'metric';
  groupIndex: number;
  index?: number;
}

/** 把目标下标钳制到 [0, max]（拖拽落点来自 DOM 位置，越界不报错只收敛）。 */
export const clampIndex = (index: number, max: number): number => Math.max(0, Math.min(index, max));

/**
 * 生成下一个全局唯一 bindingId（形如 b-N，顺序递增）。
 * 调用场景：addDimensionField/addMetricField 与 QueryPanel 的 Select diff。
 * 主要逻辑：扫描所有组的全部 bindings，取当前最大的 b-N 序号返回 b-(N+1)；
 * 删除中间某个 binding 后新增不会复用被删的号，保证不与任何存量 id 冲突。
 */
export function nextBindingId(existing: BindingInstance[][]): string {
  let max = -1;
  for (const bindings of existing) {
    for (const binding of bindings) {
      const match = /^b-(\d+)$/.exec(binding.bindingId);
      if (match) {
        const n = Number.parseInt(match[1], 10);
        if (n > max) {
          max = n;
        }
      }
    }
  }
  return `b-${max + 1}`;
}

/**
 * QueryPanel 多选 Select 的 diff：把"新选中的列名集合"与某组现有 bindings 对齐。
 * 调用场景：QueryPanel 用 AntD Select(mode=multiple) 直接改写某个字段组。
 * 主要逻辑：仍被选中的列名保留其现有 bindingId（避免用户重选导致按 bindingId 存的
 * aggregation/alias 丢失），新增列名用 nextBindingId 分配全局唯一新号，取消选中的
 * 列名对应 binding 移除；输出顺序跟随 selectedFields（用户在 Select 里看到的顺序）。
 *
 * @param groupBindings 目标组当前 bindings
 * @param selectedFields Select 返回的列名数组（有序）
 * @param allBindings 所有组（维度+指标）的 bindings，作为全局 bindingId 生成视野
 */
export function reconcileGroupBindings(
  groupBindings: BindingInstance[],
  selectedFields: string[],
  allBindings: BindingInstance[][]
): BindingInstance[] {
  const existingByField = new Map<string, BindingInstance>();
  for (const binding of groupBindings) {
    if (!existingByField.has(binding.fieldId)) {
      existingByField.set(binding.fieldId, binding);
    }
  }
  // 本次新增的 binding 追加进 id 生成视野，保证同一次变更多个新增列名号互不冲突
  const newBindings: BindingInstance[] = [];
  const idScope = [...allBindings, newBindings];
  return selectedFields.flatMap((field) => {
    const existing = existingByField.get(field);
    if (existing) {
      return [existing];
    }
    const binding: BindingInstance = { bindingId: nextBindingId(idScope), fieldId: field };
    newBindings.push(binding);
    return [binding];
  });
}

// 过滤条件操作符
export type FilterOperator =
  | 'eq'
  | 'neq'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'
  | 'like'
  | 'in'
  | 'notIn'
  | 'between'
  | 'isNull'
  | 'isNotNull'
  | 'startsWith'
  | 'endsWith'
  | 'isEmptyString'
  | 'isNotEmptyString';

// 过滤条件
export interface FilterCondition {
  id: string;
  /** 数据集列的稳定 id（`DatasetColumn.id`）。 */
  fieldId: string;
  operator: FilterOperator;
  value: any;
  valueEnd?: any;
  logic: 'and' | 'or';
  /**
   * 日期字段的筛选**意图**（`最近 7 天` / `本月` / 自定义起止 …）。
   * 存在时它是权威：`operator`/`value`/`valueEnd` 只作为「不认日期的下游」的兜底快照
   * （由 `materializeDateFilterSnapshot` 在保存时填）。见 `@/lib/dateFilter`。
   */
  date?: DateFilterIntent;
  /**
   * 「作为筛选器」：图表预览区上方渲染一枚行内筛选控件（全族通用）。
   * 日期族历史数据把它存在 `date.asFilter`，渲染时对旧数据兜底读取；
   * 新写入一律落这里（含日期族）。
   */
  asFilter?: boolean;
  /** 行内筛选控件的显示名称；空缺时回退字段名。 */
  filterLabel?: string;
}

// 图表配置接口
export interface ChartConfig {
  chartType:
    | 'table'
    | 'line'
    | 'bar'
    | 'pie'
    | 'area'
    | 'scatter'
    | 'pivot'
    | 'combo'
    | 'kpi'
    | 'histogram'
    | 'funnel'
    | 'radar'
    | 'boxplot';
  xAxisField: string | null;
  yAxisFields: string[];
  title: string;
}

export interface ChartStyleConfig {
  colors: string[];
  smooth: boolean;
  tableRowSize: 'small' | 'middle' | 'large';
  /** 堆叠模式（bar/line/area）；undefined 等价于 'none'，不在默认值里显式设置。 */
  stack?: 'none' | 'normal' | 'percent';
  /** 条形方向（仅 bar）；undefined 等价于 'vertical'。 */
  orientation?: 'vertical' | 'horizontal';
  /** 环形图开关（仅 pie）；undefined 等价于 false（实心饼图）。 */
  donut?: boolean;
  /** 表格序号列（仅 table）；undefined 等价于 false。 */
  tableShowIndex?: boolean;
  /** 表格自动换行（仅 table）；undefined 等价于 false（沿用省略号）。 */
  tableWordWrap?: boolean;
  /**
   * 表格空值显示（仅 table）：把 NULL/空字符串统一渲染为占位符。
   * undefined 等价于 'raw'（原样输出空单元格，保持既有行为）。
   */
  tableNullDisplay?: 'raw' | 'dash' | 'blank' | 'zero';
  /** 表格冻结维度列（仅 table）：横向滚动时把维度列固定在左侧；undefined 等价于 false。 */
  tableFreezeDimensions?: boolean;
  /** 数据标注（仅 bar/line/area）：在柱子/折点上显示数值标签；undefined 等价于 false。 */
  dataLabel?: boolean;
  /**
   * 数据标注位置（仅 dataLabel 为 true 时消费）；undefined 等价于 'top'。
   * 词表取轴图通用位置，不逐图型再分叉（饼图/漏斗自带标签，不走本项）。
   */
  dataLabelPosition?: 'top' | 'inside' | 'center';
}

/**
 * 参考线（issue #116 分析配置，R-63）：叠加在 bar/line/area 值轴上的 ECharts markLine。
 * - metric：目标指标的**输出列名**（wireAliasOf 口径 = 列名，别名永不进 SQL/响应），
 *   与 buildChartOption 的 series.name 同一命名空间；
 * - type=constant 时 value 必填；avg/median 由 ECharts 在实际渲染的 series 数据上计算；
 * - name 为展示名（markLine 标签），缺省回落「常量线/均值线/中位数线」。
 */
export interface ReferenceLine {
  metric: string;
  type: 'constant' | 'avg' | 'median';
  value?: number;
  name?: string;
}

/**
 * Top N（issue #130，#116 epic 第三步）：按某指标取前 N 个维度值，翻译进查询计划
 * （AST Sort+Limit，数据库完成排序截断）。
 * - limit：正整数（后端钳位 ≤10000，非法即整节不生效）；
 * - metric：排名指标的列 ID（DatasetColumn.id），缺省取首指标；
 * - order：asc|desc，缺省 desc（「取最大的 N 个」是默认心智）。
 * - mergeOther：其余取值合并为「其他」一行（issue #116 验收行）；wire 上以
 *   snake_case merge_other 发送，只在打开时携带。后端仅在 bar/line/area/pie +
 *   恰好一个维度 + **全部指标可加（sum/count）** 时接受，否则显式报错——所以
 *   界面在非可加指标下把它置灰，并在失效时于源头清掉（见 ChartBuilder 的 Top N 清理 effect）。
 */
export interface TopNConfig {
  limit: number;
  metric?: string;
  order?: 'asc' | 'desc';
  mergeOther?: boolean;
}

/**
 * 同环比对比配置（issue #129，#116 epic 第二步）：随 query_options.comparison 进
 * 查询 wire，执行器用「窗口平移基线查询」产 (上期)/(增长率%) 系列或列。
 * - type=mom 环比：上一等长周期（当前筛选窗口的天数 +1 平移）；
 * - type=yoy 同比：前一个日历年；
 * - field 为对比日期维度的列 ID（DatasetColumn.id）；缺省时后端取首维度。
 */
export interface ComparisonConfig {
  type: 'mom' | 'yoy';
  field?: string;
}

export interface ChartQueryOptions {
  pieMergeOtherBelowRatio?: number;
  /** 直方图分箱数量（R-57）；请求 wire 上以 snake_case bin_count 发送，缺省 20。 */
  binCount?: number;
  /**
   * 表格合计行（issue #131，仅 table）：请求 wire 上以 snake_case show_total 发送，
   * 由后端在**过滤后的完整数据集**上重算（不是当前页明细相加），结果在响应的 total 里。
   * undefined 等价于 false（不发该键、请求形状与改动前一致）。
   */
  showTotal?: boolean;
  /** 参考线（R-63）；纯前端渲染配置，不进查询 wire，随持久化文档 queryOptions 小节透传。 */
  referenceLines?: ReferenceLine[];
  /** Top N（#130）；wire 上以 query_options.top_n 发送，仅 bar/line/area/pie 消费。 */
  topN?: TopNConfig;
  /** 同环比（#129）；wire 上以 query_options.comparison 发送，仅 bar/line/area/table 消费。 */
  comparison?: ComparisonConfig;
}

// 查询配置 - 支持多维度组和多指标组
export interface QueryConfig {
  dimensionGroups: FieldGroup[];
  metricGroups: FieldGroup[];
  filters: FilterCondition[];
  /** 排序引用 bindingId（而非列名）：同一列在多个槽位时排序目标不歧义（R-50）。 */
  sort?: { bindingId: string; order: 'asc' | 'desc' };
  limit?: number;
}
