// useIsMobile：断点口径与 App.tsx / index.css 的 767px 一致，这里钉住行为（初始读 + resize 跟随）。
import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { useIsMobile } from '../../lib/useIsMobile';

const setWidth = (px: number) => {
  window.innerWidth = px;
  act(() => {
    window.dispatchEvent(new Event('resize'));
  });
};

afterEach(() => {
  setWidth(1024);
});

describe('useIsMobile', () => {
  it('767px 及以下判为移动端，768px 及以上判为桌面（边界对齐 767 断点）', () => {
    window.innerWidth = 767;
    const { result } = renderHook(() => useIsMobile());
    expect(result.current).toBe(true);

    setWidth(768);
    expect(result.current).toBe(false);

    setWidth(375);
    expect(result.current).toBe(true);
  });

  it('resize 事件实时更新，不留在初始值', () => {
    window.innerWidth = 1280;
    const { result } = renderHook(() => useIsMobile());
    expect(result.current).toBe(false);
    setWidth(375);
    expect(result.current).toBe(true);
  });
});
