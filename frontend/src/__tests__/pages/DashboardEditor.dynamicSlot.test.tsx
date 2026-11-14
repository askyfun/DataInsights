import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { App } from 'antd';
import type { AxiosResponse } from 'axios';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Chart, Dashboard } from '../../api';
import { messages } from '../../i18n/useLocale';
import type { ApiResponse } from '../../lib/api/client';
import DashboardEditor from '../../pages/DashboardEditor';

/**
 * 仪表盘内动态维度/指标切换器（issue #134）：
 * 切换只改会话选择 → 立刻带 selections 重取该盘 → 不写回 layout / 图表配置。
 * 取数合并/候选校验的权威在后端（service/chart 的 applyFieldSelections），
 * 这里只守住前端「下发什么」这一半契约。
 */

vi.mock('../../api', () => ({
  dashboardsApi: {
    getAll: vi.fn(),
    getById: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    query: vi.fn(),
  },
  chartsApi: {
    getAll: vi.fn(),
    getById: vi.fn(),
    getChartData: vi.fn(),
  },
  datasetsApi: {
    getAll: vi.fn(),
    getColumns: vi.fn(),
    queryDistinct: vi.fn(),
  },
}));

vi.mock('react-grid-layout', async () => {
  const React = await import('react');
  return {
    GridLayout: (props: Record<string, unknown>) =>
      React.createElement('div', { 'data-testid': 'grid' }, props.children as React.ReactNode),
    useContainerWidth: () => {
      const containerRef = React.useRef<HTMLDivElement | null>(null);
      return { width: 1200, mounted: true, containerRef, measureWidth: () => {} };
    },
    verticalCompactor: { kind: 'vertical' },
  };
});

vi.mock('echarts-for-react', () => ({ default: () => <div data-testid="echarts" /> }));

import { chartsApi, dashboardsApi, datasetsApi } from '../../api';

const mockGetById = vi.mocked(dashboardsApi.getById);
const mockChartsGetAll = vi.mocked(chartsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);
const mockQuery = vi.mocked(dashboardsApi.query);

function envelope<T>(data: T): AxiosResponse<ApiResponse<T>> {
  return { data: { code: 20000, msg: 'success', trace: '', data }, status: 200 } as never;
}

const listEnvelope = <T,>(items: T[]) => envelope({ items, total: items.length });

const dynamicChart: Chart = {
  id: 7,
  name: '动态销售',
  dataset_id: 3,
  chart_type: 'bar',
  config: JSON.stringify({
    version: 2,
    chartType: 'bar',
    title: '动态销售',
    query: {
      dimensionGroups: [
        {
          id: 'dim-group-1',
          bindings: [{ bindingId: 'b-0', fieldId: 'col-region', candidates: ['col-city'] }],
        },
      ],
      metricGroups: [
        { id: 'metric-group-1', bindings: [{ bindingId: 'b-1', fieldId: 'col-amt' }] },
      ],
      filters: [],
    },
    fieldMeta: {},
    style: {},
    queryOptions: {},
  }),
  created_at: '',
  updated_at: '',
};

const plainChart: Chart = {
  ...dynamicChart,
  id: 8,
  name: '静态销售',
  config: JSON.stringify({
    version: 2,
    chartType: 'bar',
    title: '静态销售',
    query: {
      dimensionGroups: [
        { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', fieldId: 'col-region' }] },
      ],
      metricGroups: [
        { id: 'metric-group-1', bindings: [{ bindingId: 'b-1', fieldId: 'col-amt' }] },
      ],
      filters: [],
    },
    fieldMeta: {},
    style: {},
    queryOptions: {},
  }),
};

const dashboard: Dashboard = {
  id: 'd-1',
  name: '销售总览',
  description: null,
  layout_json: JSON.stringify({
    version: 1,
    grid: { cols: 12 },
    widgets: [
      { widgetId: 'w-dyn', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
      { widgetId: 'w-plain', type: 'chart', chartId: 8, x: 6, y: 0, w: 6, h: 8 },
    ],
  }),
  status: 'draft',
  folder_id: null,
  created_at: '',
  updated_at: '',
};

const axisPayload = { x_axis: ['2026-01'], series: [{ name: 'total', data: [10] }] };

function prime() {
  mockGetById.mockResolvedValue(envelope(dashboard));
  mockChartsGetAll.mockResolvedValue(listEnvelope([dynamicChart, plainChart]));
  mockGetColumns.mockResolvedValue(
    envelope([
      { id: 'col-region', name: 'region' },
      { id: 'col-city', name: 'city' },
      { id: 'col-amt', name: 'amount' },
    ]) as never
  );
  mockQuery.mockResolvedValue(
    envelope({
      results: [
        { widgetId: 'w-dyn', chartId: 7, status: 'ok', data: { data: axisPayload } },
        { widgetId: 'w-plain', chartId: 8, status: 'ok', data: { data: axisPayload } },
      ],
    })
  );
}

const renderEditor = () =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <App component={false}>
        <MemoryRouter initialEntries={['/dashboards/d-1']}>
          <Routes>
            <Route path="/dashboards/:id" element={<DashboardEditor />} />
          </Routes>
        </MemoryRouter>
      </App>
    </IntlProvider>
  );

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe('仪表盘动态维度/指标（issue #134）', () => {
  it('有候选的块出切换器，没有候选的块不出', async () => {
    prime();
    renderEditor();

    const switcher = await screen.findByTestId('dashboard-dynamic-w-dyn-switcher');
    expect(switcher).toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-dynamic-w-plain-switcher')).not.toBeInTheDocument();
    // 默认选中作者配置的列
    expect(switcher.textContent).toContain('region');
  });

  it('切换候选 → 立即带 selections 重取，且只下发切换过的槽位', async () => {
    prime();
    renderEditor();
    await waitFor(() => expect(mockQuery).toHaveBeenCalledTimes(1));

    const select = await screen.findByTestId('dashboard-dynamic-w-dyn-b-0');
    fireEvent.mouseDown(select);
    fireEvent.click(
      await screen.findByText('city', { selector: '.ant-select-item-option-content' })
    );

    await waitFor(() => expect(mockQuery).toHaveBeenCalledTimes(2));
    const payload = mockQuery.mock.calls[1][1];
    expect(payload.selections).toEqual([
      { widgetId: 'w-dyn', bindingId: 'b-0', fieldId: 'col-city' },
    ]);
    // 选择是会话态：布局/图表配置不因切换而变（无未保存改动提示）
    expect(screen.queryByText('有未保存的改动')).not.toBeInTheDocument();
  });
});
