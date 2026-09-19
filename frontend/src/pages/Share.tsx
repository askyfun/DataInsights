import { CopyOutlined, DeleteOutlined, PlusOutlined, ShareAltOutlined } from '@ant-design/icons';
import {
  Button,
  Card,
  DatePicker,
  Empty,
  Form,
  Input,
  Modal,
  message,
  Popconfirm,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from 'antd';
import dayjs, { Dayjs } from 'dayjs';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Share, sharesApi } from '../api';
import PageHeader from '../components/PageHeader';
import { formatDateTime } from '../lib/format';
import { useStore } from '../store';

const { Text } = Typography;
const { RangePicker } = DatePicker;

interface ShareWithChartName extends Share {
  chartName?: string;
}

const SharePage: React.FC = () => {
  const intl = useIntl();
  const [shares, setShares] = useState<ShareWithChartName[]>([]);
  const [loading, setLoading] = useState(false);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [createLoading, setCreateLoading] = useState(false);
  const [shareLink, setShareLink] = useState<string | null>(null);
  const [form] = Form.useForm();

  const { charts, fetchCharts } = useStore();

  // Fetch shares from API
  const fetchShares = useCallback(async () => {
    setLoading(true);
    try {
      const response = await sharesApi.getAll();
      const currentCharts = useStore.getState().charts;
      // Enrich shares with chart names
      const enrichedShares = response.data.data.map((share: Share) => {
        const chart = currentCharts.find((c) => c.id === share.chart_id);
        return {
          ...share,
          chartName: chart?.name || `Chart #${share.chart_id}`,
        };
      });
      setShares(enrichedShares);
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'share.fetchFailed' }));
    } finally {
      setLoading(false);
    }
  }, [intl]);

  // Fetch shares and charts on mount
  useEffect(() => {
    fetchCharts();
    fetchShares();
  }, [fetchCharts, fetchShares]);

  // Create share
  const handleCreateShare = async (values: any) => {
    setCreateLoading(true);
    try {
      const data: any = {
        chart_id: values.chart_id,
      };

      if (values.password) {
        data.password = values.password;
      }

      if (values.expiry) {
        data.expires_at = values.expiry[1].toISOString();
      }

      const response = await sharesApi.create(data);
      const token = response.data.data.token;

      // Generate share link
      const link = `${window.location.origin}/share/${token}`;
      setShareLink(link);
      message.success(intl.formatMessage({ id: 'share.shareCreated' }));

      // Refresh shares list
      fetchShares();

      // Close modal but keep link visible
      form.resetFields();
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'share.createFailed' }));
    } finally {
      setCreateLoading(false);
    }
  };

  // Delete share
  const handleDeleteShare = async (token: string) => {
    try {
      // Note: There's no delete API, so we'll just remove from local list
      // This would need a backend API endpoint to actually delete
      setShares(shares.filter((s) => s.token !== token));
      message.success(intl.formatMessage({ id: 'share.shareRemoved' }));
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'share.deleteFailed' }));
    }
  };

  // Copy link to clipboard
  const handleCopyLink = (token: string) => {
    const link = `${window.location.origin}/share/${token}`;
    navigator.clipboard.writeText(link);
    message.success(intl.formatMessage({ id: 'share.linkCopied' }));
  };

  // Close share link modal
  const handleCloseLinkModal = () => {
    setShareLink(null);
    setCreateModalVisible(false);
  };

  // Table columns
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 60,
    },
    {
      title: intl.formatMessage({ id: 'share.chart' }),
      dataIndex: 'chartName',
      key: 'chartName',
      render: (name: string) => (
        <Space>
          <ShareAltOutlined />
          <Text strong>{name}</Text>
        </Space>
      ),
    },
    {
      title: intl.formatMessage({ id: 'share.token' }),
      dataIndex: 'token',
      key: 'token',
      render: (token: string) => (
        <Text code style={{ fontSize: 12 }}>
          {token.substring(0, 12)}...
        </Text>
      ),
    },
    {
      title: intl.formatMessage({ id: 'share.expire' }),
      dataIndex: 'expires_at',
      key: 'expires_at',
      render: (expiresAt: string) =>
        expiresAt ? (
          <Text type="secondary">{new Date(expiresAt).toLocaleDateString()}</Text>
        ) : (
          <Tag color="blue">{intl.formatMessage({ id: 'share.never' })}</Tag>
        ),
    },
    {
      title: intl.formatMessage({ id: 'chart.createdAt' }),
      dataIndex: 'created_at',
      key: 'created_at',
      render: (createdAt: string) =>
        createdAt ? (
          <Text type="secondary">{formatDateTime(createdAt)}</Text>
        ) : (
          <Text type="secondary">-</Text>
        ),
    },
    {
      title: intl.formatMessage({ id: 'chart.actions' }),
      key: 'actions',
      render: (_: any, record: ShareWithChartName) => (
        <Space>
          <Button type="text" icon={<CopyOutlined />} onClick={() => handleCopyLink(record.token)}>
            {intl.formatMessage({ id: 'share.copyLinkBtn' })}
          </Button>
          <Popconfirm
            title={intl.formatMessage({ id: 'share.deleteConfirm' })}
            onConfirm={() => handleDeleteShare(record.token)}
            okText={intl.formatMessage({ id: 'common.yes' })}
            cancelText={intl.formatMessage({ id: 'common.no' })}
          >
            <Button type="text" danger icon={<DeleteOutlined />}>
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
        icon={<ShareAltOutlined />}
        title={intl.formatMessage({ id: 'share.title' })}
        extra={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => setCreateModalVisible(true)}
          >
            {intl.formatMessage({ id: 'share.create' })}
          </Button>
        }
      />

      <Card>
        {loading ? (
          <div className="dr-state">
            <Spin />
          </div>
        ) : shares.length === 0 ? (
          <div className="dr-state">
            <Empty
              description={intl.formatMessage({ id: 'share.noShares' })}
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            />
          </div>
        ) : (
          <Table
            columns={columns}
            dataSource={shares}
            rowKey="id"
            pagination={{ pageSize: 10 }}
            size="small"
          />
        )}
      </Card>

      {/* Create Share Modal */}
      <Modal
        title={intl.formatMessage({ id: 'share.createShare' })}
        open={createModalVisible}
        onCancel={handleCloseLinkModal}
        footer={null}
        destroyOnHidden
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={handleCreateShare}
          initialValues={{ chart_id: undefined, password: undefined }}
        >
          <Form.Item
            name="chart_id"
            label={intl.formatMessage({ id: 'share.selectChart' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'share.pleaseSelectChart' }) },
            ]}
          >
            <Select
              placeholder={intl.formatMessage({ id: 'share.pleaseSelectChart' })}
              options={charts.map((chart) => ({
                value: chart.id,
                label: chart.name,
              }))}
              loading={!charts.length}
            />
          </Form.Item>

          <Form.Item
            name="password"
            label={intl.formatMessage({ id: 'share.passwordOptional' })}
            tooltip={intl.formatMessage({ id: 'share.setPassword' })}
          >
            <Input.Password placeholder={intl.formatMessage({ id: 'share.enterPassword' })} />
          </Form.Item>

          <Form.Item
            name="expiry"
            label={intl.formatMessage({ id: 'share.expiryDateOptional' })}
            tooltip={intl.formatMessage({ id: 'share.setExpiryDate' })}
          >
            <RangePicker
              showTime
              style={{ width: '100%' }}
              disabledDate={(current: Dayjs | null) =>
                current ? current.isBefore(dayjs()) : false
              }
            />
          </Form.Item>

          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button onClick={() => setCreateModalVisible(false)}>
                {intl.formatMessage({ id: 'common.cancel' })}
              </Button>
              <Button type="primary" htmlType="submit" loading={createLoading}>
                {intl.formatMessage({ id: 'share.createBtn' })}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      {/* Share Link Modal */}
      <Modal
        title={intl.formatMessage({ id: 'share.shareCreated' })}
        open={!!shareLink}
        onCancel={handleCloseLinkModal}
        footer={[
          <Button key="close" type="primary" onClick={handleCloseLinkModal}>
            {intl.formatMessage({ id: 'share.done' })}
          </Button>,
        ]}
      >
        <div style={{ textAlign: 'center', padding: '20px 0' }}>
          <ShareAltOutlined style={{ fontSize: 48, color: '#52c41a', marginBottom: 16 }} />
          <Typography.Title level={4}>
            {intl.formatMessage({ id: 'share.shareCreated' })}
          </Typography.Title>
          <Text type="secondary">{intl.formatMessage({ id: 'share.useLinkBelow' })}</Text>
          <div style={{ marginTop: 16 }}>
            <Input.Group compact>
              <Input style={{ width: 'calc(100% - 100px)' }} value={shareLink || ''} readOnly />
              <Button
                type="primary"
                icon={<CopyOutlined />}
                onClick={() => shareLink && navigator.clipboard.writeText(shareLink)}
              >
                {intl.formatMessage({ id: 'share.copyLink' })}
              </Button>
            </Input.Group>
          </div>
        </div>
      </Modal>
    </div>
  );
};

export default SharePage;
