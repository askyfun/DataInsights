import { describe, expect, it } from 'vitest';
import { formatMetricValue } from '../../lib/format';

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
