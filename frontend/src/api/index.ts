import type { AxiosResponse } from 'axios';
import type { components } from '../idls/gen_types';
import { type ApiResponse, apiClient } from '../lib/api/client';
import type { StandardDataType } from './datatypes';

// G = generated OpenAPI schema types (src/idls/gen_types.ts). Migration rule:
// handwritten *Response wire-payload types correspond to the bare generated
// schema (e.g. ChartQueryResponse ↔ G['ChartDataResult']), never to the
// same-named generated Envelope wrapper (G['ChartQueryResponse'] is Envelope).
type G = components['schemas'];

// Types
export type DatasourceType = 'postgresql' | 'clickhouse' | 'mysql' | 'starrocks';

// Backend masks password (json:"-"), so the generated schema has no password
// key; id/created_at/updated_at are required on the wire (no typed-literal
// construction sites exist frontend-side).
export type Datasource = Omit<G['Datasource'], 'type'> & {
  type: DatasourceType;
};

export interface DatasourceFormData {
  name: string;
  type: DatasourceType;
  host: string;
  port: number;
  database_name: string;
  username: string;
  password: string;
}

// 数据集模式
export type DatasetMode = 'direct' | 'accelerated';

// 数据类型
export type DataType =
  | 'int'
  | 'float'
  | 'decimal'
  | 'string'
  | 'date'
  | 'datetime'
  | 'array'
  | 'dict'
  | 'boolean';

// 列角色
export type ColumnRole = 'dimension' | 'metric';

// type_config is dropped from the TS surface (audit (b): zero reads/writes
// frontend-side; the runtime spread still round-trips the server value).
export type DatasetColumn = Omit<G['DatasetColumn'], 'type' | 'role' | 'type_config'> & {
  type: StandardDataType;
  role: ColumnRole;
};

// Wire truth per generated schema: metadata keys are required (null when
// unset), mode/query_type collapse to string (union collapse, audit trap 2).
export type Dataset = G['Dataset'];

export interface DatasetFormData {
  name: string;
  datasource_id: number;
  table_name?: string;
  query_sql?: string;
  query_type: string;
  mode?: DatasetMode;
  description?: string;
  tags?: string[];
  shard_enabled?: boolean;
  shard_keys?: string[];
}

// created_at/updated_at are required on the wire (empty string when the DB
// timestamp is invalid).
export type Chart = G['Chart'];

export interface ChartFormData {
  name: string;
  dataset_id: number;
  chart_type: string;
  config: string;
}

// expires_at is null (never absent) on the wire; password is json:"-" and
// has_password is the only protection signal (same as before).
export type Share = G['Share'];

export type ShareFormData = G['ShareCreateRequest'];

export type TestConnectionRequest = Omit<G['DatasourceTestConnectionRequest'], 'type'> & {
  type: DatasourceType;
};

export type TableInfo = G['TableInfo'];

export type ColumnInfo = G['ColumnInfo'];

// 数据预览
export type DatasetPreview = G['PreviewResult'];

// 表数据预览（带分页和主键信息）
export type TableDataResult = G['TableDataResult'];

// 字段分布
export type FieldDistribution = G['FieldDistribution'];

// Charts API types
export type ChartQueryAggregation =
  | 'sum'
  | 'avg'
  | 'count'
  | 'max'
  | 'min'
  | 'count_distinct'
  | 'median';

export type ChartQueryMetric = Omit<G['ChartMetricConfig'], 'agg'> & {
  agg: ChartQueryAggregation;
};

export interface ChartQueryFilter {
  field: string;
  operator:
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
  value: unknown;
  value_end?: unknown;
  logic: 'and' | 'or';
}

export type ChartQueryPagination = G['ChartPagination'];

export type ChartQuerySort = Omit<G['SortConfig'], 'order'> & {
  order: 'asc' | 'desc';
};

