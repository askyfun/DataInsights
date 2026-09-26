import {
  BarChartOutlined,
  DeleteOutlined,
  EditOutlined,
  LineChartOutlined,
  PieChartOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { Button, Card, message, Popconfirm, Space, Table, Tag, Typography } from 'antd';
import { useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, useNavigate } from 'react-router-dom';
import { chartDefinitions } from '../components/ChartBuilder/chartDefinitions';
import { ListSearch, makeTimeSorter, standardListPagination } from '../components/listPage';
import PageHeader from '../components/PageHeader';
import { type ChartType, migrateChartConfig } from '../lib/chartConfigSchema';
import { formatDateTime } from '../lib/format';
import { useStore } from '../store';

const { Text } = Typography;

const ChartsPage: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const [searchText, setSearchText] = useState('');
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

  // 图型名与图表构建页共用同一来源（chartDefinitions），同一种图型不应在两处叫两个名字。
  // 未知图型（后端已支持而前端未同步）回落显示原始值，不静默吞成空白。
  const getChartTypeLabel = (chartType: string) =>
    chartDefinitions[chartType as ChartType]?.label ?? chartType;

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

  // 与数据集列表同口径的客户端关键字过滤（名称 / 图型 / 所属数据集名）。
  const keyword = searchText.trim().toLowerCase();
  const visibleCharts = (Array.isArray(charts) ? charts : []).filter((chart) => {
    if (!keyword) return true;
    return (
      chart.name.toLowerCase().includes(keyword) ||
      (chart.chart_type ?? '').toLowerCase().includes(keyword) ||
      getDatasetName(chart.dataset_id).toLowerCase().includes(keyword)
    );
  });

  // Table columns configuration
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 60,
      // 默认视图与后端 List 的 ORDER BY id DESC 对齐：新建的排最前（与数据集列表一致）。
      sorter: (a: any, b: any) => a.id - b.id,
      defaultSortOrder: 'descend' as const,
    },
    {
      title: intl.formatMessage({ id: 'chart.chartName' }),
      dataIndex: 'name',
      key: 'name',
      sorter: (a: any, b: any) => a.name.localeCompare(b.name) || b.id - a.id,
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
          {getChartTypeIcon(chartType)} {getChartTypeLabel(chartType)}
        </Tag>
      ),
    },
    {
      title: intl.formatMessage({ id: 'chart.dataset' }),
      dataIndex: 'dataset_id',
      key: 'dataset_id',
      // 与数据集列表一致：所属实体名可点跳详情页（新页签语义交由 Link 常规导航）。
      render: (datasetId: number) => (
        <Link to={`/datasets/${datasetId}`}>
          <Tag color="blue" style={{ marginInlineEnd: 0 }}>
            {getDatasetName(datasetId)}
          </Tag>
        </Link>
      ),
    },
    {
      title: intl.formatMessage({ id: 'chart.createdAt' }),
      dataIndex: 'created_at',
      key: 'created_at',
      sorter: makeTimeSorter((row: any) => row.created_at),
      render: (text: string) => formatDateTime(text),
    },
    {
      title: intl.formatMessage({ id: 'chart.updatedAt' }),
      dataIndex: 'updated_at',
      key: 'updated_at',
      sorter: makeTimeSorter((row: any) => row.updated_at),
      render: (text: string) => formatDateTime(text),
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
    <div className="dr-page">
      <PageHeader
        icon={<BarChartOutlined />}
        title={intl.formatMessage({ id: 'chart.charts' })}
        description={intl.formatMessage({ id: 'chart.manageCharts' })}
        extra={
          <>
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
          </>
        }
      />

      <Card>
        <div className="dr-card-toolbar">
          <ListSearch
            placeholder={intl.formatMessage({ id: 'chart.searchPlaceholder' })}
            value={searchText}
            onChange={setSearchText}
          />
        </div>
        <Table
          columns={columns}
          dataSource={visibleCharts}
          rowKey="id"
          loading={chartsLoading}
          pagination={standardListPagination}
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
