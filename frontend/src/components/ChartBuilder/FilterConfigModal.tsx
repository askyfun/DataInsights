import {
  Button,
  Checkbox,
  DatePicker,
  Divider,
  Input,
  InputNumber,
  Modal,
  message,
  Segmented,
  Select,
  Space,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  Upload,
} from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import React, { useEffect, useMemo, useState } from 'react';
import { datasetsApi } from '@/api';
import { classifyFieldKind, normalizeDataType } from '@/lib/dataTypes';
import type { ChartField, FilterCondition, FilterOperator } from '@/store';
import EnumPanel from './EnumPanel';

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
 * 无值算子：不需要用户填值，直接生成 `IS NULL` / `IS NOT NULL` / 空串比较。
 * 值区不渲染输入控件。
 */
const NO_VALUE_OPERATORS: FilterOperator[] = [
  'isNull',
  'isNotNull',
  'isEmptyString',
  'isNotEmptyString',
];
const isNoValueOperator = (op: FilterOperator): boolean => NO_VALUE_OPERATORS.includes(op);

/** 数值算子（条件筛选 Tab：单值比较 + 无值算子；枚举/区间由模式与子模式承担）。 */
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
/** 字符串条件算子（对齐火山「条件筛选」：等值/包含/前后缀/空串判定/无值）。 */
const STRING_OPERATORS: FilterOperator[] = [
  'eq',
  'neq',
  'like',
  'startsWith',
  'endsWith',
  'isEmptyString',
  'isNotEmptyString',
  'isNull',
  'isNotNull',
];
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
  startsWith: '开头为',
  endsWith: '结尾为',
  isEmptyString: '为空字符串',
  isNotEmptyString: '不为空字符串',
  between: '区间',
  in: '枚举',
  isNull: '为空',
  isNotNull: '不为空',
};

/** 模式体系（对齐火山「编辑筛选」四 Tab）；子查询为远期占位，置灰不可选。 */
type FilterMode = 'precise' | 'condition' | 'manual' | 'subquery';

/**
 * 按字段类型与既有算子推导初始模式（in/notIn 归精确筛选；数值标量归条件筛选）。
 * 新建条件（isNew）恒从精确筛选起步——store 的 eq 缺省不是用户意图。
 */
const initialModeOf = (
  k: FilterValueKind,
  init: FilterCondition | undefined,
  isNew?: boolean
): FilterMode => {
  if (k !== 'number') return 'precise';
  if (isNew || !init) return 'condition';
  return init.operator === 'in' || init.operator === 'notIn' ? 'precise' : 'condition';
};

type NumberConditionSub = 'compare' | 'between' | 'enum';

type ManualDelimiter = 'newline' | 'comma' | 'space' | 'any';

/** 手动输入的分词分隔符 → 正则。 */
const DELIMITER_PATTERN: Record<ManualDelimiter, RegExp> = {
  newline: /\n/,
  comma: /[,，]/,
  space: /[ \t]+/,
  any: /[\s,，]+/,
};

/** 手动输入单值的硬边界：超长或含控制字符视为非法（胶囊标红、不进条件）。 */
const MANUAL_MAX_LEN = 256;
const manualTokenInvalid = (token: string): boolean => {
  if (token.length > MANUAL_MAX_LEN) return true;
  for (let i = 0; i < token.length; i++) {
    const c = token.charCodeAt(i);
    // 控制字符（制表/换行已作分隔符，不算）：粘贴二进制或损坏 CSV 的兜底。
    if (c < 9 || c === 11 || c === 12 || (c >= 14 && c <= 31) || c === 127) return true;
  }
  return false;
};

export interface FilterConfigPatch {
  operator: FilterOperator;
  value: unknown;
  valueEnd?: unknown;
  /** 「作为筛选器」：图表预览区上方渲染行内筛选控件（全族通用，见 FilterCondition.asFilter）。 */
  asFilter: boolean;
  /** 行内控件显示名称；空串时渲染回退字段名。 */
  filterLabel: string;
}

