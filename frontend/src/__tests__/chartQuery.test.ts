import type { AxiosResponse } from 'axios';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ChartQueryResponse } from '@/api';
import { chartsApi } from '@/api';
import type { ApiResponse } from '@/lib/api/client';
import type { ChartType } from '@/lib/chartConfigSchema';
import { buildChartOption } from '@/lib/chartOptions';
import { composeChartQueryRequest } from '@/pages/ChartBuilder';
import { useStore } from '@/store';

/**
 * 图表查询响应消费测试（D7 修复后的契约）：
 * store.executeChartQuery 不再按 chartType 把结构化响应拆成平铺行数组，
 * chartData 状态即后端聚合负载原样（ChartDataResponse 联合，与 ShareView
 * 消费面一致），由共享的 buildChartOption 直接消费。
 */

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>();
  return {
    ...actual,
    chartsApi: {
      ...actual.chartsApi,
      executeChartQuery: vi.fn(),
    },
  };
});

const mockExecuteChartQuery = vi.mocked(chartsApi.executeChartQuery);

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

function mockChartQueryResult(data: ChartQueryResponse) {
  mockExecuteChartQuery.mockResolvedValue(
    mockAxiosResponse({ code: 20000, msg: 'ok', trace: '', data })
  );
}

const barRequest = {
  dataset_id: 1,
  chart_type: 'bar',
  dims: ['product'],
  metrics: [{ field: 'revenue', agg: 'sum' as const, alias: 'revenue' }],
  filters: [],
};

describe('executeChartQuery stores the structured payload verbatim', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useStore.getState().resetChartBuilder();
    useStore.setState({
      tablePagination: { page: 1, pageSize: 10, total: 0 },
      tableColumns: [],
      chartQueryResponse: null,
    });
  });

  it('bar/line/area: AxisResponse 原样存入 chartData（不再拆平铺行）', async () => {
    const axisPayload = {
      x_axis: ['Apple', 'Banana'],
      series: [{ name: 'revenue', data: [1000, 2000] }],
    };
    mockChartQueryResult({ data: axisPayload, select_sql: 'SELECT 1', count_sql: '' });

    await useStore.getState().executeChartQuery(barRequest);

    const state = useStore.getState();
    expect(state.chartData).toEqual(axisPayload);
    // 旧行为会产出 [{product:'Apple',revenue:1000},...] 平铺行——禁止回归
    expect(Array.isArray(state.chartData)).toBe(false);
    expect(state.chartQueryResponse?.select_sql).toBe('SELECT 1');
    expect(state.chartDataLoading).toBe(false);
  });

  it('多维度多指标：series 顺序与名称原样保留（不再用 dims[0] 组行键）', async () => {
    const axisPayload = {
      x_axis: ['2024-01'],
      series: [
        { name: 'revenue - Beijing', data: [100] },
        { name: 'revenue - Shanghai', data: [200] },
        { name: 'cost - Beijing', data: [50] },
        { name: 'cost - Shanghai', data: [80] },
      ],
    };
    mockChartQueryResult({ data: axisPayload, select_sql: '', count_sql: '' });

    await useStore.getState().executeChartQuery(barRequest);

    expect(useStore.getState().chartData).toEqual(axisPayload);
  });

  it('pie: PieResponse 原样存入（不再映射 {name,value} 行）', async () => {
    const piePayload = {
      data: [{ name: 'Apple', value: 30, percentage: 37.5 }],
    };
    mockChartQueryResult({ data: piePayload, select_sql: '', count_sql: '' });

    await useStore.getState().executeChartQuery({ ...barRequest, chart_type: 'pie' });

    expect(useStore.getState().chartData).toEqual(piePayload);
  });

  it('scatter: ScatterResponse 原样存入（二元组不再散架）', async () => {
    const scatterPayload = {
      data: [
        [100, 50],
        [200, 80],
      ],
    };
    mockChartQueryResult({ data: scatterPayload, select_sql: '', count_sql: '' });

    await useStore.getState().executeChartQuery({
      dataset_id: 1,
      chart_type: 'scatter',
      dims: [],
      metrics: [
        { field: 'w', agg: 'sum', alias: 'w' },
        { field: 'h', agg: 'sum', alias: 'h' },
      ],
      filters: [],
    });

    expect(useStore.getState().chartData).toEqual(scatterPayload);
  });

  it('table: chartData 存完整 TableResponse，仍提取 pagination/columns 供 TableChart', async () => {
    const tablePayload = {
      columns: ['region', 'revenue'],
      data: [{ region: 'East', revenue: 42 }],
      pagination: { page: 2, page_size: 20, total: 100, total_pages: 5 },
    };
    mockChartQueryResult({ data: tablePayload, select_sql: '', count_sql: '' });

    await useStore.getState().executeChartQuery({ ...barRequest, chart_type: 'table' });

    const state = useStore.getState();
    expect(state.chartData).toEqual(tablePayload);
    expect(state.tablePagination).toEqual({ page: 2, pageSize: 20, total: 100 });
    expect(state.tableColumns).toEqual(['region', 'revenue']);
  });

  it('pivot: 提取 columns，不触碰 pagination（PivotResponse 无分页）', async () => {
    const pivotPayload = {
      columns: ['region', 'city', 'revenue'],
      data: [{ region: 'East', city: 'SH', revenue: 1 }],
    };
    mockChartQueryResult({ data: pivotPayload, select_sql: '', count_sql: '' });

    await useStore.getState().executeChartQuery({ ...barRequest, chart_type: 'pivot' });

    const state = useStore.getState();
    expect(state.chartData).toEqual(pivotPayload);
    expect(state.tableColumns).toEqual(['region', 'city', 'revenue']);
    expect(state.tablePagination).toEqual({ page: 1, pageSize: 10, total: 0 });
  });

  it('查询失败：chartData 回落空数组（联合的 unknown[] 臂）', async () => {
    mockExecuteChartQuery.mockRejectedValue(new Error('boom'));

    await useStore.getState().executeChartQuery(barRequest);

    const state = useStore.getState();
    expect(state.chartData).toEqual([]);
    expect(state.chartQueryResponse).toBeNull();
  });
});

