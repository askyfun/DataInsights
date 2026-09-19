import type { EChartsOption } from 'echarts';
import { describe, expect, it } from 'vitest';
import { buildChartOption, isEmptyPayload, normalizeChartStyle } from '@/lib/chartOptions';

/**
 * buildChartOption 是共享渲染器的唯一出口（D3 修复）。
 * 本测试文件刻意不 import store / 不 mount React：模块图里没有任何
 * zustand 或 React 依赖，函数可以在纯数据环境下直接调用（验收标准）。
 */

// option 结构的测试视图：先经 unknown 收窄，避免对 ECharts 巨型联合做断言
interface AxisOptionView {
  title: { text: string };
  xAxis: {
    type: string;
    data?: (string | number)[];
    name?: string;
    axisLabel?: { rotate?: number; formatter?: (val: string) => string };
  };
  yAxis: {
    type: string;
    name?: string;
    data?: (string | number)[];
    axisLabel?: { rotate?: number; formatter?: (val: string) => string };
  };
  series: {
    name?: string;
    type: string;
    data: unknown;
    smooth?: boolean;
    connectNulls?: boolean;
    areaStyle?: unknown;
    stack?: string;
  }[];
  color?: string[];
}

interface PieOptionView {
  tooltip: { trigger: string; formatter?: string };
  legend?: { orient: string; left: string };
  color?: string[];
  series: {
    name?: string;
    type: string;
    radius?: string | string[];
    data: { name: string; value: unknown }[];
    label?: { formatter: string };
  }[];
}

function view<T>(option: EChartsOption | null): T {
  expect(option).not.toBeNull();
  return option as unknown as T;
}

const baseStyle = { colors: [], smooth: false, tableRowSize: 'small' as const };

const axisPayload = {
  x_axis: ['Apple', 'Banana'],
  series: [{ name: 'revenue', data: [1000, 2000] }],
};

const piePayload = {
  data: [
    { name: 'Apple', value: 30, percentage: 37.5 },
    { name: 'Banana', value: 50, percentage: 62.5 },
  ],
};

const scatterPayload = {
  data: [
    [100, 50],
    [200, 80],
  ],
};

const histogramPayload = {
  bins: [
    { bin_start: 0, bin_end: 10, count: 3 },
    { bin_start: 10, bin_end: 20, count: 5 },
    { bin_start: 20, bin_end: 30, count: 0 },
  ],
};

const radarPayload = {
  indicators: [
    { name: 'speed', max: 90 },
    { name: 'power', max: 70 },
    { name: 'range', max: 100 },
  ],
  series: [
    { name: 'p1', values: [80, 65, 90] },
    { name: 'p2', values: [50, 70, 30] },
  ],
};

const boxplotPayload = {
  whisker_low: -50,
  q1: 2.75,
  median: 5.5,
  q3: 8.25,
  whisker_high: 100,
  outliers: [-50, 100],
  outlier_total: 2,
  truncated: false,
};

