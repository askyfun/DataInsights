import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { AxiosResponse } from 'axios';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chartsApi, datasetsApi } from '../../api';
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

vi.mock('../../components/ChartBuilder/FilterBuilder', () => ({
  default: () => <div data-testid="filter-builder" />,
}));

vi.mock('../../components/ChartBuilder/QueryConfigRow', () => ({
  default: ({
    label,
    groupIndex,
    onAddField,
    availableFields,
    rowType,
  }: {
    label: string;
    groupIndex?: number;
    onAddField?: (field: { id: string; name: string; type: 'dimension' | 'metric' }) => void;
    availableFields?: Array<{ id: string; name: string; type: 'dimension' | 'metric' }>;
    rowType: 'dimension' | 'metric' | 'filter';
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
  };
});

const mockGetDatasets = vi.mocked(datasetsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);
const mockGetChartById = vi.mocked(chartsApi.getById);
const mockExecuteChartQuery = vi.mocked(chartsApi.executeChartQuery);
const mockUpdateChart = vi.mocked(chartsApi.update);

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
      <Routes>
        <Route path="/chart-builder" element={<ChartBuilder />} />
      </Routes>
    </MemoryRouter>
  );
};

const renderNewChartBuilder = () => {
  return render(
    <MemoryRouter initialEntries={['/chart-builder?datasetId=1']}>
      <Routes>
        <Route path="/chart-builder" element={<ChartBuilder />} />
      </Routes>
    </MemoryRouter>
  );
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

    // revenue → 指标组 b-1，别名 gmv；sort 引用 bindingId（新 QueryConfig.sort 形状）
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
    // v1 平铺请求：sort.field 是该绑定的输出列名（指标 = 别名 || 列名），
    // 与改动前直接发送 TableChart sorter.field 的 wire 字节等价
    expect(request.sort).toEqual({ field: 'gmv', order: 'desc' });
    expect(request.metrics).toEqual([{ field: 'revenue', agg: 'sum', alias: 'gmv' }]);
  });

  it('sends sort.field as the raw bindingId on v2 slot-protocol requests (裁定B 选择1)', async () => {
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

    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Stacked Bar',
          dataset_id: 1,
          chart_type: 'bar',
          config: JSON.stringify({
            version: 2,
            chartType: 'bar',
            title: 'Stacked Bar',
            query: {
              dimensionGroups: [
                { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', field: 'region' }] },
                { id: 'dim-group-2', bindings: [{ bindingId: 'b-1', field: 'city' }] },
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

    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.chart_type).toBe('bar');
      expect(lastRequest?.spec_version).toBe(2);
    });

    // color_group 非空 → v2 槽位协议；sort 引用指标 binding b-2
    act(() => {
      useStore.getState().setQueryConfig({ sort: { bindingId: 'b-2', order: 'desc' } });
    });

    await waitFor(() => {
      const lastRequest =
        mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
      expect(lastRequest?.sort).toEqual({ field: 'b-2', order: 'desc' });
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

    // 点击排序会依次触发：handlePageChange 重建（antd onChange 的分页副作用，闭包里
    // 尚无新 sort）→ handleSortChange 直发 → autoQuery effect 重跑。最后两次
    // （直发 + effect）都必须携带 sort——R-50 修复前 effect 那次从不带 sort，
    // 会用未排序结果覆盖已排序数据。
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

  it('emits a v2 slot-protocol request when color_group is non-empty (裁定B)', async () => {
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

    mockGetChartById.mockResolvedValueOnce(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Stacked Bar',
          dataset_id: 1,
          chart_type: 'bar',
          config: JSON.stringify({
            version: 2,
            chartType: 'bar',
            title: 'Stacked Bar',
            query: {
              dimensionGroups: [
                { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', field: 'region' }] },
                { id: 'dim-group-2', bindings: [{ bindingId: 'b-1', field: 'city' }] },
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
      expect(lastRequest?.spec_version).toBe(2);
    });

    const request =
      mockExecuteChartQuery.mock.calls[mockExecuteChartQuery.mock.calls.length - 1]?.[0];
    expect(request).not.toHaveProperty('dims');
    expect(request).not.toHaveProperty('metrics');
    expect(request?.dimension_groups).toEqual([
      { name: 'x_axis', label: 'X 轴维度', fields: [{ field: 'region', binding_id: 'b-0' }] },
      { name: 'color_group', label: '颜色分组', fields: [{ field: 'city', binding_id: 'b-1' }] },
    ]);
    expect(request?.metric_groups).toEqual([
      {
        name: 'values',
        label: '数值',
        fields: [{ field: 'revenue', agg: 'sum', alias: 'revenue', binding_id: 'b-2' }],
      },
    ]);
  });

  it('keeps emitting the v1 flat request when color_group is empty (裁定B, 向后兼容)', async () => {
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

    // 模拟本任务之前保存的 bar 图表：只有 x_axis 一个维度组，没有 color_group 组
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

  it('emits a v2 slot-protocol request for combo even when color_group is empty (R-58)', async () => {
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

    // combo 图表：x_axis=month，主轴指标=revenue，次轴指标=growth，未填 color_group
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
    // color_group 为空 → 只有一个 x_axis 维度组
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

    // combo：x_axis=month，主轴=revenue（无别名），次轴=growth（用户设了别名「增长率」），
    // color_group 为空——combo 最常见的配置形态。
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

    // 后端按 ResolveAlias() 命名 series：growth 带别名 → series 名是「增长率」而非「growth」。
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
                      { name: '增长率', data: [0.1, 0.2] },
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

    // 请求侧：别名随 v2 请求发出（后端据此命名 series），与渲染侧的反查键必须同源。
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
      fields: [{ field: 'growth', agg: 'sum', alias: '增长率', binding_id: 'b-2' }],
    });

    // 渲染侧：alias 命名的次轴 series 必须仍反查到 secondary_values → yAxisIndex 1（line）。
    // 修复前 metricSlots 只含列名 'growth'，「增长率」反查失败会被防御性兜底静默降级到
    // yAxisIndex 0（bar）——百分比指标画到绝对值主轴刻度上，看似合理实则错误。
    await waitFor(() => {
      expect(echartsOptionCapture.current?.series).toHaveLength(2);
    });
    const series = echartsOptionCapture.current?.series ?? [];
    expect(series[0]).toMatchObject({ name: 'revenue', type: 'bar', yAxisIndex: 0 });
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
});
