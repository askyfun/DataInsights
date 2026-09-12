import * as Sentry from '@sentry/react';
import axios, { AxiosInstance, AxiosResponse } from 'axios';
import type { ApiResponse } from '../lib/api/client';
import type { QueryConfig } from '../store';
import type { StandardDataType } from './datatypes';

// Types
export type DatasourceType = 'postgresql' | 'clickhouse' | 'mysql' | 'starrocks';

export interface Datasource {
  id?: number;
  name: string;
  type: DatasourceType;
  host: string;
  port: number;
  database_name: string;
  username: string;
  // 后端已脱敏：响应不再包含 password（Update 传空则后端保留原值）
  password?: string;
  created_at?: string;
  updated_at?: string;
}

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

export interface DatasetColumn {
  name: string;
  expr: string;
  type: StandardDataType;
  comment: string;
  role: ColumnRole;
}

export interface Dataset {
  id: number;
  name: string;
  datasource_id: number;
  table_name: string | null;
  query_sql: string | null;
  query_type: string;
  mode: DatasetMode;
  accelerate_config?: string;
  description?: string;
  tags?: string;
  refresh_strategy?: string;
  preview_data?: string;
  quality_rules?: string;
  columns: string;
  shard_enabled: boolean;
  shard_keys: string;
  created_at?: string;
  updated_at?: string;
}

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

export interface Chart {
  id: number;
  name: string;
  dataset_id: number;
  chart_type: string;
  config: string;
  created_at?: string;
  updated_at?: string;
}

export interface ChartFormData {
  name: string;
  dataset_id: number;
  chart_type: string;
  config: string;
}

export interface Share {
  id: number;
  token: string;
  chart_id: number;
  // 后端已脱敏：password 不再返回，has_password 是唯一的"是否受保护"信号
  has_password: boolean;
  expires_at?: string;
  created_at?: string;
}

export interface ShareFormData {
  chart_id: number;
  password?: string;
  expires_at?: string;
}

export interface TestConnectionRequest {
  type: DatasourceType;
  host: string;
  port: number;
  database_name: string;
  username: string;
  password: string;
}

export interface TableInfo {
  name: string;
  comment: string;
}

export interface ColumnInfo {
  name: string;
  data_type: string;
  comment: string;
  role: ColumnRole;
  is_virtual: boolean;
  expression: string;
}

// 数据预览
export interface DatasetPreview {
  columns: string[];
  data: Record<string, unknown>[];
}

// 表数据预览（带分页和主键信息）
export interface TableDataResult {
  columns: string[];
  data: Record<string, unknown>[];
  total: number;
  primary_keys: string[];
  page: number;
  page_size: number;
}

// 字段分布
export interface FieldDistribution {
  field_name: string;
  total_count: number;
  unique_count: number;
  distribution: Array<{
    value: unknown;
    count: number;
    percentage: number;
  }>;
}

// Charts API types
export type ChartQueryAggregation = 'sum' | 'avg' | 'count' | 'max' | 'min';

export interface ChartQueryMetric {
  field: string;
  agg: ChartQueryAggregation;
  alias?: string;
}

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

export interface ChartQueryPagination {
  page: number;
  page_size: number;
}

export interface ChartQuerySort {
  field: string;
  order: 'asc' | 'desc';
}

export interface ChartQueryRequest {
  dataset_id: number;
  chart_type: string;
  dims: string[];
  metrics: ChartQueryMetric[];
  filters: ChartQueryFilter[];
  pagination?: ChartQueryPagination;
  sort?: ChartQuerySort;
}

export interface TableResponse {
  columns: string[];
  data: Record<string, unknown>[];
  pagination: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
}

export interface PieResponse {
  data: Array<{
    name: string;
    value: number;
    percentage: number;
  }>;
}

export interface AxisResponse {
  x_axis: string[];
  series: Array<{
    name: string;
    data: unknown[];
  }>;
}

export interface ScatterResponse {
  data: Array<[number, number]>;
}

export type ChartDataResponse =
  | TableResponse
  | PieResponse
  | AxisResponse
  | ScatterResponse
  | unknown[];

export interface ChartQueryResponse {
  data: ChartDataResponse;
  select_sql?: string;
  count_sql?: string;
}

// Create single axios instance with proper interceptors
const apiClient: AxiosInstance = axios.create({
  baseURL: `http://${window.location.hostname || 'localhost'}:8080`,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor to add X-Request-ID
apiClient.interceptors.request.use(
  (config) => {
    const requestId = crypto.randomUUID();
    config.headers['X-Request-ID'] = requestId;
    return config;
  },
  (error) => Promise.reject(error)
);

// Response interceptor - validates code field from backend
apiClient.interceptors.response.use(
  (response) => {
    const { code, msg } = response.data || {};
    // Validate response code - reject if not 20000
    if (code !== undefined && code !== 20000) {
      console.error(`API Error [${code}]: ${msg}`);
      return Promise.reject(new Error(msg || `API Error: ${code}`));
    }
    return response;
  },
  (error) => {
    const status = error.response?.status;
    const apiMsg =
      error.response?.data?.msg ||
      error.response?.data?.message ||
      error.message ||
      'An error occurred';
    console.error('API Error:', apiMsg);

    // Report non-2xx responses to Sentry
    if (status && status >= 400) {
      Sentry.captureMessage(
        `API Error: ${error.response?.config?.method?.toUpperCase()} ${error.response?.config?.url} returned ${status}: ${apiMsg}`
      );
    }

    return Promise.reject(error);
  }
);

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

  // Get chart data
  getChartData: (id: number): Promise<AxiosResponse<ApiResponse<unknown[]>>> => {
    return apiClient.get<ApiResponse<unknown[]>>(`/api/charts/${id}/data`);
  },

  // Execute query with config
  executeQuery: (
    datasetId: number,
    config: QueryConfig
  ): Promise<AxiosResponse<ApiResponse<unknown[]>>> => {
    return apiClient.post<ApiResponse<unknown[]>>(`/api/datasets/${datasetId}/query`, config);
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
