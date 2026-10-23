import { Button, Form, Input, Modal, message, Select } from 'antd';
import { useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import type { DatasetColumn } from '../../api';
import { datasetsApi } from '../../api';
import { type DataType, normalizeDataType } from '../../lib/dataTypes';
import { validateExpr } from '../../lib/expr';
import ExpressionEditor from '../ExpressionEditor';
import ModalFooter from '../ModalFooter';

export interface ComputedFieldModalProps {
  open: boolean;
  /** 为空时不渲染表单（调用方未选择数据集）。 */
  datasetId: number | null;
  onClose: () => void;
  /** 保存成功后的回调：调用方据此重拉图表字段列表。 */
  onSaved: () => void | Promise<void>;
}

/** 表单值 → wire 列形状的映射（表单名与 DatasetColumn 字段名不同）。 */
interface ComputedFieldFormValues {
  name: string;
  dataType: DataType;
  role: 'dimension' | 'metric';
  expression: string;
}

/**
 * 图表画布内「新建计算字段」（issue #181）。
 *
 * 写入路径复用 datasetsApi.updateColumns——也就是后端 UpdateColumns 的 expr 白名单闸，
 * 画布入口**不绕过** G1。保存后由调用方重拉字段，新字段带稳定列 id，可直接拖入槽位，
 * 并随图表配置（v2 的 fieldId 引用）持久化、被分享直链还原。
 *
 * 保存前拉一次最新列：avoid 用画布启动时的字段快照覆盖别人刚加的列
 * （updateColumns 是整列数组覆盖）。
 */
const ComputedFieldModal: React.FC<ComputedFieldModalProps> = ({
  open,
  datasetId,
  onClose,
  onSaved,
}) => {
  const intl = useIntl();
  const [form] = Form.useForm<ComputedFieldFormValues>();
  const [columns, setColumns] = useState<DatasetColumn[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || !datasetId) {
      return;
    }
    let cancelled = false;
    form.resetFields();
    setColumns(null);
    setLoading(true);
    datasetsApi
      .getColumns(datasetId)
      .then((response) => {
        if (!cancelled) {
          setColumns(response.data.data);
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          message.error(error instanceof Error ? error.message : '读取字段失败');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [open, datasetId, form]);

  // 只允许引用物理列（虚拟字段的 expr 相对来源表求值，引用它 SQL 里没有那一列）。
  const fieldNames = (columns ?? [])
    .filter((column) => !(column.expr && column.expr !== column.name))
    .map((column) => column.name);

  const handleSave = async () => {
    if (!datasetId) {
      return;
    }
    if (!columns) {
      message.error(intl.formatMessage({ id: 'computedField.fieldsNotReady' }));
      return;
    }
    try {
      const values = await form.validateFields();
      const name = values.name.trim();
      if (columns.some((column) => column.name === name)) {
        message.error(intl.formatMessage({ id: 'field.duplicateName' }));
        return;
      }
      const newColumn: DatasetColumn = {
        // 空 id = 请后端分配（列的稳定标识由后端 idgen 生成）
        id: '',
        name,
        type: normalizeDataType(values.dataType),
        role: values.role || 'metric',
        expr: values.expression,
        comment: '',
      };
      setSaving(true);
      await datasetsApi.updateColumns(datasetId, [...columns, newColumn]);
      message.success(intl.formatMessage({ id: 'common.success' }));
      await onSaved();
      onClose();
    } catch (error: unknown) {
      if (error && typeof error === 'object' && 'errorFields' in error) {
        return;
      }
      message.error(
        error instanceof Error ? error.message : intl.formatMessage({ id: 'common.error' })
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title={intl.formatMessage({ id: 'computedField.title' })}
      open={open}
      onCancel={onClose}
      footer={null}
      width={560}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" disabled={loading}>
        <Form.Item
          name="name"
          label={intl.formatMessage({ id: 'field.fieldName' })}
          rules={[
            { required: true, message: intl.formatMessage({ id: 'field.pleaseEnterFieldName' }) },
          ]}
        >
          <Input
            placeholder={intl.formatMessage({ id: 'computedField.namePlaceholder' })}
            data-testid="computed-field-name"
          />
        </Form.Item>
        <Form.Item
          name="role"
          label={intl.formatMessage({ id: 'field.role' })}
          initialValue="metric"
        >
          <Select
            options={[
              { value: 'metric', label: intl.formatMessage({ id: 'field.metric' }) },
              { value: 'dimension', label: intl.formatMessage({ id: 'field.dimension' }) },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="dataType"
          label={intl.formatMessage({ id: 'field.dataType' })}
          initialValue="float"
        >
          <Select<DataType>
            options={[
              { value: 'float', label: intl.formatMessage({ id: 'dataType.float' }) },
              { value: 'integer', label: intl.formatMessage({ id: 'dataType.integer' }) },
              { value: 'string', label: intl.formatMessage({ id: 'dataType.string' }) },
              { value: 'date', label: intl.formatMessage({ id: 'dataType.date' }) },
              { value: 'datetime', label: intl.formatMessage({ id: 'dataType.datetime' }) },
              { value: 'boolean', label: intl.formatMessage({ id: 'dataType.boolean' }) },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="expression"
          label={intl.formatMessage({ id: 'field.expression' })}
          extra={intl.formatMessage({ id: 'computedField.expressionHint' })}
          rules={[
            {
              required: true,
              message: intl.formatMessage({ id: 'field.pleaseEnterExpression' }),
            },
            {
              validator: (_, value: string) => {
                if (!value?.trim()) {
                  return Promise.resolve();
                }
                const result = validateExpr(value, fieldNames);
                return result.ok
                  ? Promise.resolve()
                  : Promise.reject(new Error(intl.formatMessage({ id: 'expr.invalid' })));
              },
            },
          ]}
        >
          <ExpressionEditor fields={fieldNames} rows={3} placeholder="e.g., price * quantity" />
        </Form.Item>
        <Form.Item>
          <ModalFooter>
            <Button onClick={onClose}>{intl.formatMessage({ id: 'common.cancel' })}</Button>
            <Button
              type="primary"
              loading={saving}
              disabled={!columns || loading}
              onClick={handleSave}
              data-testid="computed-field-save"
            >
              {intl.formatMessage({ id: 'common.save' })}
            </Button>
          </ModalFooter>
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default ComputedFieldModal;
