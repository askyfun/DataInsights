// 仪表盘多页面（issue #142）：页面标签条、按页取数、页面的增删改复制。
//
// 断言对象是「激活页 → 渲染哪些块 + 下发哪个 page_id」这条映射，以及页面操作的
// 文档副作用；拖拽排序的**纯逻辑**（reorderPages）在
// __tests__/lib/dashboardLayoutSchema.test.ts 里单测，真实拖拽手势留在浏览器 E2E 验。
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { App } from 'antd';
import type { AxiosResponse } from 'axios';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
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
    // 未落库的块（复制出来的那份）走这条本地取数补数据，必须是个 promise。
    getChartData: vi.fn().mockResolvedValue({
      data: { code: 20000, msg: 'success', trace: '', data: { x_axis: [], series: [] } },
    }),
  },
  datasetsApi: {
    getAll: vi
      .fn()
      .mockResolvedValue({ data: { code: 20000, msg: 'success', trace: '', data: [] } }),
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
    useContainerWidth: () => {
      const containerRef = React.useRef<HTMLDivElement | null>(null);
      return { width: 1200, mounted: true, containerRef, measureWidth: () => {} };
    },
    verticalCompactor: { kind: 'vertical' },
  };
});

vi.mock('echarts-for-react', () => ({
  default: () => <div data-testid="echarts" />,
}));

import { chartsApi, dashboardsApi } from '../../api';

const mockGetById = vi.mocked(dashboardsApi.getById);
const mockQuery = vi.mocked(dashboardsApi.query);
const mockUpdate = vi.mocked(dashboardsApi.update);
const mockChartsGetAll = vi.mocked(chartsApi.getAll);

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

const envelope = <T,>(data: T) =>
  mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data });

const chart = (id: number, name: string): Chart => ({
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

/** 两页盘：p-1 一块图，p-2 一块图 + 一条**作用于所有页**的筛选器。 */
const twoPageLayout = JSON.stringify({
  version: 2,
  grid: { cols: 12 },
  pages: [
    { id: 'p-1', name: '总览' },
    { id: 'p-2', name: '明细' },
  ],
  widgets: [
    { widgetId: 'w-p1', pageId: 'p-1', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
    { widgetId: 'w-p2', pageId: 'p-2', type: 'chart', chartId: 99, x: 0, y: 0, w: 6, h: 8 },
    {
      widgetId: 'f-global',
      pageId: 'p-2',
      type: 'filter',
      binding: { datasetId: 3, column: 'region' },
      label: '区域',
      dataType: 'string',
      operator: 'in',
      multi: true,
      scope: 'all',
      defaultValue: ['华东'],
      x: 0,
      y: 8,
      w: 3,
      h: 3,
    },
  ],
});

const dashboard: Dashboard = {
  id: 'd-1',
  name: '销售总览',
  description: null,
  layout_json: twoPageLayout,
  status: 'draft',
  folder_id: null,
  created_at: '',
  updated_at: '',
};

const axisPayload = { x_axis: ['2026-01'], series: [{ name: 'total', data: [10] }] };

const renderEditor = () =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <App component={false}>
        <MemoryRouter initialEntries={['/dashboards/d-1']}>
          <Routes>
            <Route path="/dashboards/:id" element={<DashboardEditor />} />
            <Route path="/" element={<div data-testid="list-route" />} />
          </Routes>
        </MemoryRouter>
      </App>
    </IntlProvider>
  );

/** 按 page_id 回不同页的结果：这样「取错页」会立刻表现为渲染错块。 */
function primeByPage() {
  mockGetById.mockResolvedValue(envelope(dashboard));
  mockChartsGetAll.mockResolvedValue(envelope([chart(7, '首屏图'), chart(99, '明细图')]));
  mockQuery.mockImplementation((async (_id: string, body: { page_id?: string }) =>
    envelope({
      results:
        body.page_id === 'p-2'
          ? [{ widgetId: 'w-p2', chartId: 99, status: 'ok', data: { data: axisPayload } }]
          : [{ widgetId: 'w-p1', chartId: 7, status: 'ok', data: { data: axisPayload } }],
    })) as never);
}

const tabs = () => screen.getAllByTestId('dashboard-page-tab');
const tabLabels = () => tabs().map((tab) => tab.getAttribute('data-page-id'));
const openActions = async (pageId: string, item: string) => {
  const tab = tabs().find((node) => node.getAttribute('data-page-id') === pageId);
  if (!tab) {
    throw new Error(`page tab ${pageId} not found`);
  }
  fireEvent.click(within(tab).getByTestId('dashboard-page-actions'));
  const option = await screen.findByText(item, { selector: '.ant-dropdown-menu-title-content' });
  fireEvent.click(option);
};

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe('仪表盘多页面：按页取数与页面切换', () => {
  it('首屏只取首页，并把别页里 scope=all 的筛选器一并下发', async () => {
    primeByPage();

    renderEditor();

    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());
    // 活跃页是首页；全局筛选器虽然挂在 p-2 上，但作用于所有页 → 首页取数要带上它。
    expect(mockQuery).toHaveBeenCalledWith('d-1', {
      page_id: 'p-1',
      filters: [{ widgetId: 'f-global', value: ['华东'] }],
    });
    // 只渲染首页的块。
    expect(screen.getByText('首屏图')).toBeInTheDocument();
    expect(screen.queryByText('明细图')).not.toBeInTheDocument();
    expect(tabLabels()).toEqual(['p-1', 'p-2']);
  });

  it('切页：按新页重取，且只渲染该页的块', async () => {
    primeByPage();

    renderEditor();
    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());

    const detailTab = tabs().find((node) => node.getAttribute('data-page-id') === 'p-2');
    fireEvent.click(within(detailTab as HTMLElement).getByTestId('dashboard-page-label'));

    await waitFor(() =>
      expect(mockQuery).toHaveBeenCalledWith('d-1', {
        page_id: 'p-2',
        filters: [{ widgetId: 'f-global', value: ['华东'] }],
      })
    );
    await waitFor(() => expect(screen.getByText('明细图')).toBeInTheDocument());
    expect(screen.queryByText('首屏图')).not.toBeInTheDocument();
  });

  it('作用范围改成「本页」后，首页取数不再带上别页的筛选器', async () => {
    primeByPage();

    renderEditor();
    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());

    // 切到 p-2（筛选器所在页）才能看到它的块。
    const detailTab = tabs().find((node) => node.getAttribute('data-page-id') === 'p-2');
    fireEvent.click(within(detailTab as HTMLElement).getByTestId('dashboard-page-label'));
    await waitFor(() => expect(screen.getByTestId('dashboard-filter-scope')).toBeInTheDocument());

    fireEvent.mouseDown(screen.getByTestId('dashboard-filter-scope'));
    fireEvent.click(
      await screen.findByText('本页', { selector: '.ant-select-item-option-content' })
    );

    // 回到首页：该筛选器已不作用于本页，载荷里不应再出现它。
    const firstTab = tabs().find((node) => node.getAttribute('data-page-id') === 'p-1');
    fireEvent.click(within(firstTab as HTMLElement).getByTestId('dashboard-page-label'));

    await waitFor(() =>
      expect(mockQuery).toHaveBeenLastCalledWith('d-1', { page_id: 'p-1', filters: [] })
    );
  });
});

