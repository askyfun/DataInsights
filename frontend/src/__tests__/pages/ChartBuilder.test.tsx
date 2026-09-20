import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { AxiosResponse } from 'axios';
import { type ReactNode, StrictMode } from 'react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { QueryRecord } from '../../api';
import { chartsApi, datasetsApi, queriesApi } from '../../api';
import type { ApiResponse } from '../../lib/api/client';
import type { ChartConfigDocument } from '../../lib/chartConfigSchema';
import ChartBuilder from '../../pages/ChartBuilder';
import { useStore } from '../../store';

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

// 捕获 ChartCanvas 传给 ReactECharts 的 option（本文件的渲染断言只看 option 结构，
// 不挂载真实 echarts）。vi.hoisted 保证在 vi.mock 工厂执行前已初始化。
interface CapturedChartOption {
  series?: Array<{ name?: string; type?: string; yAxisIndex?: number }>;
}
const echartsOptionCapture = vi.hoisted(() => ({ current: null as CapturedChartOption | null }));

vi.mock('echarts-for-react', () => ({
  default: ({ option }: { option: CapturedChartOption }) => {
    echartsOptionCapture.current = option;
    return <div data-testid="echarts" />;
  },
}));

vi.mock('../../components/ChartBuilder/DraggableField', () => ({
  default: () => <div data-testid="draggable-field" />,
}));

vi.mock('../../components/ChartBuilder/FilterDropZone', () => ({
  default: () => <div data-testid="filter-drop-zone" />,
}));

vi.mock('../../components/ChartBuilder/QueryConfigRow', () => ({
  default: ({
    label,
    groupIndex,
    onAddField,
    availableFields,
    rowType,
    children,
  }: {
    label: string;
    groupIndex?: number;
    onAddField?: (field: { id: string; name: string; type: 'dimension' | 'metric' }) => void;
    availableFields?: Array<{ id: string; name: string; type: 'dimension' | 'metric' }>;
    rowType: 'dimension' | 'metric' | 'filter';
    children?: ReactNode;
  }) => (
    <div data-testid="query-config-row">
      <span>{label}</span>
      <span data-testid={`group-index-${label}`}>{String(groupIndex ?? '')}</span>
      {rowType === 'metric' &&
        onAddField &&
        availableFields?.some((field) => field.type === 'metric') && (
          <button
            type="button"
            onClick={() => {
              const metricField = availableFields.find((field) => field.type === 'metric');
              if (metricField) {
                onAddField(metricField);
              }
            }}
          >
            添加{label}
          </button>
        )}
      {children}
    </div>
  ),
}));

vi.mock('../../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api')>();
  return {
    ...actual,
    datasetsApi: {
      ...actual.datasetsApi,
      getAll: vi.fn(),
      getColumns: vi.fn(),
    },
    chartsApi: {
      ...actual.chartsApi,
      getById: vi.fn(),
      executeChartQuery: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
    },
    // 查询记录落库必须打桩：真实实现会走 axios，「地址栏即分享」是查询成功后的
    // 副作用，不打桩会污染 console.error 断言（Network Error）。
    queriesApi: {
      ...actual.queriesApi,
      save: vi.fn(),
      getByShortId: vi.fn(),
    },
  };
});

const mockGetDatasets = vi.mocked(datasetsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);
const mockGetChartById = vi.mocked(chartsApi.getById);
const mockExecuteChartQuery = vi.mocked(chartsApi.executeChartQuery);
const mockUpdateChart = vi.mocked(chartsApi.update);
const mockSaveQueryRecord = vi.mocked(queriesApi.save);
const mockGetQueryRecord = vi.mocked(queriesApi.getByShortId);

/**
 * 把地址栏暴露成可断言的节点。「地址栏即分享」的验收对象就是 location.search，
 * 没有它只能间接推断，容易写出"改对了地址栏也照样通过"的假测试。
 */
function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location-search">{location.search}</div>;
}

const resetChartBuilderState = () => {
  useStore.setState({
    datasets: [],
    datasetsLoading: false,
    datasetsError: null,
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
      dimensionGroups: [
        { id: 'dim-group-main', bindings: [{ bindingId: 'b-0', field: 'region' }] },
      ],
      metricGroups: [],
      filters: [],
      limit: 1000,
    },
    autoQuery: true,
    metricAggregations: {},
    metricAliases: {},
    chartQueryOptions: {},
    chartQueryResponse: null,
    tablePagination: { page: 1, pageSize: 10, total: 0 },
    tableColumns: [],
  });
};

const renderChartBuilder = () => {
  return render(
    <MemoryRouter initialEntries={['/chart-builder?edit=1&datasetId=1']}>
      <LocationProbe />
      <Routes>
        <Route path="/chart-builder" element={<ChartBuilder />} />
      </Routes>
    </MemoryRouter>
  );
};

const renderNewChartBuilder = () => {
  return render(
    <MemoryRouter initialEntries={['/chart-builder?datasetId=1']}>
      <LocationProbe />
      <Routes>
        <Route path="/chart-builder" element={<ChartBuilder />} />
      </Routes>
    </MemoryRouter>
  );
};

/**
 * 以 `?q=<短码>` 直链进入，可附加其他参数以验证优先级。
 * `strict` 复刻 `main.tsx` 的运行时环境（dev 下 StrictMode 会 mount → cleanup → 再
 * mount）。直链还原是一次性副作用，恰恰是 StrictMode 双挂载最容易吃掉的动作，
 * 默认测试环境不带 StrictMode，所以这里必须能显式打开。
 */
const renderQueryRecordBuilder = (shortId: string, extraQuery = '', strict = false) => {
  const tree = (
    <MemoryRouter initialEntries={[`/chart-builder?q=${shortId}${extraQuery}`]}>
      <LocationProbe />
      <Routes>
        <Route path="/chart-builder" element={<ChartBuilder />} />
      </Routes>
    </MemoryRouter>
  );
  return render(strict ? <StrictMode>{tree}</StrictMode> : tree);
};

