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

/** 占比格式后缀：格式串以 `%` 结尾表示该指标按「占全集比例」显示（issue #132）。 */
export const PERCENT_FORMAT_SUFFIX = '%';

/** 该格式串是否为占比显示。 */
export function isPercentOfTotalFormat(format?: string): boolean {
  return format?.endsWith(PERCENT_FORMAT_SUFFIX) ?? false;
}

/**
 * 占比开关与「格式」串之间的读写口（字段属性弹窗用）。
 *
 * 占比不是独立的一份配置，而是格式串的一个后缀：`0,0.00%`。这样它随 fieldMeta
 * 一起持久化、撤销/重做与分享链接都天然覆盖到，不必再新增一份按 bindingId 索引的
 * Record（那要同时改 store、历史快照、文档 schema、迁移与后端归一共六处）。
 */
export function splitPercentFormat(format: string): { base: string; percent: boolean } {
  return isPercentOfTotalFormat(format)
    ? { base: format.slice(0, -PERCENT_FORMAT_SUFFIX.length), percent: true }
    : { base: format, percent: false };
}

/** 按占比开关重写格式串：打开时补 `%` 后缀（无格式则给一个两位小数的默认），关闭时剥掉。 */
export function applyPercentFormat(format: string, percent: boolean): string {
  const { base } = splitPercentFormat(format);
  if (!percent) {
    return base;
  }
  return `${base.trim() === '' ? '0,0.00' : base}${PERCENT_FORMAT_SUFFIX}`;
}

/**
 * 指标「占比」：value / grandTotal × 100，按格式串的小数位定精度，**不含** `%` 符号
 * （符号由调用方作为独立文本节点拼接，见 TableChart）。
 *
 * 分母必须是**过滤后完整数据集**的合计（后端重算），不是当前页明细相加——后者在
 * 服务端分页下会让同一行在不同页显示不同占比。分母缺失/为 0/非数值时返回空串：
 * 占比算不出来就留空，绝不回落成"用本页合计当分母"那种看起来合理的错数。
 */
export function formatPercentOfTotal(value: unknown, grandTotal: unknown, format: string): string {
  if (value == null || value === '') {
    return '';
  }
  const num = Number(value);
  const total = Number(grandTotal);
  if (!Number.isFinite(num) || !Number.isFinite(total) || total === 0) {
    return '';
  }
  return formatMetricValue(
    (num / total) * 100,
    format.slice(0, -PERCENT_FORMAT_SUFFIX.length).trim()
  );
}
