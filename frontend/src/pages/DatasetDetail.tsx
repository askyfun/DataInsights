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
  Tooltip,
  Typography,
} from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { DatasetColumn } from '../api';
import { DatasetPreview, datasetsApi } from '../api';
import { formatDateTime } from '../lib/format';
import { useStore } from '../store';

const { Title, Text, Paragraph } = Typography;

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

  const handleColumnCommentChange = (name: string, comment: string) => {
    setColumns((prev) => prev.map((col) => (col.name === name ? { ...col, comment } : col)));
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
      width: 220,
      render: (name: string, record: any) => (
        <Space orientation="vertical" size={2} style={{ maxWidth: 200 }}>
          <Space size={4}>
            {record.isVirtual && <FunctionOutlined style={{ color: '#722ed1' }} />}
            <Text
              strong={record.isVirtual}
              title={name}
              style={{
                display: 'inline-block',
                maxWidth: 170,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                verticalAlign: 'bottom',
                color: record.role === 'dimension' ? '#1890ff' : '#722ed1',
              }}
            >
              {name}
            </Text>
          </Space>
          <Space size={4} wrap>
            {shardKeySet.has(name) && (
              <Tag color="orange" style={{ marginInlineEnd: 0 }}>
                {intl.formatMessage({ id: 'field.shardKey' })}
              </Tag>
            )}
            {record.isVirtual && (
              <Tag color="purple" style={{ marginInlineEnd: 0 }}>
                {intl.formatMessage({ id: 'field.virtual' })}
              </Tag>
            )}
            {record.role === 'dimension' && !record.isVirtual && (
              <Tag color="blue" style={{ marginInlineEnd: 0 }}>
                {intl.formatMessage({ id: 'field.dimension' })}
              </Tag>
            )}
            {record.role === 'metric' && !record.isVirtual && (
              <Tag color="purple" style={{ marginInlineEnd: 0 }}>
                {intl.formatMessage({ id: 'field.metric' })}
              </Tag>
            )}
          </Space>
        </Space>
      ),
    },
    {
      title: intl.formatMessage({ id: 'field.description' }),
      dataIndex: 'comment',
      key: 'comment',
      render: (comment: string, record: any) => (
        <Input
          value={comment}
          placeholder={intl.formatMessage({ id: 'field.descriptionPlaceholder' })}
          variant="borderless"
          onChange={(event) => handleColumnCommentChange(record.name, event.target.value)}
          style={{ width: '100%', paddingInline: 0 }}
        />
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
            alignItems: 'flex-start',
            marginBottom: '16px',
          }}
        >
          <div>
            <Space align="center" size={8}>
              <AppstoreOutlined style={{ fontSize: '20px', color: '#1890ff' }} />
              <Title level={4} style={{ margin: 0 }}>
                {displayDataset?.name}
              </Title>
            </Space>
            {displayDataset?.description && (
              <Paragraph type="secondary" style={{ margin: '4px 0 0' }}>
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

        {/* 紧凑元信息条：数据源 / 来源 / 字段数 / 创建时间 / 分片 */}
        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            padding: '8px 12px',
            marginBottom: '16px',
            background: 'rgba(0, 0, 0, 0.02)',
            border: '1px solid rgba(0, 0, 0, 0.06)',
            borderRadius: 6,
            fontSize: 13,
            gap: 20,
          }}
        >
          <span>
            <DatabaseOutlined style={{ marginRight: 6, color: '#8c8c8c' }} />
            {intl.formatMessage({ id: 'dataset.detail.datasource' })}：
            {displayDataset?.datasource_id ? (
              <Link to={`/datasources/${displayDataset.datasource_id}`}>
                {datasource?.name || `#${displayDataset.datasource_id}`}
              </Link>
            ) : (
              <Text type="secondary">—</Text>
            )}
          </span>

          <span>
            <TableOutlined style={{ marginRight: 6, color: '#8c8c8c' }} />
            {intl.formatMessage({ id: 'dataset.detail.source' })}：
            {displayDataset?.query_type === 'table' ? (
              <>
                <Link to={`/datasources/${displayDataset?.datasource_id}`}>
                  <Text code>{displayDataset?.table_name || '—'}</Text>
                </Link>
                <Tag color="blue" style={{ marginLeft: 6 }}>
                  {intl.formatMessage({ id: 'dataset.detail.sourceType.physical' })}
                </Tag>
              </>
            ) : (
              <>
                <Tooltip
                  title={
                    <pre style={{ margin: 0, maxWidth: 520 }}>{displayDataset?.query_sql}</pre>
                  }
                >
                  <Text
                    code
                    style={{
                      display: 'inline-block',
                      maxWidth: 320,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      verticalAlign: 'bottom',
                    }}
                  >
                    {displayDataset?.query_sql || '—'}
                  </Text>
                </Tooltip>
                <Tag color="purple" style={{ marginLeft: 6 }}>
                  {intl.formatMessage({ id: 'dataset.detail.sourceType.sql' })}
                </Tag>
              </>
            )}
          </span>

          <span>
            {intl.formatMessage({ id: 'dataset.detail.fields' })}：{columns.length}
          </span>

          <span>
            {intl.formatMessage({ id: 'dataset.detail.createdAt' })}：
            {formatDateTime(displayDataset?.created_at)}
          </span>

          {displayDataset?.shard_enabled && (
            <span>
              {intl.formatMessage({ id: 'dataset.shard.enabled' })}：{(() => {
                try {
                  const keys: string[] = JSON.parse(displayDataset.shard_keys || '[]');
                  return keys.length > 0
                    ? keys.map((k) => (
                        <Tag key={k} color="orange" style={{ marginLeft: 4 }}>
                          {k}
                        </Tag>
                      ))
                    : '—';
                } catch {
                  return '—';
                }
              })()}
            </span>
          )}
        </div>

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
