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
import { useStore } from '../../store';

/**
 * 盘级日期筛选器的全链路：布局默认值 → 首屏下发 → 行内控件改值 → 立即重取 → 保存后生效。
 *
 * 形状契约的另一半在 `lib/dashboardFilterValue.test.ts`（前端）与后端
 * `service/dashboard/query.go` 的 buildOverrides 用例（`between` 要两元素、标量算子要一元素）。
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

const COLUMNS = [
  { id: '0000i529', name: 'date', type: 'date', role: 'dimension', expr: 'date' },
  { id: '0000i52a', name: 'region', type: 'string', role: 'dimension', expr: 'region' },
  { id: '0000i52b', name: 'amount', type: 'float', role: 'metric', expr: 'amount' },
];

/** 布局里有一块图表 + 一块日期筛选器，筛选器落库了默认值。 */
const layoutWithFilter = (defaultValue: unknown, dataType = 'date') =>
  JSON.stringify({
    version: 1,
    grid: { cols: 12 },
    widgets: [
      { widgetId: 'w-a', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
      {
        widgetId: 'w-f',
        type: 'filter',
        binding: { datasetId: 3, column: '0000i529' },
        label: '交易日期',
        dataType,
        operator: 'between',
        multi: false,
        date: { granularity: 'day', weekStart: 1 },
        defaultValue,
        x: 6,
        y: 0,
        w: 3,
        h: 3,
      },
    ],
  });

/** 只有一块图表的布局：用于「新增筛选器」路径（避免夹具里本来就有筛选器块）。 */
const layoutChartOnly = () =>
  JSON.stringify({
    version: 1,
    grid: { cols: 12 },
    widgets: [{ widgetId: 'w-a', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 }],
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
  mockQuery.mockResolvedValue(
    envelope({
      results: [
        { widgetId: 'w-a', chartId: 7, status: 'ok', data: { data: { x_axis: [], series: [] } } },
      ],
    })
  );
  mockDatasetsGetAll.mockResolvedValue(envelope([{ id: 3, name: '天气数据' }] as never));
  mockGetColumns.mockResolvedValue(envelope(COLUMNS as never));
  // 枚举候选值：响应按**列名**返键，而块里只有列 ID，所以实现是取该行的唯一值。
  mockQueryDistinct.mockResolvedValue(envelope([{ region: '华东' }, { region: '华南' }]));
}

/** 最近一次 /query 收到的筛选器载荷。 */
const lastFilters = (): { widgetId: string; value?: unknown[] }[] => {
  const calls = mockQuery.mock.calls;
  return calls.length === 0 ? [] : (calls[calls.length - 1][1].filters ?? []);
};

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  // store 是模块级单例：上一个用例灌进去的 datasets 会漏到下一个用例。
  useStore.setState({ datasets: [] });
});

