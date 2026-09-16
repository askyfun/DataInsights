import { arrayMove } from '@dnd-kit/sortable';
import { message } from 'antd';
import { create } from 'zustand';
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
      fields: [],
    });
  }
  return nextGroups;
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

// 字段组 - 支持多维度/多指标
export interface FieldGroup {
  id: string;
  fields: string[];
  alias?: string;
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
  chartType: 'table' | 'line' | 'bar' | 'pie' | 'area' | 'scatter' | 'pivot';
  xAxisField: string | null;
  yAxisFields: string[];
  title: string;
}

export interface ChartStyleConfig {
  colors: string[];
  smooth: boolean;
  tableRowSize: 'small' | 'middle' | 'large';
}

export interface ChartQueryOptions {
  pieMergeOtherBelowRatio?: number;
}

// 查询配置 - 支持多维度组和多指标组
export interface QueryConfig {
  dimensionGroups: FieldGroup[];
  metricGroups: FieldGroup[];
  filters: FilterCondition[];
  sort?: { field: string; order: 'asc' | 'desc' };
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
  selectedChartId: number | null;

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
  setSelectedChart: (id: number | null) => void;

  // Actions - Chart Builder
  fetchDatasetFields: (datasetId: number) => Promise<void>;
  setChartBuilderConfig: (config: Partial<ChartConfig>) => void;
  fetchChartData: (chartId: number) => Promise<void>;
  fetchQueryData: (datasetId: number, config: QueryConfig) => Promise<void>;
  resetChartBuilder: () => void;
  setQueryConfig: (config: Partial<QueryConfig>) => void;
  addDimensionGroup: (group?: FieldGroup) => void;
  removeDimensionGroup: (id: string) => void;
  addMetricGroup: (group?: FieldGroup) => void;
  removeMetricGroup: (id: string) => void;
  addFilter: (filter?: FilterCondition) => void;
  removeFilter: (id: string) => void;
  updateFilter: (id: string, filter: Partial<FilterCondition>) => void;
  addDimensionField: (field: ChartField, groupIndex?: number) => void;
  removeDimensionField: (fieldId: string, groupIndex?: number) => void;
  reorderDimensionField: (oldIndex: number, newIndex: number, groupIndex?: number) => void;
  addMetricField: (field: ChartField, groupIndex?: number) => void;
  removeMetricField: (fieldId: string, groupIndex?: number) => void;
  reorderMetricField: (oldIndex: number, newIndex: number, groupIndex?: number) => void;
  setDimensionLabel: (fieldId: string, label: string) => void;
  setDimensionLabels: (labels: Record<string, string>) => void;
  setMetricAggregation: (fieldId: string, aggregation: string) => void;
  setMetricAlias: (fieldId: string, alias: string) => void;
  setMetricAggregations: (aggregations: Record<string, string>) => void;
  setMetricAliases: (aliases: Record<string, string>) => void;
  setMetricUnit: (fieldId: string, unit: string) => void;
  setMetricUnits: (units: Record<string, string>) => void;
  setMetricFormat: (fieldId: string, format: string) => void;
  setMetricFormats: (formats: Record<string, string>) => void;
  setChartStyle: (style: Partial<ChartStyleConfig>) => void;
  setChartStyleState: (style: ChartStyleConfig) => void;
  setChartQueryOptionsState: (options: ChartQueryOptions) => void;
  toggleAutoQuery: () => void;
  executeChartQuery: (request: ChartQueryRequest) => Promise<void>;
  setTablePagination: (pagination: { page: number; pageSize: number; total: number }) => void;

  // Actions - Shares
  createShare: (data: ShareFormData) => Promise<string>;

