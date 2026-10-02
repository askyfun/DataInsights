import {
  AlertOutlined,
  AppstoreOutlined,
  BarChartOutlined,
  BuildOutlined,
  BulbOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  GlobalOutlined,
  MenuOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Button, Drawer, Layout, Menu, Select, Space, Typography } from 'antd';
import { lazy, Suspense, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, Route, Routes, useLocation } from 'react-router-dom';
import LoadingPlaceholder from './components/LoadingPlaceholder';
import { useLocale } from './i18n/useLocale';
import { type ThemeMode, useTheme } from './lib/theme';

// 页面级代码分割：各路由按需加载，echarts / 图表构建等重资源不再进首屏 bundle。
const AccountPage = lazy(() => import('./pages/Account'));
const AlertsPage = lazy(() => import('./pages/Alerts'));
const ChartBuilder = lazy(() => import('./pages/ChartBuilder'));
const ChartsPage = lazy(() => import('./pages/Charts'));
const DashboardEditor = lazy(() => import('./pages/DashboardEditor'));
const DashboardsPage = lazy(() => import('./pages/Dashboards'));
const DatasetPage = lazy(() => import('./pages/Dataset'));
const DatasetDetail = lazy(() => import('./pages/DatasetDetail'));
const DatasetEdit = lazy(() => import('./pages/DatasetEdit'));
const DatasourcePage = lazy(() => import('./pages/Datasource'));
const DatasourceDetailPage = lazy(() => import('./pages/DatasourceDetail'));

const { Header, Content, Footer } = Layout;
const { Title } = Typography;

const App: React.FC = () => {
  const intl = useIntl();
  const { locale, setLocale } = useLocale();
  const { preference, setMode } = useTheme();
  const themeOptions = [
    { value: 'light' as ThemeMode, label: intl.formatMessage({ id: 'theme.light' }) },
    { value: 'dark' as ThemeMode, label: intl.formatMessage({ id: 'theme.dark' }) },
    { value: 'system' as ThemeMode, label: intl.formatMessage({ id: 'theme.system' }) },
  ];
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(false);
  const location = useLocation();

  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 768);
    };
    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  useEffect(() => {
    setMobileMenuOpen(false);
  }, []);

  useEffect(() => {
    document.title = 'Data Insights';
  }, []);

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
    {
      key: '/account',
      icon: <UserOutlined />,
      label: <Link to="/account">{intl.formatMessage({ id: 'nav.account' })}</Link>,
    },
  ];

  return (
    <Layout className="layout" style={{ minHeight: '100vh' }}>
      <a href="#main-content" className="skip-link">
        Skip to main content
      </a>
      <Header
        style={{
          display: 'flex',
          alignItems: 'center',
          padding: isMobile ? '0 12px' : '0 16px',
          height: 48,
          lineHeight: '48px',
          position: 'sticky',
          top: 0,
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
            <Title level={4} style={{ margin: 0, marginLeft: 12 }}>
              Data Insights
            </Title>
          )}
        </div>
        {!isMobile ? (
          <>
            <Menu
              mode="horizontal"
              defaultSelectedKeys={['/']}
              selectedKeys={[location.pathname]}
              items={menuItems}
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

      {/* Mobile Menu Drawer */}
      <Drawer
        title={
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <div className="demo-logo" />
            <span style={{ marginLeft: 12, fontWeight: 'bold' }}>Data Insights</span>
          </div>
        }
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
          {/* 画布底色放在外壳而非各页：页面内容不足一屏时，下方露出的也是画布灰
              而不是白，页面骨架的"白面板浮在灰画布上"才不会在底部断掉。
              图表构建页自带三栏自绘底色，会完整覆盖这一层。 */}
          <Content id="main-content" style={{ background: 'var(--dr-canvas)', minHeight: 280 }}>
            <Suspense
              fallback={<LoadingPlaceholder text={intl.formatMessage({ id: 'common.loading' })} />}
            >
              <Routes>
                {/* 首页即仪表盘列表（导航项 nav.dashboard 一直指向 /）。 */}
                <Route path="/" element={<DashboardsPage />} />
                <Route path="/dashboards/:id" element={<DashboardEditor />} />
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
              </Routes>
            </Suspense>
          </Content>
        </Layout>
      </Layout>
      {/* 页脚融进画布：antd Footer 默认底色是暖灰 #f5f5f5，与冷灰画布不同源，
          会在地部多出一道色带。改为透明 + 一条上边线，只留"内容到此结束"的语义。 */}
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
        Data Insights ©2026 Created with React + Ant Design
      </Footer>
    </Layout>
  );
};

export default App;
