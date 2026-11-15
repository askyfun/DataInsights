// 顶部导航分组（菜单层面重构）。
//
// 一条硬规则：**横向主菜单只放业务功能**（看数与做数的主链路）。系统管理、
// 个人信息、退出这类"非业务"入口统一收进右上角的两个下拉——
//   - 系统管理（仅 admin）：账号/组织层面的治理动作；
//   - 用户信息：我自己的账号设置、关于与自检、退出登录。
//
// 为什么把分组写成纯数据：菜单结构是被评审/被回归的地方（"用户管理是不是又跑回
// 主菜单了"），放在 JSX 里只能靠肉眼。这里不依赖 React 与 antd，图标用名字占位、
// 文案用 i18n id，App 侧再做一次映射，于是分组规则可以被单测钉住。

/** 菜单可用的图标名。App 侧把它映射成 antd 图标组件。 */
export type NavIconName =
  | 'dashboard'
  | 'charts'
  | 'chartBuilder'
  | 'datasets'
  | 'datasources'
  | 'alerts'
  | 'users'
  | 'account'
  | 'about';

export interface NavEntry {
  /** 路由路径，同时是 antd Menu 的 key。 */
  key: string;
  /** i18n 消息 id。 */
  labelId: string;
  icon: NavIconName;
}

/** 主菜单：业务功能，按使用频率排（先看看板，再加工数据）。 */
export const BUSINESS_NAV: readonly NavEntry[] = [
  { key: '/', labelId: 'nav.dashboard', icon: 'dashboard' },
  { key: '/charts', labelId: 'nav.charts', icon: 'charts' },
  { key: '/chart-builder', labelId: 'nav.chartBuilder', icon: 'chartBuilder' },
  { key: '/datasets', labelId: 'nav.datasets', icon: 'datasets' },
  { key: '/datasources', labelId: 'nav.datasources', icon: 'datasources' },
  { key: '/alerts', labelId: 'nav.alerts', icon: 'alerts' },
];

/** 右上角「系统管理」下拉：仅 admin 可见。 */
export const ADMIN_NAV: readonly NavEntry[] = [
  { key: '/users', labelId: 'nav.users', icon: 'users' },
];

/**
 * 右上角「用户信息」下拉：登录后可见。
 * 「关于与自检」放在这里而不是系统管理——它含全员可用的报障入口（诊断包下载才限管理员）。
 */
export const ACCOUNT_NAV: readonly NavEntry[] = [
  { key: '/account', labelId: 'nav.account', icon: 'account' },
  { key: '/about', labelId: 'nav.about', icon: 'about' },
];