describe('buildChartOption：结构化聚合响应（正常路径）', () => {
  it('bar：x 类目来自 x_axis，series 原样保留名称与数据', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'bar',
        axisPayload,
        baseStyle,
        {},
        {
          title: 'Sales',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(option.title.text).toBe('Sales');
    expect(option.xAxis.type).toBe('category');
    expect(option.xAxis.data).toEqual(['Apple', 'Banana']);
    expect(option.series).toEqual([{ name: 'revenue', type: 'bar', data: [1000, 2000] }]);
  });

  it('line：带 smooth/connectNulls（ChartCanvas 版本细节统一到共享函数）', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'line',
        axisPayload,
        { ...baseStyle, smooth: true },
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(option.series[0]).toMatchObject({
      type: 'line',
      smooth: true,
      connectNulls: true,
      data: [1000, 2000],
    });
  });

  it('area：line 系列 + areaStyle', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'area',
        axisPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(option.series[0]).toMatchObject({ type: 'line', areaStyle: {}, connectNulls: true });
  });

  it('类目轴细节：>10 个类目旋转 30°，formatter 截断 "+0000 UTC" 后缀', () => {
    const many = {
      x_axis: Array.from({ length: 12 }, (_, i) => `2026-01-${String(i + 1)} 00:00:00 +0000 UTC`),
      series: [{ name: 'v', data: Array.from({ length: 12 }, (_, i) => i) }],
    };
    const option = view<AxisOptionView>(
      buildChartOption(
        'bar',
        many,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['date'],
          metrics: ['v'],
        }
      )
    );
    expect(option.xAxis.axisLabel?.rotate).toBe(30);
    expect(option.xAxis.axisLabel?.formatter?.('2026-01-01 00:00:00 +0000 UTC')).toBe(
      '2026-01-01 00:00:00'
    );

    const few = view<AxisOptionView>(
      buildChartOption(
        'bar',
        axisPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(few.xAxis.axisLabel?.rotate).toBe(0);
  });

  it('pie：payload 的 {name,value} 原样进入系列，tooltip/legend/label 走饼图约定', () => {
    const option = view<PieOptionView>(
      buildChartOption(
        'pie',
        piePayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(option.tooltip).toEqual({ trigger: 'item', formatter: '{b}: {c} ({d}%)' });
    expect(option.legend).toEqual({ orient: 'vertical', left: 'left' });
    expect(option.series[0]).toMatchObject({
      type: 'pie',
      radius: '50%',
      data: [
        { name: 'Apple', value: 30 },
        { name: 'Banana', value: 50 },
      ],
      label: { formatter: '{b}: {d}%' },
    });
  });

  it('回归：style.colors 为空时 option 不得携带显式 color 键（echarts 6 会用 undefined 覆盖默认调色板，系列全透明）', () => {
    const option = view<PieOptionView>(
      buildChartOption(
        'pie',
        piePayload,
        baseStyle,
        {},
        { title: '', dimensions: ['product'], metrics: ['revenue'] }
      )
    );
    expect(baseStyle.colors).toEqual([]);
    expect('color' in option).toBe(false);
    // 设置了 colors 时则原样携带
    const withPalette = view<PieOptionView>(
      buildChartOption(
        'pie',
        piePayload,
        { ...baseStyle, colors: ['#111111'] },
        {},
        { title: '', dimensions: ['product'], metrics: ['revenue'] }
      )
    );
    expect(withPalette.color).toEqual(['#111111']);
  });

  it('scatter：二元组数组原样透传，X/Y 轴名取指标显示名', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'scatter',
        scatterPayload,
        baseStyle,
        { revenue: '收入 (元)' },
        {
          title: '',
          dimensions: [],
          metrics: ['city_x', 'revenue'],
        }
      )
    );
    expect(option.xAxis).toMatchObject({ type: 'value', name: 'city_x' });
    expect(option.yAxis).toMatchObject({ type: 'value', name: '收入 (元)' });
    expect(option.series[0]).toEqual({
      type: 'scatter',
      data: [
        [100, 50],
        [200, 80],
      ],
    });
  });

  it('labels：系列名/轴名按 labels 映射，缺失时回落原名', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'bar',
        axisPayload,
        baseStyle,
        { product: '产品', revenue: '收入 (元)' },
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(option.xAxis.name).toBe('产品');
    expect(option.series[0]?.name).toBe('收入 (元)');
  });

  it('style.colors 非空时写入 color 调色板', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'bar',
        axisPayload,
        { ...baseStyle, colors: ['#ff4d4f'] },
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(option.color).toEqual(['#ff4d4f']);
  });
});

describe('buildChartOption：返回 null 的边界', () => {
  it('table/pivot 走 TableChart，不产出 ECharts option', () => {
    const tablePayload = {
      columns: ['region'],
      data: [{ region: 'East' }],
      pagination: { page: 1, page_size: 10, total: 1, total_pages: 1 },
    };
    expect(
      buildChartOption(
        'table',
        tablePayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['region'],
          metrics: [],
        }
      )
    ).toBeNull();
    expect(
      buildChartOption(
        'pivot',
        tablePayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['region'],
          metrics: [],
        }
      )
    ).toBeNull();
  });

  it('kpi 走 KpiCard（AntD Statistic），不产出 ECharts option', () => {
    // 标量 KpiResponse 负载（{value, label}）：无 x_axis/data 键，
    // isEmptyPayload 判空 + switch default 兜底，两重保证恒返回 null。
    expect(
      buildChartOption(
        'kpi',
        { value: 95380, label: 'total_amount' },
        baseStyle,
        {},
        {
          title: '',
          dimensions: [],
          metrics: ['amount'],
        }
      )
    ).toBeNull();
    // 未查询/空负载同样返回 null
    expect(
      buildChartOption('kpi', [], baseStyle, {}, { title: '', dimensions: [], metrics: [] })
    ).toBeNull();
  });

  it('空负载（三种空形状）返回 null', () => {
    expect(
      buildChartOption('bar', [], baseStyle, {}, { title: '', dimensions: ['a'], metrics: ['b'] })
    ).toBeNull();
    expect(
      buildChartOption(
        'bar',
        { x_axis: [], series: [] },
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['a'],
          metrics: ['b'],
        }
      )
    ).toBeNull();
    expect(
      buildChartOption(
        'pie',
        { data: [] },
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['a'],
          metrics: ['b'],
        }
      )
    ).toBeNull();
  });

  it('维度/指标不足返回 null（对应旧 ChartCanvas 空状态判断）', () => {
    // bar 缺指标
    expect(
      buildChartOption(
        'bar',
        axisPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: [],
        }
      )
    ).toBeNull();
    // bar 缺维度
    expect(
      buildChartOption(
        'bar',
        axisPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: [],
          metrics: ['revenue'],
        }
      )
    ).toBeNull();
    // scatter 只有一个指标
    expect(
      buildChartOption(
        'scatter',
        scatterPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: [],
          metrics: ['revenue'],
        }
      )
    ).toBeNull();
  });

  it('负载形状与图型不匹配返回 null（bar 收到 pie 负载）', () => {
    expect(
      buildChartOption(
        'bar',
        piePayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    ).toBeNull();
  });
});

