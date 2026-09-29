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
import { useStore } from '../../store';

/**
 * 图表联动的全链路（issue #143）：配置目标 → 点击数据项 → 下发 linkages → 一键清除。
 *
 * 契约的另一半在后端 `service/dashboard/query.go` 的 buildLinkageOverrides 用例：请求只带
 * `{sourceWidgetId, value[]}`（单值 → eq），落点列由后端从 layout 的 linkage.targets 读。
 */

// ECharts 被整模块替换，点击处理器要能被测试直接触发（真 canvas 在 jsdom 里点不到）。
const echo = vi.hoisted(() => ({ chartClick: undefined as undefined | ((p: unknown) => void) }));

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

// jsdom 量不到容器宽度，真 RGL 会永久停在 width=0 的加载态 → 整模块替换（同既有测试）。
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
  default: (props: { onEvents?: { click?: (p: unknown) => void } }) => {
    // 只有挂了点击的图才覆盖：多块图里未挂点击的那些（缺省 undefined）不该把已捕获的
    // 处理器清掉 —— 否则测试点到的永远是「最后渲染的那块」。
    const click = props.onEvents?.click;
    if (click) {
      echo.chartClick = click;
    }
    return <div data-testid="echarts" />;
  },
}));

import { chartsApi, dashboardsApi, datasetsApi } from '../../api';

const mockGetById = vi.mocked(dashboardsApi.getById);
const mockUpdate = vi.mocked(dashboardsApi.update);
const mockQuery = vi.mocked(dashboardsApi.query);
const mockChartsGetAll = vi.mocked(chartsApi.getAll);
const mockGetChartData = vi.mocked(chartsApi.getChartData);
const mockDatasetsGetAll = vi.mocked(datasetsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);

/** 非空的柱状负载：空负载会让 ChartView 落到「无法渲染」兜底臂，点不到图。 */
const AXIS_PAYLOAD = { x_axis: ['2026-01'], series: [{ name: 'total', data: [10] }] };

function envelope<T>(data: T): AxiosResponse<ApiResponse<T>> {
  return { data: { code: 20000, msg: 'success', trace: '', data }, status: 200 } as never;
}

/** v2 文档：唯一维度是 month（列 id），故这块图可作联动来源。 */
const chartConfig = (dimensions: string[]): string =>
  JSON.stringify({
    version: 2,
    chartType: 'bar',
    title: 'Monthly Sales',
    query: {
      dimensionGroups: [
        {
          id: 'dim-0',
          bindings: dimensions.map((fieldId, index) => ({ bindingId: `b-${index}`, fieldId })),
        },
      ],
      metricGroups: [{ id: 'm-0', bindings: [{ bindingId: 'b-m', fieldId: 'total' }] }],
    },
    fieldMeta: {},
  });

const chart = (id: number, name: string, dimensions: string[]): Chart => ({
  id,
  name,
  dataset_id: 3,
  chart_type: 'bar',
  config: chartConfig(dimensions),
  created_at: '',
  updated_at: '',
});

const sourceChart = chart(7, 'Monthly Sales', ['month']);
/** 目标图有两个维度：它自己不可作联动来源（点不出唯一维度），故不会覆盖捕获到的点击处理器。 */
const targetChart = chart(8, 'Sales By Region', ['region', 'city']);

const layoutDoc = (widgets: unknown[]) =>
  JSON.stringify({ version: 1, grid: { cols: 12 }, widgets });

/** 两块同数据集图表：w-a（来源，单维度）+ w-b（目标，单维度）。 */
const twoCharts = () =>
  layoutDoc([
    { widgetId: 'w-a', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
    { widgetId: 'w-b', type: 'chart', chartId: 8, x: 6, y: 0, w: 6, h: 8 },
  ]);

/** w-a 已配好「打到 w-b 的 region 列」的联动配置。 */
const linkedLayout = () =>
  layoutDoc([
    {
      widgetId: 'w-a',
      type: 'chart',
      chartId: 7,
      x: 0,
      y: 0,
      w: 6,
      h: 8,
      linkage: { targets: [{ widgetId: 'w-b', column: 'region' }] },
    },
    { widgetId: 'w-b', type: 'chart', chartId: 8, x: 6, y: 0, w: 6, h: 8 },
  ]);

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
  mockChartsGetAll.mockResolvedValue(envelope([sourceChart, targetChart]));
  mockGetChartData.mockResolvedValue(envelope(AXIS_PAYLOAD as never));
  mockUpdate.mockResolvedValue(envelope(dashboard(layout)) as never);
  mockQuery.mockResolvedValue(
    envelope({
      results: [
        { widgetId: 'w-a', chartId: 7, status: 'ok', data: { data: AXIS_PAYLOAD } },
        { widgetId: 'w-b', chartId: 8, status: 'ok', data: { data: AXIS_PAYLOAD } },
      ],
    })
  );
  mockDatasetsGetAll.mockResolvedValue(envelope([{ id: 3, name: '天气数据' }] as never));
  mockGetColumns.mockResolvedValue(
    envelope([
      { id: 'month', name: 'month', type: 'string', role: 'dimension', expr: 'month' },
      { id: 'region', name: 'region', type: 'string', role: 'dimension', expr: 'region' },
    ] as never)
  );
}

