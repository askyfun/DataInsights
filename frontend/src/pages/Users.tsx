import { PlusOutlined, ReloadOutlined, TeamOutlined } from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  Modal,
  message,
  Popconfirm,
  Result,
  Select,
  Space,
  Table,
  Tag,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import {
  type ApiUser,
  authApi,
  type ListQuery,
  type UserCreateInput,
  type UserRole,
  usersApi,
} from '../api';
import { ListSearch, useDebouncedValue, useServerList } from '../components/listPage';
import ModalFooter from '../components/ModalFooter';
import PageHeader from '../components/PageHeader';
import { formatDateTime } from '../lib/format';

// 用户管理（issue #202）：管理员建号 / 改显示名与角色 / 禁用 / 重置口令。
// 开放注册保持关闭——这里是新增账号的唯一入口。非管理员不渲染任何内容
// （不做只读壳），菜单入口在 App 侧也按角色收敛。
const UsersPage: React.FC = () => {
  const intl = useIntl();
  const [me, setMe] = useState<ApiUser | null>(null);
  const [checked, setChecked] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm] = Form.useForm<UserCreateInput>();
  const [resetTarget, setResetTarget] = useState<ApiUser | null>(null);
  const [resetForm] = Form.useForm<{ password: string }>();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    authApi
      .me()
      .then((r) => setMe(r.data.data))
      .catch(() => setMe(null))
      .finally(() => setChecked(true));
  }, []);

  const fetchPage = useCallback(
    (query: ListQuery) => usersApi.getAll(query).then((res) => res.data.data),
    []
  );
  const list = useServerList<ApiUser>(fetchPage);
  const keyword = useDebouncedValue(searchText).trim();

  useEffect(() => {
    if (keyword === (list.query.keyword ?? '')) return;
    list.setParam({ keyword: keyword || undefined });
  }, [keyword, list.query.keyword, list.setParam]);

  useEffect(() => {
    if (list.error) {
      message.error(list.error);
    }
  }, [list.error]);

  const isAdmin = me?.role === 'admin';

  const createUser = async () => {
    const values = await createForm.validateFields();
    setSaving(true);
    try {
      await usersApi.create(values);
      message.success(intl.formatMessage({ id: 'common.success' }));
      setCreateOpen(false);
      createForm.resetFields();
      list.reload();
    } finally {
      setSaving(false);
    }
  };

  const patchUser = async (u: ApiUser, patch: { role?: UserRole; disabled?: boolean }) => {
    try {
      await usersApi.update(u.id, patch);
      message.success(intl.formatMessage({ id: 'common.success' }));
      list.reload();
    } catch (err) {
      message.error(
        err instanceof Error ? err.message : intl.formatMessage({ id: 'common.error' })
      );
    }
  };

  const resetPassword = async () => {
    if (!resetTarget) return;
    const { password } = await resetForm.validateFields();
    setSaving(true);
    try {
      await usersApi.resetPassword(resetTarget.id, password);
      message.success(intl.formatMessage({ id: 'users.resetDone' }));
      setResetTarget(null);
      resetForm.resetFields();
    } finally {
      setSaving(false);
    }
  };

  const columns: ColumnsType<ApiUser> = [
    { title: intl.formatMessage({ id: 'users.col.username' }), dataIndex: 'username' },
    {
      title: intl.formatMessage({ id: 'users.col.displayName' }),
      dataIndex: 'display_name',
      render: (v: string) => v || '-',
    },
    {
      title: intl.formatMessage({ id: 'users.col.role' }),
      dataIndex: 'role',
      render: (role: string, r) => (
        <Select<UserRole>
          size="small"
          value={role === 'admin' ? 'admin' : 'user'}
          style={{ width: 120 }}
          disabled={r.id === me?.id}
          onChange={(next) => patchUser(r, { role: next })}
          options={[
            { value: 'admin', label: intl.formatMessage({ id: 'users.role.admin' }) },
            { value: 'user', label: intl.formatMessage({ id: 'users.role.user' }) },
          ]}
        />
      ),
    },
    {
      title: intl.formatMessage({ id: 'users.col.status' }),
      dataIndex: 'disabled',
      render: (disabled: boolean) =>
        disabled ? (
          <Tag color="red">{intl.formatMessage({ id: 'users.disabled' })}</Tag>
        ) : (
          <Tag color="green">{intl.formatMessage({ id: 'users.enabled' })}</Tag>
        ),
    },
    {
      title: intl.formatMessage({ id: 'users.col.created' }),
      dataIndex: 'created_at',
      render: (v: string) => formatDateTime(v),
    },
    {
      title: intl.formatMessage({ id: 'users.col.actions' }),
      render: (_: unknown, r) => (
        <Space>
          <Button size="small" onClick={() => setResetTarget(r)}>
            {intl.formatMessage({ id: 'users.resetPassword' })}
          </Button>
          {r.id === me?.id ? null : r.disabled ? (
            <Button size="small" onClick={() => patchUser(r, { disabled: false })}>
              {intl.formatMessage({ id: 'users.enable' })}
            </Button>
          ) : (
            <Popconfirm
              title={intl.formatMessage({ id: 'users.confirmDisable' })}
              onConfirm={() => patchUser(r, { disabled: true })}
            >
              <Button size="small" danger>
                {intl.formatMessage({ id: 'users.disable' })}
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ];

  if (checked && !isAdmin) {
    return (
      <div className="dr-page">
        <Result
          status="403"
          title={intl.formatMessage({ id: 'users.adminOnlyTitle' })}
          subTitle={intl.formatMessage({ id: 'users.adminOnlyDesc' })}
        />
      </div>
    );
  }

  return (
    <div className="dr-page">
      <PageHeader
        icon={<TeamOutlined />}
        title={intl.formatMessage({ id: 'users.title' })}
        description={intl.formatMessage({ id: 'users.desc' })}
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            {intl.formatMessage({ id: 'users.create' })}
          </Button>
        }
      />
      <Card>
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Alert
            type="warning"
            showIcon
            message={intl.formatMessage({ id: 'users.securityHint' })}
          />
          <ListSearch
            placeholder={intl.formatMessage({ id: 'users.searchPlaceholder' })}
            value={searchText}
            onChange={setSearchText}
            extra={
              <Button icon={<ReloadOutlined />} onClick={list.reload}>
                {intl.formatMessage({ id: 'common.refresh' })}
              </Button>
            }
          />
          <Table
            rowKey="id"
            columns={columns}
            dataSource={list.items}
            loading={list.loading}
            pagination={list.pagination}
            onChange={list.onTableChange}
            size="small"
          />
        </Space>
      </Card>

      <Modal
        open={createOpen}
        title={intl.formatMessage({ id: 'users.create' })}
        onCancel={() => setCreateOpen(false)}
        footer={
          <ModalFooter>
            <Button onClick={() => setCreateOpen(false)}>
              {intl.formatMessage({ id: 'common.cancel' })}
            </Button>
            <Button type="primary" onClick={createUser} loading={saving}>
              {intl.formatMessage({ id: 'common.create' })}
            </Button>
          </ModalFooter>
        }
        destroyOnHidden
      >
        <Alert
          type="info"
          showIcon
          message={intl.formatMessage({ id: 'users.createHint' })}
          style={{ marginBottom: 16 }}
        />
        <Form form={createForm} layout="vertical" initialValues={{ role: 'user' }}>
          <Form.Item
            name="username"
            label={intl.formatMessage({ id: 'users.col.username' })}
            rules={[{ required: true }]}
          >
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="display_name"
            label={intl.formatMessage({ id: 'users.col.displayName' })}
          >
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="password"
            label={intl.formatMessage({ id: 'users.initialPassword' })}
            rules={[{ required: true }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="role" label={intl.formatMessage({ id: 'users.col.role' })}>
            <Select
              options={[
                { value: 'user', label: intl.formatMessage({ id: 'users.role.user' }) },
                { value: 'admin', label: intl.formatMessage({ id: 'users.role.admin' }) },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={resetTarget !== null}
        title={intl.formatMessage(
          { id: 'users.resetPasswordFor' },
          {
            name: resetTarget?.username ?? '',
          }
        )}
        onCancel={() => {
          setResetTarget(null);
          resetForm.resetFields();
        }}
        footer={
          <ModalFooter>
            <Button
              onClick={() => {
                setResetTarget(null);
                resetForm.resetFields();
              }}
            >
              {intl.formatMessage({ id: 'common.cancel' })}
            </Button>
            <Button type="primary" onClick={resetPassword} loading={saving}>
              {intl.formatMessage({ id: 'common.confirm' })}
            </Button>
          </ModalFooter>
        }
        destroyOnHidden
      >
        <Alert
          type="warning"
          showIcon
          message={intl.formatMessage({ id: 'users.resetHint' })}
          style={{ marginBottom: 16 }}
        />
        <Form form={resetForm} layout="vertical">
          <Form.Item
            name="password"
            label={intl.formatMessage({ id: 'users.newPassword' })}
            rules={[{ required: true }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default UsersPage;
