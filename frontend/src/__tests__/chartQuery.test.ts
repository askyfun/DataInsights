import type { AxiosResponse } from 'axios';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ChartQueryResponse } from '@/api';
import { chartsApi } from '@/api';
import type { ApiResponse } from '@/lib/api/client';
import type { ChartType } from '@/lib/chartConfigSchema';
import { buildChartOption } from '@/lib/chartOptions';
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
