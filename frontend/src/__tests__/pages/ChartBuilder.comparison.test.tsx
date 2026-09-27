import { render, screen, waitFor } from '@testing-library/react';
import type { AxiosResponse } from 'axios';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chartsApi, datasetsApi } from '../../api';
import type { ApiResponse } from '../../lib/api/client';
import ChartBuilder, { composeChartQueryRequest } from '../../pages/ChartBuilder';
import type { QueryConfig } from '../../store';
import { useStore } from '../../store';

/**
 * 同环比分析配置（issue #129）：
 * - wire：composeChartQueryRequest 把 queryOptions.comparison 发进
 *   query_options.comparison，且只发给后端消费的图型（后端做窗口平移基线查询）；
 * - UI：配置卡片仅在门控图型 + 恰好一个维度时出现；恢复的文档失效（切图型、
 *   多维度等）时由源头清掉，不留「配了没效果」的悬空状态。
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
  { id: 'fdate', name: 'order_date', type: 'dimension' as const, dataType: 'date' },
  { id: 'fbrand', name: 'brand', type: 'dimension' as const, dataType: 'string' },
  { id: 'famount', name: 'amount', type: 'metric' as const, dataType: 'number' },
];

const queryConfigWithDims = (dimFieldIds: string[]): QueryConfig => ({
  dimensionGroups: [
    {
      id: 'dim-group-main',
      bindings: dimFieldIds.map((fieldId, i) => ({ bindingId: `b-${i}`, fieldId })),
    },
  ],
  metricGroups: [
    { id: 'metric-group-main', bindings: [{ bindingId: 'b-m0', fieldId: 'famount' }] },
  ],
  filters: [],
});

const composeWith = (queryOptions: { comparison?: { type: 'mom' | 'yoy'; field?: string } }) =>
  composeChartQueryRequest({
    datasetId: 1,
    chartType: 'bar',
    queryConfig: queryConfigWithDims(['fdate']),
    fields: FIELDS,
    metricAggregations: {},
    metricAliases: {},
    tablePagination: { page: 1, pageSize: 20 },
    queryOptions,
    includeSort: false,
  });

describe('composeChartQueryRequest：同环比 wire', () => {
  it('bar + comparison 进 query_options.comparison，v1 平铺形状不变', () => {
    const request = composeWith({ comparison: { type: 'mom' } });
    expect(request?.query_options).toEqual({ comparison: { type: 'mom' } });
    expect(request?.dims).toEqual(['fdate']);
    expect(request?.spec_version).toBeUndefined();
  });

  it('未配置同环比时请求不带 query_options 键（形状与本改动前一致）', () => {
    expect(composeWith({})?.query_options).toBeUndefined();
  });

  it('yoy 原样透传', () => {
    const request = composeWith({ comparison: { type: 'yoy', field: 'fdate' } });
    expect(request?.query_options).toEqual({ comparison: { type: 'yoy', field: 'fdate' } });
  });
});

const mockGetDatasets = vi.mocked(datasetsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);
const mockGetChartById = vi.mocked(chartsApi.getById);
const mockExecuteChartQuery = vi.mocked(chartsApi.executeChartQuery);

const chartDoc = (chartType: string, dimFieldIds: string[], comparison: unknown) =>
  JSON.stringify({
    version: 2,
    chartType,
    title: 'T',
    query: {
      dimensionGroups: [
        {
          id: 'dim-group-main',
          bindings: dimFieldIds.map((fieldId, i) => ({ bindingId: `b-${i}`, fieldId })),
        },
      ],
      metricGroups: [
        { id: 'metric-group-main', bindings: [{ bindingId: 'b-m0', fieldId: 'famount' }] },
      ],
      filters: [],
    },
    fieldMeta: { 'b-m0': { aggregation: 'sum', alias: 'amount' } },
    style: {},
    queryOptions: comparison ? { comparison } : {},
  });

const seedApis = (config: string) => {
  mockGetDatasets.mockResolvedValue(
    mockAxiosResponse({
      code: 20000,
      msg: 'ok',
      trace: '',
      data: [
        {
          id: 1,
          name: 'Sales',
          datasource_id: 1,
          table_name: 'sales',
          query_sql: null,
          query_type: 'table',
          mode: 'direct',
          accelerate_config: null,
          description: null,
          tags: '[]',
          refresh_strategy: null,
          preview_data: null,
          quality_rules: '[]',
          columns: '[]',
          shard_enabled: false,
          shard_keys: '[]',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    })
  );
  mockGetColumns.mockResolvedValue(
    mockAxiosResponse({
      code: 20000,
      msg: 'ok',
      trace: '',
      data: [
        {
          id: 'fdate',
          name: 'order_date',
          expr: 'order_date',
          type: 'date',
          comment: '',
          role: 'dimension',
        },
        {
          id: 'fbrand',
          name: 'brand',
          expr: 'brand',
          type: 'string',
          comment: '',
          role: 'dimension',
        },
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
        id: 7,
        name: 'cmp',
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
                  x_axis: ['2026-09-01'],
                  series: [
                    { name: 'amount', data: [10] },
                    { name: 'amount(上期)', data: [5] },
                    { name: 'amount(增长率%)', data: [100] },
                  ],
                },
                select_sql: 'select 1',
              },
      })
    )
  );
};

const renderBuilder = () =>
  render(
    <MemoryRouter initialEntries={['/chart-builder?edit=7&datasetId=1']}>
      <ChartBuilder />
    </MemoryRouter>
  );

describe('ChartBuilder 同环比卡片（恢复已存图表）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useStore.getState().resetChartBuilder();
  });

  it('bar + 单维度 + comparison：卡片回显，wire 带 comparison', async () => {
    seedApis(chartDoc('bar', ['fdate'], { type: 'mom' }));
    renderBuilder();

    await waitFor(() => expect(screen.getByText('同环比')).toBeInTheDocument());
    const combo = screen.getByRole('combobox', { name: '同环比类型' });
    expect(combo.closest('.ant-select')?.textContent).toContain('环比');

    await waitFor(() => {
      const last =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(last?.query_options).toEqual({ comparison: { type: 'mom' } });
    });
  });

  it('comparison.field 悬空（指向已不存在的列）时，源头清掉且不进 wire', async () => {
    seedApis(chartDoc('bar', ['fdate'], { type: 'mom', field: 'ghost' }));
    renderBuilder();

    await waitFor(() => {
      expect(useStore.getState().chartQueryOptions.comparison).toBeUndefined();
    });
    await waitFor(() => {
      const last =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(last).toBeDefined();
      expect(last?.query_options).toBeUndefined();
    });
  });

  it('恢复的文档带着失效图型（pie）时，源头清掉 comparison', async () => {
    seedApis(chartDoc('pie', ['fdate'], { type: 'yoy' }));
    renderBuilder();

    await waitFor(() => expect(useStore.getState().chartQueryOptions.comparison).toBeUndefined());
    const last = mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
    expect(last?.query_options).toBeUndefined();
  });
});
