import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { AxiosResponse } from 'axios';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chartsApi, datasetsApi } from '../../api';
import type { ApiResponse } from '../../lib/api/client';
import ChartBuilder, { composeChartQueryRequest } from '../../pages/ChartBuilder';
import type { QueryConfig } from '../../store';
import { useStore } from '../../store';

/**
 * Top N 分析配置（issue #130）：
 * - wire：composeChartQueryRequest 把 queryOptions.topN 发进 query_options.top_n，
 *   且只发给门控图型（bar/line/area/pie）；
 * - UI：卡片仅在门控图型 + 恰好一个维度时出现；恢复的文档失效（切图型、排名指标
 *   被移除）时在源头清掉，不留「配了没效果」的悬空状态。
 */

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

vi.mock('../../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>();
  return {
    ...actual,
    datasetsApi: { ...actual.datasetsApi, getAll: vi.fn(), getColumns: vi.fn() },
    chartsApi: {
      ...actual.chartsApi,
      getById: vi.fn(),
      executeChartQuery: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
    },
    queriesApi: { ...actual.queriesApi, save: vi.fn(), getByShortId: vi.fn() },
  };
});

vi.mock('echarts-for-react', () => ({
  default: () => <div data-testid="echarts" />,
}));

const FIELDS = [
  { id: 'fbrand', name: 'brand', type: 'dimension' as const, dataType: 'string' },
  { id: 'fcity', name: 'city', type: 'dimension' as const, dataType: 'string' },
  { id: 'famount', name: 'amount', type: 'metric' as const, dataType: 'number' },
];

const queryConfigWith = (dimIds: string[]): QueryConfig => ({
  dimensionGroups: [
    {
      id: 'dim-group-main',
      bindings: dimIds.map((fieldId, i) => ({ bindingId: `b-${i}`, fieldId })),
    },
  ],
  metricGroups: [
    { id: 'metric-group-main', bindings: [{ bindingId: 'b-m0', fieldId: 'famount' }] },
  ],
  filters: [],
});

const composeWith = (
  chartType: string,
  queryOptions: Parameters<typeof composeChartQueryRequest>[0]['queryOptions']
) =>
  composeChartQueryRequest({
    datasetId: 1,
    chartType: chartType as 'bar',
    queryConfig: queryConfigWith(['fbrand']),
    fields: FIELDS,
    metricAggregations: {},
    metricAliases: {},
    metricFormats: {},
    tablePagination: { page: 1, pageSize: 20 },
    queryOptions,
    includeSort: false,
  });

describe('composeChartQueryRequest：Top N wire', () => {
  it('bar + topN 进 query_options.top_n', () => {
    const request = composeWith('bar', { topN: { limit: 5, metric: 'famount', order: 'asc' } });
    expect(request?.query_options).toEqual({
      top_n: { limit: 5, metric: 'famount', order: 'asc' },
    });
  });

  it('未配置时不带 query_options 键（形状与本改动前一致）', () => {
    expect(composeWith('bar', {})?.query_options).toBeUndefined();
  });

  it('门控图型之外不下发（table 配了 topN 也不进请求）', () => {
    expect(composeWith('table', { topN: { limit: 5 } })?.query_options).toBeUndefined();
  });

  // 「其余合并为其他」（issue #116 验收行）：store 里是 camelCase mergeOther，
  // wire 上一律 snake_case merge_other，且只在打开时携带该键。
  it('mergeOther 打开 → wire 出 merge_other，且不留 camelCase', () => {
    const request = composeWith('bar', { topN: { limit: 5, metric: 'famount', mergeOther: true } });
    expect(request?.query_options).toEqual({
      top_n: { limit: 5, metric: 'famount', merge_other: true },
    });
  });

  it('mergeOther 关闭/未设置 → 不发该键（请求形状与改动前一致）', () => {
    expect(composeWith('bar', { topN: { limit: 5, mergeOther: false } })?.query_options).toEqual({
      top_n: { limit: 5 },
    });
    expect(composeWith('bar', { topN: { limit: 5 } })?.query_options).toEqual({
      top_n: { limit: 5 },
    });
  });

  it('histogram 的 bin_count 与 topN 同袋共存', () => {
    const request = composeWith('histogram', { binCount: 15, topN: { limit: 3 } });
    // histogram 不在 Top N 门控图型里 → 只发 bin_count
    expect(request?.query_options).toEqual({ bin_count: 15 });
  });
});