describe('仪表盘多页面：页面增删改复制', () => {
  it('新增页面：追加一页并激活；重命名就地生效', async () => {
    primeByPage();

    renderEditor();
    await waitFor(() => expect(tabs()).toHaveLength(2));

    fireEvent.click(screen.getByTestId('dashboard-page-add'));

    await waitFor(() => expect(tabs()).toHaveLength(3));
    const added = tabs()[2];
    expect(added.getAttribute('data-active')).toBe('true');
    expect(within(added).getByTestId('dashboard-page-label')).toHaveTextContent('页面 3');

    fireEvent.doubleClick(within(added).getByTestId('dashboard-page-label'));
    const input = await screen.findByTestId('dashboard-page-rename-input');
    fireEvent.change(input, { target: { value: '  季度回顾  ' } });
    fireEvent.blur(input);

    await waitFor(() =>
      expect(within(tabs()[2]).getByTestId('dashboard-page-label')).toHaveTextContent('季度回顾')
    );
  });

  it('复制页面：页与块一并复制，块的 widgetId 必须重新生成（唯一域是整盘）', async () => {
    primeByPage();
    mockUpdate.mockResolvedValue(envelope(dashboard));

    renderEditor();
    await waitFor(() => expect(tabs()).toHaveLength(2));

    await openActions('p-1', '复制页面');

    await waitFor(() => expect(tabs()).toHaveLength(3));
    expect(within(tabs()[1]).getByTestId('dashboard-page-label')).toHaveTextContent('总览 副本');

    fireEvent.click(screen.getByRole('button', { name: /保存/ }));
    await waitFor(() => expect(mockUpdate).toHaveBeenCalled());

    const saved = JSON.parse(
      (mockUpdate.mock.calls[0][1] as { layout_json: string }).layout_json
    ) as {
      pages: { id: string; name: string }[];
      widgets: { widgetId: string; pageId: string; chartId?: number }[];
    };
    const copy = saved.pages[1];
    const cloned = saved.widgets.filter((widget) => widget.pageId === copy.id);
    expect(copy.name).toBe('总览 副本');
    expect(cloned).toHaveLength(1);
    expect(cloned[0].chartId).toBe(7);
    // 原块的 widgetId 不能被沿用：同一盘里两个 w-p1 会让渲染与取数互相覆盖。
    expect(cloned[0].widgetId).not.toBe('w-p1');
    expect(new Set(saved.widgets.map((widget) => widget.widgetId)).size).toBe(saved.widgets.length);
  });

  it('删除页面：连块一起删，且至少保留一页（删除项在最后一页上禁用）', async () => {
    primeByPage();

    renderEditor();
    await waitFor(() => expect(tabs()).toHaveLength(2));

    await openActions('p-2', '删除页面');

    await waitFor(() => expect(tabLabels()).toEqual(['p-1']));
    // 删掉的是别页：激活页不变，因此不需要重取（仍停在 p-1 的结果上）。
    expect(mockQuery).toHaveBeenCalledTimes(1);

    // 只剩一页：提示出现，且删除项被禁用。
    expect(screen.getByText('至少保留一个页面')).toBeInTheDocument();
    const onlyTab = tabs()[0];
    fireEvent.click(within(onlyTab).getByTestId('dashboard-page-actions'));
    const remove = await screen.findByText('删除页面', {
      selector: '.ant-dropdown-menu-title-content',
    });
    expect(remove.closest('.ant-dropdown-menu-item')).toHaveClass(
      'ant-dropdown-menu-item-disabled'
    );
  });

  it('删掉当前激活页时落到相邻页并按它重取', async () => {
    primeByPage();

    renderEditor();
    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());

    await openActions('p-1', '删除页面');

    await waitFor(() =>
      expect(mockQuery).toHaveBeenLastCalledWith('d-1', {
        page_id: 'p-2',
        filters: [{ widgetId: 'f-global', value: ['华东'] }],
      })
    );
    expect(tabLabels()).toEqual(['p-2']);
  });
});
