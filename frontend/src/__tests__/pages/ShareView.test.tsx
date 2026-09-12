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

// Keep the chart renderers out of the test surface; the assertions below
// target the auth gate (password prompt vs. chart header), not ECharts.
vi.mock('echarts-for-react', () => ({
  default: () => <div data-testid="echarts" />,
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
    mockGetChartData.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'success',
        trace: '',
        data: [{ month: '2026-01', total: 10 }],
      })
    );

    renderShareView();

    await waitFor(() => expect(screen.getByText('Monthly Sales')).toBeInTheDocument());
    expect(screen.queryByText('Password Required')).not.toBeInTheDocument();
  });
});
