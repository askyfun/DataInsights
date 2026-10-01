import { arrayMove } from '@dnd-kit/sortable';
import { message } from 'antd';
import type { StateCreator } from 'zustand';
import { isNumericType, normalizeDataType } from '@/lib/dataTypes';
import {
  ChartDataResponse,
  ChartQueryRequest,
  ChartQueryResponse,
  chartsApi,
  DatasetColumn,
  datasetsApi,
} from '../api';
import type { AppState } from './index';
import {
  type BindingDropTarget,
  type BindingLocation,
  type ChartConfig,
  type ChartField,
  type ChartQueryOptions,
  type ChartStyleConfig,
  clampIndex,
  createFilterId,
  ensureFieldGroupAtIndex,
  type FieldGroup,
  type FilterCondition,
  type MoveBindingResult,
  nextBindingId,
  omitBindingMeta,
  omitBindingsMeta,
  type QueryConfig,
  reconcileGroupBindings,
} from './types';

export interface ChartBuilderSlice {
  chartBuilderFields: ChartField[];
  /** chartBuilderFields 所属的数据集 id（null = 尚未加载）：改名保存后据此判断要不要热刷新。 */
  chartBuilderFieldsDatasetId: number | null;
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
}

/**
 * 图表查询请求序号：每次 executeChartQuery 自增，响应回来时若已不是最新序号就丢弃。
 * 调用场景：一次交互可能连发多个请求（排序/翻页/自动查询），HTTP 完成顺序不确定；
 * 没有这层守卫时，先发出的旧请求后到达会覆盖新结果（典型表现：点了表头排序、数据
 * 却仍按未排序结果显示）。
 */
let chartQuerySeq = 0;

export const createChartBuilderSlice: StateCreator<AppState, [], [], ChartBuilderSlice> = (
  set
) => ({
  // Initial state - Chart Builder
  chartBuilderFields: [],
  chartBuilderFieldsDatasetId: null,
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
  // 表格/透视表默认一页 100 条（用户约定）：服务端分页的 page_size 初始值，
  // 首次查询即按 100 条拉取，减少翻页次数。
  tablePagination: { page: 1, pageSize: 100, total: 0 },
  tableColumns: [],

  // Chart Builder actions
  fetchDatasetFields: async (datasetId: number) => {
    set({ chartBuilderFieldsLoading: true, chartBuilderFieldsDatasetId: datasetId });
    try {
      const response = await datasetsApi.getColumns(datasetId);
      const columns = response.data.data;

      // 使用后端返回的 role，如果没有则自动推断
      // fieldId 使用列的稳定 id（后端分配的短 ID），不再使用位置 id 或列名——
      // 列名是可变的展示名，改名不应切断已保存图表的字段引用。
      const fields: ChartField[] = columns.map((col: DatasetColumn) => ({
        id: col.id,
        name: col.name,
        type: col.role || (isNumericType(normalizeDataType(col.type)) ? 'metric' : 'dimension'),
        dataType: normalizeDataType(col.type),
        comment: col.comment,
      }));

      set({ chartBuilderFields: fields, chartBuilderFieldsLoading: false });
    } catch (_error: any) {
      set({
        chartBuilderFields: [],
        chartBuilderFieldsDatasetId: null,
        chartBuilderFieldsLoading: false,
      });
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
      chartBuilderFieldsDatasetId: null,
      chartBuilderFieldsLoading: false,
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
      // 重置时一并复位分页：上一个数据集遗留的 total/page 会让新会话的表格
      // 显示错误的总数与页码（此前只改字段、漏掉分页）。
      tablePagination: { page: 1, pageSize: 100, total: 0 },
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
        fieldId: '',
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
      if (existingBindings.some((b) => b.fieldId === field.id)) return state;

      const bindingId = nextBindingId([
        ...dimensionGroups.map((g) => g.bindings),
        ...state.queryConfig.metricGroups.map((g) => g.bindings),
      ]);

      const newGroup: FieldGroup = {
        id: dimensionGroups[groupIndex]?.id || `dim-group-${groupIndex + 1}`,
        bindings: [...existingBindings, { bindingId, fieldId: field.id }],
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
      if (existingBindings.some((b) => b.fieldId === field.id)) return state;

      const bindingId = nextBindingId([
        ...state.queryConfig.dimensionGroups.map((g) => g.bindings),
        ...metricGroups.map((g) => g.bindings),
      ]);

      const newGroup: FieldGroup = {
        id: metricGroups[groupIndex]?.id || `metric-group-${groupIndex + 1}`,
        bindings: [...existingBindings, { bindingId, fieldId: field.id }],
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
        toGroup.bindings.some(
          (b) => b.fieldId === binding.fieldId && b.bindingId !== binding.bindingId
        )
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
});
