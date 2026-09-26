import { useCallback, useEffect, useState } from 'react';

/**
 * 主题（皮肤）单一事实源：浅色 / 深色 / 跟随系统。
 *
 * 与 useLocale 的「localStorage 持久化 + 读初值」同构，但主题要跨组件共享
 * （main.tsx 的 antd ConfigProvider、App.tsx 的切换控件、ChartView 取色都要看同一份），
 * 所以状态放在模块级 store，用订阅而非每组件各持 useState。
 *
 * 解析后的主题以 `data-theme` 属性镜像到 <html>：CSS 变量层（styles/index.css 的
 * [data-theme='dark']）与 ECharts canvas 取色都读这一个属性，保证「同一时刻只有一处真相」。
 */

/** 用户偏好：'system' 表示跟随系统深浅色。 */
export type ThemeMode = 'light' | 'dark' | 'system';
/** 实际渲染出来的主题（'system' 解析后即 light/dark）。 */
export type ResolvedTheme = 'light' | 'dark';

const PREF_KEY = 'theme';
const THEME_ATTR = 'data-theme';
const DARK_QUERY = '(prefers-color-scheme: dark)';

export function loadThemePreference(): ThemeMode {
  const stored = localStorage.getItem(PREF_KEY);
  return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system';
}

export function systemPrefersDark(): boolean {
  return typeof window !== 'undefined' && window.matchMedia(DARK_QUERY).matches;
}

/** 偏好 → 实际主题；'system' 落到系统深浅色。 */
export function resolveTheme(preference: ThemeMode): ResolvedTheme {
  return preference === 'system' ? (systemPrefersDark() ? 'dark' : 'light') : preference;
}

let preference: ThemeMode = 'system';
let resolved: ResolvedTheme = 'light';
const listeners = new Set<() => void>();

function emit(): void {
  for (const l of listeners) l();
}

/** 写 <html data-theme>，供 CSS 变量与 canvas 取色消费（必须在读 getComputedStyle 前调用）。 */
function applyToDom(): void {
  document.documentElement.setAttribute(THEME_ATTR, resolved);
}

/** 偏好/系统变化后重算解析主题，必要时通知订阅者。 */
function recompute(): void {
  const next = resolveTheme(preference);
  if (next !== resolved) {
    resolved = next;
    applyToDom();
    emit();
  }
}

/**
 * 一次性接线：装载偏好、应用初值、监听系统深浅色。
 * 在 main.tsx render 前调用，避免首帧闪错主题。
 */
export function initTheme(): void {
  preference = loadThemePreference();
  resolved = resolveTheme(preference);
  applyToDom();

  const mq = window.matchMedia(DARK_QUERY);
  const onSystemChange = (): void => recompute();
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onSystemChange);
  } else {
    // Safari 13 及以下只有已废弃的 addListener；matchMedia 语义要求监听器只挂一次。
    mq.addListener(onSystemChange);
  }
}

export function getResolvedTheme(): ResolvedTheme {
  return resolved;
}

export function getThemePreference(): ThemeMode {
  return preference;
}

/** 设置并持久化偏好；解析主题变化时通知订阅者。 */
export function setThemeMode(next: ThemeMode): void {
  preference = next;
  localStorage.setItem(PREF_KEY, next);
  recompute();
}

/** 供非 React 消费者（如命令式取色）订阅解析主题变化。 */
export function subscribeTheme(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/**
 * React 订阅：返回解析主题，变化时重渲染。
 * 组件用它把主题纳入 useMemo 依赖，从而在切换深浅色时重建 ECharts option 的取色。
 */
export function useResolvedTheme(): ResolvedTheme {
  const [value, setValue] = useState<ResolvedTheme>(resolved);
  const onChange = useCallback(() => setValue(resolved), []);
  useEffect(() => subscribeTheme(onChange), [onChange]);
  return value;
}

/**
 * React 订阅：返回 { 偏好, 解析主题, setMode }，供切换控件使用。
 * 控件要按「用户选的那一项」高亮（选 system 时即便渲染成 dark 也应显示 system），
 * 故偏好与解析值都要暴露。
 */
export function useTheme(): {
  preference: ThemeMode;
  resolved: ResolvedTheme;
  setMode: (mode: ThemeMode) => void;
} {
  const [pref, setPref] = useState<ThemeMode>(preference);
  const value = useResolvedTheme();
  const onChange = useCallback(() => setPref(preference), []);
  useEffect(() => subscribeTheme(onChange), [onChange]);
  const setMode = useCallback((mode: ThemeMode) => {
    setThemeMode(mode);
    setPref(mode);
  }, []);
  return { preference: pref, resolved: value, setMode };
}