describe('仪表盘盘级日期筛选器', () => {
  it('首屏按布局里落库的默认值下发（区间拆成两元素）', async () => {
    prime(layoutWithFilter({ kind: 'fixed', start: '2026-08-01', end: '2026-08-31' }));

    renderEditor();

    await waitFor(() => expect(mockQuery).toHaveBeenCalled());
    expect(lastFilters()).toEqual([{ widgetId: 'w-f', value: ['2026-08-01', '2026-08-31'] }]);
  });

  it('默认值「未选择」时不下发（空数组 = 未激活，不能被当成一条恒真/恒假条件）', async () => {
    prime(layoutWithFilter({ kind: 'dynamic' }));

    renderEditor();

    await waitFor(() => expect(mockQuery).toHaveBeenCalled());
    expect(lastFilters()).toEqual([]);
  });

  it('非日期类型的筛选器块不下发（语义不同，不能按日期算子合并）', async () => {
    prime(layoutWithFilter({ kind: 'fixed', start: '2026-08-01', end: '2026-08-31' }, 'string'));

    renderEditor();

    await waitFor(() => expect(mockQuery).toHaveBeenCalled());
    expect(lastFilters()).toEqual([]);
  });

  it('落库的筛选器块渲染出来，且不显示「保存后生效」提示', async () => {
    prime(layoutWithFilter({ kind: 'fixed', start: '2026-08-01', end: '2026-08-31' }));

    renderEditor();

    await waitFor(() => expect(screen.getByTestId('date-filter-control')).toBeInTheDocument());
    expect(screen.getByTestId('date-filter-control').textContent).toContain('交易日期');
    expect(screen.queryByText(/保存仪表盘后/)).not.toBeInTheDocument();
  });

  it('行内控件改筛选条件 → 立即按新取值重取（两元素区间）', async () => {
    prime(layoutWithFilter({ kind: 'fixed', start: '2026-08-01', end: '2026-08-31' }));

    renderEditor();
    await waitFor(() => expect(mockQuery).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByTestId('date-filter-control'));
    // 当前值是「固定日期」，浮层先渲染区间选择器；切到动态日期才有快捷选项网格。
    // 切模式本身也会立即生效（变成「未选择」= 未激活，图表回到全量）——这是刻意的。
    fireEvent.click(screen.getByTestId('date-filter-control-mode-dynamic'));
    fireEvent.click(screen.getByTestId('date-filter-preset-last30d'));

    await waitFor(() => {
      const payload = lastFilters();
      expect(payload).toHaveLength(1);
      expect(payload[0].widgetId).toBe('w-f');
      expect(payload[0].value).toHaveLength(2);
    });
    for (const bound of lastFilters()[0].value ?? []) {
      expect(typeof bound).toBe('string');
    }
    // 改完筛选器即成为未保存改动（取值会作为该筛选器的默认值随盘一起保存）
    expect(screen.getByText('有未保存的改动')).toBeInTheDocument();
  });

  it('新增筛选器：选数据集 + 日期字段 → 出块，未保存前不下发并提示保存后生效', async () => {
    prime(layoutChartOnly());

    renderEditor();
    await waitFor(() => expect(mockQuery).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId('dashboard-filter-configure')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('dashboard-add-filter'));
    const dialog = await screen.findByRole('dialog');
    const combos = within(dialog).getAllByRole('combobox');

    fireEvent.mouseDown(combos[0]);
    fireEvent.click(
      await screen.findByText('天气数据', { selector: '.ant-select-item-option-content' })
    );
    fireEvent.mouseDown(combos[1]);
    fireEvent.click(
      await screen.findByText('date', { selector: '.ant-select-item-option-content' })
    );
    fireEvent.click(within(dialog).getByRole('button', { name: /确\s*定/ }));

    await waitFor(() =>
      expect(screen.getByTestId('dashboard-filter-configure')).toBeInTheDocument()
    );
    expect(screen.getByText(/保存仪表盘后/)).toBeInTheDocument();
    // 未落库的筛选器后端读不到：新增块之后不该白发一次取数
    expect(mockQuery).toHaveBeenCalledTimes(1);
    expect(lastFilters()).toEqual([]);
  });

  it('「配置」进完整日期筛选弹窗，确定后落库并重取', async () => {
    prime(layoutWithFilter({ kind: 'fixed', start: '2026-08-01', end: '2026-08-31' }));

    renderEditor();
    await waitFor(() => expect(mockQuery).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByTestId('dashboard-filter-configure'));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('日期筛选')).toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole('button', { name: /确\s*定/ }));

    await waitFor(() => expect(mockQuery).toHaveBeenCalledTimes(2));
    expect(lastFilters()).toEqual([{ widgetId: 'w-f', value: ['2026-08-01', '2026-08-31'] }]);
  });

  it('保存把筛选器取值写进布局，落库后清除「保存后生效」提示', async () => {
    prime(layoutChartOnly());
    vi.mocked(dashboardsApi.update).mockResolvedValue(envelope(dashboard('')));

    renderEditor();
    await waitFor(() => expect(mockQuery).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByTestId('dashboard-add-filter'));
    const dialog = await screen.findByRole('dialog');
    const combos = within(dialog).getAllByRole('combobox');
    fireEvent.mouseDown(combos[0]);
    fireEvent.click(
      await screen.findByText('天气数据', { selector: '.ant-select-item-option-content' })
    );
    fireEvent.mouseDown(combos[1]);
    fireEvent.click(
      await screen.findByText('date', { selector: '.ant-select-item-option-content' })
    );
    fireEvent.click(within(dialog).getByRole('button', { name: /确\s*定/ }));

    await waitFor(() => expect(screen.getByText(/保存仪表盘后/)).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }));

    await waitFor(() => expect(dashboardsApi.update).toHaveBeenCalled());
    const [, payload] = vi.mocked(dashboardsApi.update).mock.calls[0];
    const saved = JSON.parse(String(payload.layout_json));
    const filterWidget = saved.widgets.find((w: { type: string }) => w.type === 'filter');
    expect(filterWidget).toBeTruthy();
    expect(filterWidget.binding.column).toBe('0000i529');
    expect(filterWidget.date).toEqual({ granularity: 'day', weekStart: 1 });
  });
});

