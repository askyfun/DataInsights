// 首启引导的判定逻辑（R-101 / #226 收口）。
//
// 一条硬规则：**要不要引导只认后端 `needs_admin`**——「库里到底有没有管理员」是服务端
// 事实，不该被某台机器的 localStorage 永久静音（旧实现写 `setup_done`，于是同一浏览器
// 在清库重建之后仍然不引导，用户看到的就只是"一个只能登录的系统"）。
// 本机唯一的作用是**本次会话别再打扰**：用户点了「先进入应用」后同一标签页不再顶替列表，
// 换个会话会再提一次，因为系统确实还缺管理员。
//
// 抽成纯函数是为了能被单测钉住——这段判定以前埋在 useEffect 里，只能靠手点验证。

/** 「本次会话别再打扰」的标记键（sessionStorage，刻意不用 localStorage）。 */
export const SETUP_SKIPPED_KEY = 'setup_skipped';

/** 旧版本用来永久静音的键，已废弃，只用于清理。 */
export const LEGACY_SETUP_DONE_KEY = 'setup_done';

export interface FirstRunSignals {
  /** 后端 /api/setup/status 的 needs_admin：库里一个管理员都没有。 */
  needsAdmin: boolean;
  /** 当前是否已有登录身份（已登录的人不需要再被引导建号）。 */
  hasUser: boolean;
  /** 本次会话里用户是否已经点过「先进入应用」/ 走完向导。 */
  skipped: boolean;
}

/** 是否用首启向导顶替页面内容。 */
export function shouldShowFirstRun(s: FirstRunSignals): boolean {
  return s.needsAdmin && !s.hasUser && !s.skipped;
}

/** 读「本次会话已跳过」。存储不可用（隐私模式/被禁用）时按未跳过处理。 */
export function setupSkipped(): boolean {
  try {
    return sessionStorage.getItem(SETUP_SKIPPED_KEY) === '1';
  } catch {
    return false;
  }
}

/** 标记「本次会话已跳过」，并清掉旧的永久静音键。 */
export function markSetupSkipped(): void {
  try {
    sessionStorage.setItem(SETUP_SKIPPED_KEY, '1');
    localStorage.removeItem(LEGACY_SETUP_DONE_KEY);
  } catch {
    /* 存储不可用时忽略：最坏结果只是下次还会引导，不影响正确性 */
  }
}