describe('buildChartOption：legacy 裸行回退（Array.isArray 判别）', () => {
  const rows = [
    { product: 'Apple', revenue: 1000 },
    { product: 'Banana', revenue: null },
  ];

  it('bar：按列名索引行，null 值保留为 null', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'bar',
        rows,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(option.xAxis.data).toEqual(['Apple', 'Banana']);
    expect(option.series[0]).toMatchObject({ name: 'revenue', type: 'bar', data: [1000, null] });
  });

  it('scatter：按指标列名取 X/Y', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'scatter',
        [
          { w: 100, h: 50 },
          { w: 200, h: 80 },
        ],
        baseStyle,
        {},
        { title: '', dimensions: [], metrics: ['w', 'h'] }
      )
    );
    expect(option.series[0]?.data).toEqual([
      [100, 50],
      [200, 80],
    ]);
  });

  it('pie：分类取维度列、数值取指标列', () => {
    const option = view<PieOptionView>(
      buildChartOption(
        'pie',
        rows,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['product'],
          metrics: ['revenue'],
        }
      )
    );
    expect(option.series[0]?.data).toEqual([
      { name: 'Apple', value: 1000 },
      { name: 'Banana', value: undefined },
    ]);
  });

  it('缺维度或缺指标返回 null', () => {
    expect(
      buildChartOption(
        'bar',
        rows,
        baseStyle,
        {},
        { title: '', dimensions: [], metrics: ['revenue'] }
      )
    ).toBeNull();
    expect(
      buildChartOption(
        'bar',
        rows,
        baseStyle,
        {},
        { title: '', dimensions: ['product'], metrics: [] }
      )
    ).toBeNull();
  });
});

describe('buildChartOption：纯函数与双路径等价性（plan §8 验收标准）', () => {
  it('同输入两次调用结果 deep-equal，且不修改输入', () => {
    const payloadClone = structuredClone(axisPayload);
    const first = buildChartOption(
      'bar',
      axisPayload,
      baseStyle,
      { revenue: '收入' },
      {
        title: 'T',
        dimensions: ['product'],
        metrics: ['revenue'],
      }
    );
    const second = buildChartOption(
      'bar',
      axisPayload,
      baseStyle,
      { revenue: '收入' },
      {
        title: 'T',
        dimensions: ['product'],
        metrics: ['revenue'],
      }
    );
    expect(first).toEqual(second);
    expect(axisPayload).toEqual(payloadClone);
  });

  it('builder 侧与 share 侧对同一图表算出相同 labels 时产出相同 option', () => {
    // builder 路径：ChartCanvas 由 dimensionLabels/metricAliases/metricUnits
    // 组合出 labels = { 列名 → 显示名 }
    const builderLabels: Record<string, string> = {
      product: '产品',
      revenue: '营收 (元)',
    };
    // share 路径：ShareView 由 chartDoc.fieldMeta 组合出 displayLabels，
    // 键值语义与 builder 完全一致（列名 → 显示名）
    const shareLabels: Record<string, string> = {
      product: '产品',
      revenue: '营收 (元)',
    };
    const context = { title: 'Sales', dimensions: ['product'], metrics: ['revenue'] };

    const fromBuilder = buildChartOption('bar', axisPayload, baseStyle, builderLabels, context);
    const fromShare = buildChartOption('bar', axisPayload, baseStyle, shareLabels, context);
    expect(fromBuilder).not.toBeNull();
    expect(fromBuilder).toEqual(fromShare);
  });
});

describe('isEmptyPayload：结构化联合与裸行两臂统一判空', () => {
  it('各形状的空/非空判定', () => {
    expect(isEmptyPayload([])).toBe(true);
    expect(isEmptyPayload([{ a: 1 }])).toBe(false);
    expect(isEmptyPayload({ x_axis: [], series: [] })).toBe(true);
    expect(isEmptyPayload(axisPayload)).toBe(false);
    expect(isEmptyPayload({ data: [] })).toBe(true);
    expect(isEmptyPayload(piePayload)).toBe(false);
    expect(isEmptyPayload(scatterPayload)).toBe(false);
    // histogram（R-57）：{bins} 形状曾落到末尾 return true → 直方图永远渲染空
    expect(isEmptyPayload({ bins: [] })).toBe(true);
    expect(isEmptyPayload(histogramPayload)).toBe(false);
    // radar（R-62）：{indicators,series} 形状同款问题——缺 'indicators' 臂则落末尾 true
    // → 雷达图永远渲染空（与 histogram 修复的对称断言）。
    expect(isEmptyPayload({ indicators: [], series: [] })).toBe(true);
    expect(isEmptyPayload(radarPayload)).toBe(false);
    // boxplot（R-52）：结构体恒存在、0 是合法分位值 → 命中 'q1' 臂即按可渲染处理（落末尾
    // true 会让箱线图永远空白）；退化零值结构也渲染贴零箱而非空白。
    expect(isEmptyPayload(boxplotPayload)).toBe(false);
    expect(
      isEmptyPayload({
        whisker_low: 0,
        q1: 0,
        median: 0,
        q3: 0,
        whisker_high: 0,
        outliers: [],
        outlier_total: 0,
        truncated: false,
      })
    ).toBe(false);
  });
});

