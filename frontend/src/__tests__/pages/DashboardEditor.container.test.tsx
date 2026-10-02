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
import { useStore } from '../../store';

/**
 * 查询容器（issue #154）：栅格块里的筛选器成员渲染 + 移出容器 + 置顶分流 + 新建容器。
 *
 * 只测渲染分流与 containerId 的会话内变更——取数口径不变（成员仍按 pageId/scope 下发），
 * 那条契约在 lib/dashboardLayoutSchema.test.ts 与后端 query_test.go 各自守。
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
  chartsApi: { getAll: vi.fn(), getById: vi.fn(), getChartData: vi.fn() },
  datasetsApi: { getAll: vi.fn(), getColumns: vi.fn(), queryDistinct: vi.fn() },
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
const mockQuery = vi.mocked(dashboardsApi.query);
const mockChartsGetAll = vi.mocked(chartsApi.getAll);
const mockGetChartData = vi.mocked(chartsApi.getChartData);
const mockDatasetsGetAll = vi.mocked(datasetsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);
const mockQueryDistinct = vi.mocked(datasetsApi.queryDistinct);

function envelope<T>(data: T): AxiosResponse<ApiResponse<T>> {
  return { data: { code: 20000, msg: 'success', trace: '', data }, status: 200 } as never;
}

const barChart: Chart = {
  id: 7,
  name: 'Monthly Sales',
  dataset_id: 3,
  chart_type: 'bar',
  config: JSON.stringify({ version: 1, chartType: 'bar', query: {}, fieldMeta: {} }),
  created_at: '',
  updated_at: '',
};

const COLUMNS = [
  { id: '0000i52a', name: 'region', type: 'string', role: 'dimension', expr: 'region' },
];

const filterWidget = (containerId?: string) => ({
  widgetId: 'w-f',
  pageId: 'p-1',
  type: 'filter',
  binding: { datasetId: 3, column: '0000i52a' },
  label: '区域',
  dataType: 'string',
  operator: 'in',
  multi: true,
  x: 6,
  y: 0,
  w: 3,
  h: 3,
  ...(containerId ? { containerId } : {}),
});

const containerWidget = (extra: Record<string, unknown> = {}) => ({
  widgetId: 'c1',
  pageId: 'p-1',
  type: 'container',
  label: '顶部控件',
  x: 0,
  y: 8,
  w: 12,
  h: 2,
  ...extra,
});

const layoutWith = (widgets: unknown[]) =>
  JSON.stringify({
    version: 2,
    grid: { cols: 12 },
    pages: [{ id: 'p-1', name: '页面 1' }],
    widgets,
  });

const dashboard = (layout: string): Dashboard => ({
  id: 'd-1',
  name: '销售总览',
  description: null,
  layout_json: layout,
  status: 'draft',
  folder_id: null,
  created_at: '',
  updated_at: '',
});

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

function prime(layout: string) {
  mockGetById.mockResolvedValue(envelope(dashboard(layout)));
  mockChartsGetAll.mockResolvedValue(envelope([barChart]));
  mockGetChartData.mockResolvedValue(envelope({ x_axis: [], series: [] } as never));
  mockQuery.mockResolvedValue(envelope({ results: [] }));
  mockDatasetsGetAll.mockResolvedValue(envelope([{ id: 3, name: '天气数据' }] as never));
  mockGetColumns.mockResolvedValue(envelope(COLUMNS as never));
  mockQueryDistinct.mockResolvedValue(envelope([{ region: '华东' }, { region: '华南' }]));
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  useStore.setState({ datasets: [] });
});

describe('查询容器渲染分流', () => {
  it('被归拢的筛选器在容器内渲染（带移出按钮），不作为顶层栅格块', async () => {
    prime(
      layoutWith([
        { widgetId: 'w-a', pageId: 'p-1', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
        containerWidget(),
        filterWidget('c1'),
      ])
    );

    renderEditor();

    // 容器作为顶层栅格块出现，成员筛选器的「移出容器」按钮在容器体内。
    await waitFor(() => expect(screen.getByTestId('container-c1')).toBeTruthy());
    expect(screen.getByTestId('container-ungroup-w-f')).toBeTruthy();
  });

  it('点「移出容器」后筛选器退回独立块（移出按钮消失）', async () => {
    prime(
      layoutWith([
        { widgetId: 'w-a', pageId: 'p-1', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
        containerWidget(),
        filterWidget('c1'),
      ])
    );

    renderEditor();
    await waitFor(() => expect(screen.getByTestId('container-ungroup-w-f')).toBeTruthy());

    fireEvent.click(screen.getByTestId('container-ungroup-w-f'));

    await waitFor(() => expect(screen.queryByTestId('container-ungroup-w-f')).toBeNull());
  });

  it('置顶容器渲染在盘顶控件条，不占顶层栅格', async () => {
    prime(
      layoutWith([
        { widgetId: 'w-a', pageId: 'p-1', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
        containerWidget({ pinned: true }),
        filterWidget('c1'),
      ])
    );

    renderEditor();

    await waitFor(() => expect(screen.getByTestId('container-strip-c1')).toBeTruthy());
    // 置顶后不再是栅格块。
    expect(screen.queryByTestId('container-c1')).toBeNull();
  });

  it('「添加容器」新建一块容器（带可编辑标题入口）', async () => {
    prime(
      layoutWith([
        { widgetId: 'w-a', pageId: 'p-1', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
        filterWidget(),
      ])
    );

    renderEditor();
    await waitFor(() => expect(screen.getByTestId('dashboard-add-container')).toBeTruthy());
    // 初始没有容器标题输入框。
    expect(screen.queryByLabelText('容器标题')).toBeNull();

    fireEvent.click(screen.getByTestId('dashboard-add-container'));

    // 新容器出现（id 随机，用其标题输入框这一确定标记断言）。
    await waitFor(() => expect(screen.getByLabelText('容器标题')).toBeTruthy());
  });
});
