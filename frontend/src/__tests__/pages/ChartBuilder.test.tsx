import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { AxiosResponse } from 'axios';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chartsApi, datasetsApi } from '../../api';
import type { ApiResponse } from '../../lib/api/client';
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
    },
  };
});

const mockGetDatasets = vi.mocked(datasetsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);
const mockGetChartById = vi.mocked(chartsApi.getById);
const mockExecuteChartQuery = vi.mocked(chartsApi.executeChartQuery);

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
      dimensionGroups: [{ id: 'dim-group-main', fields: ['field-0'] }],
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
            columns: '[]',
            shard_enabled: false,
            shard_keys: '[]',
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
            chartType: 'table',
            title: 'Sales Table',
            queryConfig: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['field-0'] }],
              metricGroups: [],
              filters: [],
              limit: 1000,
            },
          }),
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
      expect(useStore.getState().queryConfig.metricGroups[0]?.fields).toEqual(['field-1']);
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
            chartType: 'table',
            title: 'Revenue Table',
            queryConfig: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['field-0'] }],
              metricGroups: [{ id: 'metric-group-main', fields: ['field-1'] }],
              filters: [],
              limit: 1000,
            },
            metricAliases: {
              'field-1': 'gmv',
            },
            metricAggregations: {
              'field-1': 'avg',
            },
          }),
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(useStore.getState().metricAliases['field-1']).toBe('gmv');
    });

    expect(useStore.getState().metricAggregations['field-1']).toBe('avg');
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
            chartType: 'pie',
            title: 'Styled Pie',
            queryConfig: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['field-0'] }],
              metricGroups: [{ id: 'metric-group-main', fields: ['field-1'] }],
              filters: [],
              limit: 1000,
            },
            dimensionLabels: {
              'field-0': '区域',
            },
            metricUnits: {
              'field-1': '元',
            },
            metricFormats: {
              'field-1': '0,0.00',
            },
            chartStyle: {
              colors: ['#ff4d4f'],
              smooth: true,
              tableRowSize: 'middle',
            },
            chartQueryOptions: {
              pieMergeOtherBelowRatio: 5,
            },
          }),
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(useStore.getState().dimensionLabels['field-0']).toBe('区域');
    });

    expect(useStore.getState().metricUnits['field-1']).toBe('元');
    expect(useStore.getState().metricFormats['field-1']).toBe('0,0.00');
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
            chartType: 'pivot',
            title: 'Pivot Chart',
            queryConfig: {
              dimensionGroups: [
                { id: 'dim-group-rows', fields: ['field-0'] },
                { id: 'dim-group-columns', fields: ['field-0'] },
              ],
              metricGroups: [],
              filters: [],
              limit: 1000,
            },
          }),
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
            chartType: 'pie',
            title: 'Pie Chart',
            queryConfig: {
              dimensionGroups: [{ id: 'dim-group-main', fields: ['field-0'] }],
              metricGroups: [
                { id: 'metric-group-main', fields: ['field-1'] },
                { id: 'metric-group-extra', fields: ['field-1'] },
              ],
              filters: [],
              limit: 1000,
            },
          }),
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

  it('sends filters with the operator key to match the backend entity.Filter contract', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(mockExecuteChartQuery).toHaveBeenCalledTimes(1);
    });

    act(() => {
      useStore.getState().addFilter({
        id: 'filter-gt-1',
        field: 'field-0',
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

  it('loads sparse saved metric groups without crashing and normalizes them for scatter charts', async () => {
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
            chartType: 'scatter',
            title: 'Scatter Chart',
            queryConfig: {
              dimensionGroups: [],
              metricGroups: [null, { id: 'metric-group-secondary', fields: ['field-1'] }],
              filters: [],
              limit: 1000,
            },
          }),
        },
      })
    );

    renderChartBuilder();

    await waitFor(() => {
      expect(useStore.getState().chartBuilderConfig.chartType).toBe('scatter');
    });

    expect(useStore.getState().queryConfig.metricGroups).toEqual([
      { id: 'metric-group-1', fields: [] },
      { id: 'metric-group-secondary', fields: ['field-1'] },
    ]);
    expect(errorSpy).not.toHaveBeenCalled();

    errorSpy.mockRestore();
  });
});