describe('store.chartData 可被共享 buildChartOption 直接消费（builder/share 同形状）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useStore.getState().resetChartBuilder();
  });

  it('executeChartQuery 后无需任何形状转换即可产出 option', async () => {
    const axisPayload = {
      x_axis: ['Apple', 'Banana'],
      series: [{ name: 'revenue', data: [1000, 2000] }],
    };
    mockChartQueryResult({ data: axisPayload, select_sql: '', count_sql: '' });

    await useStore.getState().executeChartQuery(barRequest);

    const option = buildChartOption(
      'bar',
      useStore.getState().chartData,
      { colors: [], smooth: false, tableRowSize: 'small' },
      {},
      { title: 'Sales', dimensions: ['product'], metrics: ['revenue'] }
    );
    expect(option).not.toBeNull();
    const shaped = option as unknown as {
      xAxis: { data: string[] };
      series: { name: string; type: string; data: unknown[] }[];
    };
    expect(shaped.xAxis.data).toEqual(['Apple', 'Banana']);
    expect(shaped.series).toEqual([{ name: 'revenue', type: 'bar', data: [1000, 2000] }]);
  });
});

describe('ChartType includes area', () => {
  it('should accept area as a valid ChartType value', () => {
    // This is primarily a compile-time check.
    // If 'area' is not in the ChartType union, this file won't compile.
    const chartType: ChartType = 'area';
    expect(chartType).toBe('area');
  });

  it('should accept all expected chart types', () => {
    const types: ChartType[] = ['line', 'bar', 'pie', 'scatter', 'table', 'area'];
    expect(types).toHaveLength(6);
  });
});

describe('composeChartQueryRequest：histogram 的 query_options.bin_count 发射（R-57）', () => {
  const histogramFields = [
    { id: 'f-1', name: 'amount', type: 'metric' as const, dataType: 'number' },
  ];
  const histogramQueryConfig = {
    dimensionGroups: [],
    metricGroups: [{ id: 'metric-group-1', bindings: [{ bindingId: 'b-0', field: 'f-1' }] }],
    filters: [],
  };
  const baseInput = {
    datasetId: 1,
    queryConfig: histogramQueryConfig,
    fields: histogramFields,
    metricAggregations: {},
    metricAliases: {},
    tablePagination: { page: 1, pageSize: 10 },
    includeSort: true,
  };

  it('histogram + binCount=15：wire 携带 snake_case query_options.bin_count，走 v1 平铺路径', () => {
    const request = composeChartQueryRequest({
      ...baseInput,
      chartType: 'histogram',
      queryOptions: { binCount: 15 },
    });

    expect(request).not.toBeNull();
    expect(request?.query_options).toEqual({ bin_count: 15 });
    expect(request?.spec_version).toBeUndefined();
    expect(request?.dims).toEqual([]);
    expect(request?.metrics).toEqual([{ field: 'amount', agg: 'sum', alias: 'amount' }]);
  });

  it('histogram + binCount 未设：bin_count 缺省 20', () => {
    const request = composeChartQueryRequest({
      ...baseInput,
      chartType: 'histogram',
      queryOptions: {},
    });

    expect(request?.query_options).toEqual({ bin_count: 20 });
  });

  it('非 histogram（bar）：请求不携带 query_options 键（不污染其它图型）', () => {
    const request = composeChartQueryRequest({
      ...baseInput,
      chartType: 'bar',
      queryConfig: {
        dimensionGroups: [{ id: 'dim-group-1', bindings: [{ bindingId: 'b-1', field: 'f-1' }] }],
        metricGroups: [],
        filters: [],
      },
      queryOptions: { binCount: 15 },
    });

    expect(request).not.toBeNull();
    expect(request).not.toHaveProperty('query_options');
  });
});

