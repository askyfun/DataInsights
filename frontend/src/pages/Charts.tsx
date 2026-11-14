import {
  BarChartOutlined,
  DeleteOutlined,
  EditOutlined,
  LineChartOutlined,
  PieChartOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { Button, Card, message, Popconfirm, Space, Table, Tag, Typography } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, useNavigate } from 'react-router-dom';
import { type Chart, chartsApi, type ListQuery } from '../api';
import { chartDefinitions } from '../components/ChartBuilder/chartDefinitions';
import { ListSearch, sortOrderOf, useDebouncedValue, useServerList } from '../components/listPage';
import PageHeader from '../components/PageHeader';
import { type ChartType, migrateChartConfig } from '../lib/chartConfigSchema';
import { formatDateTime } from '../lib/format';
import { useStore } from '../store';

const { Text } = Typography;

const ChartsPage: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const [searchText, setSearchText] = useState('');
  const { deleteChart, setChartBuilderConfig, resetChartBuilder, datasets, fetchDatasets } =
    useStore();

  // 图表列表走服务端分页/排序/关键字（issue #205）；数据集名解析仍用 store 的
  // 完整集合（选择器口径，上限页一次取回）。
  const fetchPage = useCallback(
    (query: ListQuery) => chartsApi.getAll(query).then((res) => res.data.data),
    []
  );
  const list = useServerList<Chart>(fetchPage, { sort: 'id', order: 'desc' });
  const keyword = useDebouncedValue(searchText).trim();

  useEffect(() => {
    fetchDatasets();
  }, [fetchDatasets]);

  useEffect(() => {
    if (keyword === (list.query.keyword ?? '')) return;
    list.setParam({ keyword: keyword || undefined });
  }, [keyword, list.query.keyword, list.setParam]);

  // Show error message
  useEffect(() => {
    if (list.error) {
      message.error(list.error);
    }
  }, [list.error]);

  // Handle delete chart
  const handleDelete = async (id: number) => {
    try {
      await deleteChart(id);
      list.reload();
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

  // Table columns configuration
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 60,
      // 服务端排序：默认与后端 ORDER BY id DESC 对齐，新建的排最前。
      sorter: true,
      sortOrder: sortOrderOf(list.query, 'id'),
    },
    {
      title: intl.formatMessage({ id: 'chart.chartName' }),
      dataIndex: 'name',
      key: 'name',
      sorter: true,
      sortOrder: sortOrderOf(list.query, 'name'),
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
      // 创建人（R-95）：列表出口回填 owner_name，无归属（匿名/存量行）显示「—」。
      title: intl.formatMessage({ id: 'common.owner' }),
      dataIndex: 'owner_name',
      key: 'owner_name',
      render: (text: string | null) =>
        text ? <Text type="secondary">{text}</Text> : <Text type="secondary">—</Text>,
    },
    {
      title: intl.formatMessage({ id: 'chart.createdAt' }),
      dataIndex: 'created_at',
      key: 'created_at',
      sorter: true,
      sortOrder: sortOrderOf(list.query, 'created_at'),
      render: (text: string) => formatDateTime(text),
    },
    {
      title: intl.formatMessage({ id: 'chart.updatedAt' }),
      dataIndex: 'updated_at',
      key: 'updated_at',
      sorter: true,
      sortOrder: sortOrderOf(list.query, 'updated_at'),
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
            <Button icon={<ReloadOutlined />} onClick={() => list.reload()} loading={list.loading}>
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
          dataSource={list.items}
          rowKey="id"
          loading={list.loading}
          pagination={list.pagination}
          onChange={list.onTableChange}
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