describe('ChartBuilder', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    echartsOptionCapture.current = null;
    resetChartBuilderState();

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
            name: 'region',
            expr: 'region',
            type: 'string',
            comment: '',
            role: 'dimension',
          },
          {
            name: 'revenue',
            expr: 'revenue',
            type: 'number',
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
          id: 1,
          name: 'Sales Table',
          dataset_id: 1,
          chart_type: 'table',
          config: JSON.stringify({
            version: 1,
            chartType: 'table',
            title: 'Sales Table',
            query: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['region'] }],
              metricGroups: [],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    mockExecuteChartQuery.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          data: {
            columns: ['region'],
            data: [{ region: 'East' }],
            pagination: {
              page: 1,
              page_size: 10,
              total: 42,
              total_pages: 5,
            },
          },
          select_sql: 'select region from sales',
          count_sql: 'select count(*) from sales',
        },
      })
    );

    // 落库默认成功：短码取自后端的 21 位 base58 定长编码。
    mockSaveQueryRecord.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          query_id: 'CSatF9qXyRoQXFtAA3iZS',
          created_at: '2026-09-19T12:00:00Z',
          expires_at: null,
        },
      })
    );
  });

  it('does not repeat table auto query when the response only updates total pagination', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(useStore.getState().tablePagination.total).toBe(42);
    });

    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    expect(mockExecuteChartQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        chart_type: 'table',
        pagination: { page: 1, page_size: 10 },
      })
    );
  });

  it('switches query group labels based on chart definition', async () => {
    renderNewChartBuilder();

    await waitFor(() => {
      expect(screen.getAllByTestId('query-config-row').length).toBeGreaterThan(0);
    });

    expect(screen.getByText('维度')).toBeInTheDocument();
    expect(screen.getByText('指标')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /饼图$/ }));
    expect(screen.getByText('分类')).toBeInTheDocument();
    expect(screen.getByText('数值')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /散点图$/ }));
    expect(screen.getByText('X 轴指标')).toBeInTheDocument();
    expect(screen.getByText('Y 轴指标')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /透视表$/ }));
    expect(screen.getByText('行维度')).toBeInTheDocument();
    expect(screen.getByText('列维度')).toBeInTheDocument();
    expect(screen.getByText('值指标')).toBeInTheDocument();
  });

  it('maps table metric row to metric group 0 and keeps metrics in execute query request', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(screen.getByTestId('group-index-维度')).toHaveTextContent('0');
    });

    expect(screen.getByTestId('group-index-指标')).toHaveTextContent('0');

    fireEvent.click(screen.getByRole('button', { name: '添加指标' }));

    await waitFor(() => {
      expect(useStore.getState().queryConfig.metricGroups[0]?.bindings).toEqual([
        { bindingId: 'b-1', field: 'revenue' },
      ]);
    });

    expect(mockExecuteChartQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        chart_type: 'table',
        dims: ['region'],
        metrics: [{ field: 'revenue', agg: 'sum', alias: 'revenue' }],
      })
    );
  });

  it('restores metric aliases and aggregations from saved chart config', async () => {
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Revenue Table',
          dataset_id: 1,
          chart_type: 'table',
          config: JSON.stringify({
            version: 1,
            chartType: 'table',
            title: 'Revenue Table',
            query: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['region'] }],
              metricGroups: [{ id: 'metric-group-main', fields: ['revenue'] }],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {
              revenue: { aggregation: 'avg', alias: 'gmv' },
            },
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(useStore.getState().metricAliases['b-1']).toBe('gmv');
    });

    expect(useStore.getState().metricAggregations['b-1']).toBe('avg');
  });

  it('restores dimension labels, metric units, formats, style and query options from saved config', async () => {
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Styled Pie',
          dataset_id: 1,
          chart_type: 'pie',
          config: JSON.stringify({
            version: 1,
            chartType: 'pie',
            title: 'Styled Pie',
            query: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['region'] }],
              metricGroups: [{ id: 'metric-group-main', fields: ['revenue'] }],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {
              region: { label: '区域' },
              revenue: { unit: '元', format: '0,0.00' },
            },
            style: {
              colors: ['#ff4d4f'],
              smooth: true,
              tableRowSize: 'middle',
            },
            queryOptions: {
              pieMergeOtherBelowRatio: 5,
            },
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(useStore.getState().dimensionLabels['b-0']).toBe('区域');
    });

    expect(useStore.getState().metricUnits['b-1']).toBe('元');
    expect(useStore.getState().metricFormats['b-1']).toBe('0,0.00');
    expect(useStore.getState().chartStyle.colors).toEqual(['#ff4d4f']);
    expect(useStore.getState().chartStyle.smooth).toBe(true);
    expect(useStore.getState().chartStyle.tableRowSize).toBe('middle');
    expect(useStore.getState().chartQueryOptions.pieMergeOtherBelowRatio).toBe(5);
  });

  it('does not emit duplicate key warnings when the same field appears in multiple groups', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Pivot Chart',
          dataset_id: 1,
          chart_type: 'pivot',
          config: JSON.stringify({
            version: 1,
            chartType: 'pivot',
            title: 'Pivot Chart',
            query: {
              dimensionGroups: [
                { id: 'dim-group-rows', fields: ['region'] },
                { id: 'dim-group-columns', fields: ['region'] },
              ],
              metricGroups: [],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    expect(
      errorSpy.mock.calls.some(
        ([message]) =>
          typeof message === 'string' &&
          message.includes('Encountered two children with the same key')
      )
    ).toBe(false);

    errorSpy.mockRestore();
  });

  it('renders KpiCard (AntD Statistic, not ECharts) for kpi charts and sends a v1 zero-dim request', async () => {
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Revenue KPI',
          dataset_id: 1,
          chart_type: 'kpi',
          config: JSON.stringify({
            version: 1,
            chartType: 'kpi',
            title: 'Revenue KPI',
            query: {
              dimensionGroups: [],
              metricGroups: [{ id: 'metric-group-main', fields: ['revenue'] }],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {
              revenue: { unit: '元' },
            },
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );
    // kpi 的结构化响应是标量 {value, label}（后端 KpiProcessor 产物）
    mockExecuteChartQuery.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          data: { value: 95380, label: 'revenue' },
          select_sql: 'SELECT SUM(revenue) AS "revenue" FROM sales',
        },
      })
    );

    const { container } = renderChartBuilder();

    await waitFor(() => {
      expect(screen.getByText('95,380')).toBeInTheDocument();
    });

    // KPI 卡走 AntD Statistic：标题为指标 label，unit（来自 store metricUnits[bindingId]）作 suffix
    expect(container.querySelector('.ant-statistic-title')?.textContent).toBe('revenue');
    expect(screen.getByText('元')).toBeInTheDocument();
    // 不走 ECharts（ChartCanvas 在 kpi 分支之前已被绕过）
    expect(screen.queryByTestId('echarts')).not.toBeInTheDocument();

    // wire 请求：kpi 无维度槽位 → v1 平铺格式，dims 为空数组、单指标
    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          chart_type: 'kpi',
          dims: [],
          metrics: [expect.objectContaining({ field: 'revenue' })],
        })
      );
    });
  });

  it('limits pie query metrics to the visible chart definition groups', async () => {
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Pie Chart',
          dataset_id: 1,
          chart_type: 'pie',
          config: JSON.stringify({
            version: 1,
            chartType: 'pie',
            title: 'Pie Chart',
            query: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['region'] }],
              metricGroups: [
                { id: 'metric-group-main', fields: ['revenue'] },
                { id: 'metric-group-extra', fields: ['revenue'] },
              ],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    expect(mockExecuteChartQuery).toHaveBeenLastCalledWith(
      expect.objectContaining({
        chart_type: 'pie',
        dims: ['region'],
        metrics: [{ field: 'revenue', agg: 'sum', alias: 'revenue' }],
      })
    );
  });

  it('sends no config payload and renders no ratio control for pie charts', async () => {
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Pie Chart',
          dataset_id: 1,
          chart_type: 'pie',
          config: JSON.stringify({
            version: 1,
            chartType: 'pie',
            title: 'Pie Chart',
            query: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['region'] }],
              metricGroups: [{ id: 'metric-group-main', fields: ['revenue'] }],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
            queryOptions: { pieMergeOtherBelowRatio: 5 },
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    // 文档兼容性：保存的 queryOptions 仍会恢复到 store（但不再参与请求）。
    expect(useStore.getState().chartQueryOptions).toEqual({ pieMergeOtherBelowRatio: 5 });

    const request = mockExecuteChartQuery.mock.calls[0][0];
    // 请求体不得携带 config / query_options（断链控件已移除）。
    expect(request).not.toHaveProperty('config');
    expect(request).not.toHaveProperty('query_options');
    // 钉死其余请求形状不变。sort 键仅在 queryConfig.sort 存在且 bindingId 可解析时
    // 才携带（Task 1-7 起自动查询 effect 也发送 sort；本用例未设置 sort）。
    expect(Object.keys(request).sort()).toEqual([
      'chart_type',
      'dataset_id',
      'dims',
      'filters',
      'metrics',
      'pagination',
    ]);

    // UI 不再有「合并其他比例」输入控件。
    expect(screen.queryByText('饼图“其他”阈值 (%)')).not.toBeInTheDocument();
  });

  it('auto query effect sends sort with the output-name wire value on v1 flat requests (R-50)', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    // revenue → 指标组 b-1，显示名 gmv（不进 wire）；sort 引用 bindingId
    act(() => {
      const state = useStore.getState();
      state.addMetricField(state.chartBuilderFields[1], 0);
      state.setMetricAlias('b-1', 'gmv');
      state.setQueryConfig({ sort: { bindingId: 'b-1', order: 'desc' } });
    });

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(2);
    });

    const request = mockExecuteChartQuery.mock.calls[1][0];
    // v1 平铺请求：sort.field / metrics[].alias 都是输出列名 = 字段列名。
    // 别名（gmv）是纯展示配置，不再进 wire——中文别名曾把 SQL AS 子句打成
    // _invalid_identifier 切断列数据。
    expect(request.sort).toEqual({ field: 'revenue', order: 'desc' });
    expect(request.metrics).toEqual([{ field: 'revenue', agg: 'sum', alias: 'revenue' }]);
  });

  it('sends sort.field as the raw bindingId on v2 slot-protocol requests (裁定B 选择1)', async () => {
    mockGetColumns.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: [
          { name: 'month', expr: 'month', type: 'string', comment: '', role: 'dimension' },
          { name: 'revenue', expr: 'revenue', type: 'number', comment: '', role: 'metric' },
          { name: 'growth', expr: 'growth', type: 'number', comment: '', role: 'metric' },
        ],
      })
    );

    // 用 combo（双轴指标槽位 → 恒走 v2）取代此前的 bar + 第二维度组，
    // 后者在 color_group 槽位下线后已不再触发槽位协议（见下一条 legacy 用例）。
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Combo',
          dataset_id: 1,
          chart_type: 'combo',
          config: JSON.stringify({
            version: 2,
            chartType: 'combo',
            title: 'Combo',
            query: {
              dimensionGroups: [{ id: 'x_axis', bindings: [{ bindingId: 'b-0', field: 'month' }] }],
              metricGroups: [
                { id: 'primary_values', bindings: [{ bindingId: 'b-1', field: 'revenue' }] },
                { id: 'secondary_values', bindings: [{ bindingId: 'b-2', field: 'growth' }] },
              ],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('combo');
      expect(lastRequest?.spec_version).toBe(2);
    });

    // v2 槽位协议：sort 引用主轴指标 binding b-1，wire 上直接发 bindingId
    act(() => {
      useStore.getState().setQueryConfig({ sort: { bindingId: 'b-1', order: 'desc' } });
    });

    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.sort).toEqual({ field: 'b-1', order: 'desc' });
    });

    const request =
      mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
    expect(request?.spec_version).toBe(2);
  });

  it('drops the sort key when the referenced bindingId no longer exists (悬挂引用)', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    act(() => {
      useStore.getState().setQueryConfig({ sort: { bindingId: 'b-99', order: 'asc' } });
    });

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(2);
    });

    const request = mockExecuteChartQuery.mock.calls[1][0];
    // 悬挂 bindingId 不得上 wire（后端会渲染成 _invalid_identifier 破坏 SQL）
    expect(request).not.toHaveProperty('sort');
  });

  it('translates a table header sort click into a bindingId-keyed queryConfig.sort', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    // 表格首列 region（维度 binding b-0）：点击表头触发 antd sorter（首次点击 = ascend）。
    // 用 columnheader role 定位表头单元格，避免命中侧边栏的同名字段标签。
    await waitFor(() => {
      expect(screen.getByText('East')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('columnheader', { name: /region/ }));

    await waitFor(() => {
      expect(useStore.getState().queryConfig.sort).toEqual({ bindingId: 'b-0', order: 'asc' });
    });

    // 点击排序会依次触发：handleSortChange 直发 → autoQuery effect 重跑。两次都必须
    // 携带 sort——R-50 修复前 effect 那次从不带 sort，会用未排序结果覆盖已排序数据；
    // 另有一次"分页副作用"重建请求（闭包里尚无新 sort）曾与之竞态，Task 已从 TableChart
    // 侧去掉（页码未变不再回调 onPageChange），因此这里不再有第 4 次调用。
    await waitFor(() => {
      const calls = mockExecuteChartQuery.mock.calls;
      expect(calls.length).toBeGreaterThanOrEqual(3);
      for (const call of calls.slice(-2)) {
        // v1 平铺：维度绑定的输出列名 = 列名本身
        expect(call[0].sort).toEqual({ field: 'region', order: 'asc' });
      }
    });
  });

  it('sends filters with the operator key to match the backend entity.Filter contract', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    act(() => {
      useStore.getState().addFilter({
        id: 'filter-gt-1',
        field: 'region',
        operator: 'gt',
        value: 100,
        logic: 'and',
      });
    });

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(2);
    });

    const request = mockExecuteChartQuery.mock.calls[1][0];
    expect(request.filters).toHaveLength(1);
    expect(request.filters[0]).toEqual(
      expect.objectContaining({ field: 'region', operator: 'gt', value: 100, logic: 'and' })
    );
    expect(request.filters[0]).not.toHaveProperty('op');
  });

  it('loads saved metric groups with a sparse hole without crashing and normalizes them for scatter charts', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Scatter Chart',
          dataset_id: 1,
          chart_type: 'scatter',
          config: JSON.stringify({
            version: 1,
            chartType: 'scatter',
            title: 'Scatter Chart',
            query: {
              dimensionGroups: [],
              metricGroups: [null, { id: 'metric-group-secondary', fields: ['revenue'] }],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(useStore.getState().chartBuilderConfig.chartType).toBe('scatter');
    });

    // 迁移丢弃空洞条目，图表定义规范化补齐到散点图需要的 2 个指标组
    expect(useStore.getState().queryConfig.metricGroups).toEqual([
      { id: 'metric-group-secondary', bindings: [{ bindingId: 'b-0', field: 'revenue' }] },
      { id: 'metric-group-2', bindings: [] },
    ]);
    expect(errorSpy).not.toHaveBeenCalled();

    errorSpy.mockRestore();
  });

  it('migrates a legacy config with positional field ids to column-name state on load', async () => {
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Legacy Sales Table',
          dataset_id: 1,
          chart_type: 'table',
          config: JSON.stringify({
            chartType: 'table',
            xAxisField: null,
            yAxisFields: [],
            title: 'Legacy Sales Table',
            queryConfig: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['field-0'] }],
              metricGroups: [{ id: 'metric-group-main', fields: ['field-1'] }],
              filters: [
                { id: 'f-0', field: 'field-0', operator: 'eq', value: 'East', logic: 'and' },
              ],
              limit: 1000,
            },
            dimensionLabels: { 'field-0': '区域' },
            metricAggregations: { 'field-1': 'avg' },
            metricAliases: { 'field-1': 'gmv' },
            chartStyle: { colors: ['#1f77b4'], smooth: false, tableRowSize: 'middle' },
            chartQueryOptions: { pieMergeOtherBelowRatio: 5 },
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    // 旧位置 id 借助 chartBuilderFields 解析为列名，再转 v2 bindings
    await waitFor(() => {
      expect(useStore.getState().queryConfig.dimensionGroups[0]?.bindings).toEqual([
        { bindingId: 'b-0', field: 'region' },
      ]);
    });

    expect(useStore.getState().queryConfig.metricGroups[0]?.bindings).toEqual([
      { bindingId: 'b-1', field: 'revenue' },
    ]);
    expect(useStore.getState().queryConfig.filters[0]?.field).toBe('region');
    expect(useStore.getState().dimensionLabels).toEqual({ 'b-0': '区域' });
    expect(useStore.getState().metricAggregations).toEqual({ 'b-1': 'avg' });
    expect(useStore.getState().metricAliases).toEqual({ 'b-1': 'gmv' });
    expect(useStore.getState().metricUnits).toEqual({});
    expect(useStore.getState().chartStyle.tableRowSize).toBe('middle');
    expect(useStore.getState().chartQueryOptions.pieMergeOtherBelowRatio).toBe(5);
  });

  it('saves the chart config as a v2 document with bindings and bindingId-keyed fieldMeta', async () => {
    mockUpdateChart.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Sales Table',
          dataset_id: 1,
          chart_type: 'table',
          config: '',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    // 等待字段加载 + loadChartConfig 完成迁移（region 绑定为 b-0），保证后续按
    // bindingId 写入是确定性的。
    await waitFor(() => {
      expect(useStore.getState().chartBuilderFields).toHaveLength(2);
      expect(useStore.getState().queryConfig.dimensionGroups[0]?.bindings).toEqual([
        { bindingId: 'b-0', field: 'region' },
      ]);
    });

    act(() => {
      const state = useStore.getState();
      state.setDimensionLabel('b-0', '区域');
      state.addMetricField(state.chartBuilderFields[1], 0); // revenue → b-1
      state.setMetricAlias('b-1', 'gmv');
    });

    fireEvent.click(screen.getByRole('button', { name: /更新$/ }));

    await waitFor(() => {
      expect(mockUpdateChart).toHaveBeenCalledTimes(1);
    });

    const payload = mockUpdateChart.mock.calls[0][1];
    expect(payload).toEqual(
      expect.objectContaining({
        name: 'Sales Table',
        dataset_id: 1,
        chart_type: 'table',
      })
    );

    const savedDoc = JSON.parse(payload.config ?? '') as ChartConfigDocument;
    expect(savedDoc).toEqual({
      version: 2,
      chartType: 'table',
      title: 'Sales Table',
      query: {
        dimensionGroups: [
          { id: 'dim-group-main', bindings: [{ bindingId: 'b-0', field: 'region' }] },
        ],
        metricGroups: [
          { id: 'metric-group-1', bindings: [{ bindingId: 'b-1', field: 'revenue' }] },
        ],
        filters: [],
        limit: 1000,
      },
      fieldMeta: {
        'b-0': { label: '区域' },
        'b-1': { alias: 'gmv' },
      },
      style: { colors: [], smooth: false, tableRowSize: 'small' },
      queryOptions: {},
    });
    expect(savedDoc).not.toHaveProperty('xAxisField');
    expect(savedDoc).not.toHaveProperty('yAxisFields');
    expect(savedDoc).not.toHaveProperty('queryConfig');
    expect(savedDoc).not.toHaveProperty('dimensionLabels');
  });

  it('历史 bar 图表（旧 color_group 第二个维度组）加载后并入 X 轴：字段不丢、改走 v1', async () => {
    mockGetColumns.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: [
          { name: 'region', expr: 'region', type: 'string', comment: '', role: 'dimension' },
          { name: 'city', expr: 'city', type: 'string', comment: '', role: 'dimension' },
          { name: 'revenue', expr: 'revenue', type: 'number', comment: '', role: 'metric' },
        ],
      })
    );

    // 2026-09-19 之前的 bar 图表可以带第二个维度组（旧 id: color_group）
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Legacy Bar',
          dataset_id: 1,
          chart_type: 'bar',
          config: JSON.stringify({
            version: 2,
            chartType: 'bar',
            title: 'Legacy Bar',
            query: {
              dimensionGroups: [
                { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', field: 'region' }] },
                { id: 'color_group', bindings: [{ bindingId: 'b-1', field: 'city' }] },
              ],
              metricGroups: [
                { id: 'metric-group-1', bindings: [{ bindingId: 'b-2', field: 'revenue' }] },
              ],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    // 等待配置加载完成后触发的那一次查询（chart_type 已从初始 table 切到 bar）
    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('bar');
    });

    // bar 已无槽位协议触发条件 → 恒走 v1 平铺
    const request =
      mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
    expect(request?.spec_version).toBeUndefined();
    expect(request).not.toHaveProperty('dimension_groups');
    // 关键断言：第二个维度组的字段被搬进 X 轴而不是静默剪掉
    expect(request?.dims).toEqual(['region', 'city']);

    // store 侧同口径：只剩一组维度、含两个字段、bindingId 原样保留
    const groups = useStore.getState().queryConfig.dimensionGroups;
    expect(groups).toHaveLength(1);
    expect(groups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'region' },
      { bindingId: 'b-1', field: 'city' },
    ]);
  });

  it('emits the v1 flat request for bar (color_group 槽位已下线)', async () => {
    mockGetColumns.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: [
          { name: 'region', expr: 'region', type: 'string', comment: '', role: 'dimension' },
          { name: 'revenue', expr: 'revenue', type: 'number', comment: '', role: 'metric' },
        ],
      })
    );

    // 只有 x_axis 一个维度组的 bar 图表
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Plain Bar',
          dataset_id: 1,
          chart_type: 'bar',
          config: JSON.stringify({
            version: 2,
            chartType: 'bar',
            title: 'Plain Bar',
            query: {
              dimensionGroups: [
                { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', field: 'region' }] },
              ],
              metricGroups: [
                { id: 'metric-group-1', bindings: [{ bindingId: 'b-1', field: 'revenue' }] },
              ],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('bar');
    });

    const request =
      mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
    expect(request?.spec_version).toBeUndefined();
    expect(request).not.toHaveProperty('dimension_groups');
    expect(request).not.toHaveProperty('metric_groups');
    expect(request).toMatchObject({
      chart_type: 'bar',
      dims: ['region'],
      metrics: [{ field: 'revenue', agg: 'sum', alias: 'revenue' }],
    });
  });

  it('emits a v2 slot-protocol request for combo (R-58)', async () => {
    mockGetColumns.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: [
          { name: 'month', expr: 'month', type: 'string', comment: '', role: 'dimension' },
          { name: 'revenue', expr: 'revenue', type: 'number', comment: '', role: 'metric' },
          { name: 'growth', expr: 'growth', type: 'number', comment: '', role: 'metric' },
        ],
      })
    );

    // combo 图表：x_axis=month，主轴指标=revenue，次轴指标=growth
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Combo Chart',
          dataset_id: 1,
          chart_type: 'combo',
          config: JSON.stringify({
            version: 2,
            chartType: 'combo',
            title: 'Combo Chart',
            query: {
              dimensionGroups: [
                { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', field: 'month' }] },
              ],
              metricGroups: [
                { id: 'metric-group-1', bindings: [{ bindingId: 'b-1', field: 'revenue' }] },
                { id: 'metric-group-2', bindings: [{ bindingId: 'b-2', field: 'growth' }] },
              ],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('combo');
      expect(lastRequest?.spec_version).toBe(2);
    });

    const request =
      mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
    // combo 天然走 v2：不得回落到 v1 平铺 dims/metrics（否则主/次轴槽位区分丢失）
    expect(request).not.toHaveProperty('dims');
    expect(request).not.toHaveProperty('metrics');
    // 只有一个 x_axis 维度组
    expect(request?.dimension_groups).toEqual([
      { name: 'x_axis', label: 'X 轴维度', fields: [{ field: 'month', binding_id: 'b-0' }] },
    ]);
    // 两个指标槽位分别携带真实槽位名，主/次轴区分保留在 wire 上
    expect(request?.metric_groups).toEqual([
      {
        name: 'primary_values',
        label: '主轴指标',
        fields: [{ field: 'revenue', agg: 'sum', alias: 'revenue', binding_id: 'b-1' }],
      },
      {
        name: 'secondary_values',
        label: '次轴指标',
        fields: [{ field: 'growth', agg: 'sum', alias: 'growth', binding_id: 'b-2' }],
      },
    ]);
  });

  it('keeps an aliased combo secondary metric on yAxisIndex 1 (metricSlots alias 优先反查)', async () => {
    mockGetColumns.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: [
          { name: 'month', expr: 'month', type: 'string', comment: '', role: 'dimension' },
          { name: 'revenue', expr: 'revenue', type: 'number', comment: '', role: 'metric' },
          { name: 'growth', expr: 'growth', type: 'number', comment: '', role: 'metric' },
        ],
      })
    );

    // combo：x_axis=month，主轴=revenue（无别名），次轴=growth（用户设了别名「增长率」）。
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Combo Chart',
          dataset_id: 1,
          chart_type: 'combo',
          config: JSON.stringify({
            version: 2,
            chartType: 'combo',
            title: 'Combo Chart',
            query: {
              dimensionGroups: [
                { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', field: 'month' }] },
              ],
              metricGroups: [
                { id: 'metric-group-1', bindings: [{ bindingId: 'b-1', field: 'revenue' }] },
                { id: 'metric-group-2', bindings: [{ bindingId: 'b-2', field: 'growth' }] },
              ],
              filters: [],
              limit: 1000,
            },
            fieldMeta: { 'b-2': { alias: '增长率' } },
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    // 中文别名「增长率」不进 wire（会被后端 safeIdentifier 拒绝 → _invalid_identifier）：
    // wire alias 回退列名 growth，纯展示名只留在前端 labels。
    mockExecuteChartQuery.mockImplementation((request) =>
      Promise.resolve(
        mockAxiosResponse({
          code: 20000,
          msg: 'ok',
          trace: '',
          data:
            request.chart_type === 'combo'
              ? {
                  data: {
                    x_axis: ['2024-01', '2024-02'],
                    series: [
                      { name: 'revenue', data: [1000, 2000] },
                      { name: 'growth', data: [0.1, 0.2] },
                    ],
                  },
                  select_sql: 'select month, sum(revenue), sum(growth) from sales group by month',
                }
              : {
                  data: {
                    columns: ['region'],
                    data: [{ region: 'East' }],
                    pagination: { page: 1, page_size: 10, total: 1, total_pages: 1 },
                  },
                  select_sql: 'select region from sales',
                  count_sql: 'select count(*) from sales',
                },
        })
      )
    );

    renderChartBuilder();

    // 请求侧：wire alias = 列名（中文别名不上 wire），与渲染侧的反查键必须同源。
    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('combo');
    });
    const request =
      mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
    expect(request?.metric_groups?.[1]).toEqual({
      name: 'secondary_values',
      label: '次轴指标',
      fields: [{ field: 'growth', agg: 'sum', alias: 'growth', binding_id: 'b-2' }],
    });

    // 渲染侧：次轴 series 必须仍反查到 secondary_values → yAxisIndex 1（line）。
    // 修复前 metricSlots 只含列名 'growth'，带别名时反查失败会被防御性兜底静默降级到
    // yAxisIndex 0（bar）——百分比指标画到绝对值主轴刻度上，看似合理实则错误。
    await waitFor(() => {
      expect(echartsOptionCapture.current?.series).toHaveLength(2);
    });
    const series = echartsOptionCapture.current?.series ?? [];
    expect(series[0]).toMatchObject({ name: 'revenue', type: 'bar', yAxisIndex: 0 });
    // 渲染层显示名走 labels（前端 alias）：wire series 名 growth → 展示「增长率」。
    expect(series[1]).toMatchObject({ name: '增长率', type: 'line', yAxisIndex: 1 });
  });

  it('histogram：恢复的 binCount 以 wire query_options.bin_count 发出，bins 渲染为 bar（R-57）', async () => {
    mockGetColumns.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: [
          { name: 'region', expr: 'region', type: 'string', comment: '', role: 'dimension' },
          { name: 'revenue', expr: 'revenue', type: 'number', comment: '', role: 'metric' },
        ],
      })
    );

    // 已保存的 histogram 图表：单个 value 指标槽位 + 持久化 camelCase queryOptions.binCount
    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Revenue Distribution',
          dataset_id: 1,
          chart_type: 'histogram',
          config: JSON.stringify({
            version: 2,
            chartType: 'histogram',
            title: 'Revenue Distribution',
            query: {
              dimensionGroups: [],
              metricGroups: [
                { id: 'metric-group-1', bindings: [{ bindingId: 'b-0', field: 'revenue' }] },
              ],
              filters: [],
              limit: 1000,
            },
            fieldMeta: {},
            style: {},
            queryOptions: { binCount: 15 },
          }),
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );

    // histogram 响应是 { bins }（ChartHistogramResponse），其余图型回落表格负载
    mockExecuteChartQuery.mockImplementation((request) =>
      Promise.resolve(
        mockAxiosResponse({
          code: 20000,
          msg: 'ok',
          trace: '',
          data:
            request.chart_type === 'histogram'
              ? {
                  data: {
                    bins: [
                      { bin_start: 0, bin_end: 10, count: 3 },
                      { bin_start: 10, bin_end: 20, count: 5 },
                    ],
                  },
                  select_sql: 'select width_bucket(revenue, 0, 20, 15), count(*) from sales',
                }
              : {
                  data: {
                    columns: ['region'],
                    data: [{ region: 'East' }],
                    pagination: { page: 1, page_size: 10, total: 1, total_pages: 1 },
                  },
                  select_sql: 'select region from sales',
                  count_sql: 'select count(*) from sales',
                },
        })
      )
    );

    renderChartBuilder();

    // 请求侧：恢复的 binCount=15 翻译为 wire snake_case bin_count，走 v1 平铺路径
    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('histogram');
      expect(lastRequest?.query_options).toEqual({ bin_count: 15 });
    });
    const request =
      mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
    expect(request?.spec_version).toBeUndefined();
    expect(request?.dims).toEqual([]);
    expect(request?.metrics).toEqual([{ field: 'revenue', agg: 'sum', alias: 'revenue' }]);

    // 配置侧：histogram 专属的「分箱数量」InputNumber 出现在 ConfigPanel
    await waitFor(() => {
      expect(screen.getByText('分箱数量')).toBeTruthy();
    });

    // 渲染侧：bins 负载经共享 buildChartOption 产出单条 bar series（裁定 C：无专属分支）
    await waitFor(() => {
      expect(echartsOptionCapture.current?.series).toHaveLength(1);
    });
    expect(echartsOptionCapture.current?.series?.[0]).toMatchObject({
      name: 'revenue',
      type: 'bar',
    });
  });

  /**
   * 右侧配置面板的信息层级：标题置顶、设置项逐行。
   * 这三条用例锁死本轮交互优化的两处诉求——「图表标题放到最上面」与
   * 「配置不再挤在一行」——避免后续有人把标题卡片挪回面板底部。
   */
  describe('配置面板信息层级', () => {
    it('图表标题卡片排在可视化类型之前', () => {
      renderNewChartBuilder();

      const titleCardTitle = screen.getByText('图表标题');
      const chartTypeCardTitle = screen.getByText('可视化类型');

      // compareDocumentPosition 返回 4（DOCUMENT_POSITION_FOLLOWING）表示后者在前者之后
      const position = titleCardTitle.compareDocumentPosition(chartTypeCardTitle);
      expect(position & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('不再渲染冗余的「当前配置」卡片', () => {
      renderNewChartBuilder();

      expect(screen.getByText('图表标题')).toBeTruthy();
      expect(screen.queryByText('当前配置')).toBeNull();
    });

    it('样式开关按「标签 + 控件」逐行渲染，而不是挤在同一行', async () => {
      renderNewChartBuilder();

      // 默认表格图型的 styleKeys 为 ['tableRowSize']，必然渲染一行设置
      await waitFor(() => {
        expect(screen.getByText('表格行尺寸')).toBeTruthy();
      });

      const rows = document.querySelectorAll('[data-testid="config-setting-row"]');
      expect(rows.length).toBeGreaterThan(0);
      for (const row of rows) {
        // 每行恰好两个子节点：左侧标签 + 右侧控件容器
        expect(row.children).toHaveLength(2);
      }
    });
  });

  /**
   * 透视表「行列切换」快捷按钮：把行维度组与列维度组整体对调。
   * 两条路径——只有 pivot 渲染该按钮；点击后绑定整体换位且组 id 不变。
   */
  describe('透视表行列切换快捷按钮', () => {
    const seedPivotRowsColumns = async () => {
      mockGetColumns.mockResolvedValueOnce(
        mockAxiosResponse({
          code: 20000,
          msg: 'ok',
          trace: '',
          data: [
            { name: 'region', expr: 'region', type: 'string', comment: '', role: 'dimension' },
            { name: 'month', expr: 'month', type: 'integer', comment: '', role: 'dimension' },
            { name: 'revenue', expr: 'revenue', type: 'number', comment: '', role: 'metric' },
          ],
        })
      );

      renderChartBuilder();
      await waitFor(() => {
        expect(useStore.getState().chartBuilderFields).toHaveLength(3);
      });

      fireEvent.click(screen.getByRole('button', { name: /透视表/ }));
      act(() => {
        const state = useStore.getState();
        const fields = state.chartBuilderFields;
        state.addDimensionField(fields[1], 1); // month → 列维度组
      });
    };

    it('非透视表不渲染该按钮', () => {
      renderChartBuilder();

      expect(screen.queryByTestId('pivot-swap-rows-columns')).toBeNull();
    });

    it('点击后行/列维度的绑定整体对调，组 id 不变', async () => {
      await seedPivotRowsColumns();

      const before = useStore.getState().queryConfig.dimensionGroups;
      expect(before[0].bindings).toEqual([{ bindingId: 'b-0', field: 'region' }]);
      expect(before[1].bindings).toEqual([{ bindingId: 'b-1', field: 'month' }]);

      act(() => {
        fireEvent.click(screen.getByTestId('pivot-swap-rows-columns'));
      });

      const after = useStore.getState().queryConfig.dimensionGroups;
      expect(after[0].bindings).toEqual(before[1].bindings);
      expect(after[1].bindings).toEqual(before[0].bindings);
      expect(after.map((group) => group.id)).toEqual(before.map((group) => group.id));
    });

    it('透视表切成普通表格：列维度字段搬进行维度，不再被静默丢弃', async () => {
      await seedPivotRowsColumns();

      // 图型按钮的可访问名含图标 aria-label（如 "table 表格"），故用非锚定匹配
      fireEvent.click(screen.getByRole('button', { name: /表格/ }));

      const groups = useStore.getState().queryConfig.dimensionGroups;
      expect(groups).toHaveLength(1);
      expect(groups[0].bindings.map((binding) => binding.field)).toEqual(['region', 'month']);

      // 请求侧同样带上两个维度，证明字段真的进了表格的维度槽位而不是被 slice 掉
      await waitFor(() => {
        const lastRequest =
          mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
        expect(lastRequest?.chart_type).toBe('table');
        expect(lastRequest?.dims).toEqual(['region', 'month']);
      });
    });
  });
});

/**
 * 地址栏即分享（R-30′）：查询成功 → 落库 → 地址栏换成 `?q=<短码>`；
 * 带 `?q=` 进入 → 还原记录里的完整图表配置。这两半必须成对成立，
 * 只有前半是单向死链，只有后半是永远读不到的记录。
 */
describe('ChartBuilder 地址栏即分享', () => {
  const SHORT_ID = 'CSatF9qXyRoQXFtAA3iZS';

  const sharedDocument = {
    version: 2,
    chartType: 'bar',
    title: '分享来的图表',
    query: {
      dimensionGroups: [
        { id: 'dim-group-main', bindings: [{ bindingId: 'b-0', field: 'region' }] },
      ],
      metricGroups: [
        { id: 'metric-group-main', bindings: [{ bindingId: 'b-1', field: 'revenue' }] },
      ],
      filters: [],
      limit: 1000,
    },
    fieldMeta: {},
  };

  // spec 走宽入参：既要造合法信封，也要造 v:99 这种服务端脏数据（类型上不合法，
  // 但线上确实可能出现，还原路径必须扛得住）——故在此收一次断言。
  const queryRecord = (
    overrides: Partial<Omit<QueryRecord, 'spec'>> & { spec?: unknown } = {}
  ): QueryRecord => {
    const base: QueryRecord = {
      query_id: SHORT_ID,
      spec: { v: 1, document: sharedDocument },
      dataset_id: 1,
      chart_id: null,
      created_at: '2026-09-19T12:00:00Z',
      expires_at: null,
      expired: false,
      hit_count: 1,
    };
    return { ...base, ...overrides } as QueryRecord;
  };

  const resolveQueryRecord = (overrides: Parameters<typeof queryRecord>[0] = {}) => {
    mockGetQueryRecord.mockResolvedValue(
      mockAxiosResponse<QueryRecord>({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: queryRecord(overrides),
      })
    );
  };

  /** 把游离的异步收尾跑完再断言，避免 rejected promise 落到下一个用例里。 */
  const settleAsync = async () => {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  };

  it('查询成功后把地址栏换成 ?q=短码，并带上这次查询的度量', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(screen.getByTestId('location-search').textContent).toBe(`?q=${SHORT_ID}`);
    });

    expect(mockSaveQueryRecord).toHaveBeenCalled();
    const payload = mockSaveQueryRecord.mock.calls[0][0];
    expect(payload.dataset_id).toBe(1);
    expect(payload.source_type).toBe('build');
    expect(payload.spec.v).toBe(1);
    expect(payload.spec.document).toMatchObject({ version: 2, chartType: 'table' });
    // 行数取自本次查询响应（桩数据 1 行），不是前端硬编码
    expect(payload.row_count).toBe(1);
    expect(typeof payload.duration_ms).toBe('number');

    // 自签发的短码绝不能回头触发还原：否则「还原→查询→落库→还原」死死循环
    await settleAsync();
    expect(mockGetQueryRecord).not.toHaveBeenCalled();
  });

  it('同一份 spec 只落库一次（并发自动查询不重复 POST）', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(screen.getByTestId('location-search').textContent).toBe(`?q=${SHORT_ID}`);
    });
    await settleAsync();

    expect(mockSaveQueryRecord).toHaveBeenCalledTimes(1);
  });

  it('查询失败不落库，地址栏保持原样', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    // 只让首次查询失败：mockRejectedValue 会让整个用例的查询全挂，
    // 残留的 rejected promise 还会在下一个用例里冒出来刷 console。
    mockExecuteChartQuery.mockRejectedValueOnce(new Error('boom'));

    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalled();
    });
    await settleAsync();

    expect(mockSaveQueryRecord).not.toHaveBeenCalled();
    expect(screen.getByTestId('location-search').textContent).toBe('?edit=1&datasetId=1');
    errorSpy.mockRestore();
  });

  it('?q= 直链还原图表配置，并按还原后的配置发起查询', async () => {
    resolveQueryRecord();
    renderQueryRecordBuilder(SHORT_ID);

    expect(mockGetQueryRecord).toHaveBeenCalledWith(SHORT_ID);

    await waitFor(() => {
      expect(useStore.getState().chartBuilderConfig.chartType).toBe('bar');
    });
    expect(useStore.getState().chartBuilderConfig.title).toBe('分享来的图表');
    // 记录里存的维度必须真的回到槽位（而不是只还原了图型外壳）
    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings[0].field).toBe('region');

    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('bar');
      expect(lastRequest?.dims).toEqual(['region']);
    });
  });

  it('?q= 优先于 edit / datasetId，不去请求图表详情', async () => {
    resolveQueryRecord();
    renderQueryRecordBuilder(SHORT_ID, '&edit=9&datasetId=7');

    await waitFor(() => {
      expect(useStore.getState().chartBuilderConfig.chartType).toBe('bar');
    });
    expect(mockGetChartById).not.toHaveBeenCalled();
    // 数据集以记录为准，地址栏里的 datasetId=7 必须被忽略
    expect(useStore.getState().queryConfig.limit).toBe(1000);
  });

  it('已过期的记录照常完整还原，只多一条提示', async () => {
    resolveQueryRecord({ expired: true, expires_at: '2026-09-01T00:00:00Z' });
    renderQueryRecordBuilder(SHORT_ID);

    await waitFor(() => {
      expect(screen.getByText('此查询已过期，条件可能已不适用')).toBeInTheDocument();
    });
    // 过期只影响提示，不影响可用性（R-42 / U-10）
    expect(useStore.getState().chartBuilderConfig.chartType).toBe('bar');
  });

  it('记录版本无法识别时回退为空白查询并提示', async () => {
    resolveQueryRecord({ spec: { v: 99, document: sharedDocument } });
    renderQueryRecordBuilder(SHORT_ID);

    await waitFor(() => {
      expect(
        screen.getByText('这条查询记录的配置版本无法识别，已回退为空白查询')
      ).toBeInTheDocument();
    });
    expect(useStore.getState().chartBuilderConfig.chartType).toBe('table');
  });

  it('短码解析失败时提示分享链接无效', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    mockGetQueryRecord.mockRejectedValue(new Error('分享链接无效或已失效'));

    renderQueryRecordBuilder(SHORT_ID);

    await waitFor(() => {
      expect(screen.getByText('分享链接无效或已失效')).toBeInTheDocument();
    });
    errorSpy.mockRestore();
  });

  /**
   * 回归：dev 下 `main.tsx` 用 StrictMode 包裹，effect 会 mount → cleanup → 再 mount。
   * 早期实现把「已认领短码」的记账放在请求**之前**，于是第二次挂载直接早退、第一次的
   * 响应又被 cleanup 的 cancelled 丢弃 —— 后果是后端真的收到了 GET（hit_count 上涨），
   * 但界面一行都没还原。这正是分享链接「打开没反应」的原因。
   */
  it('StrictMode 双挂载下直链还原依然生效，且只请求一次', async () => {
    resolveQueryRecord();
    renderQueryRecordBuilder(SHORT_ID, '', true);

    await waitFor(() => {
      expect(useStore.getState().chartBuilderConfig.chartType).toBe('bar');
    });
    expect(useStore.getState().chartBuilderConfig.title).toBe('分享来的图表');
    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings[0].field).toBe('region');
    // 还原只是中间态，用户要的是「图出来」：槽位就位后必须按还原后的配置真发一次查询
    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('bar');
      expect(lastRequest?.dims).toEqual(['region']);
    });
    // 重复请求会污染 hit_count 这个真实指标，双挂载不该让它翻倍
    expect(mockGetQueryRecord).toHaveBeenCalledTimes(1);
  });
});
