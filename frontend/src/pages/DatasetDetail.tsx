import {
  AppstoreOutlined,
  ArrowLeftOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  EditOutlined,
  FunctionOutlined,
  ReloadOutlined,
  SaveOutlined,
  TableOutlined,
} from '@ant-design/icons';
import {
  Breadcrumb,
  Button,
  Card,
  Descriptions,
  Form,
  Input,
  Modal,
  message,
  Popconfirm,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
} from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { DatasetColumn } from '../api';
import { DatasetPreview, datasetsApi } from '../api';
import { useStore } from '../store';

const { Title, Text, Paragraph } = Typography;

const getQueryTypeInfo = (type: string) => {
  const typeMap: Record<string, { label: string; color: string }> = {
    table: { label: 'Table', color: 'blue' },
    sql: { label: 'SQL', color: 'green' },
  };
  return typeMap[type] || { label: type, color: 'blue' };
};

const DatasetDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const intl = useIntl();
  const [form] = Form.useForm();
  const datasetId = Number(id);

  const { datasets, fetchDatasets, datasources, fetchDatasources, deleteDataset } = useStore();

  const [dataset, setDataset] = useState<any>(null);
  const [datasetLoading, setDatasetLoading] = useState(true);
  const [columns, setColumns] = useState<DatasetColumn[]>([]);
  const [columnsLoading, setColumnsLoading] = useState(false);
  const [savingColumns, setSavingColumns] = useState(false);
  const [preview, setPreview] = useState<DatasetPreview | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [virtualFieldModalVisible, setVirtualFieldModalVisible] = useState(false);
  const [editingVirtualField, setEditingVirtualField] = useState<DatasetColumn | null>(null);

  const currentDataset = datasets.find((ds) => ds.id === datasetId);
  const datasource = datasources.find((ds) => ds.id === currentDataset?.datasource_id);

  useEffect(() => {
    if (datasets.length === 0) {
      fetchDatasets();
    }
  }, [datasets.length, fetchDatasets]);

  useEffect(() => {
    if (datasources.length === 0) {
      fetchDatasources();
    }
  }, [datasources.length, fetchDatasources]);

  const loadDataset = useCallback(async () => {
    setDatasetLoading(true);
    try {
      const response = await datasetsApi.getById(datasetId);
      setDataset(response.data.data);
    } catch (error: any) {
      message.error(error.message || 'Failed to load dataset');
    } finally {
      setDatasetLoading(false);
    }
  }, [datasetId]);

  useEffect(() => {
    if (datasetId) {
      loadDataset();
    }
  }, [datasetId, loadDataset]);

  const loadColumns = useCallback(async () => {
    setColumnsLoading(true);
    try {
      const response = await datasetsApi.getColumns(datasetId);
      setColumns(response.data.data);
    } catch (error: any) {
      message.error(error.message || 'Failed to load columns');
    } finally {
      setColumnsLoading(false);
    }
  }, [datasetId]);

  useEffect(() => {
    if (datasetId) {
      loadColumns();
    }
  }, [datasetId, loadColumns]);

  const loadPreview = async () => {
    setPreviewLoading(true);
    try {
      const response = await datasetsApi.getPreview(datasetId);
      setPreview(response.data.data);
    } catch (error: any) {
      message.error(error.message || 'Failed to load preview');
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleColumnRoleChange = (name: string, role: 'dimension' | 'metric') => {
    setColumns((prev) => prev.map((col) => (col.name === name ? { ...col, role } : col)));
  };

  const handleColumnTypeChange = (name: string, type: string) => {
    setColumns((prev) =>
      prev.map((col) => (col.name === name ? { ...col, type: type as DatasetColumn['type'] } : col))
    );
  };

  const handleSaveColumns = async () => {
    setSavingColumns(true);
    try {
      await datasetsApi.updateColumns(datasetId, columns);
      message.success(intl.formatMessage({ id: 'common.success' }));
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setSavingColumns(false);
    }
  };

  const handleOpenVirtualFieldModal = (record?: DatasetColumn) => {
    setEditingVirtualField(record || null);
    if (record) {
      form.setFieldsValue(record);
    } else {
      form.resetFields();
    }
    setVirtualFieldModalVisible(true);
  };

  const handleSaveVirtualField = async () => {
    try {
      const values = await form.validateFields();
      const isEditing = !!editingVirtualField;

      let newColumn: DatasetColumn;
      if (isEditing && editingVirtualField) {
        newColumn = { ...editingVirtualField, ...values };
      } else {
        newColumn = {
          name: values.name,
          type: values.type || 'string',
          role: values.role || 'dimension',
          expr: values.expression || '',
          comment: values.comment || '',
        };
      }

      let updatedColumns: DatasetColumn[];
      if (isEditing) {
        updatedColumns = columns.map((col) =>
          col.name === editingVirtualField.name ? newColumn : col
        );
      } else {
        updatedColumns = [...columns, newColumn];
      }

      setSavingColumns(true);
      await datasetsApi.updateColumns(datasetId, updatedColumns);
      setColumns(updatedColumns);

      setVirtualFieldModalVisible(false);
      form.resetFields();
      setEditingVirtualField(null);
      message.success(intl.formatMessage({ id: 'common.success' }));
    } catch (error: any) {
      if (error.errorFields) {
        return;
      }
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setSavingColumns(false);
    }
  };

  const handleDeleteVirtualField = async (name: string) => {
    try {
      const updatedColumns = columns.filter((col) => col.name !== name);
      setSavingColumns(true);
      await datasetsApi.updateColumns(datasetId, updatedColumns);
      setColumns(updatedColumns);
      message.success(intl.formatMessage({ id: 'common.success' }));
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setSavingColumns(false);
    }
  };

  const handleDelete = () => {
    Modal.confirm({
      title: intl.formatMessage({ id: 'dataset.detail.deleteConfirmTitle' }),
      content: intl.formatMessage(
        { id: 'dataset.detail.deleteConfirmContent' },
        { name: dataset?.name }
      ),
      okText: intl.formatMessage({ id: 'common.delete' }),
      okButtonProps: { danger: true },
      cancelText: intl.formatMessage({ id: 'common.cancel' }),
      onOk: async () => {
        try {
          await deleteDataset(datasetId);
          message.success(intl.formatMessage({ id: 'dataset.detail.deleteSuccess' }));
          navigate('/datasets');
        } catch (error: any) {
          message.error(error.message || 'Failed to delete dataset');
        }
      },
    });
  };

  const shardKeys: string[] = (() => {
    try {
      return JSON.parse((dataset || currentDataset)?.shard_keys || '[]');
    } catch {
      return [];
    }
  })();
  const shardKeySet = new Set(shardKeys);

  const columnsManagementTableData = [...columns]
    .sort((a, b) => {
      const aShard = shardKeySet.has(a.name) ? 0 : 1;
      const bShard = shardKeySet.has(b.name) ? 0 : 1;
      return aShard - bShard;
    })
    .map((col) => ({
      ...col,
      key: col.name,
    }));

  const fieldsTableColumns = [
    {
      title: intl.formatMessage({ id: 'field.name' }),
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: any) => (
        <Space>
          {record.isVirtual && <FunctionOutlined style={{ color: '#722ed1' }} />}
          <Text
            strong={record.isVirtual}
            style={{ color: record.role === 'dimension' ? '#1890ff' : '#722ed1' }}
          >
            {name}
          </Text>
          {shardKeySet.has(name) && (
            <Tag color="orange">{intl.formatMessage({ id: 'field.shardKey' })}</Tag>
          )}
          {record.isVirtual && (
            <Tag color="purple">{intl.formatMessage({ id: 'field.virtual' })}</Tag>
          )}
          {record.role === 'dimension' && !record.isVirtual && (
            <Tag color="blue">{intl.formatMessage({ id: 'field.dimension' })}</Tag>
          )}
          {record.role === 'metric' && !record.isVirtual && (
            <Tag color="purple">{intl.formatMessage({ id: 'field.metric' })}</Tag>
          )}
        </Space>
      ),
    },
    {
      title: intl.formatMessage({ id: 'field.type' }),
      dataIndex: 'type',
      key: 'type',
      width: 140,
      render: (dataType: string, record: any) => (
        <Select
          value={dataType}
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
          <Select.Option value="date">{intl.formatMessage({ id: 'dataType.date' })}</Select.Option>
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
      title: intl.formatMessage({ id: 'field.expression' }),
      dataIndex: 'expr',
      key: 'expr',
      width: 150,
      render: (expression: string) =>
        expression ? (
          <Text code style={{ fontSize: 12 }}>
            {expression}
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
        if (!record.isVirtual) return null;
        return (
          <Space size="small">
            <Button
              type="text"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleOpenVirtualFieldModal(record)}
            />
            <Popconfirm
              title={intl.formatMessage({ id: 'virtualField.deleteConfirm' })}
              onConfirm={() => handleDeleteVirtualField(record.name)}
              okText={intl.formatMessage({ id: 'common.yes' })}
              cancelText={intl.formatMessage({ id: 'common.no' })}
            >
              <Button type="text" size="small" danger icon={<DeleteOutlined />} />
            </Popconfirm>
          </Space>
        );
      },
    },
  ];

  const previewColumns =
    preview?.columns.map((col) => ({
      title: col,
      dataIndex: col,
      key: col,
      ellipsis: true,
    })) || [];

  if (datasetLoading) {
    return (
      <div style={{ padding: '24px' }}>
        <Spin tip={intl.formatMessage({ id: 'common.loading' })} />
      </div>
    );
  }

  if (!currentDataset && !dataset) {
    return (
      <div style={{ padding: '24px' }}>
        <Spin tip={intl.formatMessage({ id: 'common.loading' })} />
      </div>
    );
  }

  const displayDataset = dataset || currentDataset;

  return (
    <div style={{ padding: '24px' }}>
      <Card>
        <Breadcrumb
          style={{ marginBottom: '16px' }}
          items={[
            {
              title: <Link to="/datasets">{intl.formatMessage({ id: 'nav.datasets' })}</Link>,
            },
            {
              title: displayDataset?.name,
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
              <AppstoreOutlined style={{ fontSize: '24px', color: '#1890ff' }} />
              <Title level={3} style={{ margin: 0 }}>
                {displayDataset?.name}
              </Title>
              <Tag color={getQueryTypeInfo(displayDataset?.query_type).color}>
                {getQueryTypeInfo(displayDataset?.query_type).label}
              </Tag>
            </Space>
            {displayDataset?.description && (
              <Paragraph type="secondary" style={{ marginTop: '8px', marginBottom: 0 }}>
                {displayDataset.description}
              </Paragraph>
            )}
          </div>
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/datasets')}>
              {intl.formatMessage({ id: 'common.back' })}
            </Button>
            <Button
              icon={<EditOutlined />}
              onClick={() =>
                message.info(intl.formatMessage({ id: 'dataset.detail.editComingSoon' }))
              }
            >
              {intl.formatMessage({ id: 'common.edit' })}
            </Button>
            <Button icon={<DeleteOutlined />} danger onClick={handleDelete}>
              {intl.formatMessage({ id: 'common.delete' })}
            </Button>
          </Space>
        </div>

        <Descriptions bordered column={2} style={{ marginBottom: '24px' }}>
          <Descriptions.Item label={intl.formatMessage({ id: 'dataset.detail.datasource' })}>
            <Space>
              <DatabaseOutlined />
              <Text>{datasource?.name || `-`}</Text>
            </Space>
          </Descriptions.Item>
          <Descriptions.Item label={intl.formatMessage({ id: 'dataset.detail.queryType' })}>
            {getQueryTypeInfo(displayDataset?.query_type).label}
          </Descriptions.Item>
          <Descriptions.Item label={intl.formatMessage({ id: 'dataset.detail.source' })} span={2}>
            {displayDataset?.query_type === 'table' ? (
              <Space>
                <TableOutlined />
                <Text code>{displayDataset?.table_name}</Text>
              </Space>
            ) : (
              <Text code style={{ display: 'block', maxHeight: '100px', overflow: 'auto' }}>
                {displayDataset?.query_sql}
              </Text>
            )}
          </Descriptions.Item>
          <Descriptions.Item label={intl.formatMessage({ id: 'dataset.detail.createdAt' })}>
            {displayDataset?.created_at
              ? new Date(displayDataset.created_at).toLocaleString()
              : '-'}
          </Descriptions.Item>
          <Descriptions.Item label={intl.formatMessage({ id: 'dataset.detail.fields' })}>
            {columns.length} {intl.formatMessage({ id: 'dataset.detail.fieldsCount' })}
          </Descriptions.Item>
          <Descriptions.Item label={intl.formatMessage({ id: 'dataset.shard.enabled' })}>
            {displayDataset?.shard_enabled ? (
              <Tag color="orange">{intl.formatMessage({ id: 'dataset.shard.enabled.yes' })}</Tag>
            ) : (
              <Tag>{intl.formatMessage({ id: 'dataset.shard.enabled.no' })}</Tag>
            )}
          </Descriptions.Item>
          {displayDataset?.shard_enabled && (
            <Descriptions.Item label={intl.formatMessage({ id: 'dataset.shard.keys' })}>
              {(() => {
                try {
                  const keys: string[] = JSON.parse(displayDataset.shard_keys || '[]');
                  return keys.length > 0
                    ? keys.map((k) => (
                        <Tag key={k} color="orange">
                          {k}
                        </Tag>
                      ))
                    : '-';
                } catch {
                  return '-';
                }
              })()}
            </Descriptions.Item>
          )}
        </Descriptions>

        <Tabs
          defaultActiveKey="fields"
          items={[
            {
              key: 'fields',
              label: intl.formatMessage({ id: 'dataset.detail.tabFields' }),
              children: (
                <div>
                  <div
                    style={{
                      marginBottom: '16px',
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                    }}
                  >
                    <Space>
                      <Button
                        icon={<ReloadOutlined />}
                        onClick={loadColumns}
                        loading={columnsLoading}
                      >
                        {intl.formatMessage({ id: 'common.refresh' })}
                      </Button>
                      <Button
                        type="dashed"
                        icon={<FunctionOutlined />}
                        onClick={() => handleOpenVirtualFieldModal()}
                      >
                        {intl.formatMessage({ id: 'virtualField.add' })}
                      </Button>
                    </Space>
                    <Button
                      type="primary"
                      icon={<SaveOutlined />}
                      loading={savingColumns}
                      onClick={handleSaveColumns}
                    >
                      {intl.formatMessage({ id: 'common.save' })}
                    </Button>
                  </div>
                  <Table
                    columns={fieldsTableColumns}
                    dataSource={columnsManagementTableData}
                    rowKey="key"
                    loading={columnsLoading}
                    pagination={{
                      pageSize: 10,
                      showSizeChanger: true,
                    }}
                    locale={{
                      emptyText: intl.formatMessage({ id: 'dataset.detail.noFields' }),
                    }}
                    size="small"
                  />
                </div>
              ),
            },
            {
              key: 'preview',
              label: intl.formatMessage({ id: 'dataset.detail.tabPreview' }),
              children: (
                <div>
                  <div style={{ marginBottom: '16px' }}>
                    <Button
                      icon={<ReloadOutlined />}
                      onClick={loadPreview}
                      loading={previewLoading}
                    >
                      {intl.formatMessage({ id: 'common.refresh' })}
                    </Button>
                  </div>
                  <Table
                    columns={previewColumns}
                    dataSource={preview?.data || []}
                    loading={previewLoading}
                    pagination={{
                      pageSize: 10,
                      showSizeChanger: true,
                    }}
                    scroll={{ x: 'max-content' }}
                    locale={{
                      emptyText: intl.formatMessage({ id: 'dataset.detail.noPreview' }),
                    }}
                    size="small"
                  />
                </div>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        title={
          editingVirtualField
            ? intl.formatMessage({ id: 'virtualField.edit' })
            : intl.formatMessage({ id: 'virtualField.add' })
        }
        open={virtualFieldModalVisible}
        onCancel={() => {
          setVirtualFieldModalVisible(false);
          form.resetFields();
          setEditingVirtualField(null);
        }}
        footer={null}
        width={500}
      >
        <Form form={form} layout="vertical">
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
            name="dataType"
            label={intl.formatMessage({ id: 'field.dataType' })}
            initialValue="string"
          >
            <Select>
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
          </Form.Item>
          <Form.Item
            name="role"
            label={intl.formatMessage({ id: 'field.role' })}
            initialValue="dimension"
          >
            <Select>
              <Select.Option value="dimension">
                {intl.formatMessage({ id: 'field.dimension' })}
              </Select.Option>
              <Select.Option value="metric">
                {intl.formatMessage({ id: 'field.metric' })}
              </Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="expression"
            label={intl.formatMessage({ id: 'field.expression' })}
            rules={[
              {
                required: true,
                message: intl.formatMessage({ id: 'field.pleaseEnterExpression' }),
              },
            ]}
          >
            <Input.TextArea placeholder="e.g., price * quantity" rows={3} />
          </Form.Item>
          <Form.Item name="comment" label={intl.formatMessage({ id: 'field.description' })}>
            <Input.TextArea
              placeholder={intl.formatMessage({ id: 'field.descriptionPlaceholder' })}
              rows={2}
            />
          </Form.Item>
          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button
                onClick={() => {
                  setVirtualFieldModalVisible(false);
                  form.resetFields();
                  setEditingVirtualField(null);
                }}
              >
                {intl.formatMessage({ id: 'common.cancel' })}
              </Button>
              <Button type="primary" onClick={handleSaveVirtualField}>
                {intl.formatMessage({ id: 'common.save' })}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default DatasetDetailPage;
