import { act, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { systemPrefersReducedMotion, usePrefersReducedMotion } from '@/lib/reducedMotion';

/**
 * lib/reducedMotion 是「减少动态效果」（issue #67）的单一事实源。
 * 走 CSS 的动效由 styles/index.css 的媒体查询兜底；本模块只服务 CSS 够不着的两处
 * （ECharts 的 canvas 动画、dnd-kit DragOverlay 的 WAAPI 回落动画），因此测试覆盖：
 * 初值取系统设置、系统开关变化时重渲染、卸载注销监听（含 Safari 13 的 addListener 回落）。
 */

type Listener = (event: MediaQueryListEvent) => void;

/** 可控的 matchMedia：暴露 emit() 模拟系统开关变化，并记录监听器数量。 */
function mockMatchMedia(initial: boolean): {
  emit: (next: boolean) => void;
  listenerCount: () => number;
} {
  const listeners = new Set<Listener>();
  let matches = initial;
  const mql = {
    get matches(): boolean {
      return matches;
    },
    media: '(prefers-reduced-motion: reduce)',
    onchange: null,
    addListener: (listener: Listener) => {
      listeners.add(listener);
    },
    removeListener: (listener: Listener) => {
      listeners.delete(listener);
    },
    addEventListener: (_type: string, listener: Listener) => {
      listeners.add(listener);
    },
    removeEventListener: (_type: string, listener: Listener) => {
      listeners.delete(listener);
    },
    dispatchEvent: () => false,
  };
  vi.spyOn(window, 'matchMedia').mockImplementation(() => mql as unknown as MediaQueryList);
  return {
    emit(next: boolean): void {
      matches = next;
      for (const listener of listeners) {
        listener({ matches: next } as MediaQueryListEvent);
      }
    },
    listenerCount: () => listeners.size,
  };
}

function Probe() {
  return <span data-testid="reduced">{String(usePrefersReducedMotion())}</span>;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('usePrefersReducedMotion', () => {
  it('初值取系统设置（reduce 时为 true）', () => {
    mockMatchMedia(true);
    render(<Probe />);
    expect(screen.getByTestId('reduced').textContent).toBe('true');
  });

  it('系统开关变化时即时重渲染', () => {
    const mq = mockMatchMedia(false);
    render(<Probe />);
    expect(screen.getByTestId('reduced').textContent).toBe('false');

    act(() => {
      mq.emit(true);
    });
    expect(screen.getByTestId('reduced').textContent).toBe('true');
  });

  it('卸载时注销 change 监听，不留下悬挂订阅', () => {
    const mq = mockMatchMedia(false);
    const { unmount } = render(<Probe />);
    expect(mq.listenerCount()).toBe(1);
    unmount();
    expect(mq.listenerCount()).toBe(0);
  });
});

describe('systemPrefersReducedMotion', () => {
  it('读取 matchMedia 的当前值', () => {
    mockMatchMedia(true);
    expect(systemPrefersReducedMotion()).toBe(true);

    mockMatchMedia(false);
    expect(systemPrefersReducedMotion()).toBe(false);
  });
});
