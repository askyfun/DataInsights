// 图表导出（issue #120）的**纯函数**部分：文件名与落盘触发。
// 取数/编码在后端（POST /api/charts/export），这里只管「叫什么、怎么存」。

/** 单次导出的数据行数上限（与后端 maxExportLimit 之内的默认值一致）。 */
export const EXPORT_ROW_LIMIT = 10000;

export type ExportFormat = 'csv' | 'xlsx' | 'png';

/**
 * 文件名 = 图表名 + 本地时间戳 + 扩展名（issue #120 验收 3）。
 * 图表名里可能带路径分隔符或引号（用户随手起的名字），统一剔除而不是替换成
 * 别的字符后再拼 —— 时间戳保证同名图表多次导出不会互相覆盖。
 */
export function buildExportFilename(name: string, format: ExportFormat, now: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  const stamp =
    `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}` +
    `-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  const safe = name
    .trim()
    // biome-ignore lint/suspicious/noControlCharactersInRegex: 文件名里必须剔除控制字符
    .replace(/[\\/:*?"<>|\u0000-\u001f]/g, '')
    .replace(/\s+/g, '_');
  return `${safe || 'chart'}-${stamp}.${format}`;
}

/** 触发浏览器下载一个 Blob。URL 用完即撤销，避免长时间持有一份导出数据。 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  // 立即 revoke 在部分浏览器会打断下载，放到下一个宏任务。
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

/** 触发下载一个 data URL（ECharts getDataURL 的 PNG 产物）。 */
export function downloadDataURL(dataURL: string, filename: string): void {
  const a = document.createElement('a');
  a.href = dataURL;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
}
