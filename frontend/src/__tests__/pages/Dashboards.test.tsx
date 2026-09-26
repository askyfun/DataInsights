// 仪表盘列表页：契约是「后端 layout_json 是不可信输入，计数必须经迁移函数」，
// 以及删除/新建两条动作真的打到 API 并且刷新；外加归档树（文件夹）的选中过滤、
// 新建夹、删除被拒三条。
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { App } from 'antd';
import type { AxiosResponse } from 'axios';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Dashboard, DashboardFolder } from '../../api';
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
  dashboardFoldersApi: {
    getAll: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
  },
  chartsApi: {
    getAll: vi.fn(),
    getById: vi.fn(),
    getChartData: vi.fn(),
  },
}));

import { dashboardFoldersApi, dashboardsApi } from '../../api';

const mockGetAll = vi.mocked(dashboardsApi.getAll);
const mockCreate = vi.mocked(dashboardsApi.create);
const mockRemove = vi.mocked(dashboardsApi.remove);
const mockFolderGetAll = vi.mocked(dashboardFoldersApi.getAll);
const mockFolderCreate = vi.mocked(dashboardFoldersApi.create);
const mockFolderRemove = vi.mocked(dashboardFoldersApi.remove);

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
    folder_id: 'f-1',
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
    folder_id: null,
    created_at: '2026-09-25T00:00:00Z',
    updated_at: '2026-09-25T00:00:00Z',
  },
];

const folders: DashboardFolder[] = [
  {
    id: 'f-1',
    name: '经营分析',
    parent_id: null,
    created_at: '2026-09-25T00:00:00Z',
    updated_at: '2026-09-25T00:00:00Z',
  },
];

const renderPage = () =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      {/* 页面用 App.useApp() 取 message（避开 antd 6 的静态 message 告警），
          没有这层包裹 useApp() 拿到的是空对象、调用即抛错。 */}
      <App component={false}>
        <MemoryRouter initialEntries={['/']}>
          <Routes>
            <Route path="/" element={<DashboardsPage />} />
            <Route path="/dashboards/:id" element={<div data-testid="editor-route" />} />
          </Routes>
        </MemoryRouter>
      </App>
    </IntlProvider>
  );

beforeEach(() => {
  vi.clearAllMocks();
  // 列表页现在一次取两件事（盘 + 夹），默认给空夹列表；单条用例按需覆盖。
  mockFolderGetAll.mockResolvedValue(envelope([]));
});

