import {
  ArrowLeftOutlined,
  ArrowRightOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  EditOutlined,
  FunctionOutlined,
  PlusOutlined,
  ReloadOutlined,
  TableOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  message,
  Popconfirm,
  Select,
  Space,
  Spin,
  Steps,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import * as echarts from 'echarts';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, useNavigate } from 'react-router-dom';
import type { DataType } from '../api';
import {
  ColumnInfo,
  DatasetColumn,
  DatasetFormData,
  DatasetPreview,
  datasetsApi,
  datasourcesApi,
  TableInfo,
} from '../api';
import { toStandardType } from '../api/datatypes';
import { formatDateTime } from '../lib/format';
import { useStore } from '../store';

const { Title, Text } = Typography;

const DatasetPage: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const [form] = Form.useForm<DatasetFormData>();
  const [editForm] = Form.useForm<DatasetFormData>();
  const [modalVisible, setModalVisible] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [tables, setTables] = useState<TableInfo[]>([]);
  const [tablesLoading, setTablesLoading] = useState(false);
  const [selectedDatasourceId, setSelectedDatasourceId] = useState<number | null>(null);
  const [queryType, setQueryType] = useState<string>('table');
  const [editingDataset, setEditingDataset] = useState<any>(null);
  const [datasetColumns, setDatasetColumns] = useState<DatasetColumn[]>([]);
  const [searchText, setSearchText] = useState('');
  const [modalPreviewLoading, setModalPreviewLoading] = useState(false);
  const [modalPreviewData, setModalPreviewData] = useState<DatasetPreview | null>(null);
  const [savingColumns, setSavingColumns] = useState(false);
  const [virtualFieldModalVisible, setVirtualFieldModalVisible] = useState(false);
  const [editingVirtualField, setEditingVirtualField] = useState<DatasetColumn | null>(null);
  const [virtualFieldForm] = Form.useForm<DatasetColumn>();

  // 字段分布状态
  const [selectedField, setSelectedField] = useState<string>('');
  const [fieldDistribution, setFieldDistribution] = useState<any>(null);
  const [distributionLoading, setDistributionLoading] = useState(false);
  const chartRef = useRef<HTMLDivElement>(null);
  const chartInstance = useRef<echarts.ECharts | null>(null);

  // 多步创建流程状态
  const [createStep, setCreateStep] = useState(0); // 0: 选择数据源表格, 1: 字段编辑
  const [tempDatasetName, setTempDatasetName] = useState('');

  // 重置创建流程
  const resetCreateFlow = () => {
    setCreateStep(0);
    setTempDatasetName('');
    setSelectedDatasourceId(null);
    setQueryType('table');
    setDatasetColumns([]);
    setModalPreviewData(null);
    form.resetFields();
  };

  // 第一步验证
  const validateStep1 = async () => {
    try {
      const values = await form.validateFields([
        'name',
        'datasource_id',
        'query_type',
        'table_name',
        'query_sql',
      ]);
      if (!values.name || !values.datasource_id) {
        message.error(intl.formatMessage({ id: 'dataset.pleaseFillRequiredFields' }));
        return false;
      }
      if (values.query_type === 'table' && !values.table_name) {
        message.error(intl.formatMessage({ id: 'dataset.pleaseSelectTable' }));
        return false;
      }
      if (values.query_type === 'sql' && !values.query_sql) {
        message.error(intl.formatMessage({ id: 'dataset.pleaseEnterSql' }));
        return false;
      }
      return true;
    } catch {
      return false;
    }
  };

  // 点击下一步
  const handleNextStep = async () => {
    const isValid = await validateStep1();
    if (!isValid || !selectedDatasourceId) return;

    const tableName = form.getFieldValue('table_name');
    const querySql = form.getFieldValue('query_sql');
    const name = form.getFieldValue('name');

    setTempDatasetName(name);

    // 如果是表格模式，获取字段
    if (queryType === 'table' && tableName) {
      await generateDefaultColumns(selectedDatasourceId, tableName);
    }

    // 获取预览数据
    setModalPreviewLoading(true);
    try {
      const response = await datasourcesApi.getPreview(
        selectedDatasourceId,
        tableName || '',
        querySql || '',
        queryType
      );
      setModalPreviewData(response.data.data || null);
    } catch (_error: any) {
      setModalPreviewData(null);
    } finally {
      setModalPreviewLoading(false);
    }

    setCreateStep(1);
  };

  // 点击上一步
  const handlePrevStep = () => {
    setCreateStep(0);
  };

  // 最终提交（创建数据集 + 保存字段 + 跳转详情页）
  const handleFinalSubmit = async () => {
    setSubmitLoading(true);
    try {
      const values = form.getFieldsValue([
        'name',
        'datasource_id',
        'query_type',
        'table_name',
        'query_sql',
      ]);

      const data: DatasetFormData = {
        name: values.name,
        datasource_id: values.datasource_id,
        query_type: values.query_type,
        table_name: values.query_type === 'table' ? values.table_name : undefined,
        query_sql: values.query_type === 'sql' ? values.query_sql : undefined,
      };

      const newDataset = await addDataset(data);

      // 保存字段信息
      if (datasetColumns.length > 0) {
        await datasetsApi.updateColumns(newDataset.id, datasetColumns);
      }

      message.success(intl.formatMessage({ id: 'common.success' }));
      setModalVisible(false);
      resetCreateFlow();

      // 跳转到数据集详情页
      navigate(`/datasets/${newDataset.id}`);
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setSubmitLoading(false);
    }
  };

  const dataTypes: { value: DataType; label: string }[] = [
    { value: 'string', label: intl.formatMessage({ id: 'dataType.string' }) },
    { value: 'int', label: intl.formatMessage({ id: 'dataType.integer' }) },
    { value: 'float', label: intl.formatMessage({ id: 'dataType.float' }) },
    { value: 'decimal', label: intl.formatMessage({ id: 'dataType.decimal' }) },
    { value: 'date', label: intl.formatMessage({ id: 'dataType.date' }) },
    { value: 'datetime', label: intl.formatMessage({ id: 'dataType.datetime' }) },
    { value: 'boolean', label: intl.formatMessage({ id: 'dataType.boolean' }) },
  ];

  const handleOpenVirtualFieldModal = (field?: DatasetColumn) => {
    if (field) {
      setEditingVirtualField(field);
      virtualFieldForm.setFieldsValue({
        name: field.name,
        type: field.type,
        role: field.role,
        expr: field.expr,
        comment: field.comment || '',
      });
    } else {
      setEditingVirtualField(null);
      virtualFieldForm.resetFields();
      virtualFieldForm.setFieldsValue({
        role: 'dimension',
        type: 'string',
      });
    }
    setVirtualFieldModalVisible(true);
  };

  const handleSaveVirtualField = async () => {
    try {
      const values = await virtualFieldForm.validateFields();
      let updatedColumns: DatasetColumn[];

      if (editingVirtualField) {
        updatedColumns = datasetColumns.map((col) =>
          col.name === editingVirtualField.name ? { ...col, ...values } : col
        );
      } else {
        const newField: DatasetColumn = {
          name: values.name,
          type: values.type,
          role: values.role || 'dimension',
          comment: values.comment || '',
          expr: values.expr || '',
        };
        updatedColumns = [...datasetColumns, newField];
      }

      if (editingDataset) {
        setSavingColumns(true);
        await datasetsApi.updateColumns(editingDataset.id, updatedColumns);
        setDatasetColumns(updatedColumns);
        message.success(
          editingVirtualField
            ? intl.formatMessage({ id: 'virtualField.fieldUpdated' })
            : intl.formatMessage({ id: 'virtualField.fieldAdded' })
        );
      }

      setVirtualFieldModalVisible(false);
      virtualFieldForm.resetFields();
      setEditingVirtualField(null);
    } catch (error: any) {
      if (error.errorFields) {
        return;
      }
      message.error(error.message || intl.formatMessage({ id: 'virtualField.saveFailed' }));
    } finally {
      setSavingColumns(false);
    }
  };

  const {
    datasets,
    datasetsLoading,
    fetchDatasets,
    addDataset,
    updateDataset,
    deleteDataset,
    datasources,
    fetchDatasources,
    datasetsError,
  } = useStore();

  // Fetch datasets and datasources on mount
  useEffect(() => {
    fetchDatasets();
    fetchDatasources();
  }, [fetchDatasets, fetchDatasources]);

  // 渲染字段分布图表
  useEffect(() => {
    if (!fieldDistribution?.distribution || !chartRef.current) {
      return;
    }

    if (chartInstance.current) {
      chartInstance.current.dispose();
    }

    const chart = echarts.init(chartRef.current);
    chartInstance.current = chart;

    const data = fieldDistribution.distribution.slice(0, 15).map((item: any) => ({
      value: item.count,
      name: String(item.value ?? '(null)'),
    }));

    const option: echarts.EChartsOption = {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: any) => {
          const item = params[0];
          const dist = fieldDistribution.distribution[item.dataIndex];
          return `${item.name}<br/>计数: ${item.value}<br/>占比: ${dist.percentage}%`;
        },
      },
      grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
      xAxis: {
        type: 'category',
        data: data.map((d: any) => (d.name.length > 10 ? `${d.name.slice(0, 10)}...` : d.name)),
        axisLabel: { interval: 0, rotate: 45 },
      },
      yAxis: { type: 'value', name: '计数' },
      series: [
        {
          type: 'bar',
          data: data,
          itemStyle: { color: '#1890ff' },
          barWidth: '60%',
        },
      ],
    };

    chart.setOption(option);

    return () => {
      if (chartInstance.current) {
        chartInstance.current.dispose();
        chartInstance.current = null;
      }
    };
  }, [fieldDistribution]);

  // Show error message
  useEffect(() => {
    if (datasetsError) {
      message.error(datasetsError);
    }
  }, [datasetsError]);

  // Fetch tables from datasource
  const fetchTables = useCallback(
    async (datasourceId: number) => {
      setTablesLoading(true);
      try {
        const response = await datasourcesApi.getTables(datasourceId);
        setTables(response.data.data || []);
      } catch (error: any) {
        message.error(error.message || intl.formatMessage({ id: 'common.error' }));
        setTables([]);
      } finally {
        setTablesLoading(false);
      }
    },
    [intl]
  );

  // Fetch tables when datasource changes
  useEffect(() => {
    if (selectedDatasourceId && queryType === 'table') {
      fetchTables(selectedDatasourceId);
    } else {
      setTables([]);
    }
  }, [selectedDatasourceId, queryType, fetchTables]);

  // Handle datasource selection change
  const handleDatasourceChange = (value: number) => {
    setSelectedDatasourceId(value);
    form.setFieldValue('table_name', undefined);
    form.setFieldValue('query_sql', undefined);
    setDatasetColumns([]);
  };

  // Handle query type change
  const handleQueryTypeChange = (value: string) => {
    setQueryType(value);
    form.setFieldValue('table_name', undefined);
    form.setFieldValue('query_sql', undefined);
    setModalPreviewData(null);
    setDatasetColumns([]);
  };

  // Generate default columns from table
  const generateDefaultColumns = async (datasourceId: number, tableName: string) => {
    try {
      const response = await datasourcesApi.getTableColumns(datasourceId, tableName);
      const tableColumns = response.data.data || [];

      // Get datasource type for type mapping
      const datasourceType = getDatasourceType(datasourceId);

      const defaultColumns: DatasetColumn[] = tableColumns.map((col: ColumnInfo) => ({
        name: col.name,
        expr: `\`${col.name}\``,
        type: toStandardType(col.data_type || 'varchar', datasourceType),
        comment: col.comment || '',
        role: ['int', 'bigint', 'float', 'decimal', 'numeric', 'double', 'real'].includes(
          col.data_type?.toLowerCase() ?? ''
        )
          ? 'metric'
          : 'dimension',
      }));

      setDatasetColumns(defaultColumns);
    } catch (error: any) {
      console.error('Failed to fetch table columns:', error);
      setDatasetColumns([]);
    }
  };

  // Handle table/SQL selection and fetch preview
  const handleTableOrSqlChange = async () => {
    if (!selectedDatasourceId) return;

    const tableName = form.getFieldValue('table_name');
    const querySql = form.getFieldValue('query_sql');

    if ((queryType === 'table' && !tableName) || (queryType === 'sql' && !querySql)) {
      setModalPreviewData(null);
      setDatasetColumns([]);
      return;
    }

    if (queryType === 'table' && tableName) {
      await generateDefaultColumns(selectedDatasourceId, tableName);
    } else {
      setDatasetColumns([]);
    }

    setModalPreviewLoading(true);
    try {
      const response = await datasourcesApi.getPreview(
        selectedDatasourceId,
        tableName || '',
        querySql || '',
        queryType
      );
      setModalPreviewData(response.data.data || null);
    } catch (_error: any) {
      setModalPreviewData(null);
    } finally {
      setModalPreviewLoading(false);
    }
  };

  // 获取字段分布
  const fetchFieldDistribution = async (fieldName: string) => {
    if (!selectedDatasourceId || !fieldName) return;

    const tableName = form.getFieldValue('table_name');
    const querySql = form.getFieldValue('query_sql');

    setDistributionLoading(true);
    try {
      const response = await datasourcesApi.getFieldDistribution(
        selectedDatasourceId,
        tableName || '',
        querySql || '',
        queryType,
        fieldName
      );
      setFieldDistribution(response.data.data);
    } catch (_error: any) {
      setFieldDistribution(null);
    } finally {
      setDistributionLoading(false);
    }
  };

  // 字段选择变化
  const handleFieldChange = (fieldName: string) => {
    setSelectedField(fieldName);
    if (fieldName) {
      fetchFieldDistribution(fieldName);
    } else {
      setFieldDistribution(null);
    }
  };

  // 处理字段分布数据
  const handleDelete = async (id: number) => {
    try {
      await deleteDataset(id);
      message.success(intl.formatMessage({ id: 'common.success' }));
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    }
  };

  // Handle edit dataset
  const handleEdit = (record: any) => {
    setEditingDataset(record);
    setSelectedDatasourceId(record.datasource_id);
    setQueryType(record.query_type);
    editForm.setFieldsValue({
      name: record.name,
      datasource_id: record.datasource_id,
      query_type: record.query_type,
      table_name: record.table_name,
      query_sql: record.query_sql,
    });
    if (record.datasource_id && record.query_type === 'table') {
      fetchTables(record.datasource_id);
    }
    setEditModalVisible(true);
  };

  // Handle edit submit
  const handleEditSubmit = async (values: DatasetFormData) => {
    if (!editingDataset) return;
    setSubmitLoading(true);
    try {
      const data: DatasetFormData = {
        name: values.name,
        datasource_id: values.datasource_id,
        query_type: values.query_type,
        table_name: values.query_type === 'table' ? values.table_name : undefined,
        query_sql: values.query_type === 'sql' ? values.query_sql : undefined,
        // The modal does not manage these two, but the PUT handler overlays
        // every scalar it receives (an omitted bool defaults to false, an
        // omitted mode to "direct") — pass the stored row's values through so
        // renaming here never silently disables sharding configured in
        // DatasetEdit.
        mode: editingDataset.mode,
        shard_enabled: editingDataset.shard_enabled,
      };
      await updateDataset(editingDataset.id, data);
      message.success(intl.formatMessage({ id: 'common.success' }));
      setEditModalVisible(false);
      editForm.resetFields();
      setEditingDataset(null);
      setSelectedDatasourceId(null);
      setQueryType('table');
      fetchDatasets();
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setSubmitLoading(false);
    }
  };

  const handleColumnRoleChange = (name: string, role: 'dimension' | 'metric') => {
    setDatasetColumns((prev) => prev.map((col) => (col.name === name ? { ...col, role } : col)));
  };

  const handleColumnTypeChange = (name: string, type: string) => {
    setDatasetColumns((prev) =>
      prev.map((col) => (col.name === name ? { ...col, type: type as DatasetColumn['type'] } : col))
    );
  };

  // Get datasource name by id
  const getDatasourceName = (id: number) => {
    const ds = datasources.find((d) => d.id === id);
    return ds ? ds.name : `ID: ${id}`;
  };

  // Get datasource type by id
  const getDatasourceType = (id: number): string => {
    const ds = datasources.find((d) => d.id === id);
    return ds ? ds.type : 'starrocks';
  };

  // Filter datasets by search text
  const filteredDatasets = (Array.isArray(datasets) ? datasets : []).filter((ds) =>
    ds.name.toLowerCase().includes(searchText.toLowerCase())
  );

  // Table columns configuration
  const columns = [
    {
      title: intl.formatMessage({ id: 'dataset.id' }),
      dataIndex: 'id',
      key: 'id',
      width: 60,
    },
    {
      title: intl.formatMessage({ id: 'dataset.name' }),
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: any) => (
        <Link
          to={`/datasets/${record.id}`}
          style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
        >
          <DatabaseOutlined />
          <Text strong>{text}</Text>
        </Link>
      ),
    },
    {
      title: intl.formatMessage({ id: 'dataset.source.combined' }),
      key: 'source',
      ellipsis: true,
      render: (_: any, record: any) => {
        const dsId = record.datasource_id;
        const sourceLabel = record.query_type === 'table' ? record.table_name : 'SQL';
        return (
          <Space size={6}>
            <Link to={`/datasources/${dsId}`}>{getDatasourceName(dsId)}</Link>
            <Text type="secondary">/</Text>
            {record.query_type === 'table' ? (
              <Text code>{sourceLabel}</Text>
            ) : (
              <Tooltip title={record.query_sql}>
                <Text code>SQL</Text>
              </Tooltip>
            )}
          </Space>
        );
      },
    },
    {
      title: intl.formatMessage({ id: 'dataset.createdAt' }),
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => formatDateTime(text),
    },
    {
      title: intl.formatMessage({ id: 'dataset.actions' }),
      key: 'actions',
      width: 140,
      render: (_: any, record: any) => (
        <Space>
          <Button
            type="link"
            size="small"
            icon={<EditOutlined />}
            aria-label="Edit dataset"
            onClick={() => handleEdit(record)}
          >
            {intl.formatMessage({ id: 'common.edit' })}
          </Button>
          <Popconfirm
            title={intl.formatMessage({ id: 'dataset.deleteConfirm' })}
            onConfirm={() => handleDelete(record.id)}
            okText={intl.formatMessage({ id: 'common.yes' })}
            cancelText={intl.formatMessage({ id: 'common.no' })}
          >
            <Button
              type="link"
              size="small"
              danger
              icon={<DeleteOutlined />}
              aria-label="Delete dataset"
            >
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
              {intl.formatMessage({ id: 'dataset.datasets' })}
            </Title>
            <Text type="secondary">{intl.formatMessage({ id: 'dataset.manageDatasets' })}</Text>
          </div>
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => fetchDatasets()}
              loading={datasetsLoading}
            >
              {intl.formatMessage({ id: 'common.refresh' })}
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => {
                form.resetFields();
                setSelectedDatasourceId(null);
                setQueryType('table');
                setModalVisible(true);
              }}
            >
              {intl.formatMessage({ id: 'dataset.add' })}
            </Button>
          </Space>
        </div>

        <Input.Search
          placeholder={intl.formatMessage({ id: 'dataset.searchPlaceholder' })}
          allowClear
          style={{ marginBottom: 16, width: 300 }}
          onChange={(e) => setSearchText(e.target.value)}
          value={searchText}
        />

        <Table
          columns={columns}
          dataSource={filteredDatasets}
          rowKey="id"
          loading={datasetsLoading}
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

      {/* Add Dataset Modal - Multi-step */}
      <Modal
        title={intl.formatMessage({ id: 'dataset.addDataset' })}
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false);
          resetCreateFlow();
        }}
        footer={null}
        width={900}
        destroyOnHidden
      >
        <Steps
          current={createStep}
          style={{ marginBottom: 24 }}
          items={[
            { title: intl.formatMessage({ id: 'dataset.steps.selectSource' }) },
            { title: intl.formatMessage({ id: 'dataset.steps.editFields' }) },
          ]}
        />

        {createStep === 0 ? (
          <Form
            form={form}
            layout="vertical"
            initialValues={{
              query_type: 'table',
            }}
          >
            <Form.Item
              name="name"
              label={intl.formatMessage({ id: 'dataset.name' })}
              rules={[
                { required: true, message: intl.formatMessage({ id: 'dataset.pleaseEnterName' }) },
              ]}
            >
              <Input placeholder={intl.formatMessage({ id: 'dataset.pleaseEnterName' })} />
            </Form.Item>

            <Form.Item
              name="datasource_id"
              label={intl.formatMessage({ id: 'dataset.datasource' })}
              rules={[
                {
                  required: true,
                  message: intl.formatMessage({ id: 'dataset.pleaseSelectDatasource' }),
                },
              ]}
            >
              <Select
                placeholder={intl.formatMessage({ id: 'dataset.pleaseSelectDatasource' })}
                onChange={handleDatasourceChange}
                loading={datasources.length === 0}
              >
                {(Array.isArray(datasources) ? datasources : []).map((ds) => (
                  <Select.Option key={ds.id} value={ds.id}>
                    {ds.name}
                  </Select.Option>
                ))}
              </Select>
            </Form.Item>

            <Form.Item
              name="query_type"
              label={intl.formatMessage({ id: 'dataset.queryType' })}
              rules={[
                {
                  required: true,
                  message: intl.formatMessage({ id: 'dataset.pleaseSelectQueryType' }),
                },
              ]}
            >
              <Select
                placeholder={intl.formatMessage({ id: 'dataset.pleaseSelectQueryType' })}
                onChange={handleQueryTypeChange}
              >
                <Select.Option value="table">
                  <Space>
                    <TableOutlined />
                    {intl.formatMessage({ id: 'dataset.queryType.table' })}
                  </Space>
                </Select.Option>
                <Select.Option value="sql">
                  <Space>
                    <DatabaseOutlined />
                    {intl.formatMessage({ id: 'dataset.queryType.sql' })}
                  </Space>
                </Select.Option>
              </Select>
            </Form.Item>

            {queryType === 'table' ? (
              <Form.Item
                name="table_name"
                label={intl.formatMessage({ id: 'dataset.tableName' })}
                rules={[
                  {
                    required: queryType === 'table',
                    message: intl.formatMessage({ id: 'dataset.pleaseSelectTable' }),
                  },
                ]}
              >
                <Select
                  placeholder={intl.formatMessage({ id: 'dataset.selectTable' })}
                  loading={tablesLoading}
                  disabled={!selectedDatasourceId || tablesLoading}
                  onChange={() => {
                    setTimeout(handleTableOrSqlChange, 100);
                  }}
                >
                  {tables.map((table) => (
                    <Select.Option key={table.name} value={table.name}>
                      {table.name}
                    </Select.Option>
                  ))}
                </Select>
              </Form.Item>
            ) : (
              <Form.Item
                name="query_sql"
                label={intl.formatMessage({ id: 'dataset.sql' })}
                rules={[
                  {
                    required: queryType === 'sql',
                    message: intl.formatMessage({ id: 'dataset.pleaseEnterSql' }),
                  },
                ]}
              >
                <Input.TextArea
                  placeholder={intl.formatMessage({ id: 'dataset.enterSql' })}
                  rows={4}
                  disabled={!selectedDatasourceId}
                  onChange={() => {
                    setTimeout(handleTableOrSqlChange, 300);
                  }}
                />
              </Form.Item>
            )}

            {modalPreviewData?.data && modalPreviewData.data.length > 0 && (
              <div style={{ marginBottom: 16 }}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {intl.formatMessage({ id: 'dataset.dataPreview' })}
                </Text>
                <Table
                  dataSource={modalPreviewData.data.slice(0, 10)}
                  rowKey={(_: any, index?: number) => String(index ?? Math.random())}
                  size="small"
                  pagination={false}
                  columns={(modalPreviewData.columns || []).map((col: string) => ({
                    title: col,
                    dataIndex: col,
                    key: col,
                    ellipsis: true,
                  }))}
                  scroll={{ x: 'max-content' }}
                  loading={modalPreviewLoading}
                />
              </div>
            )}

            {modalPreviewData?.columns && modalPreviewData.columns.length > 0 && (
              <div style={{ marginBottom: 16 }}>
                <div
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    marginBottom: 8,
                  }}
                >
                  <Text strong>{intl.formatMessage({ id: 'dataset.fieldDistribution' })}</Text>
                  <Select
                    placeholder={intl.formatMessage({ id: 'dataset.selectField' })}
                    style={{ width: 180 }}
                    allowClear
                    value={selectedField || undefined}
                    onChange={handleFieldChange}
                    loading={distributionLoading}
                  >
                    {(modalPreviewData.columns || []).map((col: string) => (
                      <Select.Option key={col} value={col}>
                        {col}
                      </Select.Option>
                    ))}
                  </Select>
                </div>
                {fieldDistribution?.distribution && fieldDistribution.distribution.length > 0 && (
                  <Spin spinning={distributionLoading}>
                    <div ref={chartRef} style={{ width: '100%', height: 250 }} />
                    <div
                      style={{
                        marginTop: 8,
                        display: 'flex',
                        gap: 16,
                        fontSize: 12,
                        color: '#666',
                      }}
                    >
                      <span>
                        {intl.formatMessage({ id: 'dataset.totalCount' })}:{' '}
                        {fieldDistribution.total_count}
                      </span>
                      <span>
                        {intl.formatMessage({ id: 'dataset.uniqueCount' })}:{' '}
                        {fieldDistribution.unique_count}
                      </span>
                    </div>
                  </Spin>
                )}
                {selectedField && !fieldDistribution && !distributionLoading && (
                  <Text type="secondary">
                    {intl.formatMessage({ id: 'dataset.noDistributionData' })}
                  </Text>
                )}
              </div>
            )}

            <Form.Item>
              <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
                <Button
                  onClick={() => {
                    setModalVisible(false);
                    resetCreateFlow();
                  }}
                >
                  {intl.formatMessage({ id: 'common.cancel' })}
                </Button>
                <Button
                  type="primary"
                  onClick={handleNextStep}
                  disabled={!selectedDatasourceId}
                  icon={<ArrowRightOutlined />}
                >
                  {intl.formatMessage({ id: 'dataset.nextStep' })}
                </Button>
              </Space>
            </Form.Item>
          </Form>
        ) : (
          <div>
            <div style={{ marginBottom: 16 }}>
              <Text strong>{intl.formatMessage({ id: 'dataset.datasetName' })}: </Text>
              <Text>{tempDatasetName}</Text>
              <Button
                type="link"
                size="small"
                onClick={handlePrevStep}
                icon={<ArrowLeftOutlined />}
                style={{ marginLeft: 8 }}
              >
                {intl.formatMessage({ id: 'dataset.modify' })}
              </Button>
            </div>

            <div
              style={{
                marginBottom: 12,
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
              }}
            >
              <Title level={5} style={{ margin: 0 }}>
                {intl.formatMessage({ id: 'dataset.columns' })} ({datasetColumns.length})
              </Title>
              <Button
                type="dashed"
                icon={<FunctionOutlined />}
                onClick={() => handleOpenVirtualFieldModal()}
              >
                {intl.formatMessage({ id: 'virtualField.add' })}
              </Button>
            </div>

            <Table
              dataSource={datasetColumns || []}
              rowKey="name"
              size="small"
              pagination={false}
              rowSelection={{
                selectedRowKeys: datasetColumns
                  .filter((c: any) => c.visible !== false)
                  .map((c: any) => c.name),
                onChange: (selectedRowKeys) => {
                  const newColumns = datasetColumns.map((col: any) => ({
                    ...col,
                    visible: selectedRowKeys.includes(col.name),
                  }));
                  setDatasetColumns(newColumns);
                },
              }}
              columns={[
                {
                  title: intl.formatMessage({ id: 'field.name' }),
                  dataIndex: 'name',
                  key: 'name',
                  render: (name: string, record: any) => (
                    <Space>
                      {record.expr && record.expr !== `\`${record.name}\`` && (
                        <FunctionOutlined style={{ color: '#722ed1' }} />
                      )}
                      <Text
                        strong={!!record.expr}
                        style={{ color: record.role === 'dimension' ? '#1890ff' : '#722ed1' }}
                      >
                        {name}
                      </Text>
                      {record.expr && record.expr !== `\`${record.name}\`` && (
                        <Tag color="purple">{intl.formatMessage({ id: 'field.virtual' })}</Tag>
                      )}
                    </Space>
                  ),
                },
                {
                  title: intl.formatMessage({ id: 'field.type' }),
                  dataIndex: 'type',
                  key: 'type',
                  width: 140,
                  render: (type: string, record: any) => (
                    <Select
                      value={type}
                      size="small"
                      style={{ width: 110 }}
                      onChange={(value) => handleColumnTypeChange(record.name, value)}
                    >
                      <Select.Option value="string">
                        {intl.formatMessage({ id: 'dataType.string' })}
                      </Select.Option>
                      <Select.Option value="int">
                        {intl.formatMessage({ id: 'dataType.integer' })}
                      </Select.Option>
                      <Select.Option value="float">
                        {intl.formatMessage({ id: 'dataType.float' })}
                      </Select.Option>
                      <Select.Option value="date">
                        {intl.formatMessage({ id: 'dataType.date' })}
                      </Select.Option>
                      <Select.Option value="datetime">
                        {intl.formatMessage({ id: 'dataType.datetime' })}
                      </Select.Option>
                      <Select.Option value="boolean">
                        {intl.formatMessage({ id: 'dataType.boolean' })}
                      </Select.Option>
                    </Select>
                  ),
                },
                {
                  title: intl.formatMessage({ id: 'field.role' }),
                  dataIndex: 'role',
                  key: 'role',
                  width: 120,
                  render: (role: string, record: any) => (
                    <Switch
                      checked={role === 'metric'}
                      checkedChildren={intl.formatMessage({ id: 'field.metric' })}
                      unCheckedChildren={intl.formatMessage({ id: 'field.dimension' })}
                      size="small"
                      onChange={(checked) =>
                        handleColumnRoleChange(record.name, checked ? 'metric' : 'dimension')
                      }
                      style={{
                        backgroundColor: role === 'metric' ? '#722ed1' : '#1890ff',
                      }}
                    />
                  ),
                },
                {
                  title: intl.formatMessage({ id: 'field.expr' }),
                  dataIndex: 'expr',
                  key: 'expr',
                  width: 200,
                  render: (expr: string) =>
                    expr ? (
                      <Text code style={{ fontSize: 12 }}>
                        {expr}
                      </Text>
                    ) : (
                      '-'
                    ),
                },
                {
                  title: intl.formatMessage({ id: 'dataset.actions' }),
                  key: 'actions',
                  width: 100,
                  render: (_: any, record: any) => {
                    const isVirtual = record.expr && record.expr !== `\`${record.name}\``;
                    return isVirtual ? (
                      <Space size="small">
                        <Button
                          type="text"
                          size="small"
                          icon={<EditOutlined />}
                          onClick={() => handleOpenVirtualFieldModal(record)}
                        />
                        <Popconfirm
                          title={intl.formatMessage({ id: 'virtualField.deleteConfirm' })}
                          onConfirm={() => {
                            const updatedColumns = (datasetColumns || []).filter(
                              (col) => col.name !== record.name
                            );
                            setDatasetColumns(updatedColumns);
                          }}
                          okText={intl.formatMessage({ id: 'common.yes' })}
                          cancelText={intl.formatMessage({ id: 'common.no' })}
                        >
                          <Button type="text" size="small" danger icon={<DeleteOutlined />} />
                        </Popconfirm>
                      </Space>
                    ) : null;
                  },
                },
              ]}
              style={{ marginBottom: 24 }}
              locale={{
                emptyText: intl.formatMessage({ id: 'dataset.noColumnsAvailable' }),
              }}
            />

            <Form.Item>
              <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
                <Button
                  onClick={() => {
                    setModalVisible(false);
                    resetCreateFlow();
                  }}
                >
                  {intl.formatMessage({ id: 'common.cancel' })}
                </Button>
                <Button onClick={handlePrevStep} icon={<ArrowLeftOutlined />}>
                  {intl.formatMessage({ id: 'dataset.prevStep' })}
                </Button>
                <Button
                  type="primary"
                  onClick={handleFinalSubmit}
                  loading={submitLoading}
                  icon={<PlusOutlined />}
                >
                  {intl.formatMessage({ id: 'dataset.saveAndSubmit' })}
                </Button>
              </Space>
            </Form.Item>
          </div>
        )}
      </Modal>

      {/* Edit Dataset Modal */}
      <Modal
        title={intl.formatMessage({ id: 'dataset.edit' })}
        open={editModalVisible}
        onCancel={() => {
          setEditModalVisible(false);
          editForm.resetFields();
          setEditingDataset(null);
          setSelectedDatasourceId(null);
          setQueryType('table');
        }}
        footer={null}
        width={600}
      >
        <Form
          form={editForm}
          layout="vertical"
          onFinish={handleEditSubmit}
          initialValues={{
            query_type: 'table',
          }}
        >
          <Form.Item
            name="name"
            label={intl.formatMessage({ id: 'dataset.name' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'dataset.pleaseEnterName' }) },
            ]}
          >
            <Input placeholder={intl.formatMessage({ id: 'dataset.pleaseEnterName' })} />
          </Form.Item>

          <Form.Item
            name="datasource_id"
            label={intl.formatMessage({ id: 'dataset.datasource' })}
            rules={[
              {
                required: true,
                message: intl.formatMessage({ id: 'dataset.pleaseSelectDatasource' }),
              },
            ]}
          >
            <Select
              placeholder={intl.formatMessage({ id: 'dataset.pleaseSelectDatasource' })}
              onChange={handleDatasourceChange}
              loading={datasources.length === 0}
            >
              {(Array.isArray(datasources) ? datasources : []).map((ds) => (
                <Select.Option key={ds.id} value={ds.id}>
                  {ds.name}
                </Select.Option>
              ))}
            </Select>
          </Form.Item>

          <Form.Item
            name="query_type"
            label={intl.formatMessage({ id: 'dataset.queryType' })}
            rules={[
              {
                required: true,
                message: intl.formatMessage({ id: 'dataset.pleaseSelectQueryType' }),
              },
            ]}
          >
            <Select
              placeholder={intl.formatMessage({ id: 'dataset.pleaseSelectQueryType' })}
              onChange={handleQueryTypeChange}
            >
              <Select.Option value="table">
                <Space>
                  <TableOutlined />
                  {intl.formatMessage({ id: 'dataset.queryType.table' })}
                </Space>
              </Select.Option>
              <Select.Option value="sql">
                <Space>
                  <DatabaseOutlined />
                  {intl.formatMessage({ id: 'dataset.queryType.sql' })}
                </Space>
              </Select.Option>
            </Select>
          </Form.Item>

          {queryType === 'table' ? (
            <Form.Item
              name="table_name"
              label={intl.formatMessage({ id: 'dataset.tableName' })}
              rules={[
                {
                  required: queryType === 'table',
                  message: intl.formatMessage({ id: 'dataset.pleaseSelectTable' }),
                },
              ]}
            >
              <Select
                placeholder={intl.formatMessage({ id: 'dataset.selectTable' })}
                loading={tablesLoading}
                disabled={!selectedDatasourceId || tablesLoading}
              >
                {tables.map((table) => (
                  <Select.Option key={table.name} value={table.name}>
                    {table.name}
                  </Select.Option>
                ))}
              </Select>
            </Form.Item>
          ) : (
            <Form.Item
              name="query_sql"
              label={intl.formatMessage({ id: 'dataset.sql' })}
              rules={[
                {
                  required: queryType === 'sql',
                  message: intl.formatMessage({ id: 'dataset.pleaseEnterSql' }),
                },
              ]}
            >
              <Input.TextArea
                placeholder={intl.formatMessage({ id: 'dataset.enterSql' })}
                rows={4}
                disabled={!selectedDatasourceId}
              />
            </Form.Item>
          )}

          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button
                onClick={() => {
                  setEditModalVisible(false);
                  editForm.resetFields();
                  setEditingDataset(null);
                  setSelectedDatasourceId(null);
                  setQueryType('table');
                }}
              >
                {intl.formatMessage({ id: 'common.cancel' })}
              </Button>
              <Button
                type="primary"
                htmlType="submit"
                loading={submitLoading}
                icon={<EditOutlined />}
                disabled={!selectedDatasourceId}
              >
                {intl.formatMessage({ id: 'common.save' })}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={
          editingVirtualField
            ? intl.formatMessage({ id: 'virtualField.edit' })
            : intl.formatMessage({ id: 'virtualField.add' })
        }
        open={virtualFieldModalVisible}
        onCancel={() => {
          setVirtualFieldModalVisible(false);
          virtualFieldForm.resetFields();
          setEditingVirtualField(null);
        }}
        footer={null}
        width={500}
      >
        <Form form={virtualFieldForm} layout="vertical">
          <Form.Item
            name="name"
            label={intl.formatMessage({ id: 'field.fieldName' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'field.pleaseEnterFieldName' }) },
            ]}
          >
            <Input placeholder="e.g., total_price" disabled={!!editingVirtualField} />
          </Form.Item>

          <Form.Item
            name="expr"
            label={intl.formatMessage({ id: 'field.expr' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'field.pleaseEnterExpr' }) },
            ]}
            tooltip="Use `source_field` for source fields, [field] for dataset fields. Example: `amount` * 0.3, [revenue] * 0.8"
          >
            <Input.TextArea
              placeholder={intl.formatMessage({ id: 'field.pleaseEnterExpr' })}
              rows={3}
            />
          </Form.Item>

          <Form.Item
            name="type"
            label={intl.formatMessage({ id: 'field.resultType' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'field.pleaseSelectDataType' }) },
            ]}
          >
            <Select placeholder={intl.formatMessage({ id: 'field.pleaseSelectDataType' })}>
              {dataTypes.map((dt) => (
                <Select.Option key={dt.value} value={dt.value}>
                  {dt.label}
                </Select.Option>
              ))}
            </Select>
          </Form.Item>

          <Form.Item
            name="role"
            label={intl.formatMessage({ id: 'field.role' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'field.pleaseSelectRole' }) },
            ]}
          >
            <Select placeholder={intl.formatMessage({ id: 'field.pleaseSelectRole' })}>
              <Select.Option value="dimension">
                {intl.formatMessage({ id: 'field.dimension' })}
              </Select.Option>
              <Select.Option value="metric">
                {intl.formatMessage({ id: 'field.metric' })}
              </Select.Option>
            </Select>
          </Form.Item>

          <Form.Item name="comment" label={intl.formatMessage({ id: 'field.description' })}>
            <Input.TextArea
              placeholder={intl.formatMessage({ id: 'virtualField.descriptionPlaceholder' })}
              rows={2}
            />
          </Form.Item>

          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button
                onClick={() => {
                  setVirtualFieldModalVisible(false);
                  virtualFieldForm.resetFields();
                  setEditingVirtualField(null);
                }}
              >
                {intl.formatMessage({ id: 'common.cancel' })}
              </Button>
              <Button type="primary" loading={savingColumns} onClick={handleSaveVirtualField}>
                {editingVirtualField
                  ? intl.formatMessage({ id: 'common.update' })
                  : intl.formatMessage({ id: 'common.add' })}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default DatasetPage;
