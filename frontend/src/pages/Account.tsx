import { CopyOutlined, DeleteOutlined, LogoutOutlined, PlusOutlined } from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  Modal,
  message,
  Segmented,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { type ApiUser, authApi, type PATCreateResult, type TokenInfo, tokensApi } from '../api';
import PageHeader from '../components/PageHeader';
import { setBearerToken } from '../lib/api/client';

const { Paragraph, Text } = Typography;

// 账号设置页（R-82 登录 / #184 PAT 管理）。会话 token 只存内存（见 client 的
// setBearerToken）：刷新即失，需重新登录——这是刻意的（人类短期令牌的存储红线）。
const AccountPage: React.FC = () => {
  const intl = useIntl();
  const [user, setUser] = useState<ApiUser | null>(null);
  const [pats, setPats] = useState<TokenInfo[]>([]);
  const [mode, setMode] = useState('login');
  const [authLoading, setAuthLoading] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [never, setNever] = useState(false);
  const [created, setCreated] = useState<PATCreateResult | null>(null);
  const [form] = Form.useForm<{ username: string; password: string }>();
  const [patForm] = Form.useForm<{ name: string }>();

  const loadPats = useCallback(async () => {
    const r = await tokensApi.list();
    setPats(r.data.data ?? []);
  }, []);

  useEffect(() => {
    authApi
      .me()
      .then((r) => setUser(r.data.data))
      .catch(() => setUser(null));
  }, []);

  const submitAuth = async () => {
    const v = await form.validateFields();
    setAuthLoading(true);
    try {
      const fn = mode === 'register' ? authApi.register : authApi.login;
      const r = await fn(v);
      const res = r.data.data;
      setBearerToken(res.token);
      setUser(res.user);
      message.success(intl.formatMessage({ id: 'account.welcome' }, { name: res.user.username }));
      await loadPats();
    } finally {
      setAuthLoading(false);
    }
  };

  const logout = async () => {
    try {
      await authApi.logout();
    } catch {
      // 尽力而为：撤销本地会话不依赖服务端返回成功。
    }
    setBearerToken('');
    setUser(null);
    setPats([]);
  };

  const createPat = async () => {
    const v = await patForm.validateFields();
    const r = await tokensApi.create({ name: v.name, never });
    setCreated(r.data.data);
    setCreateOpen(false);
    patForm.resetFields();
    setNever(false);
    await loadPats();
  };

  const revokePat = async (id: number) => {
    await tokensApi.revoke(id);
    message.success(intl.formatMessage({ id: 'account.revoked' }));
    await loadPats();
  };

  const columns: ColumnsType<TokenInfo> = [
    { title: intl.formatMessage({ id: 'account.col.name' }), dataIndex: 'name' },
    {
      title: intl.formatMessage({ id: 'account.col.token' }),
      dataIndex: 'prefix',
      render: (p: string) => <Text code>{p}…</Text>,
    },
    { title: intl.formatMessage({ id: 'account.col.created' }), dataIndex: 'created_at' },
    {
      title: intl.formatMessage({ id: 'account.col.lastUsed' }),
      render: (_: unknown, r) =>
        r.last_used_at ? r.last_used_at : intl.formatMessage({ id: 'account.never' }),
    },
    {
      title: intl.formatMessage({ id: 'account.col.expires' }),
      render: (_: unknown, r) =>
        r.expires_at ? r.expires_at : intl.formatMessage({ id: 'account.noExpiry' }),
    },
    {
      title: intl.formatMessage({ id: 'account.col.status' }),
      render: (_: unknown, r) =>
        r.revoked ? (
          <Tag color="red">{intl.formatMessage({ id: 'account.revokedTag' })}</Tag>
        ) : (
          <Tag color="green">{intl.formatMessage({ id: 'account.activeTag' })}</Tag>
        ),
    },
    {
      title: '',
      render: (_: unknown, r) =>
        r.revoked ? null : (
          <Button size="small" danger icon={<DeleteOutlined />} onClick={() => revokePat(r.id)} />
        ),
    },
  ];

  return (
    <div style={{ padding: 24 }}>
      <PageHeader title={intl.formatMessage({ id: 'account.title' })} />

      {!user ? (
        <Card style={{ maxWidth: 420 }}>
          <Segmented
            block
            value={mode}
            onChange={(v) => setMode(v as string)}
            options={[
              { label: intl.formatMessage({ id: 'account.login' }), value: 'login' },
              { label: intl.formatMessage({ id: 'account.register' }), value: 'register' },
            ]}
            style={{ marginBottom: 16 }}
          />
          {mode === 'register' && (
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              message={intl.formatMessage({ id: 'account.registerHint' })}
            />
          )}
          <Form form={form} layout="vertical" onFinish={submitAuth}>
            <Form.Item
              name="username"
              label={intl.formatMessage({ id: 'account.username' })}
              rules={[{ required: true }]}
            >
              <Input autoComplete="username" />
            </Form.Item>
            <Form.Item
              name="password"
              label={intl.formatMessage({ id: 'account.password' })}
              rules={[{ required: true }]}
            >
              <Input.Password autoComplete="current-password" />
            </Form.Item>
            <Button type="primary" block htmlType="submit" loading={authLoading}>
              {intl.formatMessage({
                id: mode === 'register' ? 'account.register' : 'account.login',
              })}
            </Button>
          </Form>
        </Card>
      ) : (
        <>
          <Space style={{ marginBottom: 16 }} wrap>
            <Text strong>{user.username}</Text>
            <Tag>{user.role}</Tag>
            <Button icon={<LogoutOutlined />} size="small" onClick={logout}>
              {intl.formatMessage({ id: 'account.logout' })}
            </Button>
          </Space>

          <Card
            title={intl.formatMessage({ id: 'account.patTitle' })}
            extra={
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                {intl.formatMessage({ id: 'account.patCreate' })}
              </Button>
            }
          >
            <Paragraph type="secondary">{intl.formatMessage({ id: 'account.patDesc' })}</Paragraph>
            <Table
              rowKey="id"
              columns={columns}
              dataSource={pats}
              pagination={false}
              size="small"
            />
          </Card>

          <Modal
            open={createOpen}
            title={intl.formatMessage({ id: 'account.patCreate' })}
            onOk={createPat}
            onCancel={() => setCreateOpen(false)}
            okText={intl.formatMessage({ id: 'account.create' })}
          >
            <Form form={patForm} layout="vertical">
              <Form.Item
                name="name"
                label={intl.formatMessage({ id: 'account.col.name' })}
                rules={[{ required: true }]}
              >
                <Input placeholder={intl.formatMessage({ id: 'account.patNamePlaceholder' })} />
              </Form.Item>
              <Form.Item label={intl.formatMessage({ id: 'account.patNever' })}>
                <Switch checked={never} onChange={setNever} />
              </Form.Item>
            </Form>
            {never && (
              <Alert
                type="warning"
                showIcon
                message={intl.formatMessage({ id: 'account.patNeverWarn' })}
              />
            )}
          </Modal>

          <Modal
            open={created !== null}
            title={intl.formatMessage({ id: 'account.patCreatedTitle' })}
            onCancel={() => setCreated(null)}
            onOk={() => setCreated(null)}
            okText={intl.formatMessage({ id: 'account.done' })}
            cancelButtonProps={{ style: { display: 'none' } }}
          >
            <Alert
              type="warning"
              showIcon
              message={intl.formatMessage({ id: 'account.patOnce' })}
              style={{ marginBottom: 12 }}
            />
            <Space>
              <Text code copyable={{ icon: <CopyOutlined /> }}>
                {created?.token}
              </Text>
            </Space>
          </Modal>
        </>
      )}
    </div>
  );
};

export default AccountPage;
