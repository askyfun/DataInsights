import { describe, expect, it } from 'vitest';
import { ACCOUNT_NAV, ADMIN_NAV, BUSINESS_NAV } from '../../lib/navMenu';

// 菜单分组的回归钉子：主菜单只放业务功能，管理/个人信息走右上角两个入口。
// 这条规则被破坏时（比如用户管理又被加回主菜单），这里先红。
describe('navMenu（顶部导航分组）', () => {
  it('主菜单只含业务路由', () => {
    expect(BUSINESS_NAV.map((e) => e.key)).toEqual([
      '/',
      '/charts',
      '/chart-builder',
      '/datasets',
      '/datasources',
      '/alerts',
    ]);
  });

  it('系统管理/个人信息不得混进主菜单', () => {
    const business = BUSINESS_NAV.map((e) => e.key);
    const nonBusiness = [...ADMIN_NAV, ...ACCOUNT_NAV].map((e) => e.key);
    for (const key of nonBusiness) {
      expect(business).not.toContain(key);
    }
  });

  it('三组互不相交且 key 全局唯一', () => {
    const all = [...BUSINESS_NAV, ...ADMIN_NAV, ...ACCOUNT_NAV].map((e) => e.key);
    expect(new Set(all).size).toBe(all.length);
  });

  it('系统管理只放治理动作，个人信息只放自己的东西', () => {
    expect(ADMIN_NAV.map((e) => e.key)).toEqual(['/users']);
    expect(ACCOUNT_NAV.map((e) => e.key)).toEqual(['/account', '/about']);
  });

  it('每条都有 i18n 文案 id 与图标', () => {
    for (const e of [...BUSINESS_NAV, ...ADMIN_NAV, ...ACCOUNT_NAV]) {
      expect(e.labelId.startsWith('nav.')).toBe(true);
      expect(e.icon.length).toBeGreaterThan(0);
    }
  });
});
