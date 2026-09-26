import { LoadingOutlined } from '@ant-design/icons';
import {
  Checkbox,
  DatePicker,
  Divider,
  Empty,
  Input,
  InputNumber,
  Modal,
  message,
  Segmented,
  Select,
  Space,
  Typography,
} from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import React, { useEffect, useMemo, useState } from 'react';
import { datasetsApi } from '@/api';
import { classifyFieldKind, normalizeDataType } from '@/lib/dataTypes';
import type { ChartField, FilterCondition, FilterOperator } from '@/store';

const { Text } = Typography;
const { TextArea } = Input;

/** 字段数据类型三分类：决定弹窗内的输入控件族。实现在 lib/dataTypes，此处转发。 */
export type FilterValueKind = 'date' | 'number' | 'string';

export { classifyFieldKind };

/** 日期字段是否带时间部分（datetime 用 datetimepicker）。 */
const hasTimePart = (dataType: string): boolean => normalizeDataType(dataType) === 'datetime';

const DATE_FORMAT = 'YYYY-MM-DD';
const DATETIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

/**
 * 无值算子：不需要用户填值，直接生成 `IS NULL` / `IS NOT NULL`。
 * 三族白名单都包含它们，值区不渲染输入控件。
 */
const NO_VALUE_OPERATORS: FilterOperator[] = ['isNull', 'isNotNull'];
const isNoValueOperator = (op: FilterOperator): boolean => NO_VALUE_OPERATORS.includes(op);

/** 数值算子（枚举/区间由模式切换承担，这里的下拉是单值比较 + 无值算子）。 */
const NUMBER_OPERATORS: FilterOperator[] = [
  'gt',
  'gte',
  'lt',
  'lte',
  'eq',
  'neq',
  'isNull',
  'isNotNull',
];
/** 字符串算子。 */
const STRING_OPERATORS: FilterOperator[] = ['eq', 'neq', 'like', 'isNull', 'isNotNull'];
/** 日期算子。 */
const DATE_OPERATORS: FilterOperator[] = [
  'eq',
  'neq',
  'gt',
  'gte',
  'lt',
  'lte',
  'between',
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
  in: '枚举',
  isNull: '为空',
  isNotNull: '不为空',
};

type NumberMode = 'compare' | 'between' | 'enum';
type StringMode = 'enum' | 'compare';

export interface FilterConfigPatch {
  operator: FilterOperator;
  value: unknown;
  valueEnd?: unknown;
}

export interface FilterConfigModalProps {
  open: boolean;
  /** 正在配置过滤的字段；null 时弹窗不渲染内容。 */
  field: ChartField | null;
  /** 已有条件（点击过滤栏芯片进入编辑）；新建时为 undefined。 */
  initial?: FilterCondition;
  /** 新建（拖入/下拉添加）时为 true：字符串字段默认进枚举模式，而不是回显 store 的 eq 缺省。 */
  isNew?: boolean;
  /** 候选枚举值取数用的数据集 id。 */
  datasetId: number | null;
  onOk: (patch: FilterConfigPatch) => void;
  onCancel: () => void;
}

/**
 * 过滤条件配置弹窗（对齐火山引擎智能洞察的交互）：
 * 字段拖入/点击过滤栏后弹窗内完成输入，确定才写回 store——
 * 输入过程中不触发自动查询（拖拽中途态不进请求）。
 * 控件按字段数据类型分流：
 *   - 日期：比较符 + DatePicker（timestamp 带 time）/ 区间 RangePicker；
 *   - 数字：比较 / 区间 / 枚举三种模式，枚举候选值来自数据集实查、从大到小；
 *   - 字符串：枚举 / 比较（等于·不等于·包含），枚举候选值按字典序。
 * 凡枚举模式都附带「一行一个值」文本框，与下拉勾选合并生效。
 * 三族算子白名单都含无值算子 `isNull` / `isNotNull`：选中后值区不渲染输入控件，
 * 回显既有条件时原算子原样保留（不得回落默认算子，否则等于静默改写用户的筛选语义）。
 */
