import { useEffect, useState } from 'react';

/**
 * 移动端断点：与 App.tsx 顶栏折叠、styles/index.css 的 `@media (max-width: 767px)`
 * 同一口径（design-system.md §8.1），不要在这里另立标准。
 */
export const MOBILE_BREAKPOINT = 767;

/**
 * 是否移动端视口（≤767px）。监听 resize 实时更新，供「移动端阅览模式」类分支使用：
 * 仪表盘窄屏降级为单列堆叠并隐藏编辑入口（issue #158）。
 */
export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState(() => window.innerWidth <= MOBILE_BREAKPOINT);

  useEffect(() => {
    const onResize = () => setIsMobile(window.innerWidth <= MOBILE_BREAKPOINT);
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  return isMobile;
}
