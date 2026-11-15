import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Form, Input, Typography } from 'antd';
import { useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { authApi } from '../api';
import { setBearerToken } from '../lib/api/client';
import { setAuthConfig, useAuthConfig } from '../lib/authConfig';
import { setSessionUser } from '../lib/authSession';
import { safeNext } from '../lib/authz';

const { Title, Paragraph } = Typography;

// 登录 / 首启注册页（R-93 / #218）。按 /api/auth/config 的 needs_bootstrap 决定
// 呈现「初始化管理员」还是「登录」。
const LoginPage: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const cfg = useAuthConfig();
  const next = safeNext(params.get('next'));
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const needsBootstrap = cfg?.needs_bootstrap === true;

  // 直接打开 /login 时兜底探测一次配置（App 挂载也会探测）。
  useEffect(() => {
    if (cfg === null) {
      authApi
        .config()
        .then((r) => setAuthConfig(r.data.data))
        .catch(() => setAuthConfig({ enforce_mode: 'off', needs_bootstrap: false }));
    }
  }, [cfg]);

  const onFinish = async (v: { username: string; password: string; bootstrap_token?: string }) => {
    setLoading(true);
    setError(null);
    try {
      const r = needsBootstrap
        ? await authApi.register({
            username: v.username,
            password: v.password,
            bootstrap_token: v.bootstrap_token ?? '',
          })
        : await authApi.login({ username: v.username, password: v.password });
      setBearerToken(r.data.data.token);
      setSessionUser(r.data.data.user);
      navigate(next, { replace: true });
    } catch (e) {
      setError(e instanceof Error ? e.message : intl.formatMessage({ id: 'login.failed' }));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ display: 'flex', justifyContent: 'center', padding: '48px 16px' }}>
      <Card style={{ width: 380, maxWidth: '100%' }}>
        <Title level={4} style={{ marginTop: 0 }}>
          {needsBootstrap
            ? intl.formatMessage({ id: 'login.registerTitle' })
            : intl.formatMessage({ id: 'login.title' })}
        </Title>
        {needsBootstrap && (
          <Paragraph type="secondary" style={{ fontSize: 12 }}>
            {intl.formatMessage({ id: 'login.registerHint' })}
          </Paragraph>
        )}
        {error && <Alert type="error" showIcon message={error} style={{ marginBottom: 12 }} />}
        <Form layout="vertical" onFinish={onFinish} requiredMark={false}>
          <Form.Item
            name="username"
            label={intl.formatMessage({ id: 'login.username' })}
            rules={[{ required: true }]}
          >
            <Input prefix={<UserOutlined />} autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            label={intl.formatMessage({ id: 'login.password' })}
            rules={[{ required: true }]}
          >
            <Input.Password prefix={<LockOutlined />} autoComplete="current-password" />
          </Form.Item>
          {needsBootstrap && (
            <Form.Item
              name="bootstrap_token"
              label={intl.formatMessage({ id: 'login.bootstrapToken' })}
            >
              <Input />
            </Form.Item>
          )}
          <Button type="primary" htmlType="submit" block loading={loading}>
            {needsBootstrap
              ? intl.formatMessage({ id: 'login.register' })
              : intl.formatMessage({ id: 'login.submit' })}
          </Button>
        </Form>
      </Card>
    </div>
  );
};

export default LoginPage;
