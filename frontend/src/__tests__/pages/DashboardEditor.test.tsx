// 仪表盘画布页。
//
// react-grid-layout 被整模块替换：本文件的断言对象是「盘级取数结果 → 块的渲染状态」
// 这条映射，以及保存/草稿两条副作用路径；栅格几何与拖拽数学归 RGL 自己，且 jsdom 里
// 量不到容器宽度（真 RGL 会一直停在 width=0 的加载态）。RGL v2 的 props 契约由
// tsc 对 node_modules 里的 .d.ts 校验，不靠这里补。
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
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
    getChartData: vi.fn(),
  },
  datasetsApi: {
    getColumns: vi
      .fn()
      .mockResolvedValue({ data: { code: 20000, msg: 'success', trace: '', data: [] } }),
  },
}));

// 替身除了「渲染成一个 div」，还把收到的 props 存下来：
// 这样测试可以直接触发 onDragStop / onResizeStop，覆盖 handleLayoutCommit 的
// 边界收敛与邻块下推（真实 RGL 的几何在 jsdom 里无法产生）。
const gridHarness = vi.hoisted(() => ({
  props: null as Record<string, unknown> | null,
  /** 真 RefObject：用于断言「装载态容器节点是否已存在」（见 D-2 回归用例）。 */
  containerRef: null as { current: HTMLDivElement | null } | null,
}));