describe('normalizeChartStyle：持久化 style（unknown）安全窄化', () => {
  it('完整合法 shape 原样保留', () => {
    expect(
      normalizeChartStyle({ colors: ['#1f77b4'], smooth: true, tableRowSize: 'middle' })
    ).toEqual({ colors: ['#1f77b4'], smooth: true, tableRowSize: 'middle' });
  });

  it('缺失/非法字段回落默认值', () => {
    expect(normalizeChartStyle(undefined)).toEqual({
      colors: [],
      smooth: false,
      tableRowSize: 'small',
    });
    expect(normalizeChartStyle('nope')).toEqual({
      colors: [],
      smooth: false,
      tableRowSize: 'small',
    });
    expect(normalizeChartStyle({ colors: 'red', smooth: 'yes', tableRowSize: 'huge' })).toEqual({
      colors: [],
      smooth: false,
      tableRowSize: 'small',
    });
    expect(normalizeChartStyle({ colors: ['#fff', 42] })).toEqual({
      colors: ['#fff'],
      smooth: false,
      tableRowSize: 'small',
    });
  });
});

describe('buildChartOption：style.stack 堆叠渲染（bar/line/area）', () => {
  const stackedPayload = {
    x_axis: ['Apple', 'Banana'],
    series: [
      { name: 'revenue', data: [1000, 2000] },
      { name: 'cost', data: [3000, 2000] },
    ],
  };
  const context = { title: '', dimensions: ['product'], metrics: ['revenue', 'cost'] };

  it('stack 缺省（undefined）：series 不带 stack 字段（回归钉死改动前行为）', () => {
    const option = view<AxisOptionView>(
      buildChartOption('bar', stackedPayload, baseStyle, {}, context)
    );
    expect(option.series[0]).not.toHaveProperty('stack');
    expect(option.series[1]).not.toHaveProperty('stack');
    expect(option.series[0]?.data).toEqual([1000, 2000]);
    expect(option.series[1]?.data).toEqual([3000, 2000]);
  });

  it("stack='none'：与缺省一致，不加 stack 字段", () => {
    const option = view<AxisOptionView>(
      buildChartOption('bar', stackedPayload, { ...baseStyle, stack: 'none' }, {}, context)
    );
    expect(option.series[0]).not.toHaveProperty('stack');
    expect(option.series[1]).not.toHaveProperty('stack');
  });

  it("stack='normal'：每条 series 打 stack:'total'，原始数值不变", () => {
    const option = view<AxisOptionView>(
      buildChartOption('bar', stackedPayload, { ...baseStyle, stack: 'normal' }, {}, context)
    );
    expect(option.series[0]).toMatchObject({ stack: 'total', data: [1000, 2000] });
    expect(option.series[1]).toMatchObject({ stack: 'total', data: [3000, 2000] });
  });

  it("stack='percent'：每个 x 位置所有 series 归一化后之和约等于 100（误差 < 0.01）", () => {
    const option = view<AxisOptionView>(
      buildChartOption('bar', stackedPayload, { ...baseStyle, stack: 'percent' }, {}, context)
    );
    expect(option.series[0]).toMatchObject({ stack: 'total' });
    expect(option.series[1]).toMatchObject({ stack: 'total' });

    const data0 = option.series[0]?.data as number[];
    const data1 = option.series[1]?.data as number[];
    // Apple: 1000/(1000+3000)*100=25, 3000/4000*100=75
    expect(data0[0]).toBeCloseTo(25, 2);
    expect(data1[0]).toBeCloseTo(75, 2);
    // Banana: 2000/(2000+2000)*100=50, 50
    expect(data0[1]).toBeCloseTo(50, 2);
    expect(data1[1]).toBeCloseTo(50, 2);

    for (let i = 0; i < stackedPayload.x_axis.length; i++) {
      const sum = data0[i] + data1[i];
      expect(Math.abs(sum - 100)).toBeLessThan(0.01);
    }
  });

  it("stack='percent'：全 0 位置归一化为 0（不产生 NaN/Infinity）", () => {
    const zeroPayload = {
      x_axis: ['Apple'],
      series: [
        { name: 'revenue', data: [0] },
        { name: 'cost', data: [0] },
      ],
    };
    const option = view<AxisOptionView>(
      buildChartOption(
        'bar',
        zeroPayload,
        { ...baseStyle, stack: 'percent' },
        {},
        { title: '', dimensions: ['product'], metrics: ['revenue', 'cost'] }
      )
    );
    expect(option.series[0]?.data).toEqual([0]);
    expect(option.series[1]?.data).toEqual([0]);
  });

  it('line/area 同样支持 stack（不限于 bar）', () => {
    for (const chartType of ['line', 'area'] as const) {
      const option = view<AxisOptionView>(
        buildChartOption(chartType, stackedPayload, { ...baseStyle, stack: 'normal' }, {}, context)
      );
      expect(option.series[0]).toMatchObject({ stack: 'total' });
      expect(option.series[1]).toMatchObject({ stack: 'total' });
    }
  });
});

