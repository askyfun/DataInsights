// 仪表盘列表页：契约是「后端 layout_json 是不可信输入，计数必须经迁移函数」，
// 以及删除/新建两条动作真的打到 API 并且刷新。
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { AxiosResponse } from 'axios';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Dashboard } from '../../api';
import { messages } from '../../i18n/useLocale';
import type { ApiResponse } from '../../lib/api/client';
import DashboardsPage from '../../pages/Dashboards';

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
}));

import { dashboardsApi } from '../../api';

const mockGetAll = vi.mocked(dashboardsApi.getAll);
const mockCreate = vi.mocked(dashboardsApi.create);
const mockRemove = vi.mocked(dashboardsApi.remove);

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

const envelope = <T,>(data: T) =>
  mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data });

/** 构造 layout_json：widgets 里 chart 块的数量即列表页要显示的「图表块」。 */
const layoutWithCharts = (chartIds: number[]): string =>
  JSON.stringify({
    version: 1,
    grid: { cols: 12 },
    widgets: chartIds.map((chartId, index) => ({
      widgetId: `w-${index}`,
      type: 'chart',
      chartId,
      x: 0,
      y: index * 8,
      w: 6,
      h: 8,
    })),
  });

const dashboards: Dashboard[] = [
  {
    id: 'd-1',
    name: '销售总览',
    description: '月度经营看板',
    layout_json: layoutWithCharts([7, 9]),
    status: 'draft',
    created_at: '2026-09-25T00:00:00Z',
    updated_at: '2026-09-25T00:00:00Z',
  },
  {
    id: 'd-2',
    name: '坏布局',
    description: null,
    // 损坏 JSON：迁移函数必须兜底为空盘，列表页不能因此崩掉。
    layout_json: '{not json',
    status: 'draft',
    created_at: '2026-09-25T00:00:00Z',
    updated_at: '2026-09-25T00:00:00Z',
  },
];

const renderPage = () =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route path="/" element={<DashboardsPage />} />
          <Route path="/dashboards/:id" element={<div data-testid="editor-route" />} />
        </Routes>
      </MemoryRouter>
    </IntlProvider>
  );

beforeEach(() => {
  vi.clearAllMocks();
});

describe('仪表盘列表页', () => {
  it('列出名称与图表块数，且损坏的 layout_json 收敛为 0 块而不抛异常', async () => {
    mockGetAll.mockResolvedValue(envelope(dashboards));

    renderPage();

    await waitFor(() => expect(screen.getByText('销售总览')).toBeInTheDocument());
    expect(screen.getByText('坏布局')).toBeInTheDocument();
    expect(screen.getByText('月度经营看板')).toBeInTheDocument();

    const counts = Array.from(document.querySelectorAll('.ant-tag')).map((el) => el.textContent);
    expect(counts).toEqual(['2', '0']);
  });

  it('删除经确认后调用 remove 并重新拉取列表', async () => {
    mockGetAll.mockResolvedValue(envelope(dashboards));
    mockRemove.mockResolvedValue(envelope({ status: 'deleted' }));

    renderPage();

    await waitFor(() => expect(screen.getByText('销售总览')).toBeInTheDocument());

    fireEvent.click(screen.getAllByRole('button', { name: /删除/ })[0]);
    // Popconfirm 的确认按钮文案来自 common.yes。
    fireEvent.click(await screen.findByRole('button', { name: /^是$/ }));

    await waitFor(() => expect(mockRemove).toHaveBeenCalledWith('d-1'));
    await waitFor(() => expect(mockGetAll).toHaveBeenCalledTimes(2));
  });

  it('新建成功后直接进入该盘的画布页', async () => {
    mockGetAll.mockResolvedValue(envelope([]));
    mockCreate.mockResolvedValue(
      envelope({
        id: 'd-new',
        name: '未命名仪表盘',
        description: null,
        layout_json: '',
        status: 'draft',
        created_at: '',
        updated_at: '',
      })
    );

    renderPage();

    await waitFor(() => expect(screen.getByText('新建仪表盘')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /新建仪表盘/ }));

    await waitFor(() => expect(mockCreate).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByTestId('editor-route')).toBeInTheDocument());
  });
});
