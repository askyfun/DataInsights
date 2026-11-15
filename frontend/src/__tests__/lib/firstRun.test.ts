import { beforeEach, describe, expect, it } from 'vitest';
import {
  LEGACY_SETUP_DONE_KEY,
  markSetupSkipped,
  SETUP_SKIPPED_KEY,
  setupSkipped,
  shouldShowFirstRun,
} from '../../lib/firstRun';

// 回归钉子：首启引导的判据只能是「后端说没有管理员」，不能被本机 localStorage 永久静音。
// 旧实现（issue 现场）把 setup_done 写进 localStorage，于是同一浏览器在清库重建之后
// 再也不引导，首启用户看到的就是"一个只能登录的系统"。
describe('firstRun（首启引导判定）', () => {
  beforeEach(() => {
    sessionStorage.clear();
    localStorage.clear();
  });

  it('空库 + 未登录 + 本次会话没跳过 → 引导', () => {
    expect(shouldShowFirstRun({ needsAdmin: true, hasUser: false, skipped: false })).toBe(true);
  });

  it('已有管理员 → 永不引导', () => {
    expect(shouldShowFirstRun({ needsAdmin: false, hasUser: false, skipped: false })).toBe(false);
  });

  it('已登录 → 不引导（哪怕是空库）', () => {
    expect(shouldShowFirstRun({ needsAdmin: true, hasUser: true, skipped: false })).toBe(false);
  });

  it('本次会话跳过 → 不引导，但换会话会再来', () => {
    expect(shouldShowFirstRun({ needsAdmin: true, hasUser: false, skipped: true })).toBe(false);
    markSetupSkipped();
    expect(setupSkipped()).toBe(true);
    sessionStorage.clear(); // 新标签页/新会话
    expect(setupSkipped()).toBe(false);
  });

  it('跳过标记只写 sessionStorage，并清掉旧的永久静音键', () => {
    localStorage.setItem(LEGACY_SETUP_DONE_KEY, '1');
    markSetupSkipped();
    expect(sessionStorage.getItem(SETUP_SKIPPED_KEY)).toBe('1');
    // 旧键必须被清除：留着会让排查的人以为"这台机器已经配好了"。
    expect(localStorage.getItem(LEGACY_SETUP_DONE_KEY)).toBeNull();
  });

  it('存储不可用时按未跳过处理（不因隐私模式而静默失去引导）', () => {
    const original = Object.getOwnPropertyDescriptor(window, 'sessionStorage');
    Object.defineProperty(window, 'sessionStorage', {
      configurable: true,
      get() {
        throw new Error('blocked');
      },
    });
    try {
      expect(setupSkipped()).toBe(false);
      expect(() => markSetupSkipped()).not.toThrow();
    } finally {
      if (original) Object.defineProperty(window, 'sessionStorage', original);
    }
  });
});
