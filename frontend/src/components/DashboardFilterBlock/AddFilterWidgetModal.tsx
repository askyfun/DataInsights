import { Alert, Input, Modal, Select, Space, Switch, Typography } from 'antd';
import React, { useEffect, useState } from 'react';
import { type DatasetColumn, datasetsApi } from '@/api';
import { type FilterWidgetFamily, filterWidgetFamily } from '@/lib/dashboardFilterValue';
import { normalizeDataType } from '@/lib/dataTypes';
import type { FilterOperator } from '@/store';

const { Text } = Typography;

/** 新建盘级筛选器所需的配置（`DashboardFilterWidget` 的子集）。 */
export interface NewFilterWidgetConfig {
  /** 绑定：数据集 + 列 ID（`DatasetColumn.id`）。 */
  binding: { datasetId: number; column: string };
  /** 筛选器显示名。 */
  label: string;
  /** 绑定列的规范数据类型（决定控件族与是否带时分秒）。 */
  dataType: string;
  /** 初始算子（按族给默认值，用户在块上可再改）。 */
  operator: FilterOperator;
  /** 是否多选（字符串族的「多选」开关；其余族恒 false）。 */
  multi: boolean;
}

/**
 * 新建筛选器的默认算子与多选形态：按族给最常用的那种，避免新建出来就是一个没人用的组合。
 * - 字符串：多选枚举（`in`）——盘级筛选最典型的用法就是「看这几个地区」；
 * - 数值：区间（`between`）；
 * - 日期：区间（`between`，由日期控件自己决定具体端点）。
 */
export function defaultOperatorFor(family: FilterWidgetFamily): {
  operator: FilterOperator;
  multi: boolean;
} {
  switch (family) {
    case 'string':
      return { operator: 'in', multi: true };
    default:
      return { operator: 'between', multi: false };
  }
}

export interface AddFilterWidgetModalProps {
  open: boolean;
  /** 可选数据集；为空时给出「先建数据集」的提示而不是空下拉。 */
  datasets: readonly { id: number; name: string }[];
  onOk: (config: NewFilterWidgetConfig) => void;
  onCancel: () => void;
}

/**
 * 新建盘级筛选器弹窗。
 *
 * 只收集「绑哪个字段、显示成什么名字、初始算子形态」——**日期族的具体条件**（粒度/快捷选项/
 * 自定义起止）不在这一步定，建出来后由块上的「配置」进完整日期筛选弹窗再调，
 * 避免把同一组设置拆到两处入口。
 */
const AddFilterWidgetModal: React.FC<AddFilterWidgetModalProps> = ({
  open,
  datasets,
  onOk,
  onCancel,
}) => {
  const [datasetId, setDatasetId] = useState<number | null>(null);
  const [columnId, setColumnId] = useState<string | null>(null);
  const [label, setLabel] = useState('');
  const [multi, setMulti] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [columns, setColumns] = useState<DatasetColumn[]>([]);

  // 打开时重置：上次残留的数据集/字段会让「确定」看起来可用，实际绑的是别的列。
  useEffect(() => {
    if (!open) return;
    setDatasetId(null);
    setColumnId(null);
    setLabel('');
    setMulti(true);
    setColumns([]);
    setError(null);
  }, [open]);

  // 换数据集即换候选字段；用 cancelled 挡掉乱序返回的旧响应。
  useEffect(() => {
    if (!open || datasetId === null) {
      setColumns([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    setColumnId(null);
    datasetsApi
      .getColumns(datasetId)
      .then((response) => {
        if (cancelled) return;
        setColumns(response.data.data ?? []);
      })
      .catch(() => {
        if (cancelled) return;
        setColumns([]);
        setError('数据集字段加载失败，请重试');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, datasetId]);

  const selected = columns.find((column) => column.id === columnId) ?? null;
  const family: FilterWidgetFamily | null = selected
    ? filterWidgetFamily({ dataType: selected.type })
    : null;
  const canOk = datasetId !== null && selected !== null;
  const displayLabel = label.trim() !== '' ? label.trim() : (selected?.name ?? '');

  return (
    <Modal
      open={open}
      title="添加筛选器"
      width={480}
      okText="确定"
      cancelText="取消"
      okButtonProps={{ disabled: !canOk }}
      onOk={() => {
        if (datasetId === null || !selected || family === null) return;
        const preset = defaultOperatorFor(family);
        onOk({
          binding: { datasetId, column: selected.id },
          label: displayLabel,
          dataType: normalizeDataType(selected.type),
          // 字符串族的算子由「多选」开关决定，其余族用该族默认算子。
          operator: family === 'string' ? (multi ? 'in' : 'eq') : preset.operator,
          multi: family === 'string' ? multi : false,
        });
      }}
      onCancel={onCancel}
      destroyOnHidden
    >
      <Space orientation="vertical" size={12} style={{ width: '100%' }}>
        <div>
          <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>
            数据集
          </Text>
          <Select
            style={{ width: '100%' }}
            value={datasetId}
            placeholder="选择数据集"
            data-testid="add-filter-dataset"
            showSearch
            optionFilterProp="label"
            options={datasets.map((dataset) => ({ value: dataset.id, label: dataset.name }))}
            notFoundContent="暂无数据集"
            onChange={setDatasetId}
          />
        </div>

        <div>
          <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>
            字段
          </Text>
          <Select
            style={{ width: '100%' }}
            value={columnId}
            placeholder={datasetId === null ? '请先选择数据集' : '选择字段'}
            data-testid="add-filter-column"
            loading={loading}
            disabled={datasetId === null}
            options={columns.map((column) => ({
              value: column.id,
              label: column.comment ? `${column.name}（${column.comment}）` : column.name,
            }))}
            notFoundContent={datasetId === null ? null : loading ? null : '该数据集没有字段'}
            onChange={(value: string) => {
              setColumnId(value);
              const column = columns.find((item) => item.id === value);
              // 显示名默认跟随列名，用户改过就不再覆盖。
              if (column && label.trim() === '') setLabel(column.name);
            }}
          />
        </div>

        {family === 'string' && (
          <Space>
            <Switch
              checked={multi}
              data-testid="add-filter-multi"
              onChange={setMulti}
              aria-label="允许多选"
            />
            <Text type="secondary">
              允许多选（关闭时只能选一个值；两者的过滤语义不同：属于其中之一 / 等于）
            </Text>
          </Space>
        )}

        <div>
          <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>
            显示名称
          </Text>
          <Input
            style={{ width: '100%' }}
            placeholder={selected?.name ?? '筛选器名称'}
            value={label}
            data-testid="add-filter-label"
            onChange={(event) => setLabel(event.target.value)}
          />
        </div>

        {error && <Alert type="error" showIcon title={error} />}
      </Space>
    </Modal>
  );
};

export default AddFilterWidgetModal;
