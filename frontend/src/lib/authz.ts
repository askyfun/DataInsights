// 身份壳（R-93 / #218）的纯判定逻辑，零 React 依赖，便于单测钉住语义。

export type EnforceMode = 'off' | 'shadow' | 'on' | string | undefined;

/** 是否需要把当前访问者弹到登录页：仅当强制鉴权开启（on）且当前无已登录用户。 */
export function needLogin(mode: EnforceMode, hasUser: boolean): boolean {
  return mode === 'on' && !hasUser;
}

/** 顶部「未强制登录」横幅：只要探测到配置且模式不是 on，就应常驻提示。 */
export function showUnauthBanner(mode: EnforceMode): boolean {
  return mode !== undefined && mode !== 'on';
}

/** 是否允许进入用户管理：仅 admin。 */
export function canSeeUsers(role: string | undefined): boolean {
  return role === 'admin';
}

/** 登录成功后的回跳地址：仅接受站内绝对路径，防开放重定向。 */
export function safeNext(raw: string | null | undefined): string {
  if (!raw) return '/';
  if (!raw.startsWith('/') || raw.startsWith('//')) return '/';
  return raw;
}
