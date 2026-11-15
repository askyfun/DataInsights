import { describe, expect, it } from 'vitest';
import { canSeeUsers, needLogin, safeNext, showUnauthBanner } from '../../lib/authz';

describe('authz（R-93 / #218 身份壳判定）', () => {
  it('needLogin：仅 on 且无用户时为真', () => {
    expect(needLogin('on', false)).toBe(true);
    expect(needLogin('on', true)).toBe(false);
    expect(needLogin('off', false)).toBe(false);
    expect(needLogin('shadow', false)).toBe(false);
    expect(needLogin(undefined, false)).toBe(false);
  });

  it('showUnauthBanner：探测到且非 on 时显示', () => {
    expect(showUnauthBanner('off')).toBe(true);
    expect(showUnauthBanner('shadow')).toBe(true);
    expect(showUnauthBanner('on')).toBe(false);
    expect(showUnauthBanner(undefined)).toBe(false);
  });

  it('canSeeUsers：仅 admin', () => {
    expect(canSeeUsers('admin')).toBe(true);
    expect(canSeeUsers('user')).toBe(false);
    expect(canSeeUsers(undefined)).toBe(false);
  });

  it('safeNext：仅站内绝对路径，防开放重定向', () => {
    expect(safeNext('/datasets')).toBe('/datasets');
    expect(safeNext('/charts?q=1')).toBe('/charts?q=1');
    expect(safeNext('https://evil.com')).toBe('/');
    expect(safeNext('//evil.com')).toBe('/');
    expect(safeNext(null)).toBe('/');
  });
});