export interface FilterConfigModalProps {
  open: boolean;
  /** 正在配置过滤的字段；null 时弹窗不渲染内容。 */
  field: ChartField | null;
  /** 已有条件（点击过滤栏芯片进入编辑）；新建时为 undefined。 */
  initial?: FilterCondition;
  /** 新建（拖入/下拉添加）时为 true；保留给调用方语义，弹窗内模式回显不依赖它。 */
  isNew?: boolean;
  /** 候选枚举值取数用的数据集 id。 */
  datasetId: number | null;
  onOk: (patch: FilterConfigPatch) => void;
  onCancel: () => void;
}

/**
 * 筛选配置弹窗（对齐火山引擎智能洞察「编辑筛选」的模式体系）：
 * 字符串/数值字段呈现四个模式 Tab——精确筛选（枚举面板：搜索/查看已选项/全选/
 * 排序/抽样展开）、条件筛选（算子 + 取值；数值另有 比较/区间/枚举 子模式）、
 * 手动输入（粘贴/CSV + 分隔符 + 非法值标红）、子查询（远期，置灰）。
 * 各 Tab 状态独立保留，切换不丢已配内容；「确定」以上次停留的 Tab 为准写回 store。
 * in/notIn 条件回显进精确筛选；手动输入产出同一 in/notIn 算子，仅录入形态不同。
 * 输入过程中不触发自动查询（拖拽中途态不进请求）。
 * 日期字段不进本弹窗（走 DateFilterModal），但保留直调时的日期分支作兜底。
 * 三族算子白名单都含无值算子：选中后值区不渲染输入控件，
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

  const [activeMode, setActiveMode] = useState<FilterMode>(() =>
    initialModeOf(kind, initial, isNew)
  );
  // —— 精确筛选 / 条件-枚举 共用选中值（同为 in/notIn 语义）——
  const [enumValues, setEnumValues] = useState<string[]>([]);
  // 反选：勾选后枚举条件取反（in → notIn），生成 NOT IN。精确/手动共用。
  const [notInChecked, setNotInChecked] = useState(false);
  const [addedOptions, setAddedOptions] = useState<string[]>([]);
  // —— 条件筛选 ——
  const [operator, setOperator] = useState<FilterOperator>('eq');
  const [numberSub, setNumberSub] = useState<NumberConditionSub>('compare');
  const [singleText, setSingleText] = useState<string>('');
  const [singleNumber, setSingleNumber] = useState<number | null>(null);
  const [rangeStart, setRangeStart] = useState<number | null>(null);
  const [rangeEnd, setRangeEnd] = useState<number | null>(null);
  // —— 手动输入 ——
  const [manualText, setManualText] = useState('');
  const [delimiter, setDelimiter] = useState<ManualDelimiter>('newline');
  // —— 日期兜底分支 ——
  const [dateValue, setDateValue] = useState<Dayjs | null>(null);
  const [dateRange, setDateRange] = useState<[Dayjs | null, Dayjs | null] | null>(null);
  // —— 候选值实查 ——
  const [candidates, setCandidates] = useState<string[]>([]);
  const [candidatesLoaded, setCandidatesLoaded] = useState(false);
  const [candidatesLoading, setCandidatesLoading] = useState(false);
  // 「作为筛选器」：勾选后图表预览区上方出一件行内筛选控件；显示名称留空回退字段名。
  const [asFilter, setAsFilter] = useState(false);
  const [filterLabel, setFilterLabel] = useState('');

  // 打开时按已有条件 / 字段类型重置内部状态。
  useEffect(() => {
    if (!open || !field) return;
    const k = classifyFieldKind(field.dataType);
    const init = initial;
    const initOperator = init?.operator;
    setNotInChecked(initOperator === 'notIn');
    setAddedOptions([]);
    setManualText('');
    setDelimiter('newline');
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
      const enumValueList = Array.isArray(init?.value)
        ? (init.value as unknown[]).map((v) => String(v))
        : [];
      if (isNew) {
        // 新建：拖入瞬间的条件只带 store 的 eq 缺省，不是用户配置——进条件-比较。
        setActiveMode('condition');
        setNumberSub('compare');
        setEnumValues([]);
        setOperator('gte');
        setSingleNumber(null);
      } else if (init && (initOperator === 'in' || initOperator === 'notIn')) {
        // in/notIn 的统一回显入口是精确筛选面板。
        setActiveMode('precise');
        setEnumValues(enumValueList);
        setNumberSub('compare');
        setOperator('gte');
        setSingleNumber(null);
      } else if (init?.operator === 'between') {
        setActiveMode('condition');
        setNumberSub('between');
        setEnumValues([]);
        setRangeStart(init.value != null && init.value !== '' ? Number(init.value) : null);
        setRangeEnd(init.valueEnd != null && init.valueEnd !== '' ? Number(init.valueEnd) : null);
      } else {
        setActiveMode('condition');
        setNumberSub('compare');
        setEnumValues([]);
        setOperator(initOperator && NUMBER_OPERATORS.includes(initOperator) ? initOperator : 'gte');
        setSingleNumber(init?.value != null && init.value !== '' ? Number(init.value) : null);
      }
    } else {
      const enumValueList = Array.isArray(init?.value)
        ? (init.value as unknown[]).map((v) => String(v))
        : [];
      if (isNew || !init) {
        // 新建：store 的 eq 缺省不是用户配置，恒进精确筛选（与历史「字符串新建默认枚举」一致）。
        setActiveMode('precise');
        setEnumValues([]);
      } else if (initOperator === 'in' || initOperator === 'notIn') {
        setActiveMode('precise');
        setEnumValues(enumValueList);
      } else if (initOperator && STRING_OPERATORS.includes(initOperator)) {
        setActiveMode('condition');
        setEnumValues([]);
        setOperator(initOperator);
        setSingleText(init.value != null ? String(init.value) : '');
      } else {
        // 新建（或旧数据算子不认识）默认精确筛选，与字符串字段的历史缺省一致。
        setActiveMode('precise');
        setEnumValues([]);
      }
    }
    setAsFilter(init?.asFilter ?? false);
    setFilterLabel(init?.filterLabel ?? '');
  }, [open, field, initial, isNew]);

  // 需要候选值的场景：精确筛选、手动输入（非法值校验域）、数值条件-枚举子模式。
  const needCandidates =
    open &&
    field != null &&
    kind !== 'date' &&
    (activeMode === 'precise' ||
      activeMode === 'manual' ||
      (kind === 'number' && activeMode === 'condition' && numberSub === 'enum'));

  // 候选值：按数据集实查该列（单维度 + limit），客户端去重。失败不阻塞手输。
  useEffect(() => {
    if (!needCandidates || !field || datasetId == null) {
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
        setCandidatesLoaded(true);
      })
      .catch(() => {
        if (!cancelled) {
          setCandidates([]);
          setCandidatesLoaded(false);
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

  const candidateSet = useMemo(
    () => new Set([...candidates, ...addedOptions]),
    [candidates, addedOptions]
  );
  const mergedCandidates = useMemo(
    () => [...candidates, ...addedOptions.filter((v) => !candidates.includes(v))],
    [candidates, addedOptions]
  );

  const manualTokens = useMemo(() => {
    const parts = manualText.split(DELIMITER_PATTERN[delimiter]).map((s) => s.trim());
    const out: string[] = [];
    for (const t of parts) {
      if (t !== '' && !out.includes(t)) out.push(t);
    }
    return out;
  }, [manualText, delimiter]);

  // 非法判定：超长/控制字符恒非法；候选值加载成功**且有值**时，不在该列取值域内也标红
  // （提示而非拦截提交语义：合法部分照常生效；实查为空的列不做取值域校验）。
  const validateAgainstCandidates = candidatesLoaded && candidates.length > 0;
  const manualInvalid = useMemo(
    () =>
      manualTokens.map(
        (t) => manualTokenInvalid(t) || (validateAgainstCandidates && !candidateSet.has(t))
      ),
    [manualTokens, validateAgainstCandidates, candidateSet]
  );
  const manualValidTokens = useMemo(
    () => manualTokens.filter((_, i) => !manualInvalid[i]),
    [manualTokens, manualInvalid]
  );

  const dateFmt = withTime ? DATETIME_FORMAT : DATE_FORMAT;

  // 只构造「算子+取值」；asFilter/filterLabel 在 onOk 处随勾选状态统一补齐。
  const buildPatch = (): Omit<FilterConfigPatch, 'asFilter' | 'filterLabel'> | null => {
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
    if (activeMode === 'precise') {
      if (enumValues.length === 0) return null;
      return { operator: notInChecked ? 'notIn' : 'in', value: enumValues };
    }
    if (activeMode === 'manual') {
      if (manualValidTokens.length === 0) return null;
      return { operator: notInChecked ? 'notIn' : 'in', value: manualValidTokens };
    }
    if (kind === 'number') {
      if (numberSub === 'enum') {
        if (enumValues.length === 0) return null;
        return { operator: notInChecked ? 'notIn' : 'in', value: enumValues };
      }
      if (numberSub === 'between') {
        if (rangeStart == null || rangeEnd == null) return null;
        return { operator: 'between', value: rangeStart, valueEnd: rangeEnd };
      }
      if (isNoValueOperator(operator)) return { operator, value: null };
      if (singleNumber == null) return null;
      return { operator, value: singleNumber };
    }
    if (isNoValueOperator(operator)) return { operator, value: null };
    if (singleText.trim() === '') return null;
    return { operator, value: singleText.trim() };
  };

  const patch = buildPatch();
  const canOk = patch !== null;

  const renderEnumBlock = (prefix: string) => (
    <>
      <EnumPanel
        candidates={mergedCandidates}
        loading={candidatesLoading}
        value={enumValues}
        onChange={setEnumValues}
        onAddOption={(v) => setAddedOptions((prev) => (prev.includes(v) ? prev : [...prev, v]))}
        testIdPrefix={prefix}
      />
      <Checkbox
        checked={notInChecked}
        onChange={(e) => setNotInChecked(e.target.checked)}
        data-testid={`${prefix}-notin`}
      >
        反选（排除所选值）
      </Checkbox>
      {enumValues.length > 0 && (
        <Text data-testid={`${prefix}-summary`}>
          {notInChecked ? '将排除' : '将保留'} {enumValues.length} 个值：
          {enumValues.slice(0, 8).join('、')}
          {enumValues.length > 8 ? ' …' : ''}
        </Text>
      )}
    </>
  );

  const renderConditionTab = () => {
    if (kind === 'number') {
      return (
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <Segmented
            value={numberSub}
            onChange={(v) => setNumberSub(v as NumberConditionSub)}
            options={[
              { value: 'compare', label: '比较' },
              { value: 'between', label: '区间' },
              { value: 'enum', label: '枚举' },
            ]}
            data-testid="filter-modal-number-sub"
          />
          {numberSub === 'compare' && (
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
          {numberSub === 'between' && (
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
          {numberSub === 'enum' && renderEnumBlock('filter-modal-condition-enum')}
        </Space>
      );
    }
    return (
      <Space>
        <Select
          style={{ width: 130 }}
          value={operator}
          onChange={setOperator}
          options={STRING_OPERATORS.map((op) => ({ value: op, label: OPERATOR_LABELS[op] }))}
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
    );
  };

  const renderManualTab = () => (
    <Space direction="vertical" style={{ width: '100%' }} size={10}>
      <Space size={8}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          分隔符
        </Text>
        <Segmented
          size="small"
          value={delimiter}
          onChange={(v) => setDelimiter(v as ManualDelimiter)}
          options={[
            { value: 'newline', label: '换行' },
            { value: 'comma', label: '逗号' },
            { value: 'space', label: '空格' },
            { value: 'any', label: '全部' },
          ]}
          data-testid="filter-modal-delimiter"
        />
        <Upload
          accept=".csv,.txt"
          showUploadList={false}
          beforeUpload={(file) => {
            void file.text().then((text) => {
              setManualText((prev) => (prev ? `${prev}\n${text}` : text));
            });
            return false;
          }}
        >
          <Button type="link" size="small" data-testid="filter-modal-csv-upload">
            上传 CSV
          </Button>
        </Upload>
      </Space>
      <TextArea
        rows={5}
        placeholder={'粘贴值，如：\n北京\n上海\n广州'}
        value={manualText}
        onChange={(e) => setManualText(e.target.value)}
        data-testid="filter-modal-manual"
      />
      {manualTokens.length > 0 && (
        <div
          style={{ maxHeight: 120, overflowY: 'auto' }}
          data-testid="filter-modal-manual-preview"
        >
          {manualTokens.map((token, i) =>
            manualInvalid[i] ? (
              <Tooltip
                key={token}
                title={
                  candidatesLoaded && !manualTokenInvalid(token)
                    ? '不在该列候选值中'
                    : '非法值（超长或含控制字符）'
                }
              >
                <Tag color="error" style={{ marginBottom: 4 }}>
                  {token.length > 24 ? `${token.slice(0, 24)}…` : token}
                </Tag>
              </Tooltip>
            ) : (
              <Tag key={token} style={{ marginBottom: 4 }}>
                {token.length > 24 ? `${token.slice(0, 24)}…` : token}
              </Tag>
            )
          )}
        </div>
      )}
      {manualInvalid.some(Boolean) && (
        <Text type="danger" style={{ fontSize: 12 }} data-testid="filter-modal-manual-invalid">
          {manualInvalid.filter(Boolean).length} 个非法值将被忽略
        </Text>
      )}
      <Checkbox
        checked={notInChecked}
        onChange={(e) => setNotInChecked(e.target.checked)}
        data-testid="filter-modal-notin"
      >
        反选（排除所输值）
      </Checkbox>
      {manualValidTokens.length > 0 && (
        <Text data-testid="filter-modal-summary">
          {notInChecked ? '将排除' : '将保留'} {manualValidTokens.length} 个值：
          {manualValidTokens.slice(0, 8).join('、')}
          {manualValidTokens.length > 8 ? ' …' : ''}
        </Text>
      )}
    </Space>
  );

  const renderDateBody = () => (
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

  const renderModeTabs = () => (
    <Tabs
      activeKey={activeMode}
      onChange={(key) => setActiveMode(key as FilterMode)}
      size="small"
      destroyOnHidden
      items={[
        {
          key: 'precise',
          label: '精确筛选',
          children: (
            <Space direction="vertical" style={{ width: '100%' }} size={10}>
              {renderEnumBlock('filter-modal-enum')}
            </Space>
          ),
        },
        {
          key: 'condition',
          label: '条件筛选',
          children: renderConditionTab(),
        },
        {
          key: 'manual',
          label: '手动输入',
          children: renderManualTab(),
        },
        {
          key: 'subquery',
          label: <Tooltip title="远期规划：引用其他图表结果作为筛选项来源">子查询</Tooltip>,
          disabled: true,
          children: null,
        },
      ]}
      data-testid="filter-modal-mode"
    />
  );

  return (
    <Modal
      open={open}
      title={field ? `筛选 · ${field.name}` : '筛选'}
      okText="确定"
      cancelText="取消"
      okButtonProps={{ disabled: !canOk }}
      onOk={() => {
        if (patch) onOk({ ...patch, asFilter, filterLabel: filterLabel.trim() });
      }}
      onCancel={onCancel}
      width={kind === 'date' ? 480 : 560}
      destroyOnHidden
      // 配置弹窗追求"即点即开"：关闭默认的 zoom+fade 过渡（约 300ms）。
      transitionName=""
      maskTransitionName=""
    >
      {field ? (kind === 'date' ? renderDateBody() : renderModeTabs()) : null}
      <Divider style={{ margin: '12px 0' }} />
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <Checkbox
          checked={asFilter}
          onChange={(e) => setAsFilter(e.target.checked)}
          data-testid="filter-modal-as-filter"
        >
          作为筛选器
        </Checkbox>
        {asFilter && (
          <>
            <Text type="secondary">显示名称：</Text>
            <Input
              placeholder={field?.name}
              value={filterLabel}
              onChange={(e) => setFilterLabel(e.target.value)}
              style={{ width: 200 }}
              data-testid="filter-modal-filter-label"
            />
          </>
        )}
      </div>
    </Modal>
  );
};

export default FilterConfigModal;
