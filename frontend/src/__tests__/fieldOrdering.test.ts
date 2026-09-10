import { beforeEach, describe, expect, it } from 'vitest';
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

describe('AxisResponse 多维度转换', () => {
  /**
   * 模拟 store 的 executeChartQuery 对 line/bar/area 的转换逻辑。
   * 用 request.dims[0] 作为行数据的 key。
   */
  function transformAxisResponse(
    axisData: { x_axis: string[]; series: Array<{ name: string; data: unknown[] }> },
    dimName: string
  ): Record<string, unknown>[] {
    return axisData.x_axis.map((xVal, idx) => {
      const row: Record<string, unknown> = { [dimName]: xVal };
      for (const series of axisData.series) {
        row[series.name] = series.data[idx];
      }
      return row;
    });
  }

  it('单维度：key 是维度名，series 是指标名', () => {
    const axisData = {
      x_axis: ['A', 'B'],
      series: [{ name: 'revenue', data: [100, 200] }],
    };
    const result = transformAxisResponse(axisData, 'category');
    expect(result).toEqual([
      { category: 'A', revenue: 100 },
      { category: 'B', revenue: 200 },
    ]);
  });

  it('多维度：key 是第一个维度名，series 是第二个维度值', () => {
    const axisData = {
      x_axis: ['2024-01', '2024-02'],
      series: [
        { name: 'Beijing', data: [100, 150] },
        { name: 'Shanghai', data: [200, 250] },
      ],
    };
    const result = transformAxisResponse(axisData, 'date');
    expect(result).toEqual([
      { date: '2024-01', Beijing: 100, Shanghai: 200 },
      { date: '2024-02', Beijing: 150, Shanghai: 250 },
    ]);
    // 确认 key 是 "date"（第一个维度），不是 "city"（第二个维度）
    expect(result[0]).toHaveProperty('date');
    expect(result[0]).not.toHaveProperty('city');
  });

  it('多维度多指标：key 是第一个维度名，series 是 "指标-维度" 组合', () => {
    const axisData = {
      x_axis: ['2024-01'],
      series: [
        { name: 'revenue - Beijing', data: [100] },
        { name: 'revenue - Shanghai', data: [200] },
        { name: 'cost - Beijing', data: [50] },
        { name: 'cost - Shanghai', data: [80] },
      ],
    };
    const result = transformAxisResponse(axisData, 'date');
    expect(result[0]).toEqual({
      date: '2024-01',
      'revenue - Beijing': 100,
      'revenue - Shanghai': 200,
      'cost - Beijing': 50,
      'cost - Shanghai': 80,
    });
  });

  it('series 字段名和 xAxisField 不同时，xAxisData 正确提取', () => {
    const axisData = {
      x_axis: ['2024-01', '2024-02'],
      series: [
        { name: 'Beijing', data: [100, 150] },
        { name: 'Shanghai', data: [200, 250] },
      ],
    };
    const data = transformAxisResponse(axisData, 'date');
    const xAxisField = 'date';
    const xAxisData = data.map((item) => item[xAxisField]);

    expect(xAxisData).toEqual(['2024-01', '2024-02']);
    // 确保不会返回 undefined
    expect(xAxisData.every((v) => v !== undefined)).toBe(true);
  });

  it('xAxisField 错误时返回 undefined — 这就是之前的 bug', () => {
    const axisData = {
      x_axis: ['2024-01', '2024-02'],
      series: [{ name: 'Beijing', data: [100, 150] }],
    };
    const data = transformAxisResponse(axisData, 'date');

    // 错误的 xAxisField（应该是 "date" 但用了 "city"）
    const wrongXAxisField = 'city';
    const xAxisData = data.map((item) => item[wrongXAxisField]);

    expect(xAxisData).toEqual([undefined, undefined]);
  });
});