describe('仪表盘列表页', () => {
  it('列出名称与图表块数，且损坏的 layout_json 收敛为 0 块而不抛异常', async () => {
    mockGetAll.mockResolvedValue(envelope(dashboards));

    const { container } = renderPage();
    // 归档树也会渲染仪表盘叶子，所以按名称查必须限定在表格里。
    const table = () => within(container.querySelector('.ant-table') as HTMLElement);

    await waitFor(() => expect(table().getByText('销售总览')).toBeInTheDocument());
    expect(table().getByText('坏布局')).toBeInTheDocument();
    expect(table().getByText('月度经营看板')).toBeInTheDocument();

    const counts = Array.from(container.querySelectorAll('.ant-tag')).map((el) => el.textContent);
    expect(counts).toEqual(['2', '0']);
  });

  it('删除经确认后调用 remove 并重新拉取列表', async () => {
    mockGetAll.mockResolvedValue(envelope(dashboards));
    mockRemove.mockResolvedValue(envelope({ status: 'deleted' }));

    const { container } = renderPage();
    const table = () => within(container.querySelector('.ant-table') as HTMLElement);

    await waitFor(() => expect(table().getByText('销售总览')).toBeInTheDocument());

    fireEvent.click(table().getAllByRole('button', { name: /删除/ })[0]);
    // Popconfirm 的确认按钮文案来自 common.yes。
    fireEvent.click(await screen.findByRole('button', { name: /^是$/ }));

    await waitFor(() => expect(mockRemove).toHaveBeenCalledWith('d-1'));
    await waitFor(() => expect(mockGetAll).toHaveBeenCalledTimes(2));
  }, 20_000);

  it('新建成功后直接进入该盘的画布页', async () => {
    mockGetAll.mockResolvedValue(envelope([]));
    mockCreate.mockResolvedValue(
      envelope({
        id: 'd-new',
        name: '未命名仪表盘',
        description: null,
        layout_json: '',
        status: 'draft',
        folder_id: null,
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

  it('选中文件夹只显示该夹的盘；回根后新建不带 folder_id', async () => {
    mockGetAll.mockResolvedValue(envelope(dashboards));
    mockFolderGetAll.mockResolvedValue(envelope(folders));
    mockCreate.mockResolvedValue(
      envelope({
        id: 'd-new',
        name: '未命名仪表盘',
        description: null,
        layout_json: '',
        status: 'draft',
        folder_id: null,
        created_at: '',
        updated_at: '',
      })
    );

    // 树上也渲染仪表盘叶子，所以按名称查会命中两处；列表状态一律按**表格行**判定。
    const rowCount = () => document.querySelectorAll('.ant-table-tbody tr.ant-table-row').length;

    renderPage();
    await waitFor(() => expect(rowCount()).toBe(2));

    // 选中树上唯一的夹 → 列表只剩该夹的直属盘。
    fireEvent.click(screen.getByText('经营分析'));
    await waitFor(() => expect(rowCount()).toBe(1));

    // 回根（点「全部仪表盘」）→ 未归档的盘重新出现。
    fireEvent.click(screen.getByText('全部仪表盘'));
    await waitFor(() => expect(rowCount()).toBe(2));

    fireEvent.click(screen.getByRole('button', { name: /新建仪表盘/ }));
    await waitFor(() => expect(mockCreate).toHaveBeenCalledTimes(1));
    // 根视图新建不该带 folder_id（undefined 序列化后不落字段）。
    expect(mockCreate.mock.calls[0][0].folder_id).toBeUndefined();
  });

  it('选中文件夹后新建自动归档进该夹', async () => {
    mockGetAll.mockResolvedValue(envelope(dashboards));
    mockFolderGetAll.mockResolvedValue(envelope(folders));
    mockCreate.mockResolvedValue(
      envelope({
        id: 'd-new',
        name: '未命名仪表盘',
        description: null,
        layout_json: '',
        status: 'draft',
        folder_id: 'f-1',
        created_at: '',
        updated_at: '',
      })
    );

    renderPage();
    await waitFor(() => expect(screen.getByText('经营分析')).toBeInTheDocument());

    fireEvent.click(screen.getByText('经营分析'));
    fireEvent.click(screen.getByRole('button', { name: /新建仪表盘/ }));

    await waitFor(() => expect(mockCreate).toHaveBeenCalledTimes(1));
    expect(mockCreate.mock.calls[0][0].folder_id).toBe('f-1');
  });

  it('右键文件夹可新建子文件夹，请求带 parent_id', async () => {
    mockGetAll.mockResolvedValue(envelope(dashboards));
    mockFolderGetAll.mockResolvedValue(envelope(folders));
    mockFolderCreate.mockResolvedValue(
      envelope({
        id: 'f-2',
        name: '周报',
        parent_id: 'f-1',
        created_at: '',
        updated_at: '',
      })
    );

    renderPage();
    await waitFor(() => expect(screen.getByText('经营分析')).toBeInTheDocument());

    fireEvent.contextMenu(screen.getByText('经营分析'));
    fireEvent.click(await screen.findByText('新建子文件夹'));

    const input = await screen.findByRole('textbox');
    fireEvent.change(input, { target: { value: '周报' } });
    fireEvent.click(screen.getByRole('button', { name: /确.*认|Confirm/ }));

    await waitFor(() =>
      expect(mockFolderCreate).toHaveBeenCalledWith({ name: '周报', parent_id: 'f-1' })
    );
  });

  it('删除非空文件夹：后端 20400 的文案原样念给用户', async () => {
    mockGetAll.mockResolvedValue(envelope(dashboards));
    mockFolderGetAll.mockResolvedValue(envelope(folders));
    // 后端消息里带着「还剩几项」，前端不做转译，直接进 message.error。
    mockFolderRemove.mockRejectedValue(new Error('文件夹下还有 1 个仪表盘，请先移动或删除它们'));

    renderPage();
    await waitFor(() => expect(screen.getByText('经营分析')).toBeInTheDocument());

    fireEvent.contextMenu(screen.getByText('经营分析'));
    fireEvent.click(await screen.findByText('删除文件夹'));
    fireEvent.click(await screen.findByRole('button', { name: /^是$/ }));

    await waitFor(() => expect(mockFolderRemove).toHaveBeenCalledWith('f-1'));
    await waitFor(() =>
      expect(screen.getByText('文件夹下还有 1 个仪表盘，请先移动或删除它们')).toBeInTheDocument()
    );
    // 树仍在（失败不该让页面失去归档视图）。
    expect(screen.getByText('经营分析')).toBeInTheDocument();
  });
});
