export type ChartType = 'line' | 'bar' | 'pie' | 'scatter' | 'table' | 'area';
export type ChartQueryAggregation = 'sum' | 'avg' | 'count' | 'max' | 'min';

export interface CreateChartRequest {
  name: string;
  dataset_id: number;
  chart_type: ChartType;
  config: string;
}

export interface UpdateChartRequest {
  name?: string;
  dataset_id?: number;
  chart_type?: ChartType;
  config?: string;
}

export interface ChartResponse {
  id: number;
  name: string;
  dataset_id: number;
  chart_type: string;
  config: string;
  created_at?: string;
  updated_at?: string;
}

export interface ChartListResponse {
  items: ChartResponse[];
  total: number;
}

export interface ChartQueryMetric {
  field: string;
  agg: ChartQueryAggregation;
  alias?: string;
}

export type ChartQueryFilterOp =
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

export interface ChartQueryFilter {
  field: string;
  operator: ChartQueryFilterOp;
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

export interface PieResponseItem {
  name: string;
  value: number;
  percentage: number;
}

export interface PieResponse {
  data: PieResponseItem[];
}

export interface AxisSeries {
  name: string;
  data: unknown[];
}

export interface AxisResponse {
  x_axis: string[];
  series: AxisSeries[];
}

export interface ScatterResponse {
  data: [number, number][];
}

export interface GeneratedSQL {
  select: string;
  count: string;
}
export type ChartDataResponse =
  | TableResponse
  | PieResponse
  | AxisResponse
  | ScatterResponse
  | unknown[];

export interface ChartQueryResponse {
  data: ChartDataResponse;
  generated_sql?: GeneratedSQL;
}