// Wire truth: POST /api/charts/query binds ChartSpecQueryRequest (v1+v2 superset,
// see openapi.yaml). dims/metrics stay narrowed onto the handwritten element types
// but are optional now: the v2 slot protocol (spec_version=2, emitted for combo's
// dual-axis metric slots and pivot's rows/columns, see composeChartQueryRequest) omits
// them entirely in favor of dimension_groups/metric_groups. filters stays required —
// both branches always send it. spec_version/dimension_groups/metric_groups come
// straight from the generated schema (no union-narrowing needed for those fields).
export type ChartQueryRequest = Omit<G['ChartSpecQueryRequest'], 'dims' | 'metrics' | 'filters'> & {
  dims?: string[];
  metrics?: ChartQueryMetric[];
  filters: ChartQueryFilter[];
  // 查询选项扩展袋（如 histogram 的 { bin_count }）。生成物 ChartSpecQueryRequest 早于
  // openapi 的 query_options 改动（裁定 A：不重新生成 gen_types.ts），故在手写薄层补齐，
  // 形状与生成的 ChartSpec.query_options（additionalProperties: true）一致。
  query_options?: { [key: string]: unknown };
};

export type ChartDimensionGroup = G['ChartDimensionGroup'];
export type ChartDimensionField = G['ChartDimensionField'];
export type ChartMetricGroup = G['ChartMetricGroup'];
export type ChartMetricField = G['ChartMetricField'];

export type TableResponse = G['ChartTableResponse'];

export type PieResponse = G['ChartPieResponse'];

export type AxisResponse = G['ChartAxisResponse'];

export type ScatterResponse = G['ChartScatterResponse'];

export type KpiResponse = G['ChartKpiResponse'];

export type PivotResponseV2 = G['ChartPivotResponseV2'];

export type PivotRow = G['ChartPivotRow'];

export type HistogramResponse = G['ChartHistogramResponse'];

export type HistogramBin = G['ChartHistogramBin'];

export type RadarResponse = G['ChartRadarResponse'];

export type RadarIndicator = G['ChartRadarIndicator'];

export type RadarSeries = G['ChartRadarSeries'];

export type BoxplotResponse = G['ChartBoxplotResponse'];

export type ChartDataResponse =
  | G['ChartTableResponse']
  | G['ChartPieResponse']
  | G['ChartAxisResponse']
  | G['ChartScatterResponse']
  | G['ChartPivotResponse']
  | G['ChartPivotResponseV2']
  | G['ChartKpiResponse']
  | G['ChartHistogramResponse']
  | G['ChartRadarResponse']
  | G['ChartBoxplotResponse']
  | unknown[];

// pivot v2 交叉表负载判别：按响应形状（cells + col_headers + row_headers 均为数组）
// 判别，而非 chartType——v1 平铺 pivot（{columns,data}，ChartPivotResponse）与 v2
// 交叉 pivot 并存，前者与 table 一样仍走 TableChart。
export function isPivotV2Payload(x: unknown): x is PivotResponseV2 {
  return (
    typeof x === 'object' &&
    x !== null &&
    !Array.isArray(x) &&
    'cells' in x &&
    Array.isArray(x.cells) &&
    'col_headers' in x &&
    Array.isArray(x.col_headers) &&
    'row_headers' in x &&
    Array.isArray(x.row_headers)
  );
}

// NOT G['ChartQueryResponse'] (that is the Envelope wrapper): the bare wire
// payload is G['ChartDataResult'], with data narrowed from unknown to the union.
export type ChartQueryResponse = Omit<G['ChartDataResult'], 'data'> & {
  data: ChartDataResponse;
};