vi.mock('react-grid-layout', async () => {
  const React = await import('react');
  return {
    GridLayout: (props: Record<string, unknown>) => {
      gridHarness.props = props;
      return React.createElement(
        'div',
        { 'data-testid': 'grid' },
        props.children as React.ReactNode
      );
    },
    // 给真 ref（而不是 `{ current: null }` 死对象）：编辑器把容器 ref 挂在栅格外层 div 上，
    // 那个节点若在首屏 commit 里不存在，真 RGL 的 useContainerWidth 会永久停在
    // initialWidth —— 这正是 D-2（栅格宽度恒为 1280）的成因，需要一个能观测的 ref。
    useContainerWidth: () => {
      const containerRef = React.useRef<HTMLDivElement | null>(null);
      gridHarness.containerRef = containerRef;
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
const mockGetChartData = vi.mocked(chartsApi.getChartData);

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

const envelope = <T,>(data: T) =>
  mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data });

const barChart: Chart = {
  id: 7,
  name: 'Monthly Sales',
  dataset_id: 3,
  chart_type: 'bar',
  config: JSON.stringify({
    version: 1,
    chartType: 'bar',
    title: 'Monthly Sales',
    query: { dimensionGroups: [{ fields: ['month'] }], metricGroups: [{ fields: ['total'] }] },
    fieldMeta: {},
  }),
  created_at: '',
  updated_at: '',
};

const layoutDoc = JSON.stringify({
  version: 1,
  grid: { cols: 12 },
  widgets: [
    { widgetId: 'w-a', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
    { widgetId: 'w-b', type: 'chart', chartId: 99, x: 6, y: 0, w: 6, h: 8 },
  ],
});

const dashboard: Dashboard = {
  id: 'd-1',
  name: '销售总览',
  description: null,
  layout_json: layoutDoc,
  status: 'draft',
  created_at: '',
  updated_at: '',
};

const axisPayload = { x_axis: ['2026-01'], series: [{ name: 'total', data: [10] }] };

const renderEditor = () =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      {/* 页面用 App.useApp() 取 message（避开 antd 6 的静态 message 告警），
          没有这层包裹 useApp() 拿到的是空对象、调用即抛错。 */}
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

/** 默认装载：一块能出图、一块被软删。 */
function primeHappyPath() {
  mockGetById.mockResolvedValue(envelope(dashboard));
  mockChartsGetAll.mockResolvedValue(envelope([barChart]));
  mockQuery.mockResolvedValue(
    envelope({
      results: [
        { widgetId: 'w-a', chartId: 7, status: 'ok', data: { data: axisPayload } },
        { widgetId: 'w-b', chartId: 99, status: 'chart_deleted' },
      ],
    })
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe('仪表盘画布页', () => {
  it('按 widgetId 归位：ok 块出图，chart_deleted 块保留占位', async () => {
    primeHappyPath();

    renderEditor();

    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());
    expect(screen.getByText('图表已删除')).toBeInTheDocument();
    // 盘级取数只发一次，且下发的是空筛选（v1 无筛选器入口）。
    expect(mockQuery).toHaveBeenCalledWith('d-1', { filters: [] });
    // 已被 /query 覆盖的块不应再走图表自身的取数端点。
    expect(mockGetChartData).not.toHaveBeenCalled();
  });

  it('单块取数失败不整盘失败：其余块照渲染，错误块带出后端原因', async () => {
    mockGetById.mockResolvedValue(envelope(dashboard));
    mockChartsGetAll.mockResolvedValue(envelope([barChart]));
    mockQuery.mockResolvedValue(
      envelope({
        results: [
          { widgetId: 'w-a', chartId: 7, status: 'ok', data: { data: axisPayload } },
          { widgetId: 'w-b', chartId: 99, status: 'error', message: 'relation does not exist' },
        ],
      })
    );

    renderEditor();

    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());
    expect(screen.getByText('取数失败')).toBeInTheDocument();
    expect(screen.getByText('relation does not exist')).toBeInTheDocument();
  });

  it('损坏的 layout_json 收敛为空盘，不抛异常', async () => {
    mockGetById.mockResolvedValue(envelope({ ...dashboard, layout_json: '{broken' }));
    mockChartsGetAll.mockResolvedValue(envelope([barChart]));
    mockQuery.mockResolvedValue(envelope({ results: [] }));

    renderEditor();

    await waitFor(() => expect(screen.getByText('画布还是空的')).toBeInTheDocument());
    expect(screen.queryByTestId('grid')).not.toBeInTheDocument();
  });

  it('新增图表块：未保存的块走图表自身取数端点即时出图', async () => {
    mockGetById.mockResolvedValue(envelope({ ...dashboard, layout_json: '' }));
    mockChartsGetAll.mockResolvedValue(envelope([barChart]));
    mockQuery.mockResolvedValue(envelope({ results: [] }));
    mockGetChartData.mockResolvedValue(envelope(axisPayload));

    renderEditor();

    await waitFor(() => expect(screen.getByText('画布还是空的')).toBeInTheDocument());

    // antd Select 在 mousedown 时展开下拉；选项由 portal 渲染，故按选项内容取节点。
    fireEvent.mouseDown(screen.getByRole('combobox'));
    const option = await screen.findByText('Monthly Sales', {
      selector: '.ant-select-item-option-content',
    });
    fireEvent.click(option);

    // 新块尚未落库，/query 不可能有它的结果，因此必须由本地取数补上。
    await waitFor(() => expect(mockGetChartData).toHaveBeenCalledWith(7));
    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());
  });

  it('保存走 PUT：名称与布局一并落库，成功后清掉本地草稿', async () => {
    primeHappyPath();
    mockUpdate.mockResolvedValue(envelope(dashboard));

    renderEditor();

    // 打开即干净：没有改动时保存不可点，避免发一次无意义的 PUT。
    const saveButton = () => screen.getByRole('button', { name: /保存/ });
    await waitFor(() => expect(saveButton()).toBeDisabled());

    fireEvent.change(screen.getByDisplayValue('销售总览'), { target: { value: '新名字' } });

    await waitFor(() => expect(saveButton()).toBeEnabled());
    // 编辑期防丢：改动已落进本地草稿（真相源仍是后端）。
    await waitFor(() => expect(localStorage.getItem('dashboard-draft:d-1')).not.toBeNull());

    fireEvent.click(saveButton());

    await waitFor(() =>
      expect(mockUpdate).toHaveBeenCalledWith('d-1', { name: '新名字', layout_json: layoutDoc })
    );
    expect(localStorage.getItem('dashboard-draft:d-1')).toBeNull();
  });

  it('本地草稿与后端不一致时恢复，并给出丢弃入口', async () => {
    primeHappyPath();
    localStorage.setItem(
      'dashboard-draft:d-1',
      JSON.stringify({
        name: '草稿名',
        layout: JSON.stringify({ version: 1, grid: { cols: 12 }, widgets: [] }),
      })
    );

    renderEditor();

    await waitFor(() =>
      expect(screen.getByText('已恢复本地草稿（上次未保存的改动）')).toBeInTheDocument()
    );
    expect(screen.getByDisplayValue('草稿名')).toBeInTheDocument();
    // 恢复出的盘是空的（草稿里没有块），后端的那两块不应被渲染。
    expect(screen.getByText('画布还是空的')).toBeInTheDocument();
  });

  it('草稿与后端一致时视为已保存：不提示、不留残稿', async () => {
    primeHappyPath();
    localStorage.setItem(
      'dashboard-draft:d-1',
      JSON.stringify({ name: '销售总览', layout: layoutDoc })
    );

    renderEditor();

    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());
    expect(screen.queryByText('已恢复本地草稿（上次未保存的改动）')).not.toBeInTheDocument();
    expect(localStorage.getItem('dashboard-draft:d-1')).toBeNull();
  });

  // 回归：/query settle 之前会先提交一帧 results={}，若本地取数的判定只看 results，
  // 真实网络下已落库的每块都会多发一次 /api/charts/{id}/data。
  it('盘级取数未返回前不得抢先触发图表自身取数', async () => {
    mockGetById.mockResolvedValue(envelope(dashboard));
    mockChartsGetAll.mockResolvedValue(envelope([barChart]));
    let release!: (value: Awaited<ReturnType<typeof dashboardsApi.query>>) => void;
    mockQuery.mockReturnValue(
      new Promise<Awaited<ReturnType<typeof dashboardsApi.query>>>((resolve) => {
        release = resolve;
      })
    );

    renderEditor();

    await waitFor(() => expect(mockQuery).toHaveBeenCalled());
    // 已落库的块（w-a / w-b）由 /query 负责，不能在这里抢跑。
    expect(mockGetChartData).not.toHaveBeenCalled();

    release(
      envelope({
        results: [
          { widgetId: 'w-a', chartId: 7, status: 'ok', data: { data: axisPayload } },
          { widgetId: 'w-b', chartId: 99, status: 'chart_deleted' },
        ],
      })
    );

    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());
    expect(mockGetChartData).not.toHaveBeenCalled();
  });

  it('拖拽提交：越界位置被收敛，碰撞下推的邻块一并写回', async () => {
    primeHappyPath();
    mockUpdate.mockResolvedValue(envelope(dashboard));

    renderEditor();

    await waitFor(() => expect(screen.getByTestId('grid')).toBeInTheDocument());

    // 模拟 RGL 的拖拽结束回调：w-a 拖到 x=7（7+6=13 > 12，必须收敛到 x=6），
    // 邻块 w-b 被碰撞下推到 y=8。
    const onDragStop = gridHarness.props?.onDragStop as (layout: unknown[]) => void;
    act(() => {
      onDragStop([
        { i: 'w-a', x: 7, y: 0, w: 6, h: 8 },
        { i: 'w-b', x: 6, y: 8, w: 6, h: 8 },
      ]);
    });

    fireEvent.click(screen.getByRole('button', { name: /保存/ }));

    await waitFor(() => expect(mockUpdate).toHaveBeenCalled());
    const payload = mockUpdate.mock.calls[0][1] as { layout_json: string };
    const saved = JSON.parse(payload.layout_json) as {
      widgets: { widgetId: string; x: number; y: number; w: number; h: number }[];
    };
    expect(saved.widgets.find((w) => w.widgetId === 'w-a')).toMatchObject({
      x: 6,
      y: 0,
      w: 6,
      h: 8,
    });
    expect(saved.widgets.find((w) => w.widgetId === 'w-b')).toMatchObject({ x: 6, y: 8 });
    expect(saved.widgets.every((w) => w.w >= 2 && w.h >= 2)).toBe(true);
  });

  it('容错：未知 status 按失败块渲染，results 里多余的 widgetId 被忽略', async () => {
    mockGetById.mockResolvedValue(envelope(dashboard));
    mockChartsGetAll.mockResolvedValue(envelope([barChart]));
    mockQuery.mockResolvedValue(
      envelope({
        results: [
          { widgetId: 'w-a', chartId: 7, status: 'some_future_status' },
          { widgetId: 'w-b', chartId: 99, status: 'chart_deleted' },
          { widgetId: 'w-ghost', chartId: 123, status: 'ok', data: { data: axisPayload } },
        ],
      })
    );

    renderEditor();

    await waitFor(() => expect(screen.getByText('取数失败')).toBeInTheDocument());
    expect(screen.getByText('图表已删除')).toBeInTheDocument();
    // layout 里没有 w-ghost，栅格下只应有两个块。
    expect(document.querySelectorAll('[data-testid="grid"] > *')).toHaveLength(2);
  });

  // 回归（浏览器验收 D-1）：写草稿侧的 dirty 同时看 name 与 layout，而恢复侧只比 layout，
  // 于是「只改了名字」的草稿会被判成"与后端一致"→ 走删除分支，用户的未保存改名静默丢失。
  it('草稿只改了名称（布局与后端一致）时也必须恢复，不得当作已保存清掉', async () => {
    primeHappyPath();
    localStorage.setItem(
      'dashboard-draft:d-1',
      JSON.stringify({ name: '草稿改名', layout: layoutDoc })
    );

    renderEditor();

    await waitFor(() =>
      expect(screen.getByText('已恢复本地草稿（上次未保存的改动）')).toBeInTheDocument()
    );
    expect(screen.getByDisplayValue('草稿改名')).toBeInTheDocument();
    // 布局与后端一致，故仍是后端那两块；草稿要留着（此时 dirty 为真，真相源仍是后端）。
    expect(screen.getByTestId('grid')).toBeInTheDocument();
    expect(localStorage.getItem('dashboard-draft:d-1')).not.toBeNull();
  });

  // 回归（浏览器验收 D-2）：编辑器曾在装载态提前 return 一个纯 Spin 页面，首屏 commit 里
  // 没有栅格容器节点 → RGL 的 useContainerWidth「挂载即测量 + 挂 ResizeObserver」那次
  // effect 读到 null 后直接 return（依赖不含节点，不会重跑），宽度永久停在
  // initialWidth(1280)：窄屏块溢出容器、宽屏右侧留白，且此后 resize 也不再重测。
  it('装载态即渲染栅格容器节点，保证 useContainerWidth 能测到真实宽度', async () => {
    // 悬挂的 getById：页面停在装载态，容器节点必须已经存在。
    mockGetById.mockReturnValue(new Promise<never>(() => {}));
    mockChartsGetAll.mockResolvedValue(envelope([barChart]));

    renderEditor();

    await waitFor(() => expect(gridHarness.containerRef?.current).toBeTruthy());
  });
});
