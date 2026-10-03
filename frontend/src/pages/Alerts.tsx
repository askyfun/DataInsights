import {
  AlertOutlined,
  DeleteOutlined,
  EditOutlined,
  FieldTimeOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Drawer,
  Input,
  InputNumber,
  Modal,
  message,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
} from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { type AlertRule, type AlertTrigger, alertsApi, type Chart, chartsApi } from '../api';
import { ListSearch, standardListPagination } from '../components/listPage';
import ModalFooter from '../components/ModalFooter';
import PageHeader from '../components/PageHeader';
import { type ChartType, migrateChartConfig } from '../lib/chartConfigSchema';
import { formatDateTime } from '../lib/format';

type AlertOperator = 'gt' | 'lt' | 'eq';

interface MetricOption {
  label: string;
  value: string;
}

interface RuleFormState {
  name: string;
  chart_id?: number;
  metric?: string;
  operator: AlertOperator;
  threshold?: number;
}

const EMPTY_FORM: RuleFormState = { name: '', operator: 'gt' };

const AlertsPage: React.FC = () => {
  const intl = useIntl();
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [charts, setCharts] = useState<Chart[]>([]);

  // 新建/编辑弹窗
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<RuleFormState>(EMPTY_FORM);
  const [metricOptions, setMetricOptions] = useState<MetricOption[]>([]);
  const [metricLoading, setMetricLoading] = useState(false);

  // 触发记录抽屉
  const [triggerRule, setTriggerRule] = useState<AlertRule | null>(null);
  const [triggers, setTriggers] = useState<AlertTrigger[]>([]);
  const [triggersLoading, setTriggersLoading] = useState(false);

  const fetchRules = useCallback(async () => {
    setLoading(true);
    try {
      const res = await alertsApi.getAll();
      setRules(res.data.data);
    } catch (error) {
      message.error(
        error instanceof Error ? error.message : intl.formatMessage({ id: 'common.error' })
      );
    } finally {
      setLoading(false);
    }
  }, [intl]);

  const fetchCharts = useCallback(async () => {
    try {
      const res = await chartsApi.getAll();
      setCharts(res.data.data);
    } catch (error) {
      message.error(
        error instanceof Error ? error.message : intl.formatMessage({ id: 'common.error' })
      );
    }
  }, [intl]);

  // 首次挂载拉取一次即可（与 Charts.tsx 等列表页同口径）
  useEffect(() => {
    fetchRules();
    fetchCharts();
  }, [fetchRules, fetchCharts]);

  const chartName = (chartId: number) =>
    charts.find((c) => c.id === chartId)?.name ?? `#${chartId}`;

  // 指标选项从图表 config 解析：bindingId 是输出的列键，meta alias 优先。
  const loadMetricOptions = async (chartId: number) => {
    setMetricLoading(true);
    setMetricOptions([]);
    try {
      const res = await chartsApi.getById(chartId);
      const chart = res.data.data;
      const doc = migrateChartConfig(chart.config || '', chart.chart_type as ChartType);
      const options: MetricOption[] = [];
      const seen = new Set<string>();
      for (const group of doc.query.metricGroups) {
        for (const binding of group.bindings) {
          const alias = doc.fieldMeta[binding.bindingId]?.alias;
          const value = alias || binding.bindingId;
          if (!seen.has(value)) {
            seen.add(value);
            options.push({ value, label: value });
          }
        }
      }
      setMetricOptions(options);
    } catch (error) {
      message.error(
        error instanceof Error ? error.message : intl.formatMessage({ id: 'common.error' })
      );
    } finally {
      setMetricLoading(false);
    }
  };

  const openCreate = () => {
    setEditingId(null);
    setForm(EMPTY_FORM);
    setMetricOptions([]);
    setModalOpen(true);
  };

  const openEdit = (rule: AlertRule) => {
    setEditingId(rule.id);
    setForm({
      name: rule.name,
      chart_id: rule.chart_id,
      metric: rule.metric,
      operator: rule.operator,
      threshold: rule.threshold,
    });
    setMetricOptions([{ value: rule.metric, label: rule.metric }]);
    setModalOpen(true);
    loadMetricOptions(rule.chart_id);
  };

  const handleSave = async () => {
    if (!form.name.trim() || !form.chart_id || !form.metric || form.threshold === undefined) {
      message.warning(intl.formatMessage({ id: 'alert.formIncomplete' }));
      return;
    }
    setSaving(true);
    try {
      if (editingId) {
        await alertsApi.update(editingId, {
          name: form.name.trim(),
          chart_id: form.chart_id,
          metric: form.metric,
          operator: form.operator,
          threshold: form.threshold,
        });
      } else {
        await alertsApi.create({
          name: form.name.trim(),
          chart_id: form.chart_id,
          metric: form.metric,
          operator: form.operator,
          threshold: form.threshold,
        });
      }
      message.success(intl.formatMessage({ id: 'common.success' }));
      setModalOpen(false);
      fetchRules();
    } catch (error) {
      message.error(
        error instanceof Error ? error.message : intl.formatMessage({ id: 'common.error' })
      );
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await alertsApi.delete(id);
      message.success(intl.formatMessage({ id: 'common.success' }));
      fetchRules();
    } catch (error) {
      message.error(
        error instanceof Error ? error.message : intl.formatMessage({ id: 'common.error' })
      );
    }
  };

  // 行内启用开关：PUT 未提供则保留，只发 enabled。
  const handleToggle = async (rule: AlertRule, enabled: boolean) => {
    try {
      await alertsApi.update(rule.id, { enabled });
      fetchRules();
    } catch (error) {
      message.error(
        error instanceof Error ? error.message : intl.formatMessage({ id: 'common.error' })
      );
    }
  };

  const openTriggers = async (rule: AlertRule) => {
    setTriggerRule(rule);
    setTriggers([]);
    setTriggersLoading(true);
    try {
      const res = await alertsApi.getTriggers(rule.id);
      setTriggers(res.data.data);
    } catch (error) {
      message.error(
        error instanceof Error ? error.message : intl.formatMessage({ id: 'common.error' })
      );
    } finally {
      setTriggersLoading(false);
    }
  };

  const operatorLabel = (op: AlertOperator) => intl.formatMessage({ id: `alert.operator.${op}` });

  const keyword = searchText.trim().toLowerCase();
  const visibleRules = (Array.isArray(rules) ? rules : []).filter((rule) => {
    if (!keyword) return true;
    return (
      rule.name.toLowerCase().includes(keyword) ||
      chartName(rule.chart_id).toLowerCase().includes(keyword) ||
      rule.metric.toLowerCase().includes(keyword)
    );
  });

  const ruleColumns = [
    {
      title: intl.formatMessage({ id: 'alert.name' }),
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => <strong>{text}</strong>,
    },
    {
      title: intl.formatMessage({ id: 'alert.chart' }),
      dataIndex: 'chart_id',
      key: 'chart_id',
      render: (chartId: number) => chartName(chartId),
    },
    {
      title: intl.formatMessage({ id: 'alert.metric' }),
      dataIndex: 'metric',
      key: 'metric',
    },
    {
      title: intl.formatMessage({ id: 'alert.operator' }),
      dataIndex: 'operator',
      key: 'operator',
      width: 80,
      render: (op: AlertOperator) => <Tag>{operatorLabel(op)}</Tag>,
    },
    {
      title: intl.formatMessage({ id: 'alert.threshold' }),
      dataIndex: 'threshold',
      key: 'threshold',
      width: 100,
    },
    {
      title: intl.formatMessage({ id: 'alert.enabled' }),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 90,
      render: (enabled: boolean, record: AlertRule) => (
        <Switch checked={enabled} onChange={(value) => handleToggle(record, value)} />
      ),
    },
    {
      title: intl.formatMessage({ id: 'alert.lastTriggered' }),
      dataIndex: 'last_triggered_date',
      key: 'last_triggered_date',
      render: (text: string | null) => text ?? '-',
    },
    {
      title: intl.formatMessage({ id: 'alert.actions' }),
      key: 'actions',
      width: 230,
      render: (_: unknown, record: AlertRule) => (
        <Space size="small">
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>
            {intl.formatMessage({ id: 'common.edit' })}
          </Button>
          <Button
            type="link"
            size="small"
            icon={<FieldTimeOutlined />}
            onClick={() => openTriggers(record)}
          >
            {intl.formatMessage({ id: 'alert.triggers' })}
          </Button>
          <Popconfirm
            title={intl.formatMessage({ id: 'alert.deleteConfirm' })}
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

  const triggerColumns = [
    {
      title: intl.formatMessage({ id: 'alert.triggerTime' }),
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => formatDateTime(text),
    },
    {
      title: intl.formatMessage({ id: 'alert.metricValue' }),
      dataIndex: 'metric_value',
      key: 'metric_value',
    },
    {
      title: intl.formatMessage({ id: 'alert.threshold' }),
      dataIndex: 'threshold',
      key: 'threshold',
    },
    {
      title: intl.formatMessage({ id: 'alert.message' }),
      dataIndex: 'message',
      key: 'message',
    },
    {
      title: intl.formatMessage({ id: 'alert.notifyStatus' }),
      dataIndex: 'notified',
      key: 'notified',
      render: (notified: boolean, record: AlertTrigger) =>
        notified ? (
          <Tag color="green">{intl.formatMessage({ id: 'alert.notified' })}</Tag>
        ) : (
          <Tag color="red">
            {record.notify_error || intl.formatMessage({ id: 'alert.notNotified' })}
          </Tag>
        ),
    },
  ];

  return (
    <div className="dr-page">
      <PageHeader
        icon={<AlertOutlined />}
        title={intl.formatMessage({ id: 'alert.title' })}
        description={intl.formatMessage({ id: 'alert.manageAlerts' })}
        extra={
          <>
            <Button icon={<ReloadOutlined />} onClick={fetchRules} loading={loading}>
              {intl.formatMessage({ id: 'common.refresh' })}
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              {intl.formatMessage({ id: 'alert.add' })}
            </Button>
          </>
        }
      />

      <Card>
        <div className="dr-card-toolbar">
          <ListSearch
            placeholder={intl.formatMessage({ id: 'alert.searchPlaceholder' })}
            value={searchText}
            onChange={setSearchText}
          />
        </div>
        <Table
          columns={ruleColumns}
          dataSource={visibleRules}
          rowKey="id"
          loading={loading}
          pagination={standardListPagination}
          locale={{ emptyText: intl.formatMessage({ id: 'common.noData' }) }}
          size="small"
        />
      </Card>

      <Modal
        title={intl.formatMessage({
          id: editingId ? 'alert.editTitle' : 'alert.createTitle',
        })}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        footer={null}
        destroyOnHidden
      >
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <div>
            <div>{intl.formatMessage({ id: 'alert.name' })}</div>
            <Input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder={intl.formatMessage({ id: 'alert.namePlaceholder' })}
            />
          </div>
          <div>
            <div>{intl.formatMessage({ id: 'alert.chart' })}</div>
            <Select
              style={{ width: '100%' }}
              value={form.chart_id}
              onChange={(chartId: number) => {
                setForm({ ...form, chart_id: chartId, metric: undefined });
                loadMetricOptions(chartId);
              }}
              options={charts.map((chart) => ({ value: chart.id, label: chart.name }))}
              placeholder={intl.formatMessage({ id: 'alert.selectChart' })}
              showSearch
              optionFilterProp="label"
            />
          </div>
          <div>
            <div>{intl.formatMessage({ id: 'alert.metric' })}</div>
            <Select
              style={{ width: '100%' }}
              value={form.metric}
              onChange={(metric: string) => setForm({ ...form, metric })}
              options={metricOptions}
              loading={metricLoading}
              disabled={!form.chart_id}
              placeholder={intl.formatMessage({
                id: form.chart_id ? 'alert.selectMetric' : 'alert.selectChartFirst',
              })}
            />
          </div>
          <div>
            <div>{intl.formatMessage({ id: 'alert.operator' })}</div>
            <Select
              style={{ width: '100%' }}
              value={form.operator}
              onChange={(operator: AlertOperator) => setForm({ ...form, operator })}
              options={(['gt', 'lt', 'eq'] as AlertOperator[]).map((op) => ({
                value: op,
                label: operatorLabel(op),
              }))}
            />
          </div>
          <div>
            <div>{intl.formatMessage({ id: 'alert.threshold' })}</div>
            <InputNumber
              style={{ width: '100%' }}
              value={form.threshold}
              onChange={(value) => setForm({ ...form, threshold: value ?? undefined })}
            />
          </div>
          <ModalFooter>
            <Button onClick={() => setModalOpen(false)}>
              {intl.formatMessage({ id: 'common.cancel' })}
            </Button>
            <Button type="primary" loading={saving} onClick={handleSave}>
              {intl.formatMessage({ id: 'common.save' })}
            </Button>
          </ModalFooter>
        </Space>
      </Modal>

      <Drawer
        title={
          triggerRule
            ? `${intl.formatMessage({ id: 'alert.triggerRecords' })} - ${triggerRule.name}`
            : intl.formatMessage({ id: 'alert.triggerRecords' })
        }
        width={720}
        open={triggerRule !== null}
        onClose={() => setTriggerRule(null)}
        destroyOnHidden
      >
        <Table
          columns={triggerColumns}
          dataSource={triggers}
          rowKey="id"
          loading={triggersLoading}
          pagination={{ pageSize: 10 }}
          locale={{ emptyText: intl.formatMessage({ id: 'common.noData' }) }}
          size="small"
        />
      </Drawer>
    </div>
  );
};

export default AlertsPage;