const mockGetDatasets = vi.mocked(datasetsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);
const mockGetChartById = vi.mocked(chartsApi.getById);
const mockExecuteChartQuery = vi.mocked(chartsApi.executeChartQuery);

const chartDoc = (chartType: string, dimIds: string[], topN: unknown) =>
  JSON.stringify({
    version: 2,
    chartType,
    title: 'T',
    query: {
      dimensionGroups: [
        {
          id: 'dim-group-main',
          bindings: dimIds.map((fieldId, i) => ({ bindingId: `b-${i}`, fieldId })),
        },
      ],
      metricGroups: [
        { id: 'metric-group-main', bindings: [{ bindingId: 'b-m0', fieldId: 'famount' }] },
      ],
      filters: [],
    },
    fieldMeta: {},
    style: {},
    queryOptions: topN ? { topN } : {},
  });

const seedApis = (config: string) => {
  mockGetDatasets.mockResolvedValue(
    mockAxiosResponse({ code: 20000, msg: 'ok', trace: '', data: [] })
  );
  mockGetColumns.mockResolvedValue(
    mockAxiosResponse({
      code: 20000,
      msg: 'ok',
      trace: '',
      data: [
        {
          id: 'fbrand',
          name: 'brand',
          expr: 'brand',
          type: 'string',
          comment: '',
          role: 'dimension',
        },
        { id: 'fcity', name: 'city', expr: 'city', type: 'string', comment: '', role: 'dimension' },
        {
          id: 'famount',
          name: 'amount',
          expr: 'amount',
          type: 'float',
          comment: '',
          role: 'metric',
        },
      ],
    })
  );
  mockGetChartById.mockResolvedValue(
    mockAxiosResponse({
      code: 20000,
      msg: 'ok',
      trace: '',
      data: {
        id: 8,
        name: 'topn',
        dataset_id: 1,
        chart_type: JSON.parse(config).chartType,
        config,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    })
  );
  mockExecuteChartQuery.mockImplementation((request) =>
    Promise.resolve(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data:
          request.chart_type === 'pie'
            ? { data: { data: [] } }
            : {
                data: {
                  x_axis: ['A'],
                  series: [{ name: 'amount', data: [1] }],
                },
                select_sql: 'select 1',
              },
      })
    )
  );
};

const renderBuilder = () =>
  render(
    <MemoryRouter initialEntries={['/chart-builder?edit=8&datasetId=1']}>
      <ChartBuilder />
    </MemoryRouter>
  );