// Datasources API
export const datasourcesApi = {
  // Get all datasources
  getAll: (): Promise<AxiosResponse<ApiResponse<Datasource[]>>> => {
    return apiClient.get<ApiResponse<Datasource[]>>('/api/datasources');
  },

  // Get single datasource
  getById: (id: number): Promise<AxiosResponse<ApiResponse<Datasource>>> => {
    return apiClient.get<ApiResponse<Datasource>>(`/api/datasources/${id}`);
  },

  // Create datasource
  create: (data: DatasourceFormData): Promise<AxiosResponse<ApiResponse<Datasource>>> => {
    return apiClient.post<ApiResponse<Datasource>>('/api/datasources', data);
  },

  // Update datasource
  update: (
    id: number,
    data: DatasourceFormData
  ): Promise<AxiosResponse<ApiResponse<Datasource>>> => {
    return apiClient.put<ApiResponse<Datasource>>(`/api/datasources/${id}`, data);
  },

  // Delete datasource
  delete: (id: number): Promise<AxiosResponse<ApiResponse<{ status: string }>>> => {
    return apiClient.delete<ApiResponse<{ status: string }>>(`/api/datasources/${id}`);
  },

  // Test connection
  testConnection: (
    data: TestConnectionRequest
  ): Promise<AxiosResponse<ApiResponse<{ status: string }>>> => {
    return apiClient.post<ApiResponse<{ status: string }>>('/api/datasources/test', data);
  },

  getTables: (id: number): Promise<AxiosResponse<ApiResponse<TableInfo[]>>> => {
    return apiClient.get<ApiResponse<TableInfo[]>>(`/api/datasources/${id}/tables`);
  },

  getTableColumns: (
    id: number,
    tableName: string
  ): Promise<AxiosResponse<ApiResponse<ColumnInfo[]>>> => {
    return apiClient.get<ApiResponse<ColumnInfo[]>>(
      `/api/datasources/${id}/tables/${encodeURIComponent(tableName)}/columns`
    );
  },

  // Get data preview from datasource (before creating dataset)
  getPreview: (
    id: number,
    tableName: string,
    querySQL: string,
    queryType: string
  ): Promise<AxiosResponse<ApiResponse<DatasetPreview>>> => {
    return apiClient.post<ApiResponse<DatasetPreview>>(`/api/datasources/${id}/preview`, {
      table_name: tableName,
      query_sql: querySQL,
      query_type: queryType,
    });
  },

  // Get field distribution
  getFieldDistribution: (
    id: number,
    tableName: string,
    querySQL: string,
    queryType: string,
    fieldName: string,
    limit?: number
  ): Promise<AxiosResponse<ApiResponse<FieldDistribution>>> => {
    return apiClient.post<ApiResponse<FieldDistribution>>(
      `/api/datasources/${id}/field-distribution`,
      {
        table_name: tableName,
        query_sql: querySQL,
        query_type: queryType,
        field_name: fieldName,
        limit: limit || 20,
      }
    );
  },

  // 获取表数据预览（带分页、排序）
  getTableData: (
    id: number,
    tableName: string,
    page?: number,
    pageSize?: number,
    sortField?: string,
    sortOrder?: string
  ): Promise<AxiosResponse<ApiResponse<TableDataResult>>> => {
    const params = new URLSearchParams();
    if (page) params.set('page', String(page));
    if (pageSize) params.set('page_size', String(pageSize));
    if (sortField) params.set('sort_field', sortField);
    if (sortOrder) params.set('sort_order', sortOrder);
    return apiClient.get<ApiResponse<TableDataResult>>(
      `/api/datasources/${id}/tables/${encodeURIComponent(tableName)}/data?${params.toString()}`
    );
  },
};

