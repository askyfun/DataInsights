import { beforeEach, describe, expect, it } from 'vitest';
import { buildChartOption } from '@/lib/chartOptions';
import type { ChartField } from '@/store';
import { useStore } from '@/store';

/**
 * 测试字段顺序在整条链路中是否正确保持。
 *
 * 核心规则：字段顺序由 queryConfig.dimensionGroups / metricGroups 决定，
 * 不是 chartBuilderFields 数组（API 返回）的顺序。
 *
 * 之前反复出现的 bug：用 .filter() 从 chartBuilderFields 取字段，
 * 保留了源数组顺序而不是 store 顺序。
 */

// 模拟 chartBuilderFields，顺序故意和 store 不同（v1 契约：fieldId 即稳定列名）
const MOCK_FIELDS: ChartField[] = [
  { id: 'city', name: 'city', type: 'dimension', dataType: 'string' },
  { id: 'date', name: 'date', type: 'dimension', dataType: 'date' },
  { id: 'country', name: 'country', type: 'dimension', dataType: 'string' },
  { id: 'revenue', name: 'revenue', type: 'metric', dataType: 'float' },
  { id: 'cost', name: 'cost', type: 'metric', dataType: 'float' },
  { id: 'profit', name: 'profit', type: 'metric', dataType: 'float' },
];

function setupStore() {
  const store = useStore.getState();
  store.resetChartBuilder();
  useStore.setState({
    chartBuilderFields: MOCK_FIELDS,
    queryConfig: {
      dimensionGroups: [{ id: 'dim-main', fields: ['date', 'city', 'country'] }],
      metricGroups: [{ id: 'metric-main', fields: ['revenue', 'cost'] }],
      filters: [],
      limit: 1000,
    },
  });
}

/**
 * 从 chartBuilderFields + queryConfig 提取维度字段名（保序）。
 * 这是 ChartCanvas / getDimensionFields 使用的正确模式。
 */
function getDimensionFieldNames(): string[] {
  const state = useStore.getState();
  const fieldMap = new Map(state.chartBuilderFields.map((f) => [f.id, f]));
  const dimIds = state.queryConfig.dimensionGroups.flatMap((g) => g.fields);
  return dimIds.map((id) => fieldMap.get(id)?.name).filter(Boolean) as string[];
}

function getMetricFieldNames(): string[] {
  const state = useStore.getState();
  const fieldMap = new Map(state.chartBuilderFields.map((f) => [f.id, f]));
  const metIds = state.queryConfig.metricGroups.flatMap((g) => g.fields);
  return metIds.map((id) => fieldMap.get(id)?.name).filter(Boolean) as string[];
}

/**
 * 错误的模式：用 .filter() 保留源数组顺序。
 * 这是之前反复出现 bug 的写法，作为对照组。
 */
function getDimensionFieldNames_BUGGY(): string[] {
  const state = useStore.getState();
  const dimIds = state.queryConfig.dimensionGroups.flatMap((g) => g.fields);
  return state.chartBuilderFields.filter((f) => dimIds.includes(f.id)).map((f) => f.name);
}

describe('字段顺序：store 的 queryConfig 顺序优先于 chartBuilderFields 源数组顺序', () => {
  beforeEach(() => {
    setupStore();
  });

  it('chartBuilderFields 源数组顺序: city, date, country', () => {
    const sourceOrder = useStore
      .getState()
      .chartBuilderFields.filter((f) => f.type === 'dimension')
      .map((f) => f.name);
    expect(sourceOrder).toEqual(['city', 'date', 'country']);
  });

  it('queryConfig 顺序: date, city, country', () => {
    const storeOrder = useStore.getState().queryConfig.dimensionGroups[0].fields;
    expect(storeOrder).toEqual(['date', 'city', 'country']);
  });

  it('正确模式 (Map+map) 返回 store 顺序: date, city, country', () => {
    expect(getDimensionFieldNames()).toEqual(['date', 'city', 'country']);
  });

  it('错误模式 (.filter) 返回源数组顺序: city, date, country — 这是 bug', () => {
    // 这个测试证明 .filter() 的顺序是错的
    const buggyResult = getDimensionFieldNames_BUGGY();
    expect(buggyResult).toEqual(['city', 'date', 'country']);
    // 和正确顺序不同
    expect(buggyResult).not.toEqual(getDimensionFieldNames());
  });

  it('指标字段也保持 store 顺序: revenue, cost', () => {
    expect(getMetricFieldNames()).toEqual(['revenue', 'cost']);
  });

  it('指标字段 .filter() 顺序也是错的', () => {
    const state = useStore.getState();
    const metIds = state.queryConfig.metricGroups.flatMap((g) => g.fields);
    const buggy = state.chartBuilderFields.filter((f) => metIds.includes(f.id)).map((f) => f.name);
    // source order: revenue, cost, profit; store order: revenue, cost
    // .filter() 返回 source 中匹配的前两个: revenue, cost
    // 这个 case 碰巧一样，但顺序依赖源数组
    expect(buggy).toEqual(['revenue', 'cost']);
  });
});

