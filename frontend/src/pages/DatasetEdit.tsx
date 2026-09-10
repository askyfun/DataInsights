import {
  ArrowLeftOutlined,
  DatabaseOutlined,
  SaveOutlined,
  TableOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Form,
  Input,
  message,
  Radio,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { useNavigate, useParams } from 'react-router-dom';
import { DatasetColumn, DatasetFormData, datasetsApi, datasourcesApi, TableInfo } from '../api';

const { Title, Text } = Typography;
const { TextArea } = Input;

const DatasetEditPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const intl = useIntl();
  const [form] = Form.useForm<DatasetFormData & { description: string }>();

  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [tables, setTables] = useState<TableInfo[]>([]);
  const [tablesLoading, setTablesLoading] = useState(false);
  const [columnsLoading, setColumnsLoading] = useState(false);
  const [datasetColumns, setDatasetColumns] = useState<DatasetColumn[]>([]);
  const [selectedDatasourceId, setSelectedDatasourceId] = useState<number | null>(null);
  const [queryType, setQueryType] = useState<string>('table');
  const [shardEnabled, setShardEnabled] = useState(false);
  const [datasources, setDatasources] = useState<any[]>([]);

  const datasetId = id ? parseInt(id, 10) : null;

  // Fetch datasources
  useEffect(() => {
    const fetchDatasources = async () => {
      try {
        const response = await datasourcesApi.getAll();
        setDatasources(response.data.data || []);
      } catch (error: any) {
        message.error(error.message || 'Failed to fetch datasources');
      }
    };
    fetchDatasources();
  }, []);

  // Fetch tables from datasource
  const fetchTables = useCallback(async (datasourceId: number) => {
    setTablesLoading(true);
    try {
      const response = await datasourcesApi.getTables(datasourceId);
      setTables(response.data.data || []);
    } catch (error: any) {
      message.error(error.message || 'Failed to fetch tables');
      setTables([]);
    } finally {
      setTablesLoading(false);
    }
  }, []);

  // Fetch columns from dataset
  const fetchColumns = useCallback(async (dsId: number) => {
    setColumnsLoading(true);
    try {
      const response = await datasetsApi.getColumns(dsId);
      setDatasetColumns(response.data.data || []);
    } catch (error: any) {
      message.error(error.message || 'Failed to fetch columns');
      setDatasetColumns([]);
    } finally {
      setColumnsLoading(false);
    }
  }, []);

  // Fetch dataset data
  useEffect(() => {
    if (!datasetId) {
      message.error('Invalid dataset ID');
      navigate('/datasets');
      return;
    }

    const fetchDataset = async () => {
      setLoading(true);
      try {
        const response = await datasetsApi.getById(datasetId);
        const dataset = response.data.data;

        let parsedShardKeys: string[] = [];
        try {
          parsedShardKeys = dataset.shard_keys ? JSON.parse(dataset.shard_keys) : [];
        } catch {
          parsedShardKeys = [];
        }

        form.setFieldsValue({
          name: dataset.name,
          datasource_id: dataset.datasource_id,
          query_type: dataset.query_type,
          table_name: dataset.table_name ?? undefined,
          query_sql: dataset.query_sql ?? undefined,
          description: dataset.description || '',
          shard_keys: parsedShardKeys,
        });

        setSelectedDatasourceId(dataset.datasource_id);
        setQueryType(dataset.query_type);
        setShardEnabled(dataset.shard_enabled || false);

        // Fetch tables if needed
        if (dataset.datasource_id && dataset.query_type === 'table') {
          fetchTables(dataset.datasource_id);
        }

        // Fetch columns
        fetchColumns(datasetId);
      } catch (error: any) {
        message.error(error.message || 'Failed to fetch dataset');
        navigate('/datasets');
      } finally {
        setLoading(false);
      }
    };

    fetchDataset();
  }, [datasetId, navigate, form, fetchTables, fetchColumns]);

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
  };

  // Handle query type change
  const handleQueryTypeChange = (value: string) => {
    setQueryType(value);
    form.setFieldValue('table_name', undefined);
    form.setFieldValue('query_sql', undefined);
  };

  // Handle form submit
  const handleSubmit = async (values: DatasetFormData & { description: string }) => {
    if (!datasetId) return;

    setSubmitting(true);
    try {
      const data: DatasetFormData = {
        name: values.name,
        datasource_id: values.datasource_id,
        query_type: values.query_type,
        table_name: values.query_type === 'table' ? values.table_name : undefined,
        query_sql: values.query_type === 'sql' ? values.query_sql : undefined,
        description: values.description,
        shard_enabled: shardEnabled,
        shard_keys: shardEnabled ? values.shard_keys || [] : [],
      };

      await datasetsApi.update(datasetId, data);
      message.success(intl.formatMessage({ id: 'dataset.edit.success' }));
      navigate('/datasets');
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'dataset.edit.error' }));
    } finally {
      setSubmitting(false);
    }
  };

  // Handle cancel
  const handleCancel = () => {
    navigate('/datasets');
  };

  // Columns for fields table
  const columnsConfig = [
    {
      title: intl.formatMessage({ id: 'dataset.field.name' }),
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: intl.formatMessage({ id: 'dataset.field.type' }),
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => <Tag>{type}</Tag>,
    },
    {
      title: intl.formatMessage({ id: 'dataset.field.role' }),
      dataIndex: 'role',
      key: 'role',
      render: (role: string) => (
        <Tag color={role === 'metric' ? 'blue' : 'default'}>
          {intl.formatMessage({ id: `dataset.field.role.${role}` })}
        </Tag>
      ),
    },
  ];

  if (loading) {
    return (
      <div
        style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}
      >
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div style={{ padding: '24px' }}>
      <Card>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '24px',
          }}
        >
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={handleCancel}>
              {intl.formatMessage({ id: 'common.back' })}
            </Button>
            <Title level={3} style={{ margin: 0 }}>
              {intl.formatMessage({ id: 'dataset.edit.title' })}
            </Title>
          </Space>
        </div>

        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={{
            query_type: 'table',
          }}
        >
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
            <Form.Item
              name="name"
              label={intl.formatMessage({ id: 'dataset.name' })}
              rules={[
                { required: true, message: intl.formatMessage({ id: 'dataset.name.required' }) },
              ]}
            >
              <Input placeholder={intl.formatMessage({ id: 'dataset.name.placeholder' })} />
            </Form.Item>

            <Form.Item
              name="datasource_id"
              label={intl.formatMessage({ id: 'dataset.datasource' })}
              rules={[
                {
                  required: true,
                  message: intl.formatMessage({ id: 'dataset.datasource.required' }),
                },
              ]}
            >
              <Select
                placeholder={intl.formatMessage({ id: 'dataset.datasource.placeholder' })}
                onChange={handleDatasourceChange}
                loading={datasources.length === 0}
              >
                {datasources.map((ds) => (
                  <Select.Option key={ds.id} value={ds.id}>
                    {ds.name}
                  </Select.Option>
                ))}
              </Select>
            </Form.Item>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
            <Form.Item
              name="query_type"
              label={intl.formatMessage({ id: 'dataset.queryType' })}
              rules={[
                {
                  required: true,
                  message: intl.formatMessage({ id: 'dataset.queryType.required' }),
                },
              ]}
            >
              <Radio.Group onChange={(e) => handleQueryTypeChange(e.target.value)}>
                <Radio.Button value="table">
                  <Space>
                    <TableOutlined />
                    {intl.formatMessage({ id: 'dataset.queryType.table' })}
                  </Space>
                </Radio.Button>
                <Radio.Button value="sql">
                  <Space>
                    <DatabaseOutlined />
                    {intl.formatMessage({ id: 'dataset.queryType.sql' })}
                  </Space>
                </Radio.Button>
              </Radio.Group>
            </Form.Item>

            {queryType === 'table' ? (
              <Form.Item
                name="table_name"
                label={intl.formatMessage({ id: 'dataset.tableName' })}
                rules={[
                  {
                    required: queryType === 'table',
                    message: intl.formatMessage({ id: 'dataset.tableName.required' }),
                  },
                ]}
              >
                <Select
                  placeholder={intl.formatMessage({ id: 'dataset.tableName.placeholder' })}
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
                label={intl.formatMessage({ id: 'dataset.querySql' })}
                rules={[
                  {
                    required: queryType === 'sql',
                    message: intl.formatMessage({ id: 'dataset.querySql.required' }),
                  },
                ]}
              >
                <TextArea
                  placeholder="SELECT * FROM table_name"
                  rows={3}
                  disabled={!selectedDatasourceId}
                />
              </Form.Item>
            )}
          </div>

          <Form.Item name="description" label={intl.formatMessage({ id: 'dataset.description' })}>
            <TextArea
              placeholder={intl.formatMessage({ id: 'dataset.description.placeholder' })}
              rows={3}
            />
          </Form.Item>

          <Form.Item label={intl.formatMessage({ id: 'dataset.shard.enabled' })}>
            <Switch checked={shardEnabled} onChange={setShardEnabled} />
          </Form.Item>

          {shardEnabled && (
            <Form.Item
              name="shard_keys"
              label={intl.formatMessage({ id: 'dataset.shard.keys' })}
              rules={[
                {
                  required: true,
                  message: intl.formatMessage({ id: 'dataset.shard.keys.required' }),
                },
              ]}
            >
              <Select
                mode="multiple"
                placeholder={intl.formatMessage({ id: 'dataset.shard.keys.placeholder' })}
                loading={columnsLoading}
              >
                {datasetColumns.map((col) => (
                  <Select.Option key={col.name} value={col.name}>
                    {col.name}
                    <Text type="secondary" style={{ marginLeft: 8 }}>
                      {col.type}
                    </Text>
                  </Select.Option>
                ))}
              </Select>
            </Form.Item>
          )}

          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button onClick={handleCancel}>{intl.formatMessage({ id: 'common.cancel' })}</Button>
              <Button
                type="primary"
                htmlType="submit"
                loading={submitting}
                icon={<SaveOutlined />}
                disabled={!selectedDatasourceId}
              >
                {intl.formatMessage({ id: 'common.save' })}
              </Button>
            </Space>
          </Form.Item>
        </Form>

        {/* Fields Section */}
        <div style={{ marginTop: '32px', borderTop: '1px solid #f0f0f0', paddingTop: '24px' }}>
          <Title level={4}>{intl.formatMessage({ id: 'dataset.fields.title' })}</Title>
          <Text type="secondary" style={{ display: 'block', marginBottom: '16px' }}>
            {intl.formatMessage({ id: 'dataset.fields.description' })}
          </Text>

          <Spin spinning={columnsLoading}>
            <Table
              dataSource={datasetColumns}
              rowKey="name"
              size="small"
              pagination={false}
              columns={columnsConfig}
              locale={{
                emptyText: intl.formatMessage({ id: 'dataset.fields.empty' }),
              }}
            />
          </Spin>
        </div>
      </Card>
    </div>
  );
};

export default DatasetEditPage;
