// 缺陷 1 回归：combo 分支 xAxis.name 经 labelOf(nameOf(fieldId)) 应产出列名。
// 此前 DashboardEditor 不传 fieldNames，画布上 x 轴末端渲染裸列 ID（如 0000i53l）。
// 用图表 8（组合图-城市总销量vs新能源）的真实 config 与负载形状在组件级钉死。
import { render } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { describe, expect, it, vi } from 'vitest';
import type { Chart } from '../../api';
import ChartView from '../../components/ChartView/ChartView';
import { messages } from '../../i18n/useLocale';

// 捕获传给 ECharts 的 option
const echartsHarness = vi.hoisted(() => ({ options: [] as unknown[] }));

vi.mock('echarts-for-react', () => ({
  default: (props: { option: unknown }) => {
    echartsHarness.options.push(props.option);
    return <div data-testid="echarts" />;
  },
}));

// 本测试不感知路由/API：ChartView 是纯展示组件
const comboConfig = JSON.stringify({
  query: {
    sort: null,
    limit: 0,
    filters: [],
    metricGroups: [
      { id: 'primary_values', bindings: [{ fieldId: '0000i53m', bindingId: 'total_sales__sum' }] },
      { id: 'secondary_values', bindings: [{ fieldId: '0000i53n', bindingId: 'ev_sales__sum' }] },
    ],
    dimensionGroups: [{ id: 'x_axis', bindings: [{ fieldId: '0000i53l', bindingId: 'city' }] }],
  },
  version: 2,
  chartType: 'combo',
  fieldMeta: {
    total_sales__sum: { alias: 'total_sales', aggregation: 'sum' },
    ev_sales__sum: { alias: 'ev_sales', aggregation: 'sum' },
  },
});

const comboChart: Chart = {
  id: 8,
  name: '组合图-城市总销量vs新能源',
  dataset_id: 42,
  chart_type: 'combo',
  config: comboConfig,
  created_at: '',
  updated_at: '',
};

const payload = {
  x_axis: ['深圳', '上海', '杭州'],
  series: [
    { name: 'total_sales', data: [31538, 40381, 34484] },
    { name: 'ev_sales', data: [22856, 29053, 23318] },
  ],
};

// 数据集 42 的列 ID→列名（= DashboardEditor 经 datasetsApi.getColumns 注入的映射）
const fieldNames = { '0000i53l': 'city', '0000i53m': 'total_sales', '0000i53n': 'ev_sales' };

describe('ChartView combo：xAxis.name 用列名而非裸列 ID', () => {
  it('传入 fieldNames 时 xAxis.name === "city"', () => {
    echartsHarness.options.length = 0;
    render(
      <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
        <ChartView chart={comboChart} data={payload} fieldNames={fieldNames} />
      </IntlProvider>
    );
    expect(echartsHarness.options.length).toBeGreaterThan(0);
    const option = echartsHarness.options[0] as { xAxis: { name: string; data: string[] } };
    expect(option.xAxis.name).toBe('city');
    expect(option.xAxis.data).toEqual(['深圳', '上海', '杭州']);
  });

  it('fieldNames 缺失时维持旧行为（xAxis.name 回落原值，不抛异常）', () => {
    echartsHarness.options.length = 0;
    render(
      <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
        <ChartView chart={comboChart} data={payload} />
      </IntlProvider>
    );
    const option = echartsHarness.options[0] as { xAxis: { name: string } };
    expect(option.xAxis.name).toBe('0000i53l');
  });

  it('fieldId 恰为 __proto__ 时不得穿透原型链（nameOf 只认自有键）', () => {
    // 恶意/巧合的历史 config：维度 fieldId 是 "__proto__"。
    // 若 nameOf 写成 fieldNames[field]，会取到 Object.prototype（非空）→ 轴名变成对象。
    const protoChart: Chart = {
      ...comboChart,
      config: JSON.stringify({
        query: {
          sort: null,
          limit: 0,
          filters: [],
          metricGroups: [
            {
              id: 'primary_values',
              bindings: [{ fieldId: '0000i53m', bindingId: 'total_sales__sum' }],
            },
            {
              id: 'secondary_values',
              bindings: [{ fieldId: '0000i53n', bindingId: 'ev_sales__sum' }],
            },
          ],
          dimensionGroups: [{ id: 'x_axis', bindings: [{ fieldId: '__proto__', bindingId: 'p' }] }],
        },
        version: 2,
        chartType: 'combo',
        fieldMeta: {
          total_sales__sum: { alias: 'total_sales', aggregation: 'sum' },
          ev_sales__sum: { alias: 'ev_sales', aggregation: 'sum' },
        },
      }),
    };
    echartsHarness.options.length = 0;
    render(
      <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
        <ChartView chart={protoChart} data={payload} fieldNames={fieldNames} />
      </IntlProvider>
    );
    const option = echartsHarness.options[0] as { xAxis: { name: unknown } };
    expect(option.xAxis.name).toBe('__proto__');
  });
});
