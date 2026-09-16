import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { AxiosResponse } from 'axios';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Chart, Share } from '../../api';
import type { ApiResponse } from '../../lib/api/client';
import ShareView from '../../pages/ShareView';

// Mock the API module: ShareView reads envelope-shaped responses, i.e.
// `{ data: { data: payload } }` just like the real axios instance returns.
vi.mock('../../api', () => ({
  chartsApi: {
    getById: vi.fn(),
    getChartData: vi.fn(),
  },
  sharesApi: {
    getByToken: vi.fn(),
    verifyPassword: vi.fn(),
  },
}));

// Keep the chart renderers out of the test surface except for the option
// payload; the assertions below target the auth gate (password prompt vs.
// chart header) and the shape-aware consumption of the aggregated data
// envelope. Capturing `option` lets the axis test prove the payload is fed
// to ECharts verbatim.
const renderedOptions: unknown[] = [];
vi.mock('echarts-for-react', () => ({
  default: ({ option }: { option?: unknown }) => {
    renderedOptions.push(option);
    return <div data-testid="echarts" />;
  },
}));
vi.mock('../../components/ChartBuilder/TableChart', () => ({
  default: () => <div data-testid="table-chart" />,
}));

import { chartsApi, sharesApi } from '../../api';

const mockGetShareByToken = vi.mocked(sharesApi.getByToken);
const mockVerifyPassword = vi.mocked(sharesApi.verifyPassword);
const mockGetChartById = vi.mocked(chartsApi.getById);
const mockGetChartData = vi.mocked(chartsApi.getChartData);

// Same helper as ChartBuilder.test.tsx: builds a real AxiosResponse shape so
// mockResolvedValue needs no type suppression (config is a class AxiosResponse
// requires in typing but no page code reads — pinned via never, not any).
function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

// Full wire shapes per the generated schemas (expires_at/created_at are
// required keys; has_password replaces the never-exposed password).
const protectedShare: Share = {
  id: 1,
  token: 'tok',
  chart_id: 7,
  expires_at: null,
  created_at: '',
  has_password: true,
};
const openShare: Share = {
  id: 2,
  token: 'tok',
  chart_id: 7,
  expires_at: null,
  created_at: '',
  has_password: false,
};
const mockChart: Chart = {
  id: 7,
  name: 'Monthly Sales',
  dataset_id: 3,
  chart_type: 'bar',
  config: JSON.stringify({
    version: 1,
    chartType: 'bar',
    title: 'Monthly Sales',
    query: {
      dimensionGroups: [{ fields: ['month'] }],
      metricGroups: [{ fields: ['total'] }],
    },
    fieldMeta: {},
  }),
  created_at: '',
  updated_at: '',
};

function renderShareView() {
  return render(
    <MemoryRouter initialEntries={['/share/tok']}>
      <Routes>
        <Route path="/share/:token" element={<ShareView />} />
      </Routes>
    </MemoryRouter>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  renderedOptions.length = 0;
});

