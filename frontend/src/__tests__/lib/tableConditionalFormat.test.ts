import { describe, expect, it } from 'vitest';
import {
  type ConditionalFormat,
  hexToRgb,
  interpolateColor,
  resolveCellBackground,
  thresholdHit,
} from '../../lib/tableConditionalFormat';

describe('hexToRgb', () => {
  it('解析 6 位 hex', () => {
    expect(hexToRgb('#cf1322')).toEqual([207, 19, 34]);
  });

  it('解析 3 位 hex', () => {
    expect(hexToRgb('#f00')).toEqual([255, 0, 0]);
  });

  it('非法输入返回 null', () => {
    expect(hexToRgb('not-a-color')).toBeNull();
  });
});

describe('interpolateColor', () => {
  it('t=0 取起点色，t=1 取终点色', () => {
    expect(interpolateColor('#000000', '#ffffff', 0)).toBe('#000000');
    expect(interpolateColor('#000000', '#ffffff', 1)).toBe('#ffffff');
  });

  it('t=0.5 是中点色', () => {
    expect(interpolateColor('#000000', '#ffffff', 0.5)).toBe('#808080');
  });

  it('t 越界钳到 [0,1]', () => {
    expect(interpolateColor('#000000', '#ffffff', -1)).toBe('#000000');
    expect(interpolateColor('#000000', '#ffffff', 2)).toBe('#ffffff');
  });
});

describe('thresholdHit', () => {
  const cases: Array<[number, NonNullable<ConditionalFormat['op']>, number, boolean]> = [
    [5, '>', 3, true],
    [2, '>', 3, false],
    [3, '>=', 3, true],
    [3, '<=', 3, true],
    [4, '<=', 3, false],
    [3, '<', 3, false],
    [3, '=', 3, true],
    [4, '=', 3, false],
  ];
  for (const [value, op, threshold, want] of cases) {
    it(`${value} ${op} ${threshold} → ${want}`, () => {
      expect(thresholdHit(value, op, threshold)).toBe(want);
    });
  }
});

describe('resolveCellBackground', () => {
  it('threshold 命中给背景色，未命中 undefined', () => {
    const rule: ConditionalFormat = {
      metric: 'amount',
      kind: 'threshold',
      op: '>',
      value: 100,
      color: '#fa8c16',
    };
    expect(resolveCellBackground(rule, 150, [150, 50])).toBe('#fa8c16');
    expect(resolveCellBackground(rule, 50, [150, 50])).toBeUndefined();
  });

  it('threshold 缺 op/value 时无法判定，返回 undefined', () => {
    const rule: ConditionalFormat = { metric: 'amount', kind: 'threshold' };
    expect(resolveCellBackground(rule, 150, [150])).toBeUndefined();
  });

  it('scale 按列内数值 min/max 归一插值', () => {
    const rule: ConditionalFormat = {
      metric: 'amount',
      kind: 'scale',
      minColor: '#000000',
      maxColor: '#ffffff',
    };
    // 列值 [0, 100]，50 → t=0.5 → #808080
    expect(resolveCellBackground(rule, 50, [0, 50, 100])).toBe('#808080');
    // 单值列：min==max，t=0
    expect(resolveCellBackground(rule, 7, [7])).toBe('#000000');
  });

  it('scale 非数值单元格返回 undefined', () => {
    const rule: ConditionalFormat = { metric: 'amount', kind: 'scale' };
    expect(resolveCellBackground(rule, 'abc', ['abc'])).toBeUndefined();
    expect(resolveCellBackground(rule, null, [null])).toBeUndefined();
  });

  it('diff 默认涨红跌绿（国内惯例），0 不着色', () => {
    const rule: ConditionalFormat = { metric: 'delta', kind: 'diff' };
    expect(resolveCellBackground(rule, 10, [10])).toBe('#cf1322');
    expect(resolveCellBackground(rule, -1, [-1])).toBe('#389e0d');
    expect(resolveCellBackground(rule, 0, [0])).toBeUndefined();
  });

  it('diff 可反转（涨绿跌红）', () => {
    const rule: ConditionalFormat = {
      metric: 'delta',
      kind: 'diff',
      upColor: '#389e0d',
      downColor: '#cf1322',
    };
    expect(resolveCellBackground(rule, 10, [10])).toBe('#389e0d');
    expect(resolveCellBackground(rule, -1, [-1])).toBe('#cf1322');
  });

  it('缺省色板：scale 白→蓝', () => {
    const rule: ConditionalFormat = { metric: 'v', kind: 'scale' };
    expect(resolveCellBackground(rule, 0, [0, 10])).toBe('#ffffff');
    expect(resolveCellBackground(rule, 10, [0, 10])).toBe('#1677ff');
  });

  it('metric 不匹配返回 undefined（规则只作用自己的列）', () => {
    const rule: ConditionalFormat = { metric: 'a', kind: 'diff' };
    expect(resolveCellBackground(rule, 10, [10], 'b')).toBeUndefined();
  });
});
