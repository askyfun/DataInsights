import {
  BarChartOutlined,
  DeleteOutlined,
  EditOutlined,
  LineChartOutlined,
  PieChartOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { Button, Card, message, Popconfirm, Space, Table, Tag, Typography } from 'antd';
import { useEffect } from 'react';
import { useIntl } from 'react-intl';
import { useNavigate } from 'react-router-dom';
import { type ChartType, migrateChartConfig } from '../lib/chartConfigSchema';
import { useStore } from '../store';

const { Title, Text } = Typography;

const ChartsPage: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const {
    charts,
    chartsLoading,
    fetchCharts,
    deleteChart,
    chartsError,
    setChartBuilderConfig,
    resetChartBuilder,
    datasets,
    fetchDatasets,
  } = useStore();

  // Fetch charts and datasets on mount
  useEffect(() => {
    fetchCharts();
    fetchDatasets();
  }, [fetchCharts, fetchDatasets]);

  // Show error message
  useEffect(() => {
    if (chartsError) {
      message.error(chartsError);
    }
  }, [chartsError]);

  // Handle delete chart
  const handleDelete = async (id: number) => {
    try {
      await deleteChart(id);
      message.success(intl.formatMessage({ id: 'common.success' }));
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    }
  };

  // Handle edit chart - navigate to ChartBuilder with chart data
  const handleEdit = (record: any) => {
    // Reset chart builder and navigate to ChartBuilder
    resetChartBuilder();

    // v1 契约下 xAxisField/yAxisFields 已废弃，只需迁移读取 chartType/title
    const doc = migrateChartConfig(record.config || '', record.chart_type as ChartType);
    setChartBuilderConfig({
      chartType: doc.chartType,
      title: doc.title || record.name,
    });

    // Navigate to ChartBuilder page with query param to load the chart
    navigate(`/chart-builder?edit=${record.id}&datasetId=${record.dataset_id}`);
  };

  // Get chart type icon
  const getChartTypeIcon = (chartType: string) => {
    switch (chartType) {
      case 'line':
        return <LineChartOutlined />;
      case 'pie':
        return <PieChartOutlined />;
      default:
        return <BarChartOutlined />;
    }
  };

  // Get chart type tag color
  const getChartTypeColor = (chartType: string) => {
    switch (chartType) {
      case 'line':
        return 'green';
      case 'pie':
        return 'orange';
      default:
        return 'blue';
    }
  };

  // Get dataset name by ID
  const getDatasetName = (datasetId: number) => {
    const dataset = datasets.find((d) => d.id === datasetId);
    return dataset ? dataset.name : `Dataset #${datasetId}`;
  };

  // Table columns configuration
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 60,
    },
    {
      title: intl.formatMessage({ id: 'chart.chartName' }),
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: any) => (
        <Space>
          {getChartTypeIcon(record.chart_type)}
          <Button type="link" style={{ padding: 0 }} onClick={() => handleEdit(record)}>
            <Text strong>{text}</Text>
          </Button>
        </Space>
      ),
    },
    {
      title: intl.formatMessage({ id: 'chart.chartType' }),
      dataIndex: 'chart_type',
      key: 'chart_type',
      render: (chartType: string) => (
        <Tag color={getChartTypeColor(chartType)}>
          {getChartTypeIcon(chartType)} {chartType.toUpperCase()}
        </Tag>
      ),
    },
    {
      title: intl.formatMessage({ id: 'chart.dataset' }),
      dataIndex: 'dataset_id',
      key: 'dataset_id',
      render: (datasetId: number) => <Tag color="purple">{getDatasetName(datasetId)}</Tag>,
    },
    {
      title: intl.formatMessage({ id: 'chart.createdAt' }),
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => (text ? new Date(text).toLocaleString() : '-'),
    },
    {
      title: intl.formatMessage({ id: 'chart.updatedAt' }),
      dataIndex: 'updated_at',
      key: 'updated_at',
      render: (text: string) => (text ? new Date(text).toLocaleString() : '-'),
    },
    {
      title: intl.formatMessage({ id: 'chart.actions' }),
      key: 'actions',
      width: 150,
      render: (_: any, record: any) => (
        <Space size="small">
          <Button
            type="link"
            size="small"
            icon={<EditOutlined />}
            onClick={() => handleEdit(record)}
          >
            {intl.formatMessage({ id: 'common.edit' })}
          </Button>
          <Popconfirm
            title={intl.formatMessage({ id: 'chart.deleteConfirm' })}
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
    <div style={{ padding: '24px' }}>
      <Card>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '16px',
          }}
        >
          <div>
            <Title level={3} style={{ margin: 0 }}>
              {intl.formatMessage({ id: 'chart.charts' })}
            </Title>
            <Text type="secondary">{intl.formatMessage({ id: 'chart.manageCharts' })}</Text>
          </div>
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => fetchCharts()} loading={chartsLoading}>
              {intl.formatMessage({ id: 'common.refresh' })}
            </Button>
            <Button
              type="primary"
              icon={<BarChartOutlined />}
              onClick={() => {
                resetChartBuilder();
                navigate('/chart-builder');
              }}
            >
              {intl.formatMessage({ id: 'chart.add' })}
            </Button>
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={Array.isArray(charts) ? charts : []}
          rowKey="id"
          loading={chartsLoading}
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
            showTotal: (total) => `Total ${total} items`,
          }}
          locale={{
            emptyText: intl.formatMessage({ id: 'common.noData' }),
          }}
          size="small"
        />
      </Card>
    </div>
  );
};

export default ChartsPage;