describe('ChartBuilder Top N 卡片（恢复已存图表）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useStore.getState().resetChartBuilder();
  });

  it('bar + 单维度 + topN：卡片出现且回显，wire 带 top_n', async () => {
    seedApis(chartDoc('bar', ['fbrand'], { limit: 5, metric: 'famount' }));
    renderBuilder();

    await waitFor(() => expect(screen.getByText('Top N')).toBeInTheDocument());
    await waitFor(() => {
      const last =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(last?.query_options).toEqual({ top_n: { limit: 5, metric: 'famount' } });
    });
  });

  it('无 topN 时卡片关闭态；打开开关后 wire 出现 top_n', async () => {
    seedApis(chartDoc('bar', ['fbrand'], null));
    renderBuilder();

    await waitFor(() => expect(screen.getByText('Top N')).toBeInTheDocument());
    expect(screen.queryByLabelText('Top N 数量')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('switch', { name: 'Top N 开关' }));
    await waitFor(() => {
      const last =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(last?.query_options).toEqual({ top_n: { limit: 10, metric: 'famount' } });
    });
  });

  it('多维度：卡片不出现，失效的 topN 在源头被清掉', async () => {
    seedApis(chartDoc('bar', ['fbrand', 'fcity'], { limit: 5 }));
    renderBuilder();

    await waitFor(() => expect(mockExecuteChartQuery).toHaveBeenCalled());
    expect(screen.queryByText('Top N')).not.toBeInTheDocument();
    await waitFor(() => {
      expect(useStore.getState().chartQueryOptions.topN).toBeUndefined();
    });
  });

  it('排名指标悬空（列被移除）时，源头清掉 topN', async () => {
    seedApis(chartDoc('bar', ['fbrand'], { limit: 5, metric: 'ghost' }));
    renderBuilder();

    await waitFor(() => {
      expect(useStore.getState().chartQueryOptions.topN).toBeUndefined();
    });
  });
});

// 「其余合并为其他」的界面门控（issue #116 验收行）：只有全部活动指标可加
// （sum/count）才允许打开；不可加时置灰并写明原因，且已存的 mergeOther 在源头
// 被清掉——后端此时是显式报错而不是忽略，留着就等于把整张图打成一次失败查询。
const chartDocWithAgg = (topN: unknown, aggregation: string) =>
  JSON.stringify({
    version: 2,
    chartType: 'bar',
    title: 'T',
    query: {
      dimensionGroups: [
        { id: 'dim-group-main', bindings: [{ bindingId: 'b-0', fieldId: 'fbrand' }] },
      ],
      metricGroups: [
        { id: 'metric-group-main', bindings: [{ bindingId: 'b-m0', fieldId: 'famount' }] },
      ],
      filters: [],
    },
    fieldMeta: { 'b-m0': { aggregation } },
    style: {},
    queryOptions: topN ? { topN } : {},
  });

const mergeSwitch = () =>
  screen.getByRole('switch', { name: 'Top N 其余合并为其他' }) as HTMLInputElement;

describe('ChartBuilder Top N「其余合并为其他」', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useStore.getState().resetChartBuilder();
  });

  it('sum 指标：开关可用，打开后 wire 带 merge_other', async () => {
    seedApis(chartDocWithAgg({ limit: 5, metric: 'famount' }, 'sum'));
    renderBuilder();

    await waitFor(() => expect(mergeSwitch()).not.toBeDisabled());
    fireEvent.click(mergeSwitch());
    await waitFor(() => {
      const last =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(last?.query_options).toEqual({
        top_n: { limit: 5, metric: 'famount', merge_other: true },
      });
    });
  });

  it('count 指标同样可加', async () => {
    seedApis(chartDocWithAgg({ limit: 5, metric: 'famount' }, 'count'));
    renderBuilder();
    await waitFor(() => expect(mergeSwitch()).not.toBeDisabled());
  });

  it('avg 指标：开关置灰并说明原因', async () => {
    seedApis(chartDocWithAgg({ limit: 5, metric: 'famount' }, 'avg'));
    renderBuilder();

    await waitFor(() => expect(mergeSwitch()).toBeDisabled());
    expect(mergeSwitch()).not.toBeChecked();
  });

  it('存过的 mergeOther + 不可加指标：只清这一个子键，Top N 截断本身保留', async () => {
    seedApis(chartDocWithAgg({ limit: 5, metric: 'famount', mergeOther: true }, 'avg'));
    renderBuilder();

    await waitFor(() =>
      expect(useStore.getState().chartQueryOptions.topN?.mergeOther).toBeUndefined()
    );
    expect(useStore.getState().chartQueryOptions.topN?.limit).toBe(5);
    await waitFor(() => {
      const last =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(last?.query_options).toEqual({ top_n: { limit: 5, metric: 'famount' } });
    });
  });
});
