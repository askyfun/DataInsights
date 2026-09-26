import {
  DashboardOutlined,
  DeleteOutlined,
  EditOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { App, Button, Card, Layout, Popconfirm, Space, Table, Tag, Typography } from 'antd';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useIntl } from 'react-intl';
import { useNavigate } from 'react-router-dom';
import { type Dashboard, type DashboardFolder, dashboardFoldersApi, dashboardsApi } from '../api';
import DashboardFolderTree from '../components/DashboardFolderTree';
import PageHeader from '../components/PageHeader';
import { type FolderSelection, filterDashboardsBySelection } from '../lib/dashboardFolderTree';
import { migrateDashboardLayout } from '../lib/dashboardLayoutSchema';
import { formatDateTime } from '../lib/format';

const { Text } = Typography;
const { Content, Sider } = Layout;

/**
 * 盘内图表块数量。
 *
 * `layout_json` 是不可信输入（手工改过 / 旧结构 / 空串都会出现），一律经
 * `migrateDashboardLayout` 解析后再计数——直接 `JSON.parse` 遇到坏串会抛异常把整页带崩。
 */
const countChartBlocks = (layoutJson: string): number =>
  migrateDashboardLayout(layoutJson).widgets.filter((widget) => widget.type === 'chart').length;

const DashboardsPage: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  // message 取 App 上下文实例，不用静态 message.*（静态方法读不到 ConfigProvider 的
  // 主题上下文）。依赖根部的 <App> 包裹，见 main.tsx。该实例跨渲染稳定（antd 内部
  // 以 useMemo([], …) 产出），故放进下方依赖数组不会引起 effect 循环。
  const { message } = App.useApp();
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [folders, setFolders] = useState<DashboardFolder[]>([]);
  const [loading, setLoading] = useState(true);
  // 归档视图的选中项：root = 全部（不过滤）。第一期树上标的是「直接子项数」，
  // 所以选中某夹时列表也只显示该夹的直属仪表盘（不含子夹里的）——两边同一条口径。
  const [selection, setSelection] = useState<FolderSelection>({ kind: 'root' });
  // 归档树默认展开（占位整列），可向左折叠。
  const [collapsed, setCollapsed] = useState(false);

  const fetchDashboards = useCallback(async () => {
    setLoading(true);
    try {
      // 两件事一起取：树要夹列表、列表要盘，缺一个树就长不全。
      const [dashboardsResponse, foldersResponse] = await Promise.all([
        dashboardsApi.getAll(),
        dashboardFoldersApi.getAll(),
      ]);
      setDashboards(dashboardsResponse.data.data ?? []);
      setFolders(foldersResponse.data.data ?? []);
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setLoading(false);
    }
  }, [intl, message]);

  useEffect(() => {
    fetchDashboards();
  }, [fetchDashboards]);

  // 新建即进入画布页：空盘在列表里没有任何可操作的信息，多一次点击没有意义。
  // 选中了某个文件夹时，新盘直接归档到它下面（否则用户建完还要再拖一次）。
  const handleCreate = async () => {
    try {
      const response = await dashboardsApi.create({
        name: intl.formatMessage({ id: 'dashboard.untitled' }),
        folder_id: selection.kind === 'folder' ? selection.id : undefined,
      });
      navigate(`/dashboards/${response.data.data.id}`);
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await dashboardsApi.remove(id);
      message.success(intl.formatMessage({ id: 'common.success' }));
      await fetchDashboards();
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    }
  };

  // ---- 归档文件夹的三个写操作（树的 UI 在 DashboardFolderTree，这里只管落库与刷新）

  const handleCreateFolder = useCallback(
    async (parentId: string | null, name: string): Promise<boolean> => {
      try {
        await dashboardFoldersApi.create({ name, parent_id: parentId ?? undefined });
        message.success(intl.formatMessage({ id: 'common.success' }));
        await fetchDashboards();
        return true;
      } catch (error: any) {
        message.error(error.message || intl.formatMessage({ id: 'common.error' }));
        return false;
      }
    },
    [fetchDashboards, intl, message]
  );

  const handleRenameFolder = useCallback(
    async (id: string, name: string): Promise<boolean> => {
      try {
        // 只发改名：parent_id 缺省 = 「保留存量」（PUT 的未提供则保留约定）。
        await dashboardFoldersApi.update(id, { name });
        message.success(intl.formatMessage({ id: 'common.success' }));
        await fetchDashboards();
        return true;
      } catch (error: any) {
        message.error(error.message || intl.formatMessage({ id: 'common.error' }));
        return false;
      }
    },
    [fetchDashboards, intl, message]
  );

  const handleDeleteFolder = useCallback(
    async (id: string): Promise<void> => {
      try {
        await dashboardFoldersApi.remove(id);
        message.success(intl.formatMessage({ id: 'common.success' }));
        // 删的正是当前选中的夹：留在原选中项上会让列表过滤到一个已不存在的 id，
        // 页面变成「什么都没有」且无从解释，所以先回根视图。
        if (selection.kind === 'folder' && selection.id === id) {
          setSelection({ kind: 'root' });
        }
        await fetchDashboards();
      } catch (error: any) {
        // 「非空不可删」是后端 20400，消息里带着还剩几项，直接念给用户。
        message.error(error.message || intl.formatMessage({ id: 'common.error' }));
      }
    },
    [fetchDashboards, intl, message, selection]
  );

  const handleMoveDashboard = useCallback(
    async (dashboardId: string, folderId: string | null): Promise<void> => {
      const previous = dashboards;
      // 乐观更新：拖拽的落点必须当场可见，等服务端往返会让用户以为没拖上。
      setDashboards(
        dashboards.map((d) => (d.id === dashboardId ? { ...d, folder_id: folderId } : d))
      );
      try {
        await dashboardsApi.update(dashboardId, { folder_id: folderId ?? '' });
        message.success(intl.formatMessage({ id: 'dashboard.folderMoved' }));
        await fetchDashboards();
      } catch (error: any) {
        setDashboards(previous); // 失败就把拖过去的那一格还原，别留下假象
        message.error(error.message || intl.formatMessage({ id: 'common.error' }));
      }
    },
    [dashboards, fetchDashboards, intl, message]
  );

  // 移动文件夹 = 改 parent_id。null 走空串哨兵（「移到根级」的唯一表达）。
  const handleMoveFolder = useCallback(
    async (folderId: string, parentId: string | null): Promise<void> => {
      const previous = folders;
      setFolders(folders.map((f) => (f.id === folderId ? { ...f, parent_id: parentId } : f)));
      try {
        await dashboardFoldersApi.update(folderId, { parent_id: parentId ?? '' });
        message.success(intl.formatMessage({ id: 'dashboard.folderMoved' }));
        await fetchDashboards();
      } catch (error: any) {
        setFolders(previous);
        message.error(error.message || intl.formatMessage({ id: 'common.error' }));
      }
    },
    [fetchDashboards, folders, intl, message]
  );

  const visibleDashboards = useMemo(
    () => filterDashboardsBySelection(dashboards, selection),
    [dashboards, selection]
  );

  const columns = [
    {
      title: intl.formatMessage({ id: 'dashboard.name' }),
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: Dashboard) => (
        <Space>
          <DashboardOutlined />
          <Button
            type="link"
            style={{ padding: 0 }}
            onClick={() => navigate(`/dashboards/${record.id}`)}
          >
            <Text strong>{text}</Text>
          </Button>
        </Space>
      ),
    },
    {
      title: intl.formatMessage({ id: 'dashboard.description' }),
      dataIndex: 'description',
      key: 'description',
      render: (text: string | null) =>
        text ? <Text type="secondary">{text}</Text> : <Text type="secondary">—</Text>,
    },
    {
      title: intl.formatMessage({ id: 'dashboard.chartBlocks' }),
      key: 'chartBlocks',
      width: 110,
      render: (_: unknown, record: Dashboard) => (
        <Tag color="blue">{countChartBlocks(record.layout_json)}</Tag>
      ),
    },
    {
      title: intl.formatMessage({ id: 'dashboard.updatedAt' }),
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 180,
      render: (text: string) => formatDateTime(text),
    },
    {
      title: intl.formatMessage({ id: 'dashboard.actions' }),
      key: 'actions',
      width: 170,
      render: (_: unknown, record: Dashboard) => (
        <Space size="small">
          <Button
            type="link"
            size="small"
            icon={<EditOutlined />}
            onClick={() => navigate(`/dashboards/${record.id}`)}
          >
            {intl.formatMessage({ id: 'common.edit' })}
          </Button>
          <Popconfirm
            title={intl.formatMessage({ id: 'dashboard.deleteConfirm' })}
            onConfirm={() => handleDelete(record.id)}
            okText={intl.formatMessage({ id: 'common.yes' })}
            cancelText={intl.formatMessage({ id: 'common.no' })}
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>
              {intl.formatMessage({ id: 'common.delete' })}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className="dr-page">
      <PageHeader
        icon={<DashboardOutlined />}
        title={intl.formatMessage({ id: 'nav.dashboard' })}
        description={intl.formatMessage({ id: 'dashboard.manage' })}
        extra={
          <>
            <Button icon={<ReloadOutlined />} onClick={() => fetchDashboards()} loading={loading}>
              {intl.formatMessage({ id: 'common.refresh' })}
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
              {intl.formatMessage({ id: 'dashboard.add' })}
            </Button>
          </>
        }
      />

      <Layout style={{ background: 'transparent' }}>
        <Sider
          width={260}
          collapsedWidth={0}
          collapsible
          collapsed={collapsed}
          trigger={null}
          style={{
            background: 'transparent',
            paddingRight: collapsed ? 0 : 12,
          }}
        >
          <Card
            size="small"
            title={intl.formatMessage({ id: 'nav.dashboard' })}
            extra={
              <Button
                type="text"
                size="small"
                icon={<MenuFoldOutlined />}
                onClick={() => setCollapsed(true)}
                aria-label={intl.formatMessage({ id: 'dashboard.folderCollapse' })}
              />
            }
            styles={{ body: { maxHeight: 'calc(100vh - 220px)', overflow: 'auto' } }}
          >
            <DashboardFolderTree
              folders={folders}
              dashboards={dashboards}
              selection={selection}
              onSelect={setSelection}
              onCreateFolder={handleCreateFolder}
              onRenameFolder={handleRenameFolder}
              onDeleteFolder={handleDeleteFolder}
              onMoveDashboard={handleMoveDashboard}
              onMoveFolder={handleMoveFolder}
            />
          </Card>
        </Sider>

        <Content>
          {collapsed && (
            <Button
              icon={<MenuUnfoldOutlined />}
              onClick={() => setCollapsed(false)}
              aria-label={intl.formatMessage({ id: 'dashboard.folderExpand' })}
              style={{ marginBottom: 8 }}
            >
              {intl.formatMessage({ id: 'dashboard.folderExpand' })}
            </Button>
          )}
          <Card>
            <Table
              columns={columns}
              dataSource={Array.isArray(visibleDashboards) ? visibleDashboards : []}
              rowKey="id"
              loading={loading}
              pagination={{
                pageSize: 10,
                showSizeChanger: true,
                showTotal: (total) => intl.formatMessage({ id: 'common.totalItems' }, { total }),
              }}
              locale={{ emptyText: intl.formatMessage({ id: 'common.noData' }) }}
              size="small"
            />
          </Card>
        </Content>
      </Layout>
    </div>
  );
};

export default DashboardsPage;
