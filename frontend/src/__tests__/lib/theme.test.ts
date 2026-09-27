import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  getResolvedTheme,
  getThemePreference,
  initTheme,
  loadThemePreference,
  resolveTheme,
  setThemeMode,
  subscribeTheme,
  systemPrefersDark,
} from '@/lib/theme';

/**
 * theme.ts 是模块级单例 store（供跨组件共享），测例之间会相互串状态，
 * 因此每条用例先清 localStorage、重置 matchMedia，再按需 initTheme 复位。
 */
function mockPrefersDark(matches: boolean): void {
  vi.spyOn(window, 'matchMedia').mockImplementation((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }));
}

beforeEach(() => {
  localStorage.clear();
  mockPrefersDark(false);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('resolveTheme', () => {
  it('light/dark 偏好直接解析为同名主题', () => {
    expect(resolveTheme('light')).toBe('light');
    expect(resolveTheme('dark')).toBe('dark');
  });

  it('system 偏好跟随系统深浅色', () => {
    mockPrefersDark(false);
    expect(resolveTheme('system')).toBe('light');
    mockPrefersDark(true);
    expect(resolveTheme('system')).toBe('dark');
  });
});

describe('loadThemePreference', () => {
  it('无存储值时默认 system', () => {
    expect(loadThemePreference()).toBe('system');
  });

  it('回读合法值，非法值回落 system', () => {
    localStorage.setItem('theme', 'dark');
    expect(loadThemePreference()).toBe('dark');
    localStorage.setItem('theme', 'nonsense');
    expect(loadThemePreference()).toBe('system');
  });
});

describe('setThemeMode', () => {
  it('持久化偏好并更新解析主题', () => {
    initTheme();
    setThemeMode('dark');
    expect(localStorage.getItem('theme')).toBe('dark');
    expect(getThemePreference()).toBe('dark');
    expect(getResolvedTheme()).toBe('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
  });

  it('system 偏好按系统深浅色解析', () => {
    mockPrefersDark(true);
    initTheme();
    setThemeMode('system');
    expect(getResolvedTheme()).toBe('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
  });
});

describe('initTheme', () => {
  it('装载已存偏好并写 <html data-theme>', () => {
    localStorage.setItem('theme', 'dark');
    initTheme();
    expect(getResolvedTheme()).toBe('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
  });
});

describe('subscribeTheme', () => {
  it('解析主题变化时通知，订阅者可退订', () => {
    mockPrefersDark(false);
    initTheme();
    setThemeMode('light');
    const spy = vi.fn();
    const off = subscribeTheme(spy);
    setThemeMode('dark');
    expect(spy).toHaveBeenCalledTimes(1);
    off();
    setThemeMode('light');
    expect(spy).toHaveBeenCalledTimes(1);
  });

  it('偏好切换但解析结果不变时不通知', () => {
    mockPrefersDark(false);
    initTheme();
    setThemeMode('light');
    const spy = vi.fn();
    subscribeTheme(spy);
    // light → system（系统为 light），解析仍为 light，不应触发重绘通知
    setThemeMode('system');
    expect(spy).not.toHaveBeenCalled();
  });
});

describe('systemPrefersDark', () => {
  it('读取 prefers-color-scheme: dark 的匹配结果', () => {
    mockPrefersDark(false);
    expect(systemPrefersDark()).toBe(false);
    mockPrefersDark(true);
    expect(systemPrefersDark()).toBe(true);
  });
});
