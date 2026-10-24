// 仪表盘移动端阅览模式（issue #158）：窄屏单列堆叠、编辑入口收敛、筛选器仍可取值；
// resize 回桌面宽度后必须原样恢复栅格（分支切换的双向边界）。
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { App } from 'antd';
import type { AxiosResponse } from 'axios';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Chart, Dashboard } from '../../api';
import { messages } from '../../i18n/useLocale';
import type { ApiResponse } from '../../lib/api/client';
import DashboardEditor from '../../pages/DashboardEditor';

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
    getColumns: vi
      .fn()
      .mockResolvedValue({ data: { code: 20000, msg: 'success', trace: '', data: [] } }),
    queryDistinct: vi
      .fn()
      .mockResolvedValue({ data: { code: 20000, msg: 'success', trace: '', data: [] } }),
  },
}));

vi.mock('react-grid-layout', async () => {
  const React = await import('react');
  return {
    GridLayout: (props: Record<string, unknown>) =>
      React.createElement('div', { 'data-testid': 'grid' }, props.children as React.ReactNode),
    useContainerWidth: () => ({
      width: 1200,
      mounted: true,
      containerRef: React.useRef(null),
      measureWidth: () => {},
    }),
    verticalCompactor: { kind: 'vertical' },
  };
});

vi.mock('echarts-for-react', () => ({
  default: () => <div data-testid="echarts" />,
}));

import { chartsApi, dashboardsApi } from '../../api';

const mockGetById = vi.mocked(dashboardsApi.getById);
const mockQuery = vi.mocked(dashboardsApi.query);
const mockChartsGetAll = vi.mocked(chartsApi.getAll);

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

const envelope = <T,>(data: T) =>
  mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data });

const listEnvelope = <T,>(items: T[]) => envelope({ items, total: items.length });

const mkChart = (id: number, name: string): Chart => ({
  id,
  name,
  dataset_id: 3,
  chart_type: 'bar',
  config: JSON.stringify({
    version: 1,
    chartType: 'bar',
    title: name,
    query: { dimensionGroups: [{ fields: ['month'] }], metricGroups: [{ fields: ['total'] }] },
    fieldMeta: {},
  }),
  created_at: '',
  updated_at: '',
});

const layoutDoc = JSON.stringify({
  version: 1,
  grid: { cols: 12 },
  widgets: [
    { widgetId: 'w-a', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
    // (6,4) 排在 (0,8) 前面：堆叠序按 (y, x)，不是 widget 顺序。
    { widgetId: 'w-b', type: 'chart', chartId: 8, x: 6, y: 4, w: 6, h: 8 },
    { widgetId: 'w-c', type: 'chart', chartId: 9, x: 0, y: 8, w: 6, h: 8 },
    {
      widgetId: 'w-f',
      type: 'filter',
      binding: { datasetId: 3, column: 'city' },
      label: '城市',
      dataType: 'string',
      operator: 'in',
      multi: true,
      x: 0,
      y: 16,
      w: 3,
      h: 3,
    },
  ],
});

const dashboard: Dashboard = {
  id: 'd-1',
  name: '销售总览',
  description: null,
  layout_json: layoutDoc,
  status: 'draft',
  folder_id: null,
  created_at: '',
  updated_at: '',
};

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

const axisPayload = { x_axis: ['2026-01'], series: [{ name: 'total', data: [10] }] };

const primeHappyPath = () => {
  mockGetById.mockResolvedValue(envelope(dashboard));
  mockChartsGetAll.mockResolvedValue(
    listEnvelope([mkChart(7, '图A'), mkChart(8, '图B'), mkChart(9, '图C')])
  );
  mockQuery.mockResolvedValue(
    envelope({
      results: [
        { widgetId: 'w-a', chartId: 7, status: 'ok', data: { data: axisPayload } },
        { widgetId: 'w-b', chartId: 8, status: 'ok', data: { data: axisPayload } },
        { widgetId: 'w-c', chartId: 9, status: 'ok', data: { data: axisPayload } },
      ],
    })
  );
};

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

afterEach(() => {
  window.innerWidth = 1024;
  window.dispatchEvent(new Event('resize'));
});

describe('仪表盘移动端阅览模式', () => {
  it('窄屏：单列堆叠按 (y, x) 排序，编辑入口全部收敛，筛选器仍可取值', async () => {
    window.innerWidth = 375;
    primeHappyPath();

    renderEditor();

    const stack = await screen.findByTestId('dashboard-mobile-stack');
    // 堆叠序 = 视觉阅读序（y 优先，x 次之），与 widget 数组顺序无关。
    const titles = Array.from(stack.querySelectorAll('.ant-card-head-title')).map(
      (el) => el.textContent
    );
    expect(titles).toEqual(['图A', '图B', '图C', '城市']);

    // 栅格与全部编辑入口不渲染。
    expect(screen.queryByTestId('grid')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /保存/ })).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue('销售总览')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-add-filter')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-add-container')).not.toBeInTheDocument();
    expect(screen.queryByTestId('block-menu-w-a')).not.toBeInTheDocument();
    // 页面标签条只读。
    expect(screen.queryByTestId('dashboard-page-add')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-page-actions')).not.toBeInTheDocument();
    // 筛选器收敛后仍保留取值控件（无 scope / 移除）。
    expect(screen.queryByTestId('dashboard-filter-remove')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-filter-scope')).not.toBeInTheDocument();

    // 三块图都出了（/query 结果逐块渲染，异步到位）。
    await waitFor(() => expect(screen.getAllByTestId('echarts')).toHaveLength(3));
  });

  it('resize 回桌面宽度：恢复栅格与编辑入口（双向边界）', async () => {
    window.innerWidth = 375;
    primeHappyPath();

    renderEditor();
    await screen.findByTestId('dashboard-mobile-stack');

    window.innerWidth = 1024;
    fireEvent(window, new Event('resize'));

    await waitFor(() => expect(screen.getByTestId('grid')).toBeInTheDocument());
    await waitFor(() => expect(screen.getByRole('button', { name: /保存/ })).toBeInTheDocument());
    expect(screen.queryByTestId('dashboard-mobile-stack')).not.toBeInTheDocument();
  });
});
