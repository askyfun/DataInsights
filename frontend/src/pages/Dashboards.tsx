import {
  DashboardOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { Button, Card, message, Popconfirm, Space, Table, Tag, Typography } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { useNavigate } from 'react-router-dom';
import { type Dashboard, dashboardsApi } from '../api';
import PageHeader from '../components/PageHeader';
import { migrateDashboardLayout } from '../lib/dashboardLayoutSchema';
import { formatDateTime } from '../lib/format';

const { Text } = Typography;

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
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchDashboards = useCallback(async () => {
    setLoading(true);
    try {
      const response = await dashboardsApi.getAll();
      setDashboards(response.data.data ?? []);
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setLoading(false);
    }
  }, [intl]);

  useEffect(() => {
    fetchDashboards();
  }, [fetchDashboards]);

  // 新建即进入画布页：空盘在列表里没有任何可操作的信息，多一次点击没有意义。
  const handleCreate = async () => {
    try {
      const response = await dashboardsApi.create({
        name: intl.formatMessage({ id: 'dashboard.untitled' }),
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

      <Card>
        <Table
          columns={columns}
          dataSource={Array.isArray(dashboards) ? dashboards : []}
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
    </div>
  );
};

export default DashboardsPage;
