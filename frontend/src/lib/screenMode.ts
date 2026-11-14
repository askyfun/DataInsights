/**
 * 仪表盘「数字大屏」播放模式（issue #159）——纯逻辑，零 UI 依赖。
 *
 * 与 dateFilter.ts 同性质的「语义单一事实源」：等比缩放、轮播间隔、刷新间隔的
 * 解析规则都集中在这里，组件只消费结果。这样缩放/定时逻辑可以被单测锁住，
 * 也避免把「间隔字符串 → 毫秒」这种解析散落到播放器组件里。
 *
 * 设计画布口径：固定 1920×1080（16:9 投屏主分辨率）。等比缩放按「设计宽高比 vs
 * 实际视口」取 fit 缩放系数（letterbox 黑边），不裁切不拉伸——对齐 DataWind
 * 数字大屏「等比缩放」口径（AC-1）。
 *
 * URL 参数约定（`/dashboards/:id/screen`）：
 *   - `?carousel=<秒>`：按页轮播间隔，0 或省略 = 不轮播（AC-2）。
 *   - `?refresh=<秒>`：数据自动刷新间隔，0 或省略 = 不自动刷新（AC-2）。
 * 两参数都是独立可配，互不依赖。
 */

/** 大屏设计画布宽高比（16:9）。 */
export const SCREEN_DESIGN_WIDTH = 1920;
export const SCREEN_DESIGN_HEIGHT = 1080;
export const SCREEN_DESIGN_ASPECT = SCREEN_DESIGN_WIDTH / SCREEN_DESIGN_HEIGHT;

/** 解析后的缩放结果（fit 等比缩放）。 */
export interface ScreenScale {
  /** 等比缩放系数（视口宽/高哪个更紧就取哪个）。 */
  scale: number;
  /** 缩放后画布实际宽度（px）。 */
  width: number;
  /** 缩放后画布实际高度（px）。 */
  height: number;
  /** 左右 letterbox 偏移（px），画布居中用。 */
  offsetX: number;
  /** 上下 letterbox 偏移（px）。 */
  offsetY: number;
}

/**
 * 计算 fit 等比缩放：把 1920×1080 画布缩放到 `viewportW × viewportH` 内，
 * 保持 16:9 不变形、完整可见（含黑边），返回缩放系数与居中偏移。
 *
 * 视口为 0（首帧未测量）时返回 scale=0，调用方应以「未就绪」处理。
 */
export function fitScreenScale(viewportW: number, viewportH: number): ScreenScale {
  if (viewportW <= 0 || viewportH <= 0) {
    return { scale: 0, width: 0, height: 0, offsetX: 0, offsetY: 0 };
  }
  const scale = Math.min(viewportW / SCREEN_DESIGN_WIDTH, viewportH / SCREEN_DESIGN_HEIGHT);
  const width = SCREEN_DESIGN_WIDTH * scale;
  const height = SCREEN_DESIGN_HEIGHT * scale;
  return {
    scale,
    width,
    height,
    offsetX: (viewportW - width) / 2,
    offsetY: (viewportH - height) / 2,
  };
}

/** 轮播间隔解析：字符串秒 → 毫秒；非法/≤0 → 不轮播。 */
export function parseIntervalSeconds(raw: string | null | undefined): number {
  if (raw == null) {
    return 0;
  }
  const value = Number(raw);
  if (!Number.isFinite(value) || value <= 0) {
    return 0;
  }
  return Math.round(value * 1000);
}

/**
 * URL 参数 → 播放配置。
 * `carousel` 为轮播间隔秒（0=关），`refresh` 为数据刷新间隔秒（0=关）。
 * 两者默认都为 0（不轮播、不自动刷新）——大屏首屏是静态的，用户显式开启。
 */
export interface ScreenPlaybackConfig {
  /** 按页轮播间隔（ms）；0 = 不轮播。 */
  carouselIntervalMs: number;
  /** 数据自动刷新间隔（ms）；0 = 不自动刷新。 */
  refreshIntervalMs: number;
}

export function parsePlaybackConfig(search: string): ScreenPlaybackConfig {
  const params = new URLSearchParams(search);
  return {
    carouselIntervalMs: parseIntervalSeconds(params.get('carousel')),
    refreshIntervalMs: parseIntervalSeconds(params.get('refresh')),
  };
}