describe('ShareView live password gate (has_password envelope)', () => {
  it('shows the password prompt when a protected share resolves via envelope', async () => {
    mockGetShareByToken.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: protectedShare })
    );

    renderShareView();

    await waitFor(() => expect(screen.getByText('Password Required')).toBeInTheDocument());
    // The prompt must come from the successful envelope's has_password only;
    // charts are never fetched before verification succeeds.
    expect(mockGetChartById).not.toHaveBeenCalled();
  });

  it('does not fake a password prompt when the share fetch fails (401/403 branch is unreachable)', async () => {
    // Business errors reject with a bare Error (no `.response`) because the
    // backend wraps every failure in a 200 envelope; the interceptor turns
    // non-20000 codes into `new Error(msg)`.
    mockGetShareByToken.mockRejectedValue(new Error('share not found'));

    renderShareView();

    // Post-refactor behavior: a failed fetch must NOT render "Password
    // Required" (the old phantom 401 branch could only do that on statuses
    // that never occur on these routes).
    await waitFor(() => expect(screen.queryByText('Password Required')).not.toBeInTheDocument());
    expect(mockGetShareByToken).toHaveBeenCalledTimes(1);
  });

  it('shows the backend message when password verification rejects with a bare Error', async () => {
    mockGetShareByToken.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: protectedShare })
    );
    mockVerifyPassword.mockRejectedValue(new Error('invalid password'));

    renderShareView();

    await waitFor(() => expect(screen.getByText('Password Required')).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText('Enter password'), {
      target: { value: 'nope' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'View Chart' }));

    // The message surfaced to the user is the rejected Error's message — the
    // backend envelope `msg` forwarded by the axios interceptor.
    await waitFor(() => expect(screen.getByText('invalid password')).toBeInTheDocument());
    expect(screen.queryByText('Invalid password')).not.toBeInTheDocument();
    expect(mockGetChartById).not.toHaveBeenCalled();
  });

  it('renders the chart title for an unprotected share', async () => {
    mockGetShareByToken.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: openShare })
    );
    mockGetChartById.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: mockChart })
    );
    // v1 bar 图表：后端经聚合管道返回与 builder 预览同形的 AxisResponse。
    mockGetChartData.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'success',
        trace: '',
        data: { x_axis: ['2026-01', '2026-02'], series: [{ name: 'total', data: [10, 20] }] },
      })
    );

    renderShareView();

    await waitFor(() => expect(screen.getByText('Monthly Sales')).toBeInTheDocument());
    expect(screen.queryByText('Password Required')).not.toBeInTheDocument();
  });

  it('feeds an axis payload (builder preview shape) to ECharts without row indexing', async () => {
    mockGetShareByToken.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: openShare })
    );
    mockGetChartById.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: mockChart })
    );
    mockGetChartData.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'success',
        trace: '',
        data: { x_axis: ['华北', '华东'], series: [{ name: 'Revenue', data: [36635, 37730] }] },
      })
    );

    renderShareView();

    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());
    expect(screen.queryByText('Unable to display chart')).not.toBeInTheDocument();

    // The processed AxisResponse must reach ECharts verbatim: x categories
    // from x_axis, one series per payload series (alias kept as the series
    // name), values not re-derived from raw rows.
    const option = renderedOptions[renderedOptions.length - 1] as {
      xAxis: { data: string[] };
      series: { name: string; type: string; data: unknown[] }[];
    };
    expect(option.xAxis.data).toEqual(['华北', '华东']);
    expect(option.series).toEqual([{ name: 'Revenue', type: 'bar', data: [36635, 37730] }]);
  });

  it('renders a KPI card (AntD Statistic, not ECharts) for kpi shares', async () => {
    mockGetShareByToken.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: openShare })
    );
    const kpiChart: Chart = {
      id: 7,
      name: 'Revenue KPI',
      dataset_id: 3,
      chart_type: 'kpi',
      config: JSON.stringify({
        version: 1,
        chartType: 'kpi',
        title: 'Revenue KPI',
        query: {
          dimensionGroups: [],
          metricGroups: [{ fields: ['total'] }],
        },
        // v1 fieldMeta 键为列名，迁移后复制到唯一 metric binding 的 bindingId 上
        fieldMeta: { total: { unit: '元' } },
      }),
      created_at: '',
      updated_at: '',
    };
    mockGetChartById.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: kpiChart })
    );
    // kpi 的结构化响应是标量 {value, label}（后端 KpiProcessor 产物）
    mockGetChartData.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'success',
        trace: '',
        data: { value: 42, label: 'total' },
      })
    );

    renderShareView();

    await waitFor(() => expect(screen.getByText('42')).toBeInTheDocument());
    // KpiCard（真实组件，未 mock）：Statistic 标题为 label，unit 来自持久化 fieldMeta
    expect(screen.getByText('total')).toBeInTheDocument();
    expect(screen.getByText('元')).toBeInTheDocument();
    // 不落入 ECharts 臂，也不落入 "Unable to display chart" 兜底
    expect(screen.queryByTestId('echarts')).not.toBeInTheDocument();
    expect(screen.queryByText('Unable to display chart')).not.toBeInTheDocument();
  });
});
