import {
  AlertOutlined,
  AppstoreOutlined,
  BarChartOutlined,
  BuildOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DesktopOutlined,
  DownOutlined,
  GlobalOutlined,
  InfoCircleOutlined,
  LogoutOutlined,
  MenuOutlined,
  MoonOutlined,
  SettingOutlined,
  SunOutlined,
  TeamOutlined,
  UserOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import {
  Avatar,
  Button,
  Drawer,
  Dropdown,
  Layout,
  Menu,
  type MenuProps,
  Result,
  Space,
  Typography,
} from 'antd';
import { lazy, Suspense, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, Navigate, Outlet, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { authApi, systemApi } from './api';

import FirstRunGuide from './components/FirstRunGuide';
import LoadingPlaceholder from './components/LoadingPlaceholder';
import { type LocaleType, useLocale } from './i18n/useLocale';
import { clearBearerToken, getBearerToken, setUnauthorizedHandler } from './lib/api/client';
import { setAuthConfig, useAuthConfig } from './lib/authConfig';
import {
  setSessionReady,
  setSessionUser,
  useSessionReady,
  useSessionUser,
} from './lib/authSession';
import { canSeeUsers, needLogin, showUnauthBanner } from './lib/authz';
import { setupSkipped, shouldShowFirstRun } from './lib/firstRun';
import {
  ACCOUNT_NAV,
  ADMIN_NAV,
  BUSINESS_NAV,
  type NavEntry,
  type NavIconName,
} from './lib/navMenu';
import { type ThemeMode, useTheme } from './lib/theme';

// 页面级代码分割：各路由按需加载，echarts / 图表构建等重资源不再进首屏 bundle。
const AboutPage = lazy(() => import('./pages/About'));
const AccountPage = lazy(() => import('./pages/Account'));
const AlertsPage = lazy(() => import('./pages/Alerts'));
const ChartBuilder = lazy(() => import('./pages/ChartBuilder'));
const ChartsPage = lazy(() => import('./pages/Charts'));
const DashboardEditor = lazy(() => import('./pages/DashboardEditor'));
const DashboardScreen = lazy(() => import('./pages/DashboardScreen'));
const DashboardsPage = lazy(() => import('./pages/Dashboards'));
const DatasetPage = lazy(() => import('./pages/Dataset'));
const DatasetDetail = lazy(() => import('./pages/DatasetDetail'));
const DatasetEdit = lazy(() => import('./pages/DatasetEdit'));
const DatasourcePage = lazy(() => import('./pages/Datasource'));
const DatasourceDetailPage = lazy(() => import('./pages/DatasourceDetail'));
const LoginPage = lazy(() => import('./pages/Login'));
const UsersPage = lazy(() => import('./pages/Users'));

const { Header, Content, Footer } = Layout;
const { Title } = Typography;

// navMenu 里只写图标名（纯数据可单测），真实图标在这里落地。
const NAV_ICONS: Record<NavIconName, React.ReactNode> = {
  dashboard: <DashboardOutlined />,
  charts: <BarChartOutlined />,
  chartBuilder: <BuildOutlined />,
  datasets: <AppstoreOutlined />,
  datasources: <DatabaseOutlined />,
  alerts: <AlertOutlined />,
  users: <TeamOutlined />,
  account: <UserOutlined />,
  about: <InfoCircleOutlined />,
};

// 访问守卫（R-93 / #218）：在路由层统一拦截。
//   - 探测未完成 → 加载态（避免误判成「未登录」而闪跳登录页）；
//   - 库里还没有管理员（needs_bootstrap）且未登录 → 先跑首启向导（任何路由都拦）；
//   - 强制鉴权 on 且未登录 → 跳 /login?next=<原路径>；
//   - adminOnly 且非管理员 → 无权限页（而非只读壳）。
const Guard: React.FC<{ adminOnly?: boolean }> = ({ adminOnly = false }) => {
  const cfg = useAuthConfig();
  const user = useSessionUser();
  const ready = useSessionReady();
  const location = useLocation();
  const intl = useIntl();
  // 本次会话是否已点过「先逛逛」/走完向导。判据本身仍由后端 needs_bootstrap 决定。
  const [setupSkippedThisSession, setSetupSkippedThisSession] = useState(() => setupSkipped());
  if (cfg === null || !ready) {
    return <LoadingPlaceholder text={intl.formatMessage({ id: 'common.loading' })} />;
  }
  // 首启闸门放在登录跳转之前：空库 + enforce=on 时，把新用户丢到 /login 只解决了
  // 「建号」，而向导还要带他载示例、出第一张图。深链（/charts、/datasets）同样拦。
  if (
    shouldShowFirstRun({
      needsAdmin: cfg.needs_bootstrap === true,
      hasUser: !!user,
      skipped: setupSkippedThisSession,
    })
  ) {
    return (
      <FirstRunGuide
        onDone={() => setSetupSkippedThisSession(true)}
        allowSkip={cfg.enforce_mode !== 'on'}
      />
    );
  }
  if (needLogin(cfg.enforce_mode, !!user)) {
    return (
      <Navigate
        to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`}
        replace
      />
    );
  }
  if (adminOnly && !canSeeUsers(user?.role)) {
    return (
      <Result
        status="403"
        title={intl.formatMessage({ id: 'forbidden.title' })}
        subTitle={intl.formatMessage({ id: 'forbidden.desc' })}
      />
    );
  }
  return <Outlet />;
};

/**
 * 菜单右侧的语言 / 主题切换器：只用图标，含义靠悬浮提示与 aria-label 表达，省横向空间。
 * 桌面头栏横向并排、移动端抽屉内纵向堆叠，两处共用同一份实现避免交互分叉。
 */
const LocaleThemeSwitchers: React.FC<{ vertical?: boolean }> = ({ vertical = false }) => {
  const intl = useIntl();
  const { locale, setLocale } = useLocale();
  const { preference, resolved, setMode } = useTheme();

  // 主题按钮图标跟随当前偏好：跟随系统用显示器图标，其余按解析出的深浅色显示日/月。
  const themeIcon =
    preference === 'system' ? (
      <DesktopOutlined />
    ) : resolved === 'dark' ? (
      <MoonOutlined />
    ) : (
      <SunOutlined />
    );

  const localeItems: MenuProps['items'] = [
    { key: 'zh-CN', label: '中文' },
    { key: 'en-US', label: 'English' },
  ];
  const themeItems: MenuProps['items'] = [
    { key: 'light', icon: <SunOutlined />, label: intl.formatMessage({ id: 'theme.light' }) },
    { key: 'dark', icon: <MoonOutlined />, label: intl.formatMessage({ id: 'theme.dark' }) },
    { key: 'system', icon: <DesktopOutlined />, label: intl.formatMessage({ id: 'theme.system' }) },
  ];

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: vertical ? 'column' : 'row',
        alignItems: vertical ? 'flex-start' : 'center',
        gap: vertical ? 8 : 4,
      }}
    >
      <Dropdown
        trigger={['click']}
        placement="bottomRight"
        menu={{
          items: localeItems,
          selectedKeys: [locale],
          onClick: ({ key }) => setLocale(key as LocaleType),
        }}
      >
        <Button
          type="text"
          icon={<GlobalOutlined />}
          title={intl.formatMessage({ id: 'locale.label' })}
          aria-label={intl.formatMessage({ id: 'locale.label' })}
        />
      </Dropdown>
      <Dropdown
        trigger={['click']}
        placement="bottomRight"
        menu={{
          items: themeItems,
          selectedKeys: [preference],
          onClick: ({ key }) => setMode(key as ThemeMode),
        }}
      >
        <Button
          type="text"
          icon={themeIcon}
          title={intl.formatMessage({ id: 'theme.label' })}
          aria-label={intl.formatMessage({ id: 'theme.label' })}
        />
      </Dropdown>
    </div>
  );
};

const App: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(false);
  const me = useSessionUser();
  const authCfg = useAuthConfig();
  const [backendVersion, setBackendVersion] = useState('');
  const location = useLocation();
  // 首启态（库里还没管理员）不提示「未强制登录」：他连账号都还没有，这条横幅只是噪音，
  // 还会让人误以为"这系统本来就不用登录"。有管理员之后才回到常规提示。
  const bannerShown = showUnauthBanner(authCfg?.enforce_mode) && authCfg?.needs_bootstrap !== true;

  // 水合：sessionStorage 里的令牌在刷新后仍在 → me() 成功即保持登录（#218 AC2）。
  useEffect(() => {
    if (getBearerToken()) {
      authApi
        .me()
        .then((r) => setSessionUser(r.data.data))
        .catch(() => {
          clearBearerToken();
          setSessionUser(null);
        })
        .finally(() => setSessionReady(true));
    } else {
      setSessionUser(null);
      setSessionReady(true);
    }
  }, []);

  // 能力探测：启动读 /api/auth/config（enforce_mode + needs_bootstrap）。
  useEffect(() => {
    authApi
      .config()
      .then((r) => setAuthConfig(r.data.data))
      .catch(() => setAuthConfig({ enforce_mode: 'off', needs_bootstrap: false }));
  }, []);

  // 统一拦截：任意请求收到 20200（未认证）→ 清令牌 + 跳登录并回跳。
  useEffect(() => {
    setUnauthorizedHandler(() => {
      setSessionUser(null);
      navigate(`/login?next=${encodeURIComponent(location.pathname + location.search)}`);
    });
    return () => setUnauthorizedHandler(null);
  }, [navigate, location]);

  // 版本串只有后端一个来源（构建时注入），页脚不写死产品版本。
  useEffect(() => {
    let cancelled = false;
    systemApi
      .getVersion()
      .then((res) => {
        if (!cancelled) setBackendVersion(res.data.data.version);
      })
      .catch((err: unknown) => {
        console.debug('failed to load backend version', err);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const checkMobile = () => setIsMobile(window.innerWidth < 768);
    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: 仅需在路由变化时收起移动端抽屉
  useEffect(() => {
    setMobileMenuOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    document.title = 'TableWave';
  }, []);

  const doLogout = async () => {
    try {
      await authApi.logout();
    } catch {
      /* 令牌可能已失效：退出以本地清理为准 */
    }
    clearBearerToken();
    setSessionUser(null);
    navigate('/login');
  };

  const onMenuClick = ({ key }: { key: string }) => {
    if (key === 'logout') void doLogout();
  };

  // 菜单分组见 lib/navMenu.ts：主菜单只放业务功能，管理与个人信息走右上角两个下拉。
  const toMenuItems = (entries: readonly NavEntry[]): NonNullable<MenuProps['items']> =>
    entries.map((e) => ({
      key: e.key,
      icon: NAV_ICONS[e.icon],
      label: <Link to={e.key}>{intl.formatMessage({ id: e.labelId })}</Link>,
    }));

  const businessItems = toMenuItems(BUSINESS_NAV);
  const adminItems = toMenuItems(ADMIN_NAV);
  const accountItems = toMenuItems(ACCOUNT_NAV);

  const logoutItem: NonNullable<MenuProps['items']>[number] = {
    key: 'logout',
    icon: <LogoutOutlined />,
    danger: true,
    label: intl.formatMessage({ id: 'nav.logout' }),
  };
  // 首启态（库里还没有管理员）：入口话术从「登录」换成「初始化管理员」。
  // 只在 /login 里才变成注册表单这件事不能让用户自己去猜——右上角先说清楚。
  const needsBootstrap = authCfg?.needs_bootstrap === true;
  const entryLabelId = needsBootstrap ? 'nav.bootstrap' : 'nav.login';

  const loginItem: NonNullable<MenuProps['items']>[number] = {
    key: '/login',
    icon: <UserOutlined />,
    label: <Link to="/login">{intl.formatMessage({ id: entryLabelId })}</Link>,
  };

  // 右上角「用户信息」：账号设置 / 关于与自检 / 退出登录。
  const userItems: NonNullable<MenuProps['items']> = me
    ? [...accountItems, { type: 'divider' as const }, logoutItem]
    : [loginItem];

  // 移动端抽屉：业务项 + 系统管理分组 + 个人信息，信息架构与桌面端一致。
  const drawerItems: NonNullable<MenuProps['items']> = [
    ...businessItems,
    ...(canSeeUsers(me?.role)
      ? [
          {
            type: 'group' as const,
            label: intl.formatMessage({ id: 'nav.system' }),
            children: adminItems,
          },
        ]
      : []),
    { type: 'divider' as const },
    ...(me ? [...accountItems, logoutItem] : [loginItem]),
  ];

  return (
    <Layout className="layout" style={{ minHeight: '100vh' }}>
      <a href="#main-content" className="skip-link">
        Skip to main content
      </a>
      {bannerShown && (
        <div
          data-testid="unauth-banner"
          style={{
            position: 'sticky',
            top: 0,
            zIndex: 200,
            background: '#fffbe6',
            borderBottom: '1px solid #ffe58f',
            color: '#ad6800',
            textAlign: 'center',
            fontSize: 12,
            padding: '5px 12px',
          }}
        >
          <WarningOutlined /> {intl.formatMessage({ id: 'banner.unauth' })}
        </div>
      )}
      <Header
        style={{
          display: 'flex',
          alignItems: 'center',
          padding: isMobile ? '0 12px' : '0 16px',
          height: 48,
          lineHeight: '48px',
          position: 'sticky',
          top: bannerShown ? 31 : 0,
          zIndex: 100,
          background: 'var(--dr-surface)',
          borderBottom: '1px solid var(--dr-border)',
        }}
      >
        {isMobile && (
          <Button
            type="text"
            icon={<MenuOutlined style={{ fontSize: 18 }} />}
            onClick={() => setMobileMenuOpen(true)}
            style={{ marginRight: 12 }}
            aria-label="打开菜单"
          />
        )}
        <div style={{ display: 'flex', alignItems: 'center', marginRight: isMobile ? 8 : 32 }}>
          <div className="demo-logo" />
          {!isMobile && (
            <div
              style={{ display: 'flex', flexDirection: 'column', marginLeft: 12, lineHeight: 1.15 }}
            >
              <Title level={5} style={{ margin: 0 }}>
                TableWave
              </Title>
              <span style={{ fontSize: 10, color: 'var(--dr-text-3)', letterSpacing: '0.03em' }}>
                Data Intelligence Platform
              </span>
            </div>
          )}
        </div>
        {!isMobile ? (
          <>
            <Menu
              mode="horizontal"
              defaultSelectedKeys={['/']}
              selectedKeys={[location.pathname]}
              items={businessItems}
              onClick={onMenuClick}
              style={{
                flex: 1,
                minWidth: 0,
                background: 'transparent',
                border: 'none',
                lineHeight: '46px',
              }}
            />
            <Space style={{ marginLeft: 12 }} size={4}>
              <LocaleThemeSwitchers />
              {canSeeUsers(me?.role) && (
                <Dropdown
                  trigger={['click']}
                  placement="bottomRight"
                  menu={{
                    items: adminItems,
                    onClick: onMenuClick,
                    selectedKeys: [location.pathname],
                  }}
                >
                  <Button type="text" icon={<SettingOutlined />} data-testid="nav-system">
                    {intl.formatMessage({ id: 'nav.system' })}
                  </Button>
                </Dropdown>
              )}
              {me ? (
                <Dropdown
                  trigger={['click']}
                  placement="bottomRight"
                  menu={{
                    items: userItems,
                    onClick: onMenuClick,
                    selectedKeys: [location.pathname],
                  }}
                >
                  <Button type="text" data-testid="nav-user" style={{ paddingInline: 8 }}>
                    <Space size={6}>
                      <Avatar size={20} icon={<UserOutlined />} />
                      <span
                        style={{
                          maxWidth: 120,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                          display: 'inline-block',
                          verticalAlign: 'middle',
                        }}
                      >
                        {me.username}
                      </span>
                      <DownOutlined style={{ fontSize: 10 }} />
                    </Space>
                  </Button>
                </Dropdown>
              ) : (
                <Button
                  type={needsBootstrap ? 'primary' : 'text'}
                  data-testid="nav-login"
                  onClick={() => navigate('/login')}
                >
                  {intl.formatMessage({ id: entryLabelId })}
                </Button>
              )}
            </Space>
          </>
        ) : null}
      </Header>

      <Drawer
        title={<span style={{ fontWeight: 'bold' }}>TableWave</span>}
        placement="left"
        onClose={() => setMobileMenuOpen(false)}
        open={mobileMenuOpen}
        size={280}
        styles={{ body: { padding: 0 } }}
      >
        <Menu
          mode="inline"
          selectedKeys={[location.pathname]}
          items={drawerItems}
          onClick={onMenuClick}
          style={{ border: 'none' }}
        />
        <div style={{ padding: '16px', borderTop: '1px solid var(--dr-border)' }}>
          <LocaleThemeSwitchers vertical />
        </div>
      </Drawer>

      <Layout>
        <Layout style={{ padding: '0' }}>
          <Content id="main-content" style={{ background: 'var(--dr-canvas)', minHeight: 280 }}>
            <Suspense
              fallback={<LoadingPlaceholder text={intl.formatMessage({ id: 'common.loading' })} />}
            >
              <Routes>
                <Route path="/login" element={<LoginPage />} />
                <Route element={<Guard />}>
                  <Route path="/" element={<DashboardsPage />} />
                  <Route path="/dashboards/:id" element={<DashboardEditor />} />
                  <Route path="/dashboards/:id/screen" element={<DashboardScreen />} />
                  <Route path="/datasources" element={<DatasourcePage />} />
                  <Route path="/datasources/:id" element={<DatasourceDetailPage />} />
                  <Route path="/datasets" element={<DatasetPage />} />
                  <Route path="/datasets/new" element={<DatasetEdit />} />
                  <Route path="/datasets/:id" element={<DatasetDetail />} />
                  <Route path="/datasets/:id/edit" element={<DatasetEdit />} />
                  <Route path="/chart-builder" element={<ChartBuilder />} />
                  <Route path="/charts" element={<ChartsPage />} />
                  <Route path="/alerts" element={<AlertsPage />} />
                  <Route path="/account" element={<AccountPage />} />
                  <Route path="/about" element={<AboutPage />} />
                </Route>
                <Route element={<Guard adminOnly />}>
                  <Route path="/users" element={<UsersPage />} />
                </Route>
              </Routes>
            </Suspense>
          </Content>
        </Layout>
      </Layout>
      <Footer
        style={{
          textAlign: 'center',
          background: 'transparent',
          borderTop: '1px solid var(--dr-border)',
          color: 'var(--dr-text-3)',
          fontSize: 12,
          padding: '16px',
        }}
      >
        TableWave{backendVersion ? ` · ${backendVersion}` : ''} ©2026 Created with React + Ant
        Design
      </Footer>
    </Layout>
  );
};

export default App;
