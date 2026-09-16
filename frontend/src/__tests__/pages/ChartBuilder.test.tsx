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

vi.mock('echarts-for-react', () => ({
  default: () => <div data-testid="echarts" />,
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
    // 钉死其余请求形状不变。
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
});