const FilterConfigModal: React.FC<FilterConfigModalProps> = ({
  open,
  field,
  initial,
  isNew,
  datasetId,
  onOk,
  onCancel,
}) => {
  const kind = classifyFieldKind(field?.dataType ?? '');
  const withTime = field ? hasTimePart(field.dataType) : false;

  const [operator, setOperator] = useState<FilterOperator>('eq');
  const [numberMode, setNumberMode] = useState<NumberMode>('compare');
  const [stringMode, setStringMode] = useState<StringMode>('enum');
  const [singleText, setSingleText] = useState<string>('');
  const [singleNumber, setSingleNumber] = useState<number | null>(null);
  const [rangeStart, setRangeStart] = useState<number | null>(null);
  const [rangeEnd, setRangeEnd] = useState<number | null>(null);
  const [dateValue, setDateValue] = useState<Dayjs | null>(null);
  const [dateRange, setDateRange] = useState<[Dayjs | null, Dayjs | null] | null>(null);
  const [enumValues, setEnumValues] = useState<string[]>([]);
  const [extraLines, setExtraLines] = useState<string>('');
  // 反选：勾选后枚举条件取反（in → notIn），生成 NOT IN。
  const [notInChecked, setNotInChecked] = useState(false);
  const [candidates, setCandidates] = useState<string[]>([]);
  const [candidatesLoading, setCandidatesLoading] = useState(false);

  // 打开时按已有条件 / 字段类型重置内部状态。
  useEffect(() => {
    if (!open || !field) return;
    const k = classifyFieldKind(field.dataType);
    const init = initial;
    const initOperator = init?.operator;
    setNotInChecked(initOperator === 'notIn');
    if (k === 'date') {
      setOperator(initOperator && DATE_OPERATORS.includes(initOperator) ? initOperator : 'eq');
      if (init?.operator === 'between') {
        setDateRange([
          init.value ? dayjs(String(init.value)) : null,
          init.valueEnd ? dayjs(String(init.valueEnd)) : null,
        ]);
        setDateValue(null);
      } else {
        setDateValue(init?.value ? dayjs(String(init.value)) : null);
        setDateRange(null);
      }
    } else if (k === 'number') {
      const mode: NumberMode =
        init?.operator === 'in' || init?.operator === 'notIn'
          ? 'enum'
          : init?.operator === 'between'
            ? 'between'
            : 'compare';
      setNumberMode(mode);
      if (mode === 'enum') {
        setEnumValues(Array.isArray(init?.value) ? (init.value as string[]).map(String) : []);
      } else if (mode === 'between') {
        setRangeStart(init?.value != null && init.value !== '' ? Number(init.value) : null);
        setRangeEnd(init?.valueEnd != null && init.valueEnd !== '' ? Number(init.valueEnd) : null);
      } else {
        setOperator(initOperator && NUMBER_OPERATORS.includes(initOperator) ? initOperator : 'gte');
        setSingleNumber(init?.value != null && init.value !== '' ? Number(init.value) : null);
      }
    } else {
      // 新建条件默认枚举模式；仅当已有条件本身是算子（比较符或无值算子）时才回显为条件模式。
      const mode: StringMode =
        !isNew &&
        init &&
        init.operator !== 'in' &&
        init.operator !== 'notIn' &&
        STRING_OPERATORS.includes(init.operator)
          ? 'compare'
          : 'enum';
      setStringMode(mode);
      if (mode === 'enum') {
        setEnumValues(Array.isArray(init?.value) ? (init.value as string[]).map(String) : []);
      } else {
        setOperator(initOperator && STRING_OPERATORS.includes(initOperator) ? initOperator : 'eq');
        setSingleText(init?.value != null ? String(init.value) : '');
      }
    }
    setExtraLines('');
  }, [open, field, initial, isNew]);

  const needCandidates =
    open && field != null && (kind === 'number' || kind === 'string')
      ? (kind === 'number' ? numberMode : stringMode) === 'enum'
      : false;

  // 候选值：按数据集实查该列（单维度 + limit），客户端去重。失败不阻塞手输。
  useEffect(() => {
    if (!needCandidates || !field || datasetId == null) {
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
        if (!cancelled) {
          setCandidates([]);
          message.warning('候选值加载失败，可直接在文本框手动输入');
        }
      })
      .finally(() => {
        if (!cancelled) setCandidatesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [needCandidates, field, datasetId]);

  const extraTextValues = useMemo(
    () =>
      Array.from(
        new Set(
          extraLines
            .split('\n')
            .map((line) => line.trim())
            .filter((line) => line !== '')
        )
      ),
    [extraLines]
  );

  const mergedEnumValues = useMemo(() => {
    const merged = [...enumValues];
    for (const v of extraTextValues) {
      if (!merged.includes(v)) merged.push(v);
    }
    return merged;
  }, [enumValues, extraTextValues]);

  const dateFmt = withTime ? DATETIME_FORMAT : DATE_FORMAT;

  const buildPatch = (): FilterConfigPatch | null => {
    if (kind === 'date') {
      if (operator === 'between') {
        if (!dateRange?.[0] || !dateRange[1]) return null;
        return {
          operator,
          value: dateRange[0].format(dateFmt),
          valueEnd: dateRange[1].format(dateFmt),
        };
      }
      if (isNoValueOperator(operator)) return { operator, value: null };
      if (!dateValue) return null;
      return { operator, value: dateValue.format(dateFmt) };
    }
    if (kind === 'number') {
      if (numberMode === 'enum') {
        if (mergedEnumValues.length === 0) return null;
        return { operator: notInChecked ? 'notIn' : 'in', value: mergedEnumValues };
      }
      if (numberMode === 'between') {
        if (rangeStart == null || rangeEnd == null) return null;
        return { operator: 'between', value: rangeStart, valueEnd: rangeEnd };
      }
      if (isNoValueOperator(operator)) return { operator, value: null };
      if (singleNumber == null) return null;
      return { operator, value: singleNumber };
    }
    if (stringMode === 'enum') {
      if (mergedEnumValues.length === 0) return null;
      return { operator: notInChecked ? 'notIn' : 'in', value: mergedEnumValues };
    }
    if (isNoValueOperator(operator)) return { operator, value: null };
    if (singleText.trim() === '') return null;
    return { operator, value: singleText.trim() };
  };

  const patch = buildPatch();
  const canOk = patch !== null;

  const renderBody = () => {
    if (!field) return null;
    if (kind === 'date') {
      return (
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <Select
            style={{ width: 160 }}
            value={operator}
            onChange={setOperator}
            options={DATE_OPERATORS.map((op) => ({ value: op, label: OPERATOR_LABELS[op] }))}
            data-testid="filter-modal-operator"
          />
          {isNoValueOperator(operator) ? null : operator === 'between' ? (
            <DatePicker.RangePicker
              style={{ width: '100%' }}
              showTime={withTime}
              value={dateRange}
              onChange={(range) => setDateRange(range)}
              data-testid="filter-modal-range"
            />
          ) : (
            <DatePicker
              style={{ width: '100%' }}
              showTime={withTime}
              value={dateValue}
              onChange={setDateValue}
              data-testid="filter-modal-date"
            />
          )}
        </Space>
      );
    }
    if (kind === 'number') {
      return (
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <Segmented
            value={numberMode}
            onChange={(v) => setNumberMode(v as NumberMode)}
            options={[
              { value: 'compare', label: '比较' },
              { value: 'between', label: '区间' },
              { value: 'enum', label: '枚举' },
            ]}
            data-testid="filter-modal-mode"
          />
          {numberMode === 'compare' && (
            <Space>
              <Select
                style={{ width: 120 }}
                value={operator}
                onChange={setOperator}
                options={NUMBER_OPERATORS.map((op) => ({
                  value: op,
                  label: OPERATOR_LABELS[op],
                }))}
                data-testid="filter-modal-operator"
              />
              {!isNoValueOperator(operator) && (
                <InputNumber
                  style={{ width: 160 }}
                  value={singleNumber}
                  onChange={(v) => setSingleNumber(v ?? null)}
                  data-testid="filter-modal-number"
                />
              )}
            </Space>
          )}
          {numberMode === 'between' && (
            <Space>
              <InputNumber
                placeholder="最小值"
                value={rangeStart}
                onChange={(v) => setRangeStart(v ?? null)}
                data-testid="filter-modal-min"
              />
              <Text type="secondary">~</Text>
              <InputNumber
                placeholder="最大值"
                value={rangeEnd}
                onChange={(v) => setRangeEnd(v ?? null)}
                data-testid="filter-modal-max"
              />
            </Space>
          )}
          {numberMode === 'enum' && renderEnumInputs()}
        </Space>
      );
    }
    return (
      <Space direction="vertical" style={{ width: '100%' }} size={12}>
        <Segmented
          value={stringMode}
          onChange={(v) => setStringMode(v as StringMode)}
          options={[
            { value: 'enum', label: '枚举' },
            { value: 'compare', label: '条件' },
          ]}
          data-testid="filter-modal-mode"
        />
        {stringMode === 'enum' ? (
          renderEnumInputs()
        ) : (
          <Space>
            <Select
              style={{ width: 120 }}
              value={operator}
              onChange={setOperator}
              options={STRING_OPERATORS.map((op) => ({
                value: op,
                label: OPERATOR_LABELS[op],
              }))}
              data-testid="filter-modal-operator"
            />
            {!isNoValueOperator(operator) && (
              <Input
                style={{ width: 200 }}
                value={singleText}
                onChange={(e) => setSingleText(e.target.value)}
                data-testid="filter-modal-text"
              />
            )}
          </Space>
        )}
      </Space>
    );
  };

  /** 枚举模式公共块：候选下拉（可搜索多选）+ 一行一个值文本框。 */
  const renderEnumInputs = () => (
    <>
      <div>
        <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
          候选值（{kind === 'number' ? '按数值从大到小' : '按字典序'}）
          {candidatesLoading && <LoadingOutlined style={{ marginLeft: 6 }} spin />}
        </Text>
        {candidates.length === 0 && !candidatesLoading ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无候选值，可直接手动输入" />
        ) : (
          <Select
            mode="multiple"
            style={{ width: '100%' }}
            maxTagCount="responsive"
            placeholder="选择候选值"
            value={enumValues}
            onChange={setEnumValues}
            loading={candidatesLoading}
            options={candidates.map((v) => ({ value: v, label: v }))}
            data-testid="filter-modal-enum"
          />
        )}
      </div>
      <div>
        <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
          手动补充（一行一个值）
        </Text>
        <TextArea
          rows={3}
          placeholder={'北京\n上海\n广州'}
          value={extraLines}
          onChange={(e) => setExtraLines(e.target.value)}
          data-testid="filter-modal-extra"
        />
      </div>
      <Checkbox
        checked={notInChecked}
        onChange={(e) => setNotInChecked(e.target.checked)}
        data-testid="filter-modal-notin"
      >
        反选（排除所选值）
      </Checkbox>
      {mergedEnumValues.length > 0 && (
        <>
          <Divider style={{ margin: 0 }} />
          <Text data-testid="filter-modal-summary">
            {notInChecked ? '将排除' : '将保留'} {mergedEnumValues.length} 个值：
            {mergedEnumValues.slice(0, 8).join('、')}
            {mergedEnumValues.length > 8 ? ' …' : ''}
          </Text>
        </>
      )}
    </>
  );

  return (
    <Modal
      open={open}
      title={field ? `筛选 · ${field.name}` : '筛选'}
      okText="确定"
      cancelText="取消"
      okButtonProps={{ disabled: !canOk }}
      onOk={() => {
        if (patch) onOk(patch);
      }}
      onCancel={onCancel}
      width={480}
      destroyOnHidden
      // 配置弹窗追求"即点即开"：关闭默认的 zoom+fade 过渡（约 300ms）。
      transitionName=""
      maskTransitionName=""
    >
      {renderBody()}
    </Modal>
  );
};

export default FilterConfigModal;
