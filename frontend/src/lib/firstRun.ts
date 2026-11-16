// 首启引导的判定逻辑（R-101 / #226 收口）。
//
// 一条硬规则：**要不要引导只认后端 `needs_admin`**——「库里到底有没有管理员」是服务端
// 事实，不该被某台机器的 localStorage 永久静音（旧实现写 `setup_done`，于是同一浏览器
// 在清库重建之后仍然不引导，用户看到的就只是"一个只能登录的系统"）。
// 本机唯一的作用是**本次会话别再打扰**：用户点了「先进入应用」后同一标签页不再顶替列表，
// 换个会话会再提一次，因为系统确实还缺管理员。
//
// 唯一的例外是「向导正在跑」：第①步建号成功已经让 hasUser 变真，若判据仍要求 !hasUser，
// 向导会在成功那一刻自我卸载，②③ 步永远执行不到。这个例外也只是会话级标记
// （SETUP_IN_PROGRESS_KEY），不改变「是否需要引导」最终由后端事实决定这一点。
//
// 抽成纯函数是为了能被单测钉住——这段判定以前埋在 useEffect 里，只能靠手点验证。

/** 「本次会话别再打扰」的标记键（sessionStorage，刻意不用 localStorage）。 */
export const SETUP_SKIPPED_KEY = 'setup_skipped';

/**
 * 「向导正跑到一半」的标记键（sessionStorage）。
 *
 * 为什么需要它：第①步建完管理员后 `hasUser` 立刻为真，如果判据只认
 * `needsAdmin && !hasUser`，向导会在建号成功的那一刻把自己卸掉——②载示例、
 * ③出第一张图 两步永远没机会执行（#226 现场就是这样，新人建完号直接被丢进空列表页）。
 * 这个标记由向导在建号成功后写入，走完/跳过时清掉，只活一个会话。
 */
export const SETUP_IN_PROGRESS_KEY = 'setup_in_progress';

/** 旧版本用来永久静音的键，已废弃，只用于清理。 */
export const LEGACY_SETUP_DONE_KEY = 'setup_done';

export interface FirstRunSignals {
  /** 后端 /api/setup/status 的 needs_admin：库里一个管理员都没有。 */
  needsAdmin: boolean;
  /** 当前是否已有登录身份（已登录的人不需要再被引导建号）。 */
  hasUser: boolean;
  /** 本次会话里用户是否已经点过「先进入应用」/ 走完向导。 */
  skipped: boolean;
  /**
   * 本次会话里向导已经建好管理员、但 ②③ 步还没走完。它为真时**压过 hasUser**：
   * 建号本身是向导的第一步，不是退出的信号。
   */
  inProgress?: boolean;
}

/** 是否用首启向导顶替页面内容。 */
export function shouldShowFirstRun(s: FirstRunSignals): boolean {
  // 「跳过」永远优先：用户点了「先进入应用」，任何进程内状态都不该再拦他。
  if (s.skipped) return false;
  if (s.inProgress) return true;
  return s.needsAdmin && !s.hasUser;
}

/** 读「本次会话已跳过」。存储不可用（隐私模式/被禁用）时按未跳过处理。 */
export function setupSkipped(): boolean {
  try {
    return sessionStorage.getItem(SETUP_SKIPPED_KEY) === '1';
  } catch {
    return false;
  }
}

/** 读「向导正跑到一半」。存储不可用时按未开始处理（最坏只是退回旧行为）。 */
export function setupInProgress(): boolean {
  try {
    return sessionStorage.getItem(SETUP_IN_PROGRESS_KEY) === '1';
  } catch {
    return false;
  }
}

/** 标记「向导正跑到一半」（建号成功后由向导写入）。 */
export function markSetupInProgress(): void {
  try {
    sessionStorage.setItem(SETUP_IN_PROGRESS_KEY, '1');
  } catch {
    /* 存储不可用时忽略：最坏是该会话里向导仍会在建号后消失，不影响后续可用性 */
  }
}

/** 清掉「向导正跑到一半」（走完向导 / 点「先进入应用」时）。 */
export function clearSetupInProgress(): void {
  try {
    sessionStorage.removeItem(SETUP_IN_PROGRESS_KEY);
  } catch {
    /* 同上 */
  }
}

/** 标记「本次会话已跳过」，并清掉向导进行中与旧的永久静音键。 */
export function markSetupSkipped(): void {
  try {
    sessionStorage.setItem(SETUP_SKIPPED_KEY, '1');
    sessionStorage.removeItem(SETUP_IN_PROGRESS_KEY);
    localStorage.removeItem(LEGACY_SETUP_DONE_KEY);
  } catch {
    /* 存储不可用时忽略：最坏结果只是下次还会引导，不影响正确性 */
  }
}
