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
import ClickToEdit from '../components/ClickToEdit';
import ModalFooter from '../components/ModalFooter';
import PageHeader from '../components/PageHeader';
import { formatDateTime } from '../lib/format';
import { useStore } from '../store';

const { Text } = Typography;

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

  // 切到「数据预览」tab 时自动加载一次（此前必须手点刷新，空态误导为无数据）
  const handleTabChange = (key: string) => {
    if (key === 'preview' && !preview && !previewLoading) {
      loadPreview();
    }
  };

  // 本地状态的身份一律用列 id，不用列名：列名自本特性起可改，按名索引在改名后
  // 会错位（改完 A 的名字，后续对 A 的编辑会落到名字撞上的另一列）。
  const handleColumnRoleChange = (id: string, role: 'dimension' | 'metric') => {
    setColumns((prev) => prev.map((col) => (col.id === id ? { ...col, role } : col)));
  };

  const handleColumnTypeChange = (id: string, type: string) => {
    setColumns((prev) =>
      prev.map((col) => (col.id === id ? { ...col, type: type as DatasetColumn['type'] } : col))
    );
  };

  const handleColumnCommentChange = (id: string, comment: string) => {
    setColumns((prev) => prev.map((col) => (col.id === id ? { ...col, comment } : col)));
  };

  const handleColumnNameChange = (id: string, name: string) => {
    setColumns((prev) => prev.map((col) => (col.id === id ? { ...col, name } : col)));
  };

  const handleSaveColumns = async () => {
    // 列名是可变展示名，但会作为 SQL 输出别名与响应负载键使用，同名无法区分。
    const trimmed = columns.map((col) => ({ ...col, name: col.name.trim() }));
    if (trimmed.some((col) => col.name === '')) {
      message.error(intl.formatMessage({ id: 'field.pleaseEnterFieldName' }));
      return;
    }
    const names = trimmed.map((col) => col.name);
    if (new Set(names).size !== names.length) {
      message.error(intl.formatMessage({ id: 'field.duplicateName' }));
      return;
    }

    setSavingColumns(true);
    try {
      await datasetsApi.updateColumns(datasetId, trimmed);
      // 字段改名/描述变更后热刷新图表构建页的字段缓存：store 里的 chartBuilderFields
      // 是跨路由共享的快照，若它正属于本数据集，就地重拉——否则用户回到图表查询页时，
      // 查询配置里的字段芯片/筛选芯片仍显示旧名（预览区域走查询响应、会自动更新）。
      const builderState = useStore.getState();
      if (
        builderState.chartBuilderFieldsDatasetId === datasetId &&
        builderState.chartBuilderFields.length > 0
      ) {
        await builderState.fetchDatasetFields(datasetId);
      }
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
          // 空 id = 请后端分配（列的稳定标识由后端 idgen 生成）
          id: '',
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
          col.id === editingVirtualField.id ? newColumn : col
        );
      } else {
        updatedColumns = [...columns, newColumn];
      }

      setSavingColumns(true);
      await datasetsApi.updateColumns(datasetId, updatedColumns);
      // 从服务端重取而非落本地数组：新增列的 id 由后端分配，留在本地副本里它仍是空串，
      // 下一次编辑/删除按 id 匹配就会命中所有未回填的行。
      await loadColumns();

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

  const handleDeleteVirtualField = async (id: string) => {
    try {
      const updatedColumns = columns.filter((col) => col.id !== id);
      setSavingColumns(true);
      await datasetsApi.updateColumns(datasetId, updatedColumns);
      await loadColumns();
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

  // shard_keys 存列 ID；历史数据里存的还是列名，故两种都认（迁移后只剩 ID）
  const isShardKey = (col: DatasetColumn) => shardKeySet.has(col.id) || shardKeySet.has(col.name);

  const columnsManagementTableData = [...columns]
    .sort((a, b) => {
      const aShard = isShardKey(a) ? 0 : 1;
      const bShard = isShardKey(b) ? 0 : 1;
      return aShard - bShard;
    })
    .map((col) => ({
      ...col,
      key: col.id,
    }));

  const fieldsTableColumns = [
    {
      title: intl.formatMessage({ id: 'field.name' }),
      dataIndex: 'name',
      key: 'name',
      width: 260,
      render: (name: string, record: DatasetColumn) => (
        // 单行布局：虚拟字段图标 + 可点开即改的字段名 + 附属小标签（分片/虚拟）。
        // 维度/指标不再在这里重复显示——role 列的开关与语义色已承载该信息。
        <Space size={4} style={{ maxWidth: 240 }} wrap>
          {record.expr && record.expr !== record.name && (
            <FunctionOutlined style={{ color: 'var(--dr-date)' }} aria-hidden />
          )}
          <ClickToEdit
            value={name}
            onChange={(value) => handleColumnNameChange(record.id, value)}
            placeholder={intl.formatMessage({ id: 'field.pleaseEnterFieldName' })}
            // 与图表构建页的字段语义同源：维度蓝 / 指标绿
            color={record.role === 'dimension' ? 'var(--dr-dim)' : 'var(--dr-metric)'}
            testId={`field-name-${record.id}`}
            ariaLabel={intl.formatMessage({ id: 'field.name' })}
            style={{ maxWidth: 130 }}
          />
          {isShardKey(record) && (
            <Tag color="orange" style={{ marginInlineEnd: 0 }}>
              {intl.formatMessage({ id: 'field.shardKey' })}
            </Tag>
          )}
          {record.expr && record.expr !== record.name && (
            <Tag color="purple" style={{ marginInlineEnd: 0 }}>
              {intl.formatMessage({ id: 'field.virtual' })}
            </Tag>
          )}
        </Space>
      ),
    },
    {
      title: intl.formatMessage({ id: 'field.description' }),
      dataIndex: 'comment',
      key: 'comment',
      render: (comment: string, record: any) => (
        // 与字段名列同一套「点开即改」交互：默认像只读文本，点击出现光标可直接打字。
        <ClickToEdit
          value={comment}
          onChange={(value) => handleColumnCommentChange(record.id, value)}
          placeholder={intl.formatMessage({ id: 'field.descriptionPlaceholder' })}
          testId={`field-comment-${record.id}`}
          ariaLabel={intl.formatMessage({ id: 'field.description' })}
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
          onChange={(value) => handleColumnTypeChange(record.id, value)}
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
            handleColumnRoleChange(record.id, checked ? 'metric' : 'dimension')
          }
          style={{
            // 角色语义色统一：指标绿 / 维度蓝（与字段名文字色、图表构建页同源），
            // 不再让「维度」态落回 antd 默认灰——灰在本页易与禁用态混淆。
            backgroundColor: role === 'metric' ? 'var(--dr-metric)' : 'var(--dr-dim)',
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
      render: (_: unknown, record: DatasetColumn) => {
        // 虚拟字段 = 表达式不等于列名本身（物理列的 expr 恰为来源列名）
        if (!record.expr || record.expr === record.name) return null;
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
              onConfirm={() => handleDeleteVirtualField(record.id)}
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
      <div className="dr-page">
        <div className="dr-state">
          <Spin tip={intl.formatMessage({ id: 'common.loading' })} />
        </div>
      </div>
    );
  }

  if (!currentDataset && !dataset) {
    return (
      <div className="dr-page">
        <div className="dr-state">
          <Spin tip={intl.formatMessage({ id: 'common.loading' })} />
        </div>
      </div>
    );
  }

  const displayDataset = dataset || currentDataset;

  return (
    <div className="dr-page">
      <PageHeader
        icon={<AppstoreOutlined />}
        breadcrumb={
          <Breadcrumb
            items={[
              {
                title: <Link to="/datasets">{intl.formatMessage({ id: 'nav.datasets' })}</Link>,
              },
              {
                title: displayDataset?.name,
              },
            ]}
          />
        }
        title={displayDataset?.name}
        description={displayDataset?.description}
        extra={
          <>
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
          </>
        }
      />

      <Card>
        {/* 紧凑元信息条：数据源 / 来源 / 字段数 / 创建时间 / 分片 */}
        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            padding: '8px 12px',
            marginBottom: '16px',
            background: 'var(--dr-sunken)',
            border: '1px solid var(--dr-border)',
            borderRadius: 6,
            fontSize: 13,
            gap: 20,
          }}
        >
          <span>
            <DatabaseOutlined style={{ marginRight: 6, color: 'var(--dr-text-3)' }} />
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
            <TableOutlined style={{ marginRight: 6, color: 'var(--dr-text-3)' }} />
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
          onChange={handleTabChange}
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
            <Input placeholder="e.g., total_price" />
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
            <ModalFooter>
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
            </ModalFooter>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default DatasetDetailPage;
