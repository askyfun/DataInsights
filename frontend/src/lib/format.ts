/** 时间戳 → 本地时间字符串；空值/非法日期返回 '-'。 */
export function formatDateTime(text?: string | null): string {
  if (!text) return '-';
  const d = new Date(text);
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString();
}