describe('buildChartOption：style.orientation 横向条形（仅 bar）', () => {
  const context = { title: '', dimensions: ['product'], metrics: ['revenue'] };

  it("orientation 缺省/'vertical'：轴布局不变（回归钉死纵向柱状图）", () => {
    for (const style of [baseStyle, { ...baseStyle, orientation: 'vertical' as const }]) {
      const option = view<AxisOptionView>(buildChartOption('bar', axisPayload, style, {}, context));
      expect(option.xAxis.type).toBe('category');
      expect(option.xAxis.data).toEqual(['Apple', 'Banana']);
      expect(option.yAxis.type).toBe('value');
      expect(option.series[0]).toMatchObject({ type: 'bar', data: [1000, 2000] });
    }
  });

  it("orientation='horizontal'：xAxis/yAxis 类型与类目数据交换，series 数据不变", () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'bar',
        axisPayload,
        { ...baseStyle, orientation: 'horizontal' },
        { product: '产品' },
        context
      )
    );
    expect(option.xAxis.type).toBe('value');
    expect(option.xAxis).not.toHaveProperty('data');
    expect(option.yAxis.type).toBe('category');
    expect(option.yAxis.data).toEqual(['Apple', 'Banana']);
    expect(option.yAxis.name).toBe('产品');
    expect(option.series[0]).toMatchObject({ type: 'bar', data: [1000, 2000] });
  });

  it("orientation='horizontal' + stack='normal' 共存：轴交换且每条 series 仍打 stack:'total'", () => {
    const stackedPayload = {
      x_axis: ['Apple', 'Banana'],
      series: [
        { name: 'revenue', data: [1000, 2000] },
        { name: 'cost', data: [3000, 2000] },
      ],
    };
    const option = view<AxisOptionView>(
      buildChartOption(
        'bar',
        stackedPayload,
        { ...baseStyle, orientation: 'horizontal', stack: 'normal' },
        {},
        { title: '', dimensions: ['product'], metrics: ['revenue', 'cost'] }
      )
    );
    expect(option.xAxis.type).toBe('value');
    expect(option.yAxis.type).toBe('category');
    expect(option.yAxis.data).toEqual(['Apple', 'Banana']);
    expect(option.series[0]).toMatchObject({ stack: 'total', data: [1000, 2000] });
    expect(option.series[1]).toMatchObject({ stack: 'total', data: [3000, 2000] });
  });

  it("orientation='horizontal' 对 line/area 无效（仅 bar 消费，轴不交换）", () => {
    for (const chartType of ['line', 'area'] as const) {
      const option = view<AxisOptionView>(
        buildChartOption(
          chartType,
          axisPayload,
          { ...baseStyle, orientation: 'horizontal' },
          {},
          context
        )
      );
      expect(option.xAxis.type).toBe('category');
      expect(option.xAxis.data).toEqual(['Apple', 'Banana']);
      expect(option.yAxis.type).toBe('value');
    }
  });

  it('legacy 裸行路径同样支持横向交换', () => {
    const rows = [
      { product: 'Apple', revenue: 1000 },
      { product: 'Banana', revenue: 2000 },
    ];
    const option = view<AxisOptionView>(
      buildChartOption('bar', rows, { ...baseStyle, orientation: 'horizontal' }, {}, context)
    );
    expect(option.xAxis.type).toBe('value');
    expect(option.yAxis.type).toBe('category');
    expect(option.yAxis.data).toEqual(['Apple', 'Banana']);
    expect(option.series[0]).toMatchObject({ type: 'bar', data: [1000, 2000] });
  });
});

describe('buildChartOption：style.donut 环形图（仅 pie）', () => {
  const context = { title: '', dimensions: ['product'], metrics: ['revenue'] };

  it("donut=true：series radius 变为 ['40%', '70%']，其余饼图约定不变", () => {
    const option = view<PieOptionView>(
      buildChartOption('pie', piePayload, { ...baseStyle, donut: true }, {}, context)
    );
    expect(option.series[0]).toMatchObject({
      type: 'pie',
      radius: ['40%', '70%'],
      data: [
        { name: 'Apple', value: 30 },
        { name: 'Banana', value: 50 },
      ],
    });
    expect(option.tooltip).toEqual({ trigger: 'item', formatter: '{b}: {c} ({d}%)' });
  });

  it("donut=false/缺省：radius 保持 '50%'（回归钉死实心饼图）", () => {
    for (const style of [baseStyle, { ...baseStyle, donut: false }]) {
      const option = view<PieOptionView>(buildChartOption('pie', piePayload, style, {}, context));
      expect(option.series[0]?.radius).toBe('50%');
    }
  });

  it('legacy 裸行路径同样支持 donut', () => {
    const rows = [{ product: 'Apple', revenue: 1000 }];
    const option = view<PieOptionView>(
      buildChartOption('pie', rows, { ...baseStyle, donut: true }, {}, context)
    );
    expect(option.series[0]?.radius).toEqual(['40%', '70%']);
  });
});

