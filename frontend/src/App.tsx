import {
  AlertOutlined,
  AppstoreOutlined,
  BarChartOutlined,
  BuildOutlined,
  BulbOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  GlobalOutlined,
  LogoutOutlined,
  MenuOutlined,
  TeamOutlined,
  UserOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { Button, Drawer, Layout, Menu, Result, Select, Space, Typography } from 'antd';
import { lazy, Suspense, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, Navigate, Outlet, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { authApi, systemApi } from './api';

import LoadingPlaceholder from './components/LoadingPlaceholder';
import { useLocale } from './i18n/useLocale';
import { clearBearerToken, getBearerToken, setUnauthorizedHandler } from './lib/api/client';
import { setAuthConfig, useAuthConfig } from './lib/authConfig';
import {
  setSessionReady,
  setSessionUser,
  useSessionReady,
  useSessionUser,
} from './lib/authSession';
import { canSeeUsers, needLogin, showUnauthBanner } from './lib/authz';
import { type ThemeMode, useTheme } from './lib/theme';

// 页面级代码分割：各路由按需加载，echarts / 图表构建等重资源不再进首屏 bundle。
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

// 访问守卫（R-93 / #218）：在路由层统一拦截。
//   - 探测未完成 → 加载态（避免误判成「未登录」而闪跳登录页）；
//   - 强制鉴权 on 且未登录 → 跳 /login?next=<原路径>；
//   - adminOnly 且非管理员 → 无权限页（而非只读壳）。
const Guard: React.FC<{ adminOnly?: boolean }> = ({ adminOnly = false }) => {
  const cfg = useAuthConfig();
  const user = useSessionUser();
  const ready = useSessionReady();
  const location = useLocation();
  const intl = useIntl();
  if (cfg === null || !ready) {
    return <LoadingPlaceholder text={intl.formatMessage({ id: 'common.loading' })} />;
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

const App: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const { locale, setLocale } = useLocale();
  const { preference, setMode } = useTheme();
  const themeOptions = [
    { value: 'light' as ThemeMode, label: intl.formatMessage({ id: 'theme.light' }) },
    { value: 'dark' as ThemeMode, label: intl.formatMessage({ id: 'theme.dark' }) },
    { value: 'system' as ThemeMode, label: intl.formatMessage({ id: 'theme.system' }) },
  ];
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(false);
  const me = useSessionUser();
  const authCfg = useAuthConfig();
  const [backendVersion, setBackendVersion] = useState('');
  const location = useLocation();
  const bannerShown = showUnauthBanner(authCfg?.enforce_mode);

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

  const menuItems = [
    {
      key: '/',
      icon: <DashboardOutlined />,
      label: <Link to="/">{intl.formatMessage({ id: 'nav.dashboard' })}</Link>,
    },
    {
      key: '/charts',
      icon: <BarChartOutlined />,
      label: <Link to="/charts">{intl.formatMessage({ id: 'nav.charts' })}</Link>,
    },
    {
      key: '/chart-builder',
      icon: <BuildOutlined />,
      label: <Link to="/chart-builder">{intl.formatMessage({ id: 'nav.chartBuilder' })}</Link>,
    },
    {
      key: '/datasets',
      icon: <AppstoreOutlined />,
      label: <Link to="/datasets">{intl.formatMessage({ id: 'nav.datasets' })}</Link>,
    },
    {
      key: '/datasources',
      icon: <DatabaseOutlined />,
      label: <Link to="/datasources">{intl.formatMessage({ id: 'nav.datasources' })}</Link>,
    },
    {
      key: '/alerts',
      icon: <AlertOutlined />,
      label: <Link to="/alerts">{intl.formatMessage({ id: 'nav.alerts' })}</Link>,
    },
    ...(me?.role === 'admin'
      ? [
          {
            key: '/users',
            icon: <TeamOutlined />,
            label: <Link to="/users">{intl.formatMessage({ id: 'nav.users' })}</Link>,
          },
        ]
      : []),
    ...(me
      ? [
          {
            key: '/account',
            icon: <UserOutlined />,
            label: <Link to="/account">{me.username}</Link>,
          },
          {
            key: 'logout',
            icon: <LogoutOutlined />,
            label: intl.formatMessage({ id: 'nav.logout' }),
          },
        ]
      : [
          {
            key: '/login',
            icon: <UserOutlined />,
            label: <Link to="/login">{intl.formatMessage({ id: 'nav.login' })}</Link>,
          },
        ]),
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
              items={menuItems}
              onClick={onMenuClick}
              style={{
                flex: 1,
                minWidth: 0,
                background: 'transparent',
                border: 'none',
                lineHeight: '46px',
              }}
            />
            <Space style={{ marginLeft: 16 }}>
              <GlobalOutlined />
              <Select
                value={locale}
                onChange={(value) => setLocale(value)}
                style={{ width: 100 }}
                options={[
                  { value: 'zh-CN', label: '中文' },
                  { value: 'en-US', label: 'English' },
                ]}
              />
              <BulbOutlined />
              <Select
                aria-label={intl.formatMessage({ id: 'theme.label' })}
                value={preference}
                onChange={(value) => setMode(value)}
                style={{ width: 96 }}
                options={themeOptions}
              />
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
          items={menuItems}
          onClick={onMenuClick}
          style={{ border: 'none' }}
        />
        <div style={{ padding: '16px', borderTop: '1px solid var(--dr-border)' }}>
          <Space>
            <GlobalOutlined />
            <Select
              value={locale}
              onChange={(value) => setLocale(value)}
              style={{ width: 100 }}
              options={[
                { value: 'zh-CN', label: '中文' },
                { value: 'en-US', label: 'English' },
              ]}
            />
          </Space>
          <Space style={{ marginTop: 12 }}>
            <BulbOutlined />
            <Select
              aria-label={intl.formatMessage({ id: 'theme.label' })}
              value={preference}
              onChange={(value) => setMode(value)}
              style={{ width: 96 }}
              options={themeOptions}
            />
          </Space>
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
