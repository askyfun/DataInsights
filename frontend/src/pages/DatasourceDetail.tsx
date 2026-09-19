import {
  ArrowLeftOutlined,
  ColumnWidthOutlined,
  DatabaseOutlined,
  EyeOutlined,
  ReloadOutlined,
  TableOutlined,
} from '@ant-design/icons';
import {
  Breadcrumb,
  Button,
  Card,
  Modal,
  message,
  Pagination,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { ColumnInfo, TableDataResult, TableInfo } from '../api';
import { datasourcesApi } from '../api';
import { useStore } from '../store';

const { Title, Text } = Typography;

const getTypeInfo = (type: string) => {
  const typeMap: Record<string, { label: string; color: string }> = {
    postgresql: { label: 'PostgreSQL', color: 'blue' },
    mysql: { label: 'MySQL', color: 'green' },
    clickhouse: { label: 'ClickHouse', color: 'purple' },
    starrocks: { label: 'StarRocks', color: 'orange' },
  };
  return typeMap[type] || { label: type, color: 'blue' };
};

// getApiErrorMessage extracts the backend error message from unknown API errors and keeps
// fallback text for network errors or unexpected response shapes.
const getApiErrorMessage = (error: unknown, fallback: string) => {
  const err = error as { message?: unknown };
  return typeof err.message === 'string' ? err.message : fallback;
};

// normalizeTableDataResult protects the preview table from legacy or Go nil-slice responses
// that may serialize arrays as null, while preserving valid pagination metadata.
const normalizeTableDataResult = (result: TableDataResult | null | undefined): TableDataResult => ({
  columns: Array.isArray(result?.columns) ? result.columns : [],
  data: Array.isArray(result?.data) ? result.data : [],
  total: result?.total ?? 0,
  primary_keys: Array.isArray(result?.primary_keys) ? result.primary_keys : [],
  page: result?.page ?? 1,
  page_size: result?.page_size ?? 20,
});

const DatasourceDetailPage: React.FC = () => {
  const intl = useIntl();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const datasourceId = Number(id);

  const { datasources, fetchDatasources } = useStore();

  const [tables, setTables] = useState<TableInfo[]>([]);
  const [tablesLoading, setTablesLoading] = useState(false);
  const [columnsCache, setColumnsCache] = useState<Record<string, ColumnInfo[]>>({});
  const [loadingColumns, setLoadingColumns] = useState<Record<string, boolean>>({});

  // 表数据预览状态
  const [previewVisible, setPreviewVisible] = useState(false);
  const [previewTableName, setPreviewTableName] = useState('');
  const [previewData, setPreviewData] = useState<TableDataResult | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewPage, setPreviewPage] = useState(1);
  const [previewPageSize, setPreviewPageSize] = useState(20);
  const [previewSortField, setPreviewSortField] = useState('');
  const [previewSortOrder, setPreviewSortOrder] = useState<'asc' | 'desc'>('asc');

  const datasource = datasources.find((ds) => ds.id === datasourceId);

  useEffect(() => {
    if (datasources.length === 0) {
      fetchDatasources();
    }
  }, [datasources.length, fetchDatasources]);

  const loadTables = useCallback(async () => {
    setTablesLoading(true);
    try {
      const response = await datasourcesApi.getTables(datasourceId);
      setTables(Array.isArray(response.data.data) ? response.data.data : []);
    } catch (error: unknown) {
      message.error(
        getApiErrorMessage(error, intl.formatMessage({ id: 'datasource.detail.loadTablesFailed' }))
      );
    } finally {
      setTablesLoading(false);
    }
  }, [datasourceId, intl.formatMessage]);

  useEffect(() => {
    if (datasourceId) {
      loadTables();
    }
  }, [datasourceId, loadTables]);

  const loadColumns = async (tableName: string) => {
    if (columnsCache[tableName]) {
      return;
    }

    setLoadingColumns((prev) => ({ ...prev, [tableName]: true }));
    try {
      const response = await datasourcesApi.getTableColumns(datasourceId, tableName);
      const columns = Array.isArray(response.data.data) ? response.data.data : [];
      setColumnsCache((prev) => ({ ...prev, [tableName]: columns }));
    } catch (error: unknown) {
      message.error(
        getApiErrorMessage(
          error,
          intl.formatMessage({ id: 'datasource.detail.loadColumnsFailed' }, { name: tableName })
        )
      );
    } finally {
      setLoadingColumns((prev) => ({ ...prev, [tableName]: false }));
    }
  };

  // 加载表数据预览
  const loadTableData = async (
    tableName: string,
    page: number = 1,
    pageSize: number = 20,
    sortField: string = '',
    sortOrder: string = 'asc'
  ) => {
    setPreviewLoading(true);
    try {
      const response = await datasourcesApi.getTableData(
        datasourceId,
        tableName,
        page,
        pageSize,
        sortField,
        sortOrder
      );
      setPreviewData(normalizeTableDataResult(response.data.data));
    } catch (error: unknown) {
      message.error(
        getApiErrorMessage(error, intl.formatMessage({ id: 'datasource.detail.loadDataFailed' }))
      );
    } finally {
      setPreviewLoading(false);
    }
  };

  // 打开数据预览弹窗
  const handlePreviewData = (tableName: string) => {
    setPreviewTableName(tableName);
    setPreviewPage(1);
    setPreviewPageSize(20);
    setPreviewSortField('');
    setPreviewSortOrder('asc');
    setPreviewVisible(true);
    loadTableData(tableName, 1, 20, '', 'asc');
  };

  // 分页变化处理
  const handlePreviewPageChange = (page: number, pageSize: number) => {
    setPreviewPage(page);
    setPreviewPageSize(pageSize);
    loadTableData(previewTableName, page, pageSize, previewSortField, previewSortOrder);
  };

  const columnsColumns = [
    {
      title: intl.formatMessage({ id: 'datasource.detail.name' }),
      dataIndex: 'name',
      key: 'name',
      width: 200,
      render: (text: string) => (
        <Space>
          <ColumnWidthOutlined />
          <Text strong>{text}</Text>
        </Space>
      ),
    },
    {
      title: intl.formatMessage({ id: 'datasource.detail.type' }),
      dataIndex: 'data_type',
      key: 'data_type',
      width: 150,
      render: (text: string) => <Tag color="blue">{text}</Tag>,
    },
    {
      title: intl.formatMessage({ id: 'datasource.detail.comment' }),
      dataIndex: 'comment',
      key: 'comment',
    },
  ];

  const tableColumns = [
    {
      title: intl.formatMessage({ id: 'datasource.detail.tableName' }),
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => (
        <Space>
          <TableOutlined />
          <Text strong>{text}</Text>
        </Space>
      ),
    },
    {
      title: intl.formatMessage({ id: 'datasource.detail.comment' }),
      dataIndex: 'comment',
      key: 'comment',
    },
    {
      title: intl.formatMessage({ id: 'datasource.detail.action' }),
      key: 'action',
      width: 120,
      render: (_: unknown, record: TableInfo) => (
        <Button
          type="link"
          size="small"
          icon={<EyeOutlined />}
          onClick={() => handlePreviewData(record.name)}
        >
          {intl.formatMessage({ id: 'datasource.detail.preview' })}
        </Button>
      ),
    },
  ];

  if (!datasource) {
    return (
      <div style={{ padding: '24px' }}>
        <Space orientation="vertical" align="center">
          <Spin />
          <Text type="secondary">{intl.formatMessage({ id: 'datasource.detail.loading' })}</Text>
        </Space>
      </div>
    );
  }

  return (
    <div style={{ padding: '24px' }}>
      <Card>
        <Breadcrumb
          style={{ marginBottom: '16px' }}
          items={[
            {
              title: (
                <Link to="/datasources">
                  {intl.formatMessage({ id: 'datasource.detail.dataSources' })}
                </Link>
              ),
            },
            {
              title: datasource.name,
            },
          ]}
        />

        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '24px',
          }}
        >
          <div>
            <Space>
              <DatabaseOutlined style={{ fontSize: '24px', color: '#1890ff' }} />
              <Title level={3} style={{ margin: 0 }}>
                {datasource.name}
              </Title>
              <Tag color={getTypeInfo(datasource.type).color}>
                {getTypeInfo(datasource.type).label}
              </Tag>
            </Space>
            <Text type="secondary">
              {datasource.host}:{datasource.port} / {datasource.database_name}
            </Text>
          </div>
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/datasources')}>
              {intl.formatMessage({ id: 'datasource.detail.back' })}
            </Button>
            <Button icon={<ReloadOutlined />} onClick={loadTables} loading={tablesLoading}>
              {intl.formatMessage({ id: 'datasource.detail.refreshTables' })}
            </Button>
          </Space>
        </div>

        <div style={{ marginBottom: '24px' }}>
          <Text strong style={{ marginBottom: '8px', display: 'block' }}>
            {intl.formatMessage({ id: 'datasource.detail.tablesCount' }, { count: tables.length })}
          </Text>
          <Table
            columns={tableColumns}
            dataSource={tables}
            rowKey="name"
            loading={tablesLoading}
            expandable={{
              expandedRowRender: (record: TableInfo) => {
                const tableColumns = columnsCache[record.name] || [];
                const isLoading = loadingColumns[record.name];

                if (isLoading) {
                  return (
                    <div style={{ padding: '16px', textAlign: 'center' }}>
                      <Space orientation="vertical" align="center">
                        <Spin />
                        <Text type="secondary">
                          {intl.formatMessage({ id: 'datasource.detail.loadingColumns' })}
                        </Text>
                      </Space>
                    </div>
                  );
                }

                return (
                  <Table
                    columns={columnsColumns}
                    dataSource={tableColumns}
                    rowKey="name"
                    size="small"
                    pagination={false}
                    locale={{
                      emptyText: intl.formatMessage({ id: 'datasource.detail.noColumns' }),
                    }}
                  />
                );
              },
              onExpand: (expanded: boolean, record: TableInfo) => {
                if (expanded) {
                  loadColumns(record.name);
                }
              },
            }}
            pagination={{
              pageSize: 10,
              showSizeChanger: true,
            }}
            locale={{
              emptyText: intl.formatMessage({ id: 'datasource.detail.noTables' }),
            }}
            size="small"
          />
        </div>
      </Card>

      <Modal
        title={intl.formatMessage(
          { id: 'datasource.detail.previewTitle' },
          { name: previewTableName }
        )}
        open={previewVisible}
        onCancel={() => setPreviewVisible(false)}
        footer={null}
        width="90%"
        style={{ top: 20 }}
      >
        {previewData && (
          <>
            <div style={{ marginBottom: 8 }}>
              <Text type="secondary">
                {intl.formatMessage(
                  { id: 'datasource.detail.totalRowsAndKeys' },
                  {
                    total: previewData.total,
                    keys:
                      previewData.primary_keys.join(', ') ||
                      intl.formatMessage({ id: 'datasource.detail.none' }),
                  }
                )}
              </Text>
            </div>
            <Table
              columns={previewData.columns.map((col) => ({
                title: (
                  <Space>
                    {col}
                    {previewData.primary_keys.includes(col) && (
                      <Tag color="gold" style={{ marginLeft: 4 }}>
                        {intl.formatMessage({ id: 'datasource.detail.pk' })}
                      </Tag>
                    )}
                  </Space>
                ),
                dataIndex: col,
                key: col,
                sorter: previewData.primary_keys.includes(col),
                sortOrder:
                  previewSortField === col
                    ? previewSortOrder === 'asc'
                      ? 'ascend'
                      : 'descend'
                    : undefined,
                ellipsis: true,
                render: (val: unknown) => {
                  if (val === null || val === undefined)
                    return (
                      <Text type="secondary">
                        {intl.formatMessage({ id: 'datasource.detail.null' })}
                      </Text>
                    );
                  if (typeof val === 'boolean') return val ? 'true' : 'false';
                  return String(val);
                },
              }))}
              dataSource={previewData.data.map((row, idx) => ({ ...row, _rowKey: idx }))}
              rowKey="_rowKey"
              loading={previewLoading}
              pagination={false}
              size="small"
              scroll={{ x: 'max-content', y: 400 }}
              onChange={(_pagination, _filters, sorter) => {
                const sortResult = Array.isArray(sorter) ? sorter[0] : sorter;
                if (sortResult.field && sortResult.order) {
                  const field = String(sortResult.field);
                  const order = sortResult.order === 'ascend' ? 'asc' : 'desc';
                  setPreviewSortField(field);
                  setPreviewSortOrder(order as 'asc' | 'desc');
                  loadTableData(previewTableName, previewPage, previewPageSize, field, order);
                } else {
                  setPreviewSortField('');
                  setPreviewSortOrder('asc');
                  loadTableData(previewTableName, previewPage, previewPageSize, '', 'asc');
                }
              }}
            />
            <div style={{ marginTop: 16, textAlign: 'right' }}>
              <Pagination
                current={previewPage}
                pageSize={previewPageSize}
                total={previewData.total}
                showSizeChanger
                showTotal={(total) => intl.formatMessage({ id: 'common.totalRows' }, { total })}
                onChange={handlePreviewPageChange}
              />
            </div>
          </>
        )}
      </Modal>
    </div>
  );
};

export default DatasourceDetailPage;
