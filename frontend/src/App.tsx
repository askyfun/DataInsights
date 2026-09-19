import {
  AppstoreOutlined,
  BarChartOutlined,
  BuildOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  GlobalOutlined,
  MenuOutlined,
} from '@ant-design/icons';
import { Button, Drawer, Layout, Menu, Select, Space, Typography } from 'antd';
import { useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, Route, Routes, useLocation } from 'react-router-dom';
import { useLocale } from './i18n/useLocale';
import ChartBuilder from './pages/ChartBuilder';
import ChartsPage from './pages/Charts';
import DatasetPage from './pages/Dataset';
import DatasetDetail from './pages/DatasetDetail';
import DatasetEdit from './pages/DatasetEdit';
import DatasourcePage from './pages/Datasource';
import DatasourceDetailPage from './pages/DatasourceDetail';
import SharePage from './pages/Share';
import ShareView from './pages/ShareView';

const { Header, Content, Footer } = Layout;
const { Title } = Typography;

const App: React.FC = () => {
  const intl = useIntl();
  const { locale, setLocale } = useLocale();
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
    document.title = 'DataRay';
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
          background: '#fff',
          borderBottom: '1px solid #f0f0f0',
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
              DataRay
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
            </Space>
          </>
        ) : null}
      </Header>

      {/* Mobile Menu Drawer */}
      <Drawer
        title={
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <div className="demo-logo" />
            <span style={{ marginLeft: 12, fontWeight: 'bold' }}>DataRay</span>
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
        <div style={{ padding: '16px', borderTop: '1px solid #f0f0f0' }}>
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
        </div>
      </Drawer>

      <Layout>
        <Layout style={{ padding: '0' }}>
          <Content id="main-content" style={{ background: '#fff', minHeight: 280 }}>
            <Routes>
              <Route
                path="/"
                element={
                  <div style={{ padding: 24 }}>{intl.formatMessage({ id: 'home.welcome' })}</div>
                }
              />
              <Route path="/datasources" element={<DatasourcePage />} />
              <Route path="/datasources/:id" element={<DatasourceDetailPage />} />
              <Route path="/datasets" element={<DatasetPage />} />
              <Route path="/datasets/new" element={<DatasetEdit />} />
              <Route path="/datasets/:id" element={<DatasetDetail />} />
              <Route path="/datasets/:id/edit" element={<DatasetEdit />} />
              <Route path="/chart-builder" element={<ChartBuilder />} />
              <Route path="/charts" element={<ChartsPage />} />
              <Route path="/shares" element={<SharePage />} />
              <Route path="/share/:token" element={<ShareView />} />
            </Routes>
          </Content>
        </Layout>
      </Layout>
      <Footer style={{ textAlign: 'center' }}>DataRay ©2026 Created with React + Ant Design</Footer>
    </Layout>
  );
};

export default App;
