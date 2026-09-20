/** 时间戳 → 本地时间字符串；空值/非法日期返回 '-'。 */
export function formatDateTime(text?: string | null): string {
  if (!text) return '-';
  const d = new Date(text);
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString();
}

/**
 * 按指标属性里的「格式」字符串格式化数值（如 0,0.00 / 0.00 / 0）。
 * 语义对齐常见 numeral 风格写法：逗号 = 千分位，小数点后位数 = 小数位数。
 * 空格式/非数值原样返回字符串，不做静默改写。
 */
export function formatMetricValue(value: unknown, format?: string): string {
  if (value == null || value === '') {
    return value == null ? '' : String(value);
  }
  if (!format) {
    return String(value);
  }
  const num = Number(value);
  if (!Number.isFinite(num)) {
    return String(value);
  }
  const dotIndex = format.indexOf('.');
  const decimals = dotIndex >= 0 ? format.length - dotIndex - 1 : 0;
  return num.toLocaleString('en-US', {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
    useGrouping: format.includes(','),
  });
}