describe('字段顺序：添加字段后顺序正确', () => {
  beforeEach(() => {
    const store = useStore.getState();
    store.resetChartBuilder();
    useStore.setState({ chartBuilderFields: MOCK_FIELDS });
  });

  it('依次添加字段，顺序和添加顺序一致', () => {
    const { addDimensionField } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    // 按 date, city, country 顺序添加
    addDimensionField(fields[1]); // date
    addDimensionField(fields[0]); // city
    addDimensionField(fields[2]); // country

    expect(getDimensionFieldNames()).toEqual(['date', 'city', 'country']);
  });

  it('交换顺序后，新顺序正确', () => {
    const { addDimensionField, reorderDimensionField } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionField(fields[1]); // date
    addDimensionField(fields[0]); // city
    addDimensionField(fields[2]); // country

    // 把 date 移到末尾
    reorderDimensionField(0, 2);

    expect(getDimensionFieldNames()).toEqual(['city', 'country', 'date']);
  });
});

describe('结构化 AxisResponse：顺序原样保留（executeChartQuery 不再拆平铺行）', () => {
  /**
   * 旧架构里 store 会把 AxisResponse 拆成行数组（用 dims[0] 作行键），
   * x 轴/系列顺序依赖行键匹配，出过 xAxisField 错配的 bug。
   * D7 修复后 chartData 即结构化负载原样，x 轴顺序 = x_axis 顺序、
   * 系列顺序 = series 顺序，由共享的 buildChartOption 直接消费。
   */
  function buildAxisOption(
    axisData: { x_axis: string[]; series: Array<{ name: string; data: unknown[] }> },
    dimensions: string[],
    metrics: string[]
  ) {
    const option = buildChartOption(
      'bar',
      axisData,
      { colors: [], smooth: false, tableRowSize: 'small' },
      {},
      { title: '', dimensions, metrics }
    );
    expect(option).not.toBeNull();
    return option as unknown as {
      xAxis: { data: unknown[] };
      series: { name: string; data: unknown[] }[];
    };
  }

  it('单维度：x 轴是 x_axis 原序，系列是指标名', () => {
    const option = buildAxisOption(
      { x_axis: ['A', 'B'], series: [{ name: 'revenue', data: [100, 200] }] },
      ['category'],
      ['revenue']
    );
    expect(option.xAxis.data).toEqual(['A', 'B']);
    expect(option.series).toEqual([{ name: 'revenue', type: 'bar', data: [100, 200] }]);
  });

  it('多维度：系列名是第二个维度的值，顺序原样保留', () => {
    const option = buildAxisOption(
      {
        x_axis: ['2024-01', '2024-02'],
        series: [
          { name: 'Beijing', data: [100, 150] },
          { name: 'Shanghai', data: [200, 250] },
        ],
      },
      ['date', 'city'],
      ['revenue']
    );
    expect(option.xAxis.data).toEqual(['2024-01', '2024-02']);
    expect(option.series.map((s) => s.name)).toEqual(['Beijing', 'Shanghai']);
    expect(option.series[1]?.data).toEqual([200, 250]);
  });

  it('多维度多指标：系列名是 "指标 - 维度" 组合，顺序原样保留', () => {
    const option = buildAxisOption(
      {
        x_axis: ['2024-01'],
        series: [
          { name: 'revenue - Beijing', data: [100] },
          { name: 'revenue - Shanghai', data: [200] },
          { name: 'cost - Beijing', data: [50] },
          { name: 'cost - Shanghai', data: [80] },
        ],
      },
      ['date', 'city'],
      ['revenue', 'cost']
    );
    expect(option.series.map((s) => s.name)).toEqual([
      'revenue - Beijing',
      'revenue - Shanghai',
      'cost - Beijing',
      'cost - Shanghai',
    ]);
  });

  it('系列数据不再经行键索引：维度名与系列名不一致也不会丢数据（旧 bug 已消除）', () => {
    // 旧平铺架构：行键是 dims[0]，若 xAxisField 配错（date vs city），
    // 取值全为 undefined。结构化消费下系列数据来自 payload 本身，与维度名无关。
    const option = buildAxisOption(
      { x_axis: ['2024-01', '2024-02'], series: [{ name: 'Beijing', data: [100, 150] }] },
      ['date', 'city'],
      ['revenue']
    );
    expect(option.series[0]?.data).toEqual([100, 150]);
    expect(option.series[0]?.data.every((v) => v !== undefined)).toBe(true);
  });
});
