import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  EyeOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  message,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import { useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { useNavigate } from 'react-router-dom';
import { DatasourceFormData, DatasourceType } from '../api';
import PageHeader from '../components/PageHeader';
import { formatDateTime } from '../lib/format';
import { useStore } from '../store';

const { Text } = Typography;

const DatasourcePage: React.FC = () => {
  const intl = useIntl();
  const [form] = Form.useForm<DatasourceFormData>();
  const [modalVisible, setModalVisible] = useState(false);
  const [testLoading, setTestLoading] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);

  const {
    datasources,
    datasourcesLoading,
    fetchDatasources,
    addDatasource,
    updateDatasource,
    deleteDatasource,
    datasourcesError,
  } = useStore();

  const navigate = useNavigate();

  useEffect(() => {
    fetchDatasources();
  }, [fetchDatasources]);

  useEffect(() => {
    if (datasourcesError) {
      message.error(datasourcesError);
    }
  }, [datasourcesError]);

  const handleSubmit = async (values: DatasourceFormData) => {
    setSubmitLoading(true);
    try {
      if (editingId) {
        await updateDatasource(editingId, values);
        message.success(intl.formatMessage({ id: 'common.success' }));
      } else {
        await addDatasource(values);
        message.success(intl.formatMessage({ id: 'common.success' }));
      }
      setModalVisible(false);
      form.resetFields();
      setEditingId(null);
      fetchDatasources();
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setSubmitLoading(false);
    }
  };

  const handleTestConnection = async () => {
    try {
      const values = await form.validateFields();
      setTestLoading(true);
      const { datasourcesApi } = await import('../api');
      await datasourcesApi.testConnection({
        type: values.type,
        host: values.host,
        port: values.port,
        database_name: values.database_name,
        username: values.username,
        password: values.password,
      });
      message.success(intl.formatMessage({ id: 'datasource.connectionSuccess' }));
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'datasource.connectionFailed' }));
    } finally {
      setTestLoading(false);
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await deleteDatasource(id);
      message.success(intl.formatMessage({ id: 'common.success' }));
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    }
  };

  const handleEdit = (record: any) => {
    setEditingId(record.id);
    form.setFieldsValue({
      name: record.name,
      type: record.type || 'postgresql',
      host: record.host,
      port: record.port,
      database_name: record.database_name,
      username: record.username,
      password: record.password,
    });
    setModalVisible(true);
  };

  const getTypeInfo = (type: string) => {
    const typeMap: Record<string, { label: string; color: string }> = {
      postgresql: { label: 'PostgreSQL', color: 'blue' },
      mysql: { label: 'MySQL', color: 'green' },
      clickhouse: { label: 'ClickHouse', color: 'purple' },
      starrocks: { label: 'StarRocks', color: 'orange' },
    };
    return typeMap[type] || { label: type, color: 'blue' };
  };

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 60,
    },
    {
      title: intl.formatMessage({ id: 'datasource.name' }),
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: any) => {
        const typeInfo = getTypeInfo(record.type);
        return (
          <Space>
            <DatabaseOutlined />
            <Text strong>{text}</Text>
            <Tag color={typeInfo.color}>{typeInfo.label}</Tag>
          </Space>
        );
      },
    },
    {
      title: intl.formatMessage({ id: 'datasource.host' }),
      dataIndex: 'host',
      key: 'host',
      render: (text: string) => <Tag color="blue">{text}</Tag>,
    },
    {
      title: intl.formatMessage({ id: 'datasource.port' }),
      dataIndex: 'port',
      key: 'port',
      width: 100,
    },
    {
      title: intl.formatMessage({ id: 'datasource.database' }),
      dataIndex: 'database_name',
      key: 'database_name',
    },
    {
      title: intl.formatMessage({ id: 'datasource.username' }),
      dataIndex: 'username',
      key: 'username',
    },
    {
      title: intl.formatMessage({ id: 'datasource.createdAt' }),
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => formatDateTime(text),
    },
    {
      title: intl.formatMessage({ id: 'datasource.actions' }),
      key: 'actions',
      width: 200,
      render: (_: any, record: any) => (
        <Space size="small">
          <Button
            type="link"
            size="small"
            icon={<EyeOutlined />}
            aria-label="View data source details"
            onClick={() => navigate(`/datasources/${record.id}`)}
          >
            {intl.formatMessage({ id: 'common.view' })}
          </Button>
          <Button
            type="link"
            size="small"
            aria-label="Edit data source"
            onClick={() => handleEdit(record)}
          >
            {intl.formatMessage({ id: 'common.edit' })}
          </Button>
          <Popconfirm
            title={intl.formatMessage({ id: 'datasource.deleteConfirmTitle' })}
            description={intl.formatMessage({ id: 'datasource.deleteConfirmDesc' })}
            onConfirm={() => handleDelete(record.id)}
            okText={intl.formatMessage({ id: 'common.yes' })}
            cancelText={intl.formatMessage({ id: 'common.no' })}
          >
            <Button
              type="link"
              size="small"
              danger
              icon={<DeleteOutlined />}
              aria-label="Delete data source"
            >
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
        icon={<DatabaseOutlined />}
        title={intl.formatMessage({ id: 'datasource.dataSources' })}
        description={intl.formatMessage({ id: 'datasource.manageConnections' })}
        extra={
          <>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => fetchDatasources()}
              loading={datasourcesLoading}
            >
              {intl.formatMessage({ id: 'common.refresh' })}
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => {
                form.resetFields();
                setEditingId(null);
                setModalVisible(true);
              }}
            >
              {intl.formatMessage({ id: 'datasource.add' })}
            </Button>
          </>
        }
      />

      <Card>
        <Table
          columns={columns}
          dataSource={Array.isArray(datasources) ? datasources : []}
          rowKey="id"
          loading={datasourcesLoading}
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

      <Modal
        title={
          editingId
            ? intl.formatMessage({ id: 'datasource.edit' })
            : intl.formatMessage({ id: 'datasource.add' })
        }
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false);
          form.resetFields();
          setEditingId(null);
        }}
        footer={null}
        width={600}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={{
            type: 'postgresql',
            port: 5432,
          }}
        >
          <Form.Item
            name="name"
            label={intl.formatMessage({ id: 'datasource.name' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'datasource.pleaseEnterName' }) },
            ]}
          >
            <Input placeholder="My Database" />
          </Form.Item>

          <Form.Item
            name="type"
            label={intl.formatMessage({ id: 'datasource.selectType' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'datasource.selectType' }) },
            ]}
          >
            <Select
              placeholder={intl.formatMessage({ id: 'datasource.selectType' })}
              onChange={(value: DatasourceType) => {
                const defaultPorts: Record<DatasourceType, number> = {
                  postgresql: 5432,
                  clickhouse: 8123,
                  mysql: 3306,
                  starrocks: 9030,
                };
                form.setFieldsValue({
                  port: defaultPorts[value],
                });
              }}
            >
              <Select.Option value="postgresql">PostgreSQL</Select.Option>
              <Select.Option value="mysql">MySQL</Select.Option>
              <Select.Option value="clickhouse">ClickHouse</Select.Option>
              <Select.Option value="starrocks">StarRocks</Select.Option>
            </Select>
          </Form.Item>

          <Form.Item
            name="host"
            label={intl.formatMessage({ id: 'datasource.host' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'datasource.pleaseEnterHost' }) },
            ]}
          >
            <Input placeholder="localhost" />
          </Form.Item>

          <Form.Item
            name="port"
            label={intl.formatMessage({ id: 'datasource.port' })}
            rules={[
              { required: true, message: intl.formatMessage({ id: 'datasource.pleaseEnterPort' }) },
            ]}
          >
            <InputNumber style={{ width: '100%' }} placeholder="5432" />
          </Form.Item>

          <Form.Item
            name="database_name"
            label={intl.formatMessage({ id: 'datasource.database' })}
            rules={[
              {
                required: true,
                message: intl.formatMessage({ id: 'datasource.pleaseEnterDatabase' }),
              },
            ]}
          >
            <Input placeholder="mydb" />
          </Form.Item>

          <Form.Item
            name="username"
            label={intl.formatMessage({ id: 'datasource.username' })}
            rules={[
              {
                required: true,
                message: intl.formatMessage({ id: 'datasource.pleaseEnterUsername' }),
              },
            ]}
          >
            <Input placeholder="postgres" autoComplete="username" />
          </Form.Item>

          <Form.Item name="password" label={intl.formatMessage({ id: 'datasource.password' })}>
            <Input.Password placeholder="********" autoComplete="current-password" />
          </Form.Item>

          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button
                icon={<CheckCircleOutlined />}
                onClick={handleTestConnection}
                loading={testLoading}
              >
                {intl.formatMessage({ id: 'datasource.testConnection' })}
              </Button>
              <Button
                icon={<CloseCircleOutlined />}
                onClick={() => {
                  setModalVisible(false);
                  form.resetFields();
                  setEditingId(null);
                }}
              >
                {intl.formatMessage({ id: 'common.cancel' })}
              </Button>
              <Button
                type="primary"
                htmlType="submit"
                loading={submitLoading}
                icon={<PlusOutlined />}
              >
                {editingId
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

export default DatasourcePage;