describe('normalizeChartStyle：orientation/donut 安全窄化', () => {
  it('合法值原样保留', () => {
    expect(normalizeChartStyle({ orientation: 'horizontal', donut: true })).toEqual({
      colors: [],
      smooth: false,
      tableRowSize: 'small',
      orientation: 'horizontal',
      donut: true,
    });
    expect(normalizeChartStyle({ orientation: 'vertical' })).toEqual({
      colors: [],
      smooth: false,
      tableRowSize: 'small',
      orientation: 'vertical',
    });
  });

  it('非法/缺失值不带键（分别等价于 vertical/false）', () => {
    expect(normalizeChartStyle({ orientation: 'diagonal', donut: 'yes' })).toEqual({
      colors: [],
      smooth: false,
      tableRowSize: 'small',
    });
    expect(normalizeChartStyle({ donut: false })).toEqual({
      colors: [],
      smooth: false,
      tableRowSize: 'small',
    });
  });
});

// combo 双轴组合图的 option 结构视图：yAxis 是数组（左/右两个值轴），series 带 yAxisIndex
interface ComboOptionView {
  yAxis: { type: string; position?: string }[];
  series: {
    name?: string;
    type: string;
    yAxisIndex?: number;
    connectNulls?: boolean;
    data: unknown;
  }[];
}

describe('buildChartOption：combo 双轴组合图（R-58）', () => {
  // 后端 AxisProcessor 逐指标产出一条 series，name=ResolveAlias()（默认列名）
  const comboPayload = {
    x_axis: ['2024-01', '2024-02'],
    series: [
      { name: 'revenue', data: [1000, 2000] },
      { name: 'growth', data: [0.1, 0.2] },
    ],
  };
  const comboContext = {
    title: 'Combo',
    dimensions: ['month'],
    metrics: ['revenue', 'growth'],
    metricSlots: [
      { slot: 'primary_values', metrics: ['revenue'] },
      { slot: 'secondary_values', metrics: ['growth'] },
    ],
  };

  it('metricSlots 驱动双 Y 轴：primary→yAxisIndex 0（bar），secondary→yAxisIndex 1（line）', () => {
    const option = view<ComboOptionView>(
      buildChartOption('combo', comboPayload, baseStyle, {}, comboContext)
    );
    // yAxis 必须是长度为 2 的数组（combo 与单轴 bar/line/area 的关键差异），次轴在右侧
    expect(Array.isArray(option.yAxis)).toBe(true);
    expect(option.yAxis).toHaveLength(2);
    expect(option.yAxis[1]?.position).toBe('right');
    // 两个 series 分别落到左右轴，且主轴为柱、次轴为线
    expect(option.series[0]).toMatchObject({
      name: 'revenue',
      type: 'bar',
      yAxisIndex: 0,
      data: [1000, 2000],
    });
    expect(option.series[1]).toMatchObject({
      name: 'growth',
      type: 'line',
      yAxisIndex: 1,
      connectNulls: true,
      data: [0.1, 0.2],
    });
  });

  it('labels 生效：series 名按 labels 映射，但 yAxisIndex 仍按原 series.name 反查槽位', () => {
    const option = view<ComboOptionView>(
      buildChartOption(
        'combo',
        comboPayload,
        baseStyle,
        { revenue: '营收', growth: '增长率' },
        comboContext
      )
    );
    expect(option.series[0]).toMatchObject({ name: '营收', type: 'bar', yAxisIndex: 0 });
    expect(option.series[1]).toMatchObject({ name: '增长率', type: 'line', yAxisIndex: 1 });
  });

  it('metricSlots 缺失（防御）：全部退化到 yAxisIndex 0，但仍产出双轴数组、不抛异常', () => {
    const option = view<ComboOptionView>(
      buildChartOption(
        'combo',
        comboPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['month'],
          metrics: ['revenue', 'growth'],
        }
      )
    );
    expect(option.yAxis).toHaveLength(2);
    expect(option.series[0]).toMatchObject({ type: 'bar', yAxisIndex: 0 });
    expect(option.series[1]).toMatchObject({ type: 'bar', yAxisIndex: 0 });
  });

  it('复合 series 名（"别名 - 维度值"）反查不到槽位：退化到 yAxisIndex 0', () => {
    // 历史图表残留第二个维度组时，后端按 dims[1:] 拆系列产出复合名；
    // combo 渲染分支不精确支持该形态，复合名不匹配列名 → 统一落主轴（不崩溃）
    const colorGroupPayload = {
      x_axis: ['2024-01'],
      series: [
        { name: 'revenue - Beijing', data: [100] },
        { name: 'growth - Beijing', data: [0.1] },
      ],
    };
    const option = view<ComboOptionView>(
      buildChartOption('combo', colorGroupPayload, baseStyle, {}, comboContext)
    );
    expect(option.yAxis).toHaveLength(2);
    expect(option.series[0]).toMatchObject({ type: 'bar', yAxisIndex: 0 });
    expect(option.series[1]).toMatchObject({ type: 'bar', yAxisIndex: 0 });
  });

  it('维度/指标不足或负载无 x_axis 时返回 null', () => {
    // 缺维度
    expect(
      buildChartOption(
        'combo',
        comboPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: [],
          metrics: ['revenue', 'growth'],
          metricSlots: comboContext.metricSlots,
        }
      )
    ).toBeNull();
    // 缺指标
    expect(
      buildChartOption(
        'combo',
        comboPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['month'],
          metrics: [],
          metricSlots: comboContext.metricSlots,
        }
      )
    ).toBeNull();
    // 负载形状不匹配（pie 负载无 x_axis）
    expect(buildChartOption('combo', piePayload, baseStyle, {}, comboContext)).toBeNull();
  });
});

