import { CaretDownOutlined } from '@ant-design/icons';
import { Input, InputNumber, Select, Tag } from 'antd';
import React, { useEffect, useState } from 'react';
import { datasetsApi } from '@/api';
import { classifyFieldKind } from '@/lib/dataTypes';
import type { ChartField, FilterCondition, FilterOperator } from '@/store';

/** 数值行内算子（与 FilterConfigModal 的 NUMBER_OPERATORS 同词表）。 */
const NUMBER_OPERATORS: FilterOperator[] = [
  'gt',
  'gte',
  'lt',
  'lte',
  'eq',
  'neq',
  'between',
  'isNull',
  'isNotNull',
];
/** 字符串行内算子（与 FilterConfigModal 的 STRING_OPERATORS + 枚举两算子同词表）。 */
const STRING_OPERATORS: FilterOperator[] = [
  'in',
  'notIn',
  'eq',
  'neq',
  'like',
  'isNull',
  'isNotNull',
];

const OPERATOR_LABELS: Partial<Record<FilterOperator, string>> = {
  eq: '等于',
  neq: '不等于',
  gt: '大于',
  gte: '大于等于',
  lt: '小于',
  lte: '小于等于',
  like: '包含',
  between: '区间',
  in: '属于',
  notIn: '不属于',
  isNull: '为空',
  isNotNull: '不为空',
};

const isNoValueOperator = (op: FilterOperator): boolean => op === 'isNull' || op === 'isNotNull';

export interface FilterValueControlProps {
  field: ChartField;
  filter: FilterCondition;
  /** 候选值实查用的数据集 id；null 时不取候选值（多选框仍可手输标签）。 */
  datasetId: number | null;
  /** 变更回写：operator/value/valueEnd 的局部 patch，由调用方 updateFilter 合并。 */
  onChange: (patch: Partial<Pick<FilterCondition, 'operator' | 'value' | 'valueEnd'>>) => void;
}

/**
 * 「作为筛选器」的字符串/数值条件在图表预览区上方出的行内控件
 * （与日期族的 DateFilterControl 同位同责：浮层里改条件即时生效、不设确认）。
 * 控件形态按字段族分流，语义与 FilterConfigModal 对齐：
 *   - 数值：算子下拉 + InputNumber（区间两格、无值算子无值区）；
 *   - 字符串：in/notIn 为候选值多选（实查该列，同弹窗枚举模式），eq/neq/like 为文本输入。
 * 候选值实查失败不阻塞：多选框退化为可自由输入的 tags 模式。
 */
const FilterValueControl: React.FC<FilterValueControlProps> = ({
  field,
  filter,
  datasetId,
  onChange,
}) => {
  const kind = classifyFieldKind(field.dataType);
  const operators = kind === 'number' ? NUMBER_OPERATORS : STRING_OPERATORS;
  const isEnumMode = kind !== 'number' && (filter.operator === 'in' || filter.operator === 'notIn');

  const [candidates, setCandidates] = useState<string[]>([]);
  const [candidatesLoading, setCandidatesLoading] = useState(false);

  // 枚举模式才实查候选值（与 FilterConfigModal 的枚举模式同一取数）。
  useEffect(() => {
    if (!isEnumMode || datasetId == null) {
      setCandidates([]);
      return;
    }
    let cancelled = false;
    setCandidatesLoading(true);
    datasetsApi
      .queryDistinct(datasetId, field.name)
      .then((response) => {
        if (cancelled) return;
        const rows = response.data.data ?? [];
        const values = Array.from(
          new Set(
            rows
              .map((row) => (row?.[field.name] == null ? '' : String(row[field.name])))
              .filter((v) => v !== '')
          )
        );
        const numeric = values.every((v) => v !== '' && Number.isFinite(Number(v)));
        values.sort(numeric ? (a, b) => Number(b) - Number(a) : (a, b) => a.localeCompare(b, 'zh'));
        setCandidates(values);
      })
      .catch(() => {
        if (!cancelled) setCandidates([]);
      })
      .finally(() => {
        if (!cancelled) setCandidatesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [isEnumMode, datasetId, field.name]);

  const renderValueEditor = () => {
    if (isNoValueOperator(filter.operator)) return null;
    if (kind === 'number') {
      if (filter.operator === 'between') {
        return (
          <>
            <InputNumber
              size="small"
              style={{ width: 90 }}
              value={typeof filter.value === 'number' ? filter.value : undefined}
              onChange={(v) => onChange({ value: v })}
              data-testid={`chart-filter-min-${filter.id}`}
            />
            <InputNumber
              size="small"
              style={{ width: 90 }}
              value={typeof filter.valueEnd === 'number' ? filter.valueEnd : undefined}
              onChange={(v) => onChange({ valueEnd: v })}
              data-testid={`chart-filter-max-${filter.id}`}
            />
          </>
        );
      }
      return (
        <InputNumber
          size="small"
          style={{ width: 110 }}
          value={typeof filter.value === 'number' ? filter.value : undefined}
          onChange={(v) => onChange({ value: v })}
          data-testid={`chart-filter-value-${filter.id}`}
        />
      );
    }
    if (isEnumMode) {
      const selected = Array.isArray(filter.value) ? (filter.value as unknown[]).map(String) : [];
      return (
        <Select
          size="small"
          mode="multiple"
          style={{ minWidth: 140, maxWidth: 260 }}
          maxTagCount="responsive"
          placeholder="选择值"
          value={selected}
          onChange={(next: string[]) => onChange({ value: next })}
          loading={candidatesLoading}
          options={candidates.map((v) => ({ value: v, label: v }))}
          // 候选值失败/不全时可自由输入，语义同弹窗的「手动补充」文本框。
          tokenSeparators={[',']}
          data-testid={`chart-filter-enum-${filter.id}`}
        />
      );
    }
    return (
      <Input
        size="small"
        style={{ width: 130 }}
        value={filter.value == null ? '' : String(filter.value)}
        onChange={(e) => onChange({ value: e.target.value })}
        data-testid={`chart-filter-value-${filter.id}`}
      />
    );
  };

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 6,
        padding: '2px 8px',
        border: '1px solid var(--dr-border)',
        borderRadius: 6,
        background: 'var(--dr-sunken)',
      }}
      data-testid={`chart-filter-control-${filter.id}`}
    >
      <Tag
        color={kind === 'number' ? 'purple' : 'blue'}
        style={{
          margin: 0,
          maxWidth: 120,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {filter.filterLabel || field.name}
      </Tag>
      <Select
        size="small"
        variant="borderless"
        suffixIcon={<CaretDownOutlined />}
        style={{ width: 92 }}
        value={filter.operator}
        onChange={(op: FilterOperator) => {
          // 切算子时归位不匹配的值：无值算子清空；between 清 valueEnd 交给两格输入。
          if (isNoValueOperator(op)) onChange({ operator: op, value: null, valueEnd: undefined });
          else if (op === 'between') onChange({ operator: op });
          else onChange({ operator: op });
        }}
        options={operators.map((op) => ({ value: op, label: OPERATOR_LABELS[op] ?? op }))}
        data-testid={`chart-filter-operator-${filter.id}`}
      />
      {renderValueEditor()}
    </div>
  );
};

export default FilterValueControl;