describe('composeChartQueryRequest：funnel 强制 value 降序（R-59，验收行767）', () => {
  const funnelFields = [
    { id: 'f-1', name: 'stage', type: 'dimension' as const, dataType: 'string' },
    { id: 'f-2', name: 'cnt', type: 'metric' as const, dataType: 'number' },
  ];
  const funnelQueryConfig = {
    dimensionGroups: [{ id: 'dim-group-1', bindings: [{ bindingId: 'b-0', field: 'f-1' }] }],
    metricGroups: [{ id: 'metric-group-1', bindings: [{ bindingId: 'b-1', field: 'f-2' }] }],
    filters: [],
  };
  const baseInput = {
    datasetId: 1,
    chartType: 'funnel' as const,
    queryConfig: funnelQueryConfig,
    fields: funnelFields,
    metricAggregations: {},
    metricAliases: {},
    tablePagination: { page: 1, pageSize: 10 },
    queryOptions: {},
    includeSort: true,
  };

  it('funnel：v1 平铺形状（dims=[stages]、metrics=[value]）+ 强制 sort value desc', () => {
    const request = composeChartQueryRequest(baseInput);

    expect(request).not.toBeNull();
    expect(request?.spec_version).toBeUndefined();
    expect(request?.dims).toEqual(['stage']);
    expect(request?.metrics).toEqual([{ field: 'cnt', agg: 'sum', alias: 'cnt' }]);
    expect(request?.sort).toEqual({ field: 'cnt', order: 'desc' });
  });

  it('value 输出名走 metricAliases 覆盖（与 v1 sortWireField 同口径）', () => {
    const request = composeChartQueryRequest({
      ...baseInput,
      metricAliases: { 'b-1': 'total_cnt' },
    });

    expect(request?.sort).toEqual({ field: 'total_cnt', order: 'desc' });
    expect(request?.metrics).toEqual([{ field: 'cnt', agg: 'sum', alias: 'total_cnt' }]);
  });

  it('覆盖性：用户此前设的 sort（stages asc）被无条件覆盖为 value desc', () => {
    const request = composeChartQueryRequest({
      ...baseInput,
      queryConfig: { ...funnelQueryConfig, sort: { bindingId: 'b-0', order: 'asc' as const } },
    });

    expect(request?.sort).toEqual({ field: 'cnt', order: 'desc' });
  });

  it('includeSort:false 不影响强制降序（funnel 恒发 value desc sort）', () => {
    const request = composeChartQueryRequest({ ...baseInput, includeSort: false });

    expect(request?.sort).toEqual({ field: 'cnt', order: 'desc' });
  });

  it('value 绑定缺失（防御）：不注入 sort，但请求照常发出', () => {
    const request = composeChartQueryRequest({
      ...baseInput,
      queryConfig: { ...funnelQueryConfig, metricGroups: [{ id: 'metric-group-1', bindings: [] }] },
    });

    expect(request).not.toBeNull();
    expect(request).not.toHaveProperty('sort');
  });

  it('value 绑定缺失 + 恢复的用户 sort 并存：不泄露 sort（funnelSortPayload 返回空而非回退）', () => {
    // 退化角落：value 绑定查不到（funnelSortPayload 无法算出强制降序）但持久化恢复带了
    // 一个仍可解析的用户 sort（stages b-0 asc）。若 funnelSortPayload 返回 undefined，下方
    // `?? ` 会回退到原始 sort 表达式、把 stages asc 泄露进请求；返回 {} 则彻底不带 sort。
    const request = composeChartQueryRequest({
      ...baseInput,
      queryConfig: {
        ...funnelQueryConfig,
        metricGroups: [{ id: 'metric-group-1', bindings: [] }],
        sort: { bindingId: 'b-0', order: 'asc' as const },
      },
    });

    expect(request).not.toBeNull();
    expect(request).not.toHaveProperty('sort');
  });

  it('非 funnel（bar）+ 无 queryConfig.sort：请求不含 sort 键（既有行为未污染）', () => {
    const request = composeChartQueryRequest({
      ...baseInput,
      chartType: 'bar',
      queryConfig: {
        dimensionGroups: [{ id: 'dim-group-1', bindings: [{ bindingId: 'b-0', field: 'f-1' }] }],
        metricGroups: [{ id: 'metric-group-1', bindings: [{ bindingId: 'b-1', field: 'f-2' }] }],
        filters: [],
      },
    });

    expect(request).not.toBeNull();
    expect(request).not.toHaveProperty('sort');
  });
});