describe('buildChartOption：histogram 直方图（R-57）', () => {
  it('bins 渲染为 bar：类目为箱区间、series data 为各箱 count（isEmptyPayload 缺 bins 臂时此断言必失败）', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'histogram',
        histogramPayload,
        baseStyle,
        {},
        {
          title: 'Distribution',
          dimensions: [],
          metrics: ['amount'],
        }
      )
    );
    expect(option.title.text).toBe('Distribution');
    // 直方图为横向条形：类目轴（箱区间）在 y 且 inverse 自上而下，数值轴在 x
    expect(option.xAxis.type).toBe('value');
    expect(option.yAxis).toEqual({
      type: 'category',
      data: ['0 ~ 10', '10 ~ 20', '20 ~ 30'],
      inverse: true,
    });
    expect(option.series).toEqual([{ name: 'amount', type: 'bar', data: [3, 5, 0] }]);
  });

  it('消费 style.colors 调色板', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'histogram',
        histogramPayload,
        { ...baseStyle, colors: ['#ff0000'] },
        {},
        { title: '', dimensions: [], metrics: ['amount'] }
      )
    );
    expect(option.color).toEqual(['#ff0000']);
  });

  it('bins 为空返回 null（渲染空状态而非空图）', () => {
    expect(
      buildChartOption(
        'histogram',
        { bins: [] },
        baseStyle,
        {},
        { title: '', dimensions: [], metrics: ['amount'] }
      )
    ).toBeNull();
  });

  it('context.metrics 缺失时 series 名回退 count', () => {
    const option = view<AxisOptionView>(
      buildChartOption(
        'histogram',
        histogramPayload,
        baseStyle,
        {},
        { title: '', dimensions: [], metrics: [] }
      )
    );
    expect(option.series[0].name).toBe('count');
  });
});

describe('buildChartOption：funnel 漏斗图（R-59）', () => {
  // funnel option 的测试视图：series[0].sort/gap/label.position 是 funnel 与 pie 的关键差异
  interface FunnelOptionView {
    tooltip: { trigger: string; formatter?: string };
    legend?: { orient: string; left: string };
    series: {
      name?: string;
      type: string;
      sort?: string;
      gap?: number;
      label?: { position?: string; formatter?: string };
      data: { name: string; value: unknown }[];
    }[];
    color?: string[];
  }

  const funnelContext = { title: '转化漏斗', dimensions: ['stage'], metrics: ['cnt'] };

  it('PieResponse 负载渲染为 funnel 系列（非 pie、非 null），强制 sort:descending', () => {
    const option = view<FunnelOptionView>(
      buildChartOption('funnel', piePayload, baseStyle, {}, funnelContext)
    );
    expect(option.series[0].type).toBe('funnel');
    expect(option.series[0].sort).toBe('descending');
    expect(option.series[0].gap).toBe(2);
    expect(option.series[0].label).toEqual({ position: 'inside', formatter: '{b}: {c}' });
    expect(option.series[0].name).toBe('cnt');
    // {name,value} 原样映射（percentage 等额外键不带入 series data）
    expect(option.series[0].data).toEqual([
      { name: 'Apple', value: 30 },
      { name: 'Banana', value: 50 },
    ]);
    expect(option.tooltip).toEqual({ trigger: 'item', formatter: '{b}: {c}' });
    expect(option.legend).toEqual({ orient: 'vertical', left: 'left' });
  });

  it('消费 style.colors 调色板', () => {
    const option = view<FunnelOptionView>(
      buildChartOption(
        'funnel',
        piePayload,
        { ...baseStyle, colors: ['#ff0000'] },
        {},
        funnelContext
      )
    );
    expect(option.color).toEqual(['#ff0000']);
  });

  it('指标/维度缺失时返回 null（渲染空状态而非空图）', () => {
    expect(
      buildChartOption(
        'funnel',
        piePayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['stage'],
          metrics: [],
        }
      )
    ).toBeNull();
    expect(
      buildChartOption(
        'funnel',
        piePayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: [],
          metrics: ['cnt'],
        }
      )
    ).toBeNull();
  });
});

