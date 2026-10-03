import { useCallback, useEffect, useState } from 'react';

/**
 * 「减少动态效果」（无障碍）单一事实源。
 *
 * Web Interface Guidelines 要求尊重系统开关 `prefers-reduced-motion: reduce`。
 * 走 CSS 的动效（antd 弹窗/抽屉过渡、dnd-kit 排序位移、本仓自定义过渡）由
 * styles/index.css 的全局媒体查询统一压平，无需逐个组件处理；但有两处动效
 * **不受 CSS 约束**，只能在 JS 里显式关闭，都由本模块的 hook 驱动：
 *   - ECharts 渲染在 canvas 上，动画是 option 字段（见 buildChartOption 的 reducedMotion）；
 *   - dnd-kit 的 DragOverlay 回落动画走 Web Animations API（element.animate），
 *     CSS 的 !important 对它无效（见 ChartBuilder 的 dropAnimation）。
 * 两处共用这一个 hook，避免 matchMedia 判断散落到各组件。
 */

const QUERY = '(prefers-reduced-motion: reduce)';

/** 系统当前是否要求减少动态效果（非 React 环境安全回落 false）。 */
export function systemPrefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && window.matchMedia(QUERY).matches;
}

/**
 * React 订阅：返回系统是否要求减少动态效果，开关变化时重渲染。
 * 订阅语义与 theme.ts 的 initTheme 一致（Safari 13 及以下只有已废弃的 addListener）。
 */
export function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState<boolean>(systemPrefersReducedMotion);

  const onChange = useCallback((event: MediaQueryListEvent): void => {
    setReduced(event.matches);
  }, []);

  useEffect(() => {
    const mq = window.matchMedia(QUERY);
    // 挂载与系统切换之间可能已变过：先同步一次再订阅后续变化。
    setReduced(mq.matches);
    if (typeof mq.addEventListener === 'function') {
      mq.addEventListener('change', onChange);
      return () => mq.removeEventListener('change', onChange);
    }
    // Safari 13 及以下只有已废弃的 addListener。
    mq.addListener(onChange);
    return () => mq.removeListener(onChange);
  }, [onChange]);

  return reduced;
}
