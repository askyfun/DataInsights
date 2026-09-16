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
    data: (string | number)[];
    name?: string;
    axisLabel?: { rotate?: number; formatter?: (val: string) => string };
  };
  yAxis: { type: string; name?: string };
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
  series: {
    name?: string;
    type: string;
    radius?: string;
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