describe('buildChartOption：radar 雷达图（R-62）', () => {
  interface RadarOptionView {
    legend?: { data?: string[]; orient?: string; left?: string };
    radar?: {
      indicator: { name: string; max: number }[];
    };
    series: {
      type: string;
      data: { name: string; value: number[] }[];
    }[];
    color?: string[];
  }

  it('RadarResponse 渲染为 ECharts radar：radar.indicator 与 indicators 同序、单条 series.type=radar、series.data 每项 {name,value}', () => {
    const option = view<RadarOptionView>(
      buildChartOption(
        'radar',
        radarPayload,
        baseStyle,
        {},
        {
          title: '',
          dimensions: ['ind'],
          metrics: ['v'],
        }
      )
    );
    // radar.indicator 与后端 indicators 同序同值（轴名 + max 直接透传）
    expect(option.radar?.indicator).toEqual([
      { name: 'speed', max: 90 },
      { name: 'power', max: 70 },
      { name: 'range', max: 100 },
    ]);
    // 单一 ECharts series 承载所有雷达多边形，type='radar'，data 每项 {name, value}
    expect(option.series).toHaveLength(1);
    expect(option.series[0].type).toBe('radar');
    expect(option.series[0].data).toEqual([
      { name: 'p1', value: [80, 65, 90] },
      { name: 'p2', value: [50, 70, 30] },
    ]);
    // legend.data 展示各系列名（多系列场景的图例区分）
    expect(option.legend?.data).toEqual(['p1', 'p2']);
  });

  it('style.colors 生效：palette 透传到 option.color', () => {
    const option = view<RadarOptionView>(
      buildChartOption(
        'radar',
        radarPayload,
        { ...baseStyle, colors: ['#ff0000', '#00ff00'] },
        {},
        { title: '', dimensions: ['ind'], metrics: ['v'] }
      )
    );
    expect(option.color).toEqual(['#ff0000', '#00ff00']);
  });

  it('indicators 为空 → isEmptyPayload 拦截 → 返回 null（若 isEmptyPayload 缺 indicators 臂，本用例通过与否即回归探针）', () => {
    expect(
      buildChartOption(
        'radar',
        { indicators: [], series: [] },
        baseStyle,
        {},
        { title: '', dimensions: ['ind'], metrics: ['v'] }
      )
    ).toBeNull();
  });
});

describe('buildChartOption：boxplot 箱线图（R-52）', () => {
  interface BoxplotOptionView {
    xAxis?: { type?: string };
    yAxis?: { type?: string; data?: string[] };
    series: { type: string; data: unknown[]; name?: string }[];
    color?: string[];
  }

  it('BoxplotResponse 渲染为 boxplot 系列（五数概括 [min,q1,median,q3,max]）+ scatter 离群点系列', () => {
    const option = view<BoxplotOptionView>(
      buildChartOption(
        'boxplot',
        boxplotPayload,
        baseStyle,
        {},
        { title: '', dimensions: [], metrics: ['amount'] }
      )
    );
    // 水平 value 轴承载统计值；类目轴单条箱（名取 metrics[0] 列名）
    expect(option.xAxis?.type).toBe('value');
    expect(option.yAxis?.type).toBe('category');
    expect(option.yAxis?.data).toEqual(['amount']);
    // series[0] = boxplot：数据项即五数概括
    expect(option.series[0].type).toBe('boxplot');
    expect(option.series[0].data).toEqual([[-50, 2.75, 5.5, 8.25, 100]]);
    // series[1] = scatter：离群点 [值, 类目索引0]
    expect(option.series[1].type).toBe('scatter');
    expect(option.series[1].data).toEqual([
      [-50, 0],
      [100, 0],
    ]);
  });

  it('style.colors 生效：palette 透传到 option.color', () => {
    const option = view<BoxplotOptionView>(
      buildChartOption(
        'boxplot',
        boxplotPayload,
        { ...baseStyle, colors: ['#123456'] },
        {},
        { title: '', dimensions: [], metrics: ['amount'] }
      )
    );
    expect(option.color).toEqual(['#123456']);
  });

  it('退化零值结构（后端空数据集）不拦截 → 渲染贴零箱而非 null', () => {
    const option = view<BoxplotOptionView>(
      buildChartOption(
        'boxplot',
        {
          whisker_low: 0,
          q1: 0,
          median: 0,
          q3: 0,
          whisker_high: 0,
          outliers: [],
          outlier_total: 0,
          truncated: false,
        },
        baseStyle,
        {},
        { title: '', dimensions: [], metrics: ['amount'] }
      )
    );
    expect(option.series[0].data).toEqual([[0, 0, 0, 0, 0]]);
    expect(option.series[1].data).toEqual([]);
  });
});
