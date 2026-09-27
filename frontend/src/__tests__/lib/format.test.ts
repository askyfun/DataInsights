import { describe, expect, it } from 'vitest';
import {
  applyPercentFormat,
  formatMetricValue,
  formatPercentOfTotal,
  isPercentOfTotalFormat,
  splitPercentFormat,
} from '../../lib/format';

/**
 * 指标「格式」格式化器：逗号 = 千分位，小数点后位数 = 小数位数；
 * 空格式/非数值原样返回（不做静默改写）。
 */
describe('formatMetricValue', () => {
  it('0,0.00：千分位 + 两位小数', () => {
    expect(formatMetricValue(1003345, '0,0.00')).toBe('1,003,345.00');
    expect(formatMetricValue('1234.5', '0,0.00')).toBe('1,234.50');
  });

  it('0：整数无小数位', () => {
    expect(formatMetricValue(1234.6, '0')).toBe('1235');
  });

  it('0.00：无千分位两位小数', () => {
    expect(formatMetricValue(1234.5, '0.00')).toBe('1234.50');
  });

  it('空格式原样返回', () => {
    expect(formatMetricValue(1234.5, '')).toBe('1234.5');
    expect(formatMetricValue(1234.5)).toBe('1234.5');
  });

  it('非数值/空值原样返回', () => {
    expect(formatMetricValue('abc', '0,0.00')).toBe('abc');
    expect(formatMetricValue(null, '0,0.00')).toBe('');
    expect(formatMetricValue(undefined, '0,0.00')).toBe('');
  });
});

/**
 * 指标占比（issue #132）：格式串以 `%` 结尾即按占比显示，分母是后端重算的全集合计。
 */
describe('formatPercentOfTotal / 占比格式', () => {
  it('按格式串的小数位定精度，分母来自全集合计', () => {
    expect(formatPercentOfTotal(30, 120, '0,0.00%')).toBe('25.00');
    expect(formatPercentOfTotal(30, 120, '0%')).toBe('25');
    expect(formatPercentOfTotal(1, 3, '0,0.00%')).toBe('33.33');
  });

  it('`%` 后缀不得被当成小数位数的一部分（否则会出现 25.000）', () => {
    expect(formatPercentOfTotal(50, 100, '0,0.00%')).toBe('50.00');
    expect(formatMetricValue(50, '0,0.00%')).not.toBe('50.00');
  });

  it('分母缺失 / 为 0 / 非数值时留空，不回落成"本页合计当分母"的错数', () => {
    expect(formatPercentOfTotal(30, undefined, '0,0.00%')).toBe('');
    expect(formatPercentOfTotal(30, 0, '0,0.00%')).toBe('');
    expect(formatPercentOfTotal(30, 'abc', '0,0.00%')).toBe('');
    expect(formatPercentOfTotal(null, 100, '0,0.00%')).toBe('');
  });

  it('开关与格式串互转：打开补后缀（空格式给默认两位小数），关闭剥后缀', () => {
    expect(applyPercentFormat('0,0', true)).toBe('0,0%');
    expect(applyPercentFormat('', true)).toBe('0,0.00%');
    expect(applyPercentFormat('0,0.00%', false)).toBe('0,0.00');
    expect(splitPercentFormat('0,0.00%')).toEqual({ base: '0,0.00', percent: true });
    expect(splitPercentFormat('0,0.00')).toEqual({ base: '0,0.00', percent: false });
    expect(isPercentOfTotalFormat('0,0.00%')).toBe(true);
    expect(isPercentOfTotalFormat(undefined)).toBe(false);
  });
});
