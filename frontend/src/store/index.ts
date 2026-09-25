import { arrayMove } from '@dnd-kit/sortable';
import { message } from 'antd';
import { create } from 'zustand';
import { isNumericType, normalizeDataType } from '@/lib/dataTypes';
import {
  Chart,
  ChartDataResponse,
  ChartFormData,
  ChartQueryRequest,
  ChartQueryResponse,
  chartsApi,
  Dataset,
  DatasetColumn,
  DatasetFormData,
  Datasource,
  DatasourceFormData,
  datasetsApi,
  datasourcesApi,
  ShareFormData,
  sharesApi,
} from '../api';

/**
 * 按索引补齐字段组数组，避免直接按下标赋值时产生稀疏数组。
 * 调用场景：散点图/透视表会把字段直接写入第 2 个组，若第 1 个组缺失会出现 empty slot。
 * 主要逻辑：复制数组后补足到目标索引，每个缺失位置都写入空字段组。
 */
const ensureFieldGroupAtIndex = (
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
const omitBindingMeta = (
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
const createFilterId = (): string => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `filter-${crypto.randomUUID()}`;
  }
  return `filter-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
};

/** 五个按 bindingId 索引的元数据 Record（整组删除/取消选中路径批量清理时的输入输出形状） */
interface BindingMetaRecords {
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
const omitBindingsMeta = (
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
  id: string;
  name: string;
  type: FieldType;
  dataType: string;
  comment?: string;
}

/**
 * 字段绑定实例：一次"把某列拖入某个槽位"的唯一记录。
 * bindingId 全局唯一（形如 b-0/b-1，顺序递增），field 为稳定列名。
 * 同一列被拖入两个不同组会得到两个不同 bindingId，从而各自持有独立的
 * aggregation/alias/unit/format 元数据（修复 D2：按列名共享导致的互相覆盖）。
 */
export interface BindingInstance {
  bindingId: string;
  field: string;
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
const clampIndex = (index: number, max: number): number => Math.max(0, Math.min(index, max));

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
    if (!existingByField.has(binding.field)) {
      existingByField.set(binding.field, binding);
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
    const binding: BindingInstance = { bindingId: nextBindingId(idScope), field };
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
  | 'isNotNull';

// 过滤条件
export interface FilterCondition {
  id: string;
  field: string;
  operator: FilterOperator;
  value: any;
  valueEnd?: any;
  logic: 'and' | 'or';
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
}

export interface ChartQueryOptions {
  pieMergeOtherBelowRatio?: number;
  /** 直方图分箱数量（R-57）；请求 wire 上以 snake_case bin_count 发送，缺省 20。 */
  binCount?: number;
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

// Store state interface
export interface AppState {
  // Datasources
  datasources: Datasource[];
  datasourcesLoading: boolean;
  datasourcesError: string | null;

  // Datasets
  datasets: Dataset[];
  datasetsLoading: boolean;
  datasetsError: string | null;

  // Charts
  charts: Chart[];
  chartsLoading: boolean;
  chartsError: string | null;

  // Chart Builder State
  chartBuilderFields: ChartField[];
  chartBuilderFieldsLoading: boolean;
  chartBuilderConfig: ChartConfig;
  chartData: ChartDataResponse;
  chartDataLoading: boolean;
  queryConfig: QueryConfig;
  autoQuery: boolean;
  dimensionLabels: Record<string, string>;
  metricAggregations: Record<string, string>;
  metricAliases: Record<string, string>;
  metricUnits: Record<string, string>;
  metricFormats: Record<string, string>;
  chartStyle: ChartStyleConfig;
  chartQueryOptions: ChartQueryOptions;
  chartQueryResponse: ChartQueryResponse | null;
  tablePagination: { page: number; pageSize: number; total: number };
  tableColumns: string[];

  // Selected items
  selectedDatasourceId: number | null;
  selectedDatasetId: number | null;

  // Actions - Datasources
  fetchDatasources: () => Promise<void>;
  addDatasource: (data: DatasourceFormData) => Promise<Datasource>;
  updateDatasource: (id: number, data: DatasourceFormData) => Promise<Datasource>;
  deleteDatasource: (id: number) => Promise<void>;
  setSelectedDatasource: (id: number | null) => void;

  // Actions - Datasets
  fetchDatasets: () => Promise<void>;
  addDataset: (data: DatasetFormData) => Promise<Dataset>;
  updateDataset: (id: number, data: DatasetFormData) => Promise<Dataset>;
  deleteDataset: (id: number) => Promise<void>;
  setSelectedDataset: (id: number | null) => void;

  // Actions - Charts
  fetchCharts: () => Promise<void>;
  addChart: (data: ChartFormData) => Promise<Chart>;
  updateChart: (id: number, data: Partial<ChartFormData>) => Promise<Chart>;
  deleteChart: (id: number) => Promise<void>;

  // Actions - Chart Builder
  fetchDatasetFields: (datasetId: number) => Promise<void>;
  setChartBuilderConfig: (config: Partial<ChartConfig>) => void;
  fetchChartData: (chartId: number) => Promise<void>;
  resetChartBuilder: () => void;
  setQueryConfig: (config: Partial<QueryConfig>) => void;
  addDimensionGroup: (group?: FieldGroup) => void;
  removeDimensionGroup: (id: string) => void;
  addMetricGroup: (group?: FieldGroup) => void;
  removeMetricGroup: (id: string) => void;
  reconcileGroupFields: (
    groupType: 'dimension' | 'metric',
    groupId: string,
    selectedFields: string[]
  ) => void;
  /**
   * 追加一条过滤条件（多条件恒为「且」）。
   * 只传 field 即可：id/operator/value/logic 由 store 补默认值，
   * 避免调用方各自造 id 撞号。
   */
  addFilter: (filter?: Partial<FilterCondition>) => void;
  removeFilter: (id: string) => void;
  updateFilter: (id: string, filter: Partial<FilterCondition>) => void;
  addDimensionField: (field: ChartField, groupIndex?: number) => void;
  removeDimensionField: (bindingId: string, groupIndex?: number) => void;
  reorderDimensionField: (oldIndex: number, newIndex: number, groupIndex?: number) => void;
  addMetricField: (field: ChartField, groupIndex?: number) => void;
  removeMetricField: (bindingId: string, groupIndex?: number) => void;
  reorderMetricField: (oldIndex: number, newIndex: number, groupIndex?: number) => void;
  /**
   * 在字段组之间移动绑定，或组内换序（拖拽查询配置区的字段标签）。
   * 返回结果供调用方决定是否提示（moved/noop 静默，rejected 提示目标组已存在同名列）。
   */
  moveBinding: (source: BindingLocation, target: BindingDropTarget) => MoveBindingResult;
  /**
   * 交换两个维度组的绑定内容（组 id 保持不变，槽位名按组下标解释）。
   * 调用场景：透视表「行列切换」快捷按钮——rows 与 columns 就是维度组 0/1。
   */
  swapDimensionGroups: (indexA: number, indexB: number) => void;
  setDimensionLabel: (bindingId: string, label: string) => void;
  setDimensionLabels: (labels: Record<string, string>) => void;
  setMetricAggregation: (bindingId: string, aggregation: string) => void;
  setMetricAlias: (bindingId: string, alias: string) => void;
  setMetricAggregations: (aggregations: Record<string, string>) => void;
  setMetricAliases: (aliases: Record<string, string>) => void;
  setMetricUnit: (bindingId: string, unit: string) => void;
  setMetricUnits: (units: Record<string, string>) => void;
  setMetricFormat: (bindingId: string, format: string) => void;
  setMetricFormats: (formats: Record<string, string>) => void;
  setChartStyle: (style: Partial<ChartStyleConfig>) => void;
  setChartStyleState: (style: ChartStyleConfig) => void;
  setChartQueryOptionsState: (options: ChartQueryOptions) => void;
  toggleAutoQuery: () => void;
  /**
   * 执行一次图表查询。返回本次查询是否**成功且未被更新请求取代**——调用方据此决定
   * 要不要落库并把地址栏换成短码（失败或过期响应的配置不能变成可分享链接）。
   */
  executeChartQuery: (request: ChartQueryRequest) => Promise<boolean>;
  setTablePagination: (pagination: { page: number; pageSize: number; total: number }) => void;

  // Actions - Shares
  createShare: (data: ShareFormData) => Promise<string>;
}

/**
 * 图表查询请求序号：每次 executeChartQuery 自增，响应回来时若已不是最新序号就丢弃。
 * 调用场景：一次交互可能连发多个请求（排序/翻页/自动查询），HTTP 完成顺序不确定；
 * 没有这层守卫时，先发出的旧请求后到达会覆盖新结果（典型表现：点了表头排序、数据
 * 却仍按未排序结果显示）。
 */
let chartQuerySeq = 0;

// Create store
export const useStore = create<AppState>((set) => ({
  // Initial state - Datasources
  datasources: [],
  datasourcesLoading: false,
  datasourcesError: null,

  // Initial state - Datasets
  datasets: [],
  datasetsLoading: false,
  datasetsError: null,

  // Initial state - Charts
  charts: [],
  chartsLoading: false,
  chartsError: null,

  // Initial state - Chart Builder
  chartBuilderFields: [],
  chartBuilderFieldsLoading: false,
  chartBuilderConfig: {
    chartType: 'table',
    xAxisField: null,
    yAxisFields: [],
    title: 'New Chart',
  },
  chartData: [],
  chartDataLoading: false,
  queryConfig: {
    dimensionGroups: [],
    metricGroups: [],
    filters: [],
    limit: 1000,
  },
  autoQuery: true,
  dimensionLabels: {},
  metricAggregations: {},
  metricAliases: {},
  metricUnits: {},
  metricFormats: {},
  chartStyle: {
    colors: [],
    smooth: false,
    tableRowSize: 'small',
  },
  chartQueryOptions: {},
  chartQueryResponse: null,
  tablePagination: { page: 1, pageSize: 10, total: 0 },
  tableColumns: [],

  // Selected items
  selectedDatasourceId: null,
  selectedDatasetId: null,

  // Datasources actions
  fetchDatasources: async () => {
    set({ datasourcesLoading: true, datasourcesError: null });
    try {
      const response = await datasourcesApi.getAll();
      set({ datasources: response.data.data, datasourcesLoading: false });
    } catch (error: any) {
      set({
        datasourcesError: error.message || 'Failed to fetch',
        datasourcesLoading: false,
      });
    }
  },

  addDatasource: async (data: DatasourceFormData) => {
    const response = await datasourcesApi.create(data);
    const item = response.data.data;
    set((state) => ({
      datasources: [...state.datasources, item],
    }));
    return item;
  },

  updateDatasource: async (id: number, data: DatasourceFormData) => {
    const response = await datasourcesApi.update(id, data);
    const item = response.data.data;
    set((state) => ({
      datasources: state.datasources.map((ds) => (ds.id === id ? item : ds)),
    }));
    return item;
  },

  deleteDatasource: async (id: number) => {
    await datasourcesApi.delete(id);
    set((state) => ({
      datasources: state.datasources.filter((ds) => ds.id !== id),
      selectedDatasourceId: state.selectedDatasourceId === id ? null : state.selectedDatasourceId,
    }));
  },

  setSelectedDatasource: (id: number | null) => {
    set({ selectedDatasourceId: id });
  },

  // Datasets actions
  fetchDatasets: async () => {
    set({ datasetsLoading: true, datasetsError: null });
    try {
      const response = await datasetsApi.getAll();
      set({ datasets: response.data.data, datasetsLoading: false });
    } catch (error: any) {
      set({
        datasetsError: error.message || 'Failed to fetch',
        datasetsLoading: false,
      });
    }
  },

  addDataset: async (data: DatasetFormData) => {
    const response = await datasetsApi.create(data);
    const item = response.data.data;
    set((state) => ({
      datasets: [...state.datasets, item],
    }));
    return item;
  },

  updateDataset: async (id: number, data: DatasetFormData) => {
    const response = await datasetsApi.update(id, data);
    const item = response.data.data;
    set((state) => ({
      datasets: state.datasets.map((ds) => (ds.id === id ? item : ds)),
    }));
    return item;
  },

  deleteDataset: async (id: number) => {
    await datasetsApi.delete(id);
    set((state) => ({
      datasets: state.datasets.filter((ds) => ds.id !== id),
      selectedDatasetId: state.selectedDatasetId === id ? null : state.selectedDatasetId,
    }));
  },

  setSelectedDataset: (id: number | null) => {
    set({ selectedDatasetId: id });
  },

  // Charts actions
  fetchCharts: async () => {
    set({ chartsLoading: true, chartsError: null });
    try {
      const response = await chartsApi.getAll();
      set({ charts: response.data.data, chartsLoading: false });
    } catch (error: any) {
      set({
        chartsError: error.message || 'Failed to fetch',
        chartsLoading: false,
      });
    }
  },

  addChart: async (data: ChartFormData) => {
    const response = await chartsApi.create(data);
    const item = response.data.data;
    set((state) => ({
      charts: [...state.charts, item],
    }));
    return item;
  },

  updateChart: async (id: number, data: Partial<ChartFormData>) => {
    const response = await chartsApi.update(id, data);
    const item = response.data.data;
    set((state) => ({
      charts: state.charts.map((c) => (c.id === id ? item : c)),
    }));
    return item;
  },

  deleteChart: async (id: number) => {
    await chartsApi.delete(id);
    set((state) => ({
      charts: state.charts.filter((c) => c.id !== id),
    }));
  },

  // Shares actions
  createShare: async (data: ShareFormData) => {
    const response = await sharesApi.create(data);
    return response.data.data.token;
  },

  // Chart Builder actions
  fetchDatasetFields: async (datasetId: number) => {
    set({ chartBuilderFieldsLoading: true });
    try {
      const response = await datasetsApi.getColumns(datasetId);
      const columns = response.data.data;

      // 使用后端返回的 role，如果没有则自动推断
      // fieldId 直接使用列名（v1 持久化契约的稳定标识），不再使用位置 id
      const fields: ChartField[] = columns.map((col: DatasetColumn) => ({
        id: col.name,
        name: col.name,
        type: col.role || (isNumericType(normalizeDataType(col.type)) ? 'metric' : 'dimension'),
        dataType: normalizeDataType(col.type),
        comment: col.comment,
      }));

      set({ chartBuilderFields: fields, chartBuilderFieldsLoading: false });
    } catch (_error: any) {
      set({ chartBuilderFields: [], chartBuilderFieldsLoading: false });
    }
  },

  setChartBuilderConfig: (config: Partial<ChartConfig>) => {
    set((state) => ({
      chartBuilderConfig: { ...state.chartBuilderConfig, ...config },
    }));
  },

  fetchChartData: async (chartId: number) => {
    set({ chartDataLoading: true });
    try {
      const response = await chartsApi.getChartData(chartId);
      // chartData 状态即结构化联合（ChartDataResponse）：v1 配置的聚合负载
      // 与 legacy 裸行回退都在联合内，直接存储，不做形状转换。
      set({ chartData: response.data.data, chartDataLoading: false });
    } catch (_error: any) {
      set({ chartData: [], chartDataLoading: false });
    }
  },

  resetChartBuilder: () => {
    set({
      chartBuilderFields: [],
      chartBuilderConfig: {
        chartType: 'table',
        xAxisField: null,
        yAxisFields: [],
        title: 'New Chart',
      },
      chartData: [],
      queryConfig: {
        dimensionGroups: [],
        metricGroups: [],
        filters: [],
        limit: 1000,
      },
      dimensionLabels: {},
      metricAggregations: {},
      metricAliases: {},
      metricUnits: {},
      metricFormats: {},
      chartStyle: {
        colors: [],
        smooth: false,
        tableRowSize: 'small',
      },
      chartQueryOptions: {},
    });
  },

  setQueryConfig: (config: Partial<QueryConfig>) => {
    set((state) => ({
      queryConfig: { ...state.queryConfig, ...config },
    }));
  },

  addDimensionGroup: (group?: FieldGroup) => {
    set((state) => {
      const newGroup = group || {
        id: `dim-group-${Date.now()}`,
        bindings: [],
      };
      return {
        queryConfig: {
          ...state.queryConfig,
          dimensionGroups: [...state.queryConfig.dimensionGroups, newGroup],
        },
      };
    });
  },

  removeDimensionGroup: (id: string) => {
    set((state) => {
      const removed = state.queryConfig.dimensionGroups.find((g) => g.id === id);
      const removedIds = removed ? removed.bindings.map((b) => b.bindingId) : [];
      return {
        queryConfig: {
          ...state.queryConfig,
          dimensionGroups: state.queryConfig.dimensionGroups.filter((g) => g.id !== id),
        },
        // 整组删除会移除该组全部 binding：在同一次 set 里清理它们的元数据，防止
        // nextBindingId(max+1) 复用号导致跨列污染（与 removeDimensionField 同一防护）
        ...omitBindingsMeta(state, removedIds),
      };
    });
  },

  addMetricGroup: (group?: FieldGroup) => {
    set((state) => {
      const newGroup = group || {
        id: `metric-group-${Date.now()}`,
        bindings: [],
      };
      return {
        queryConfig: {
          ...state.queryConfig,
          metricGroups: [...state.queryConfig.metricGroups, newGroup],
        },
      };
    });
  },

  removeMetricGroup: (id: string) => {
    set((state) => {
      const removed = state.queryConfig.metricGroups.find((g) => g.id === id);
      const removedIds = removed ? removed.bindings.map((b) => b.bindingId) : [];
      return {
        queryConfig: {
          ...state.queryConfig,
          metricGroups: state.queryConfig.metricGroups.filter((g) => g.id !== id),
        },
        // 整组删除会移除该组全部 binding：在同一次 set 里清理它们的元数据，防止
        // nextBindingId(max+1) 复用号导致跨列污染（与 removeMetricField 同一防护）
        ...omitBindingsMeta(state, removedIds),
      };
    });
  },

  reconcileGroupFields: (groupType, groupId, selectedFields) => {
    set((state) => {
      const isDimension = groupType === 'dimension';
      const groups = isDimension
        ? state.queryConfig.dimensionGroups
        : state.queryConfig.metricGroups;
      // 全局 bindingId 视野：所有维度组 + 指标组的现有 bindings（供 nextBindingId 分配新号）
      const allBindings = [
        ...state.queryConfig.dimensionGroups.map((g) => g.bindings),
        ...state.queryConfig.metricGroups.map((g) => g.bindings),
      ];
      const target = groups.find((g) => g.id === groupId);
      const nextBindings = target
        ? reconcileGroupBindings(target.bindings, selectedFields, allBindings)
        : [];
      // 本次被移除（取消选中）的 bindingId：旧组里有、reconcile 后不再有
      const kept = new Set(nextBindings.map((b) => b.bindingId));
      const removedIds = target
        ? target.bindings.map((b) => b.bindingId).filter((bindingId) => !kept.has(bindingId))
        : [];
      const updatedGroups = groups.map((g) =>
        g.id === groupId ? { ...g, bindings: nextBindings } : g
      );
      return {
        queryConfig: {
          ...state.queryConfig,
          ...(isDimension ? { dimensionGroups: updatedGroups } : { metricGroups: updatedGroups }),
        },
        // 清理被取消选中的 bindingId 元数据，防止 nextBindingId(max+1) 复用号导致跨列污染
        ...omitBindingsMeta(state, removedIds),
      };
    });
  },

  addFilter: (filter) => {
    set((state) => {
      const newFilter: FilterCondition = {
        id: createFilterId(),
        field: '',
        operator: 'eq',
        value: '',
        logic: 'and',
        ...filter,
      };
      return {
        queryConfig: {
          ...state.queryConfig,
          filters: [...state.queryConfig.filters, newFilter],
        },
      };
    });
  },

  removeFilter: (id: string) => {
    set((state) => ({
      queryConfig: {
        ...state.queryConfig,
        filters: state.queryConfig.filters.filter((f) => f.id !== id),
      },
    }));
  },

  updateFilter: (id: string, filterUpdate: Partial<FilterCondition>) => {
    set((state) => ({
      queryConfig: {
        ...state.queryConfig,
        filters: state.queryConfig.filters.map((f) =>
          f.id === id ? { ...f, ...filterUpdate } : f
        ),
      },
    }));
  },

  addDimensionField: (field: ChartField, groupIndex = 0) => {
    set((state) => {
      const dimensionGroups = ensureFieldGroupAtIndex(
        state.queryConfig.dimensionGroups,
        groupIndex,
        'dim-group'
      );
      const existingBindings = dimensionGroups[groupIndex]?.bindings || [];
      // 同一 group 内不允许重复列名；不同 group 之间允许相同列名（D2 场景）
      if (existingBindings.some((b) => b.field === field.id)) return state;

      const bindingId = nextBindingId([
        ...dimensionGroups.map((g) => g.bindings),
        ...state.queryConfig.metricGroups.map((g) => g.bindings),
      ]);

      const newGroup: FieldGroup = {
        id: dimensionGroups[groupIndex]?.id || `dim-group-${groupIndex + 1}`,
        bindings: [...existingBindings, { bindingId, field: field.id }],
      };

      dimensionGroups[groupIndex] = newGroup;

      return {
        queryConfig: {
          ...state.queryConfig,
          dimensionGroups,
        },
      };
    });
  },

  removeDimensionField: (bindingId: string, groupIndex) => {
    set((state) => ({
      queryConfig: {
        ...state.queryConfig,
        dimensionGroups: state.queryConfig.dimensionGroups.map((g, index) =>
          groupIndex === undefined || index === groupIndex
            ? { ...g, bindings: g.bindings.filter((b) => b.bindingId !== bindingId) }
            : g
        ),
      },
      // 同步清理该 bindingId 的元数据，防止 nextBindingId(max+1) 复用号导致跨列污染
      dimensionLabels: omitBindingMeta(state.dimensionLabels, bindingId),
      metricAggregations: omitBindingMeta(state.metricAggregations, bindingId),
      metricAliases: omitBindingMeta(state.metricAliases, bindingId),
      metricUnits: omitBindingMeta(state.metricUnits, bindingId),
      metricFormats: omitBindingMeta(state.metricFormats, bindingId),
    }));
  },

  reorderDimensionField: (oldIndex: number, newIndex: number, groupIndex = 0) => {
    set((state) => {
      const group = state.queryConfig.dimensionGroups[groupIndex];
      if (!group) return state;

      const dimensionGroups = [...state.queryConfig.dimensionGroups];
      dimensionGroups[groupIndex] = {
        ...group,
        bindings: arrayMove(group.bindings, oldIndex, newIndex),
      };

      return {
        queryConfig: {
          ...state.queryConfig,
          dimensionGroups,
        },
      };
    });
  },

  addMetricField: (field: ChartField, groupIndex = 0) => {
    set((state) => {
      const metricGroups = ensureFieldGroupAtIndex(
        state.queryConfig.metricGroups,
        groupIndex,
        'metric-group'
      );
      const existingBindings = metricGroups[groupIndex]?.bindings || [];
      // 同一 group 内不允许重复列名；不同 group 之间允许相同列名（D2 场景）
      if (existingBindings.some((b) => b.field === field.id)) return state;

      const bindingId = nextBindingId([
        ...state.queryConfig.dimensionGroups.map((g) => g.bindings),
        ...metricGroups.map((g) => g.bindings),
      ]);

      const newGroup: FieldGroup = {
        id: metricGroups[groupIndex]?.id || `metric-group-${groupIndex + 1}`,
        bindings: [...existingBindings, { bindingId, field: field.id }],
      };

      metricGroups[groupIndex] = newGroup;

      return {
        queryConfig: {
          ...state.queryConfig,
          metricGroups,
        },
      };
    });
  },

  removeMetricField: (bindingId: string, groupIndex) => {
    set((state) => ({
      queryConfig: {
        ...state.queryConfig,
        metricGroups: state.queryConfig.metricGroups.map((g, index) =>
          groupIndex === undefined || index === groupIndex
            ? { ...g, bindings: g.bindings.filter((b) => b.bindingId !== bindingId) }
            : g
        ),
      },
      // 同步清理该 bindingId 的元数据，防止 nextBindingId(max+1) 复用号导致跨列污染
      dimensionLabels: omitBindingMeta(state.dimensionLabels, bindingId),
      metricAggregations: omitBindingMeta(state.metricAggregations, bindingId),
      metricAliases: omitBindingMeta(state.metricAliases, bindingId),
      metricUnits: omitBindingMeta(state.metricUnits, bindingId),
      metricFormats: omitBindingMeta(state.metricFormats, bindingId),
    }));
  },

  reorderMetricField: (oldIndex: number, newIndex: number, groupIndex = 0) => {
    set((state) => {
      const group = state.queryConfig.metricGroups[groupIndex];
      if (!group) return state;

      const metricGroups = [...state.queryConfig.metricGroups];
      metricGroups[groupIndex] = {
        ...group,
        bindings: arrayMove(group.bindings, oldIndex, newIndex),
      };

      return {
        queryConfig: {
          ...state.queryConfig,
          metricGroups,
        },
      };
    });
  },

  /**
   * 把某个绑定移到目标字段组的目标位置（同组内即换序）。
   * 调用场景：拖拽查询配置区的字段标签——维度组之间（透视表行/列维度）、指标组之间
   * （主/次轴指标）以及组内换序，三条路径共用这一个出口。
   * 主要逻辑：按 (kind, groupIndex) 定位源组与目标组；同组走 arrayMove，跨组则从源组
   * 摘除后 splice 插入目标组。bindingId 原样保留，因此该绑定的别名/聚合/单位/格式等
   * 元数据随字段一起搬走。跨 kind、组不存在、源绑定不存在一律不变更状态；
   * 目标组已有同名列则拒绝（与 addDimensionField 的单组去重口径一致）。
   */
  moveBinding: (source, target) => {
    if (source.kind !== target.kind) {
      return 'noop';
    }

    let result: MoveBindingResult = 'noop';

    set((state) => {
      const groups =
        source.kind === 'dimension'
          ? state.queryConfig.dimensionGroups
          : state.queryConfig.metricGroups;
      const fromGroup = groups[source.groupIndex];
      const toGroup = groups[target.groupIndex];
      if (!fromGroup || !toGroup) return state;

      const fromIndex = fromGroup.bindings.findIndex((b) => b.bindingId === source.bindingId);
      if (fromIndex < 0) return state;

      const binding = fromGroup.bindings[fromIndex];
      const sameGroup = source.groupIndex === target.groupIndex;

      // 同组内落在原位（或落在本组空白处）时顺序不变，静默返回，不制造无意义更新。
      if (sameGroup && (target.index === undefined || target.index === fromIndex)) return state;

      if (
        toGroup.bindings.some((b) => b.field === binding.field && b.bindingId !== binding.bindingId)
      ) {
        result = 'rejected';
        return state;
      }

      const nextGroups = [...groups];
      if (sameGroup) {
        nextGroups[source.groupIndex] = {
          ...fromGroup,
          bindings: arrayMove(
            fromGroup.bindings,
            fromIndex,
            clampIndex(target.index as number, fromGroup.bindings.length - 1)
          ),
        };
      } else {
        const toBindings = [...toGroup.bindings];
        toBindings.splice(
          target.index === undefined
            ? toBindings.length
            : clampIndex(target.index, toBindings.length),
          0,
          binding
        );
        nextGroups[source.groupIndex] = {
          ...fromGroup,
          bindings: fromGroup.bindings.filter((b) => b.bindingId !== binding.bindingId),
        };
        nextGroups[target.groupIndex] = { ...toGroup, bindings: toBindings };
      }

      result = 'moved';
      return {
        queryConfig: {
          ...state.queryConfig,
          ...(source.kind === 'dimension'
            ? { dimensionGroups: nextGroups }
            : { metricGroups: nextGroups }),
        },
      };
    });

    return result;
  },

  /**
   * 交换两个维度组的 bindings 数组。
   * 主要逻辑：只换 bindings，不换组自身的 id/label——槽位语义（rows/columns）由组下标决定，
   * 组 id 保持稳定可让 React key 与持久化文档不抖动。下标越界或相同则不动。
   */
  swapDimensionGroups: (indexA, indexB) => {
    set((state) => {
      const groups = state.queryConfig.dimensionGroups;
      if (indexA === indexB) return state;
      const groupA = groups[indexA];
      const groupB = groups[indexB];
      if (!groupA || !groupB) return state;

      const nextGroups = [...groups];
      nextGroups[indexA] = { ...groupA, bindings: groupB.bindings };
      nextGroups[indexB] = { ...groupB, bindings: groupA.bindings };

      return { queryConfig: { ...state.queryConfig, dimensionGroups: nextGroups } };
    });
  },

  setDimensionLabel: (bindingId: string, label: string) => {
    set((state) => ({
      dimensionLabels: { ...state.dimensionLabels, [bindingId]: label },
    }));
  },
  setDimensionLabels: (labels: Record<string, string>) => {
    set({ dimensionLabels: labels });
  },

  setMetricAggregation: (bindingId: string, aggregation: string) => {
    set((state) => ({
      metricAggregations: { ...state.metricAggregations, [bindingId]: aggregation },
    }));
  },

  setMetricAggregations: (aggregations: Record<string, string>) => {
    set({ metricAggregations: aggregations });
  },

  setMetricAlias: (bindingId: string, alias: string) => {
    set((state) => ({
      metricAliases: { ...state.metricAliases, [bindingId]: alias },
    }));
  },

  setMetricAliases: (aliases: Record<string, string>) => {
    set({ metricAliases: aliases });
  },

  setMetricUnit: (bindingId: string, unit: string) => {
    set((state) => ({
      metricUnits: { ...state.metricUnits, [bindingId]: unit },
    }));
  },

  setMetricUnits: (units: Record<string, string>) => {
    set({ metricUnits: units });
  },

  setMetricFormat: (bindingId: string, format: string) => {
    set((state) => ({
      metricFormats: { ...state.metricFormats, [bindingId]: format },
    }));
  },

  setMetricFormats: (formats: Record<string, string>) => {
    set({ metricFormats: formats });
  },

  setChartStyle: (style: Partial<ChartStyleConfig>) => {
    set((state) => ({
      chartStyle: { ...state.chartStyle, ...style },
    }));
  },

  setChartStyleState: (style: ChartStyleConfig) => {
    set({ chartStyle: style });
  },

  setChartQueryOptionsState: (options: ChartQueryOptions) => {
    set({ chartQueryOptions: options });
  },

  toggleAutoQuery: () => {
    set((state) => ({ autoQuery: !state.autoQuery }));
  },

  executeChartQuery: async (request: ChartQueryRequest): Promise<boolean> => {
    const seq = ++chartQuerySeq;
    set({ chartDataLoading: true });
    try {
      const response = await chartsApi.executeChartQuery(request);
      const result = response.data.data; // { data, select_sql, count_sql }
      // chartData 直接存结构化响应（与 ShareView 消费面一致），不再按
      // chartType 拆平铺行；table/pivot 额外提取 columns（两者都有）与
      // 分页（仅 table 有）供 TableChart 使用。
      const chartData = result?.data ?? [];

      // 过期响应（期间又发起了更新的查询）一律不落地，避免旧结果覆盖新结果。
      if (seq !== chartQuerySeq) return false;

      if (!Array.isArray(chartData) && 'columns' in chartData) {
        const tablePatch =
          'pagination' in chartData
            ? {
                tablePagination: {
                  page: chartData.pagination.page,
                  pageSize: chartData.pagination.page_size,
                  total: chartData.pagination.total,
                },
              }
            : {};
        set({
          chartData,
          chartQueryResponse: result,
          tableColumns: chartData.columns || [],
          ...tablePatch,
          chartDataLoading: false,
        });
      } else {
        set({ chartData, chartQueryResponse: result, chartDataLoading: false });
      }
      return true;
    } catch (error: any) {
      if (seq !== chartQuerySeq) return false;
      console.error('Chart query failed:', error);
      // 查询失败必须对用户可见：后端 400（如空筛选字段）此前被静默吞掉，
      // 预览直接变空而没有任何提示。
      message.error(error.message || '图表查询失败');
      set({ chartData: [], chartQueryResponse: null, chartDataLoading: false });
      return false;
    }
  },

  setTablePagination: (pagination: { page: number; pageSize: number; total: number }) => {
    set({ tablePagination: pagination });
  },
}));

export default useStore;
