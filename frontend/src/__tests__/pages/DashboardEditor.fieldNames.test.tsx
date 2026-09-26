// DashboardEditor → ChartView 的 fieldNames 契约。
//
// 单独成文件的原因：这里用捕获替身替换整个 ChartView 来断言 props，而
// DashboardEditor.test.tsx 依赖真实 ChartView 渲染 echarts-for-react 的替身，
// vi.mock 是文件级作用域，不能混用。
//
// 背景（2026-09-25 浏览器验收缺陷 1）：编辑器此前不传 fieldNames，combo 分支
// xAxis.name 经 labelOf 兜底把裸列 ID（如 `0000i531`）画进了坐标轴。
import { render, waitFor } from '@testing-library/react';
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
    getColumns: vi.fn(),
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
      containerRef: { current: null },
      measureWidth: () => {},
    }),
    verticalCompactor: { kind: 'vertical' },
  };
});

const chartViewHarness = vi.hoisted(() => ({ props: null as Record<string, unknown> | null }));

vi.mock('../../components/ChartView/ChartView', () => ({
  default: (props: Record<string, unknown>) => {
    chartViewHarness.props = props;
    return <div data-testid="chartview" />;
  },
}));

import { chartsApi, dashboardsApi, datasetsApi } from '../../api';

const mockGetById = vi.mocked(dashboardsApi.getById);
const mockQuery = vi.mocked(dashboardsApi.query);
const mockChartsGetAll = vi.mocked(chartsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

const envelope = <T,>(data: T) =>
  mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data });

const comboChart: Chart = {
  id: 8,
  name: '组合图',
  dataset_id: 42,
  chart_type: 'combo',
  config: JSON.stringify({
    version: 1,
    chartType: 'combo',
    query: {
      dimensionGroups: [{ fields: ['0000i53i'] }],
      metricGroups: [{ fields: ['0000i53j'] }],
    },
    fieldMeta: {},
  }),
  created_at: '',
  updated_at: '',
};

const layoutDoc = JSON.stringify({
  version: 1,
  grid: { cols: 12 },
  widgets: [{ widgetId: 'w-a', type: 'chart', chartId: 8, x: 0, y: 0, w: 6, h: 8 }],
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
      {/* 页面用 App.useApp() 取 message，需要根部的 <App> 包裹（见 main.tsx）。 */}
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
  mockGetById.mockResolvedValue(envelope(dashboard));
  mockChartsGetAll.mockResolvedValue(envelope([comboChart]));
  mockQuery.mockResolvedValue(
    envelope({
      results: [
        {
          widgetId: 'w-a',
          chartId: 8,
          status: 'ok',
          data: { data: { x_axis: ['苏州'], series: [] } },
        },
      ],
    })
  );
});

describe('DashboardEditor → ChartView 的 fieldNames 契约', () => {
  it('为块引用的图表拉取数据集列，并把列 ID→列名映射传给 ChartView', async () => {
    mockGetColumns.mockResolvedValue(
      envelope([
        {
          id: '0000i53i',
          name: 'city',
          expr: 'city',
          type: 'string',
          type_config: { precision: 0, scale: 0 },
          comment: '',
          role: 'dimension',
        },
        {
          id: '0000i53j',
          name: 'total_sales',
          expr: 'total_sales',
          type: 'integer',
          type_config: { precision: 0, scale: 0 },
          comment: '',
          role: 'metric',
        },
      ])
    );

    renderEditor();

    await waitFor(() => expect(mockGetColumns).toHaveBeenCalledWith(42));
    await waitFor(() => expect(chartViewHarness.props).not.toBeNull());
    expect(chartViewHarness.props?.fieldNames).toEqual({
      '0000i53i': 'city',
      '0000i53j': 'total_sales',
    });
  });

  it('列加载失败时不阻断渲染（fieldNames 回落空映射）', async () => {
    mockGetColumns.mockRejectedValue(new Error('dataset gone'));

    renderEditor();

    await waitFor(() => expect(chartViewHarness.props).not.toBeNull());
    expect(chartViewHarness.props?.fieldNames).toEqual({});
  });
});
