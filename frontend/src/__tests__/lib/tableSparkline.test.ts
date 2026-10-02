import { describe, expect, it } from 'vitest';
import {
  buildSparklineSeries,
  detectDateDimension,
  rowGroupKey,
  sparklinePoints,
} from '@/lib/tableSparkline';

/**
 * 迷你图列（issue #156 AC4）的纯逻辑契约：表格查询只回扁平行，行内 sparkline 的
 * 序列由**前端在渲染层分组**得到 —— 按「非日期维度」分组、组内按日期列升序取指标值。
 * 日期维度列由数据形状探测（YYYY-MM-DD 前缀），免传列类型元数据；探测不到就不启用。
 */
const rows = [
  { city: '北京', day: '2026-09-01', temp: 20 },
  { city: '北京', day: '2026-09-02', temp: 22 },
  { city: '上海', day: '2026-09-01', temp: 25 },
  { city: '上海', day: '2026-09-02', temp: null },
];

describe('detectDateDimension', () => {
  it('按维度顺序返回第一个日期形状列', () => {
    expect(detectDateDimension(rows, ['city', 'day'])).toBe('day');
  });

  it('非 YYYY-MM-DD 前缀的取值不算日期维度', () => {
    expect(detectDateDimension([{ d: '09/01' }], ['d'])).toBeNull();
    expect(detectDateDimension([{ d: '2026-13-40' }], ['d'])).toBeNull();
  });

  it('样本中混入非日期值即不命中（宁可不用也不猜错）', () => {
    const mixed = [{ d: '2026-09-01' }, { d: '华东' }];
    expect(detectDateDimension(mixed, ['d'])).toBeNull();
  });

  it('没有维度列时返回 null', () => {
    expect(detectDateDimension(rows, [])).toBeNull();
  });
});

describe('buildSparklineSeries / rowGroupKey', () => {
  it('按非日期维度分组、组内按日期升序，空值/非数值点被剔除', () => {
    const series = buildSparklineSeries(rows, ['city'], 'day', 'temp');
    expect([...series.keys()]).toEqual(['北京', '上海']);
    expect(series.get('北京')).toEqual([20, 22]);
    expect(series.get('上海')).toEqual([25]); // null 点剔除
  });

  it('多维度时分组键是各维度取值的组合', () => {
    const r = [
      { a: 'x', b: '1', day: '2026-09-01', v: 1 },
      { a: 'x', b: '2', day: '2026-09-01', v: 2 },
    ];
    const series = buildSparklineSeries(r, ['a', 'b'], 'day', 'v');
    expect(series.size).toBe(2);
    expect(rowGroupKey(r[0], ['a', 'b'])).toBe(rowGroupKey(r[0], ['a', 'b']));
  });
});

describe('sparklinePoints', () => {
  it('归一为 viewBox 内的 polyline 点串，留边距', () => {
    const pts = sparklinePoints([0, 10], 80, 24);
    const nums = pts.split(' ').flatMap((p) => p.split(',').map(Number));
    expect(nums.every((n) => Number.isFinite(n))).toBe(true);
    expect(Math.min(...nums.filter((_, i) => i % 2 === 0))).toBeGreaterThanOrEqual(1);
    expect(Math.max(...nums.filter((_, i) => i % 2 === 0))).toBeLessThanOrEqual(79);
  });

  it('少于 2 点或全等值时返回空串（调用方不画线）', () => {
    expect(sparklinePoints([5], 80, 24)).toBe('');
    expect(sparklinePoints([], 80, 24)).toBe('');
    // 全等值画水平中线，不是空串
    expect(sparklinePoints([7, 7, 7], 80, 24)).not.toBe('');
  });
});