  // Clear errors
  clearError: () => void;
}

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
  selectedChartId: null,

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
      selectedChartId: state.selectedChartId === id ? null : state.selectedChartId,
    }));
  },

  setSelectedChart: (id: number | null) => {
    set({ selectedChartId: id });
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
        type:
          col.role ||
          (['int', 'float', 'decimal', 'numeric', 'double', 'real'].includes(
            col.type?.toLowerCase() ?? ''
          )
            ? 'metric'
            : 'dimension'),
        dataType: col.type,
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
        fields: [],
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
    set((state) => ({
      queryConfig: {
        ...state.queryConfig,
        dimensionGroups: state.queryConfig.dimensionGroups.filter((g) => g.id !== id),
      },
    }));
  },

  addMetricGroup: (group?: FieldGroup) => {
    set((state) => {
      const newGroup = group || {
        id: `metric-group-${Date.now()}`,
        fields: [],
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
    set((state) => ({
      queryConfig: {
        ...state.queryConfig,
        metricGroups: state.queryConfig.metricGroups.filter((g) => g.id !== id),
      },
    }));
  },

  addFilter: (filter?: FilterCondition) => {
    set((state) => {
      const newFilter = filter || {
        id: `filter-${Date.now()}`,
        field: '',
        operator: 'eq',
        value: '',
        logic: 'and',
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
      const existingFields = dimensionGroups[groupIndex]?.fields || [];
      if (existingFields.includes(field.id)) return state;

      const newGroup = {
        id: dimensionGroups[groupIndex]?.id || `dim-group-${groupIndex + 1}`,
        fields: [...existingFields, field.id],
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

  removeDimensionField: (fieldId: string, groupIndex) => {
    set((state) => ({
      queryConfig: {
        ...state.queryConfig,
        dimensionGroups: state.queryConfig.dimensionGroups.map((g, index) =>
          groupIndex === undefined || index === groupIndex
            ? { ...g, fields: g.fields.filter((f) => f !== fieldId) }
            : g
        ),
      },
    }));
  },

  reorderDimensionField: (oldIndex: number, newIndex: number, groupIndex = 0) => {
    set((state) => {
      const group = state.queryConfig.dimensionGroups[groupIndex];
      if (!group) return state;

      const dimensionGroups = [...state.queryConfig.dimensionGroups];
      dimensionGroups[groupIndex] = {
        ...group,
        fields: arrayMove(group.fields, oldIndex, newIndex),
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
      const existingFields = metricGroups[groupIndex]?.fields || [];
      if (existingFields.includes(field.id)) return state;

      const newGroup = {
        id: metricGroups[groupIndex]?.id || `metric-group-${groupIndex + 1}`,
        fields: [...existingFields, field.id],
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

  removeMetricField: (fieldId: string, groupIndex) => {
    set((state) => ({
      queryConfig: {
        ...state.queryConfig,
        metricGroups: state.queryConfig.metricGroups.map((g, index) =>
          groupIndex === undefined || index === groupIndex
            ? { ...g, fields: g.fields.filter((f) => f !== fieldId) }
            : g
        ),
      },
    }));
  },

  reorderMetricField: (oldIndex: number, newIndex: number, groupIndex = 0) => {
    set((state) => {
      const group = state.queryConfig.metricGroups[groupIndex];
      if (!group) return state;

      const metricGroups = [...state.queryConfig.metricGroups];
      metricGroups[groupIndex] = { ...group, fields: arrayMove(group.fields, oldIndex, newIndex) };

      return {
        queryConfig: {
          ...state.queryConfig,
          metricGroups,
        },
      };
    });
  },

  setDimensionLabel: (fieldId: string, label: string) => {
    set((state) => ({
      dimensionLabels: { ...state.dimensionLabels, [fieldId]: label },
    }));
  },

  setDimensionLabels: (labels: Record<string, string>) => {
    set({ dimensionLabels: labels });
  },

  setMetricAggregation: (fieldId: string, aggregation: string) => {
    set((state) => ({
      metricAggregations: { ...state.metricAggregations, [fieldId]: aggregation },
    }));
  },

  setMetricAggregations: (aggregations: Record<string, string>) => {
    set({ metricAggregations: aggregations });
  },

  setMetricAlias: (fieldId: string, alias: string) => {
    set((state) => ({
      metricAliases: { ...state.metricAliases, [fieldId]: alias },
    }));
  },

  setMetricAliases: (aliases: Record<string, string>) => {
    set({ metricAliases: aliases });
  },

  setMetricUnit: (fieldId: string, unit: string) => {
    set((state) => ({
      metricUnits: { ...state.metricUnits, [fieldId]: unit },
    }));
  },

  setMetricUnits: (units: Record<string, string>) => {
    set({ metricUnits: units });
  },

  setMetricFormat: (fieldId: string, format: string) => {
    set((state) => ({
      metricFormats: { ...state.metricFormats, [fieldId]: format },
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

  fetchQueryData: async (datasetId: number, config: QueryConfig) => {
    set({ chartDataLoading: true });
    try {
      const response = await chartsApi.executeQuery(datasetId, config);
      set({ chartData: response.data.data, chartDataLoading: false });
    } catch (_error: any) {
      set({ chartData: [], chartDataLoading: false });
    }
  },

  executeChartQuery: async (request: ChartQueryRequest) => {
    set({ chartDataLoading: true });
    try {
      const response = await chartsApi.executeChartQuery(request);
      const result = response.data.data; // { data, select_sql, count_sql }
      // chartData 直接存结构化响应（与 ShareView 消费面一致），不再按
      // chartType 拆平铺行；table/pivot 额外提取 columns（两者都有）与
      // 分页（仅 table 有）供 TableChart 使用。
      const chartData = result?.data ?? [];

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
    } catch (error: any) {
      console.error('Chart query failed:', error);
      // 查询失败必须对用户可见：后端 400（如空筛选字段）此前被静默吞掉，
      // 预览直接变空而没有任何提示。
      message.error(error.message || '图表查询失败');
      set({ chartData: [], chartQueryResponse: null, chartDataLoading: false });
    }
  },

  setTablePagination: (pagination: { page: number; pageSize: number; total: number }) => {
    set({ tablePagination: pagination });
  },

  // Clear errors
  clearError: () => {
    set({
      datasourcesError: null,
      datasetsError: null,
      chartsError: null,
    });
  },
}));

export default useStore;
