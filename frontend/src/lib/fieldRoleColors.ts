/**
 * 字段角色的语义色单一来源（#265）。
 *
 * 全站约定：**维度蓝（--dr-dim）/ 指标绿（--dr-metric）**，深色主题由同名 CSS 变量
 * 的深色变体承载（见 styles/index.css）。所有页面（数据集创建/详情/编辑、上传审阅、
 * 图表构建页）统一调用本函数，禁止再在页面里内联硬编码 `--dr-*` 或 antd 预设色名，
 * 否则极易出现「同一行两种颜色」「颜色反转」这类漂移。
 */
export type FieldRole = 'dimension' | 'metric';

/** 返回角色对应的 CSS 变量色；非 metric 一律按维度处理（蓝）。 */
export function roleColor(role: string | null | undefined): string {
  return role === 'metric' ? 'var(--dr-metric)' : 'var(--dr-dim)';
}