/** 最近一次 /query 收到的联动载荷。 */
const lastLinkages = (): { sourceWidgetId: string; value: unknown[] }[] => {
  const calls = mockQuery.mock.calls;
  return calls.length === 0 ? [] : (calls[calls.length - 1][1].linkages ?? []);
};

/** 触发来源图的 ECharts 数据项点击。 */
const clickDataItem = (name: string) => {
  act(() => {
    echo.chartClick?.({ componentType: 'series', name });
  });
};

beforeEach(() => {
  vi.clearAllMocks();
  echo.chartClick = undefined;
  localStorage.clear();
  useStore.setState({ datasets: [] });
});

describe('仪表盘图表联动', () => {
  it('点击数据项 → /query 带上该来源的联动取值（单值收成单元素数组）', async () => {
    prime(linkedLayout());
    renderEditor();

    await waitFor(() => expect(echo.chartClick).toBeTypeOf('function'));
    // 首屏没有点击，不下发联动。
    expect(lastLinkages()).toEqual([]);

    clickDataItem('华东');

    await waitFor(() =>
      expect(lastLinkages()).toEqual([{ sourceWidgetId: 'w-a', value: ['华东'] }])
    );
  });

  it('目标块出「被谁联动」的提示，一键清除后回到无联动载荷', async () => {
    prime(linkedLayout());
    renderEditor();

    await waitFor(() => expect(echo.chartClick).toBeTypeOf('function'));
    clickDataItem('华东');

    // 目标块（w-b）上出现来源标题 + 取值。
    expect(await screen.findByTestId('linkage-in-w-b')).toHaveTextContent('Monthly Sales: 华东');

    fireEvent.click(screen.getByTestId('linkage-clear-all'));

    await waitFor(() => expect(lastLinkages()).toEqual([]));
    expect(screen.queryByTestId('linkage-in-w-b')).not.toBeInTheDocument();
  });

  it('联动设置勾选目标后落库：保存的 layout 带上 linkage.targets（同数据集沿用来源键列）', async () => {
    prime(twoCharts());
    renderEditor();

    await waitFor(() => expect(mockGetById).toHaveBeenCalled());
    fireEvent.click(await screen.findByTestId('block-menu-w-a'));
    fireEvent.click(await screen.findByText('联动设置'));

    fireEvent.click(await screen.findByTestId('linkage-target-w-b'));
    fireEvent.click(screen.getByTestId('linkage-ok'));
    fireEvent.click(screen.getByTestId('dashboard-save'));

    await waitFor(() => expect(mockUpdate).toHaveBeenCalled());
    const saved = JSON.parse(mockUpdate.mock.calls[0][1].layout_json ?? '{}');
    const source = saved.widgets.find((widget: { widgetId: string }) => widget.widgetId === 'w-a');
    expect(source.linkage).toEqual({ targets: [{ widgetId: 'w-b', column: 'month' }] });
  });

  it('多维度图（点不出唯一维度）不挂点击，点了不下发联动', async () => {
    prime(layoutDoc([{ widgetId: 'w-a', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 }]));
    // prime 会灌一份图表清单，这里再覆盖成「来源图有两个维度」的那份。
    mockChartsGetAll.mockResolvedValue(envelope([chart(7, 'Monthly Sales', ['month', 'region'])]));
    renderEditor();

    await waitFor(() => expect(mockGetById).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByTestId('echarts')).toBeInTheDocument());
    expect(echo.chartClick).toBeUndefined();
  });
});