// Datasets API
export const datasetsApi = {
  // Get all datasets
  getAll: (): Promise<AxiosResponse<ApiResponse<Dataset[]>>> => {
    return apiClient.get<ApiResponse<Dataset[]>>('/api/datasets');
  },

  // Get single dataset
  getById: (id: number): Promise<AxiosResponse<ApiResponse<Dataset>>> => {
    return apiClient.get<ApiResponse<Dataset>>(`/api/datasets/${id}`);
  },

  // Create dataset
  create: (data: DatasetFormData): Promise<AxiosResponse<ApiResponse<Dataset>>> => {
    return apiClient.post<ApiResponse<Dataset>>('/api/datasets', data);
  },

  // Update dataset. Wire convention matches the create contract
  // (datasetCreateIn / DatasetCreateRequest): tags/shard_keys arrive as
  // JSON-array strings, never array bodies. The create callers never send
  // those fields (backend defaults them to "[]"), while the update callers
  // (store.updateDataset, DatasetEdit submit) pass DatasetFormData with
  // string[] fields — so the stringification lives here, at the single
  // choke point every update request goes through (never double-encoded).
  update: (id: number, data: DatasetFormData): Promise<AxiosResponse<ApiResponse<Dataset>>> => {
    return apiClient.put<ApiResponse<Dataset>>(`/api/datasets/${id}`, {
      ...data,
      ...(data.tags !== undefined ? { tags: JSON.stringify(data.tags) } : {}),
      ...(data.shard_keys !== undefined ? { shard_keys: JSON.stringify(data.shard_keys) } : {}),
    });
  },

  // Delete dataset
  delete: (id: number): Promise<AxiosResponse<ApiResponse<{ status: string }>>> => {
    return apiClient.delete<ApiResponse<{ status: string }>>(`/api/datasets/${id}`);
  },

  // Get columns from dataset
  getColumns: (id: number): Promise<AxiosResponse<ApiResponse<DatasetColumn[]>>> => {
    return apiClient.get<ApiResponse<DatasetColumn[]>>(`/api/datasets/${id}/columns`);
  },

  // Update columns
  updateColumns: (
    id: number,
    columns: DatasetColumn[]
  ): Promise<AxiosResponse<ApiResponse<Dataset>>> => {
    return apiClient.post<ApiResponse<Dataset>>(`/api/datasets/${id}/columns`, columns);
  },

  // Get data preview
  getPreview: (id: number): Promise<AxiosResponse<ApiResponse<DatasetPreview>>> => {
    return apiClient.get<ApiResponse<DatasetPreview>>(`/api/datasets/${id}/preview`);
  },
};

// Charts API
export const chartsApi = {
  // Get all charts
  getAll: (): Promise<AxiosResponse<ApiResponse<Chart[]>>> => {
    return apiClient.get<ApiResponse<Chart[]>>('/api/charts');
  },

  // Get single chart
  getById: (id: number): Promise<AxiosResponse<ApiResponse<Chart>>> => {
    return apiClient.get<ApiResponse<Chart>>(`/api/charts/${id}`);
  },

  // Create chart
  create: (data: ChartFormData): Promise<AxiosResponse<ApiResponse<Chart>>> => {
    return apiClient.post<ApiResponse<Chart>>('/api/charts', data);
  },

  // Update chart
  update: (
    id: number,
    data: Partial<ChartFormData>
  ): Promise<AxiosResponse<ApiResponse<Chart>>> => {
    return apiClient.put<ApiResponse<Chart>>(`/api/charts/${id}`, data);
  },

  // Delete chart
  delete: (id: number): Promise<AxiosResponse<ApiResponse<{ status: string }>>> => {
    return apiClient.delete<ApiResponse<{ status: string }>>(`/api/charts/${id}`);
  },

  // Get chart data. v1-configured charts return the same aggregated payload
  // as POST /charts/query's ChartDataResult.data (shape discriminated by
  // chart_type); legacy/malformed/empty configs fall back to the bare
  // DataRow[] raw preview — ChartDataResponse covers both arms.
  getChartData: (id: number): Promise<AxiosResponse<ApiResponse<ChartDataResponse>>> => {
    return apiClient.get<ApiResponse<ChartDataResponse>>(`/api/charts/${id}/data`);
  },

  // Execute chart query
  executeChartQuery: (
    request: ChartQueryRequest
  ): Promise<AxiosResponse<ApiResponse<ChartQueryResponse>>> => {
    return apiClient.post<ApiResponse<ChartQueryResponse>>('/api/charts/query', request);
  },
};

// Shares API
export const sharesApi = {
  // Get all shares
  getAll: (): Promise<AxiosResponse<ApiResponse<Share[]>>> => {
    return apiClient.get<ApiResponse<Share[]>>('/api/shares');
  },

  // Create share
  create: (data: ShareFormData): Promise<AxiosResponse<ApiResponse<Share>>> => {
    return apiClient.post<ApiResponse<Share>>('/api/shares', data);
  },

  // Get share by token
  getByToken: (token: string): Promise<AxiosResponse<ApiResponse<Share>>> => {
    return apiClient.get<ApiResponse<Share>>(`/api/shares/${token}`);
  },

  // Verify share password
  verifyPassword: (token: string, password: string): Promise<AxiosResponse<ApiResponse<Share>>> => {
    return apiClient.post<ApiResponse<Share>>(`/api/shares/${token}/verify`, { password });
  },
};

export default apiClient;
