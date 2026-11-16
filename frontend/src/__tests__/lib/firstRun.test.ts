import { beforeEach, describe, expect, it } from 'vitest';
import {
  clearSetupInProgress,
  LEGACY_SETUP_DONE_KEY,
  markSetupInProgress,
  markSetupSkipped,
  SETUP_IN_PROGRESS_KEY,
  SETUP_SKIPPED_KEY,
  setupInProgress,
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

  // 回归钉子（#226 现场实测）：向导第①步建完管理员后 `hasUser` 立刻变真，
  // 旧判据 `needsAdmin && !hasUser` 就地翻假 → 整个组件被卸载，②载示例 / ③出第一张图
  // 两步成了死代码，新人建完号直接被丢进空列表页。修法是把「向导正跑到一半」记为独立信号，
  // 它在本次会话内压过 hasUser，但仍然让位给「跳过」。
  it('向导内建完管理员（hasUser 已为真）仍继续走 ②③ 步', () => {
    expect(
      shouldShowFirstRun({ needsAdmin: true, hasUser: true, skipped: false, inProgress: true })
    ).toBe(true);
  });

  it('inProgress 不依赖后端 needsAdmin（探测结果在会话内不会自动刷新）', () => {
    expect(
      shouldShowFirstRun({ needsAdmin: false, hasUser: true, skipped: false, inProgress: true })
    ).toBe(true);
  });

  it('跳过优先于 inProgress：点了「先进入应用」必须立刻放行', () => {
    expect(
      shouldShowFirstRun({ needsAdmin: true, hasUser: true, skipped: true, inProgress: true })
    ).toBe(false);
  });

  it('inProgress 只写 sessionStorage，且可被清掉（走完向导即结束）', () => {
    expect(setupInProgress()).toBe(false);
    markSetupInProgress();
    expect(sessionStorage.getItem(SETUP_IN_PROGRESS_KEY)).toBe('1');
    expect(setupInProgress()).toBe(true);
    clearSetupInProgress();
    expect(setupInProgress()).toBe(false);
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