describe('字符串 / 数值族盘级筛选器', () => {
  /** 只有一块图表 + 一块筛选器的最小布局。 */
  const layoutWith = (widget: Record<string, unknown>) =>
    JSON.stringify({
      version: 1,
      grid: { cols: 12 },
      widgets: [{ widgetId: 'w-a', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 }, widget],
    });

  const numberWidget = (defaultValue: unknown, operator = 'between') => ({
    widgetId: 'w-n',
    type: 'filter',
    binding: { datasetId: 3, column: '0000i52b' },
    label: '金额',
    dataType: 'float',
    operator,
    multi: false,
    defaultValue,
    x: 6,
    y: 0,
    w: 3,
    h: 3,
  });

  const textWidget = (defaultValue: unknown) => ({
    widgetId: 'w-t',
    type: 'filter',
    binding: { datasetId: 3, column: '0000i52a' },
    label: '地区',
    dataType: 'string',
    operator: 'in',
    multi: true,
    defaultValue,
    x: 6,
    y: 0,
    w: 3,
    h: 3,
  });

  it('数值族 between：默认值按两元素下发，块渲染上下界两个输入', async () => {
    prime(layoutWith(numberWidget([10, 20])));

    renderEditor();

    await waitFor(() => expect(mockQuery).toHaveBeenCalled());
    expect(lastFilters()).toEqual([{ widgetId: 'w-n', value: [10, 20] }]);
    expect(screen.getByTestId('dashboard-filter-min')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-filter-max')).toBeInTheDocument();
    // 数值族没有日期那套「配置」入口（粒度/周计算逻辑与它无关）
    expect(screen.queryByTestId('dashboard-filter-configure')).not.toBeInTheDocument();
  });

  it('数值族换算子：between → 大于 后改为单元素下发', async () => {
    prime(layoutWith(numberWidget([10, 20])));

    renderEditor();
    await waitFor(() => expect(mockQuery).toHaveBeenCalledTimes(1));

    fireEvent.mouseDown(screen.getByTestId('dashboard-filter-operator'));
    fireEvent.click(
      await screen.findByText('大于', { selector: '.ant-select-item-option-content' })
    );

    await waitFor(() => {
      expect(lastFilters()).toEqual([{ widgetId: 'w-n', value: [10] }]);
    });
    // 换成单侧算子后只剩一个输入
    expect(screen.queryByTestId('dashboard-filter-max')).not.toBeInTheDocument();
  });

  it('字符串族：候选值实查该列，已选值按数组下发', async () => {
    prime(layoutWith(textWidget(['华东', '华南'])));

    renderEditor();

    await waitFor(() => expect(mockQuery).toHaveBeenCalled());
    expect(lastFilters()).toEqual([{ widgetId: 'w-t', value: ['华东', '华南'] }]);
    // 候选值走 queryDistinct，参数是该筛选器绑定的数据集 + 列 ID
    await waitFor(() => expect(mockQueryDistinct).toHaveBeenCalledWith(3, '0000i52a'));
    expect(screen.getByTestId('dashboard-filter-values')).toBeInTheDocument();
  });

  it('字符串族：未选值（空数组）不下发，但控件照常渲染', async () => {
    prime(layoutWith(textWidget([])));

    renderEditor();

    await waitFor(() => expect(mockQuery).toHaveBeenCalled());
    expect(lastFilters()).toEqual([]);
    expect(screen.getByTestId('dashboard-filter-values')).toBeInTheDocument();
  });

  it('三族可以同盘共存：各自按自己的算子收形', async () => {
    const layout = JSON.stringify({
      version: 1,
      grid: { cols: 12 },
      widgets: [
        { widgetId: 'w-a', type: 'chart', chartId: 7, x: 0, y: 0, w: 6, h: 8 },
        {
          widgetId: 'w-d',
          type: 'filter',
          binding: { datasetId: 3, column: '0000i529' },
          label: '交易日期',
          dataType: 'date',
          operator: 'between',
          multi: false,
          date: { granularity: 'day', weekStart: 1 },
          defaultValue: { kind: 'fixed', start: '2026-08-01', end: '2026-08-31' },
          x: 6,
          y: 0,
          w: 3,
          h: 3,
        },
        numberWidget([10, 20]),
        textWidget(['华东']),
      ],
    });
    prime(layout);

    renderEditor();

    await waitFor(() => expect(mockQuery).toHaveBeenCalled());
    expect(lastFilters()).toEqual([
      { widgetId: 'w-d', value: ['2026-08-01', '2026-08-31'] },
      { widgetId: 'w-n', value: [10, 20] },
      { widgetId: 'w-t', value: ['华东'] },
    ]);
  });
});
