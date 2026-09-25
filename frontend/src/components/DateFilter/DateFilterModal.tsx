import { Checkbox, Input, Modal, Select, Typography } from 'antd';
import React, { useCallback, useRef, useState } from 'react';
import {
  type DateFilterMode,
  type DateFilterSettings,
  type DateFilterValue,
  type DateGranularity,
  GRANULARITY_OPTIONS,
  HOUR_GRANULARITY_OPTION,
  isDateFilterComplete,
  MODE_LABELS,
  presetsForGranularity,
  type WeekStart,
} from '@/lib/dateFilter';
import DateFilterEditor, { blankValueForMode } from './DateFilterEditor';

const { Text } = Typography;

/** 「范围」组里的四个模式（互斥切换）。 */
const RANGE_MODES: DateFilterMode[] = ['dynamic', 'fixed', 'advanced', 'special'];
/** 「单个日期」组：与范围组互斥，只提供单日选择。 */
const SINGLE_MODES: DateFilterMode[] = ['single'];

export interface DateFilterModalPayload {
  value: DateFilterValue;
  granularity: DateGranularity;
  weekStart: WeekStart;
  /** 是否勾选「作为时间范围筛选器」——勾了才在页面/仪表盘上放行内控件。 */
  asFilter: boolean;
  /** 行内控件的显示名称。 */
  filterLabel: string;
}

export interface DateFilterModalProps {
  open: boolean;
  /** 字段展示名，用于标题与默认显示名称。 */
  fieldName?: string;
  /** 字段带时分秒 → 开放小时粒度与时间选择。 */
  withTime?: boolean;
  /** 已有筛选值（点击已有条件进入编辑）；新建时 undefined。 */
  initial?: DateFilterValue;
  initialGranularity?: DateGranularity;
  initialWeekStart?: WeekStart;
  initialAsFilter?: boolean;
  initialFilterLabel?: string;
  /** 数据集里真实存在的日期（YYYY-MM-DD），用于日历置灰。 */
  availableDates?: string[];
  onOk: (payload: DateFilterModalPayload) => void;
  onCancel: () => void;
}

function initialValues(initial?: DateFilterValue): Record<DateFilterMode, DateFilterValue> {
  const base: Record<DateFilterMode, DateFilterValue> = {
    dynamic: blankValueForMode('dynamic'),
    fixed: blankValueForMode('fixed'),
    advanced: blankValueForMode('advanced'),
    special: blankValueForMode('special'),
    single: blankValueForMode('single'),
  };
  if (initial) {
    base[initial.kind] = initial;
  }
  return base;
}

/**
 * 日期筛选弹窗（对齐火山引擎智能数据洞察「日期筛选」的交互）。
 *
 * 布局：左侧「粒度 + 范围组 + 单个日期组」导航，右侧「时间预览 + 编辑体」，
 * 底部「作为时间范围筛选器 + 显示名称」。
 *
 * 设计取舍：
 * - 每个模式各自记住自己的编辑状态（`values`）。来回切模式不该丢掉刚才填的区间，
 *   这与文档里「四种模式切换使用」的用法一致。
 * - 未选择时「确定」禁用；高级模式「起止同时无限制」也禁用（文档明确禁止）。
 */
const DateFilterModal: React.FC<DateFilterModalProps> = ({
  open,
  fieldName,
  withTime,
  initial,
  initialGranularity,
  initialWeekStart,
  initialAsFilter,
  initialFilterLabel,
  availableDates,
  onOk,
  onCancel,
}) => {
  const [mode, setMode] = useState<DateFilterMode>(initial?.kind ?? 'dynamic');
  const [granularity, setGranularity] = useState<DateGranularity>(initialGranularity ?? 'day');
  const [weekStart, setWeekStart] = useState<WeekStart>(initialWeekStart ?? 1);
  const [values, setValues] = useState<Record<DateFilterMode, DateFilterValue>>(() =>
    initialValues(initial)
  );
  const [asFilter, setAsFilter] = useState(initialAsFilter === true);
  const [filterLabel, setFilterLabel] = useState(initialFilterLabel ?? '');

  // 打开时按 initial 重放一次状态；关闭时不重置，避免关闭动画期间闪内容。
  const lastInitial = useRef<string>('');
  const signature = JSON.stringify({
    open,
    field: fieldName,
    withTime,
    initial,
    initialGranularity,
    initialWeekStart,
    initialAsFilter,
    initialFilterLabel,
  });
  if (signature !== lastInitial.current) {
    lastInitial.current = signature;
    if (open) {
      setMode(initial?.kind ?? 'dynamic');
      setGranularity(initialGranularity ?? 'day');
      setWeekStart(initialWeekStart ?? 1);
      setValues(initialValues(initial));
      setAsFilter(initialAsFilter === true);
      setFilterLabel(initialFilterLabel ?? '');
    }
  }

  const settings: DateFilterSettings = { granularity, weekStart, withTime };
  const value = values[mode];
  const complete = isDateFilterComplete(value);
  const displayLabel = filterLabel.trim() !== '' ? filterLabel : (fieldName ?? '');

  const handleValueChange = useCallback((next: DateFilterValue) => {
    setValues((previous) => ({ ...previous, [next.kind]: next }));
  }, []);

  const renderNavItem = (item: DateFilterMode) => {
    const active = mode === item;
    return (
      <div
        key={item}
        role="tab"
        aria-selected={active}
        tabIndex={0}
        data-testid={`date-filter-mode-${item}`}
        onClick={() => setMode(item)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') setMode(item);
        }}
        style={{
          padding: '8px 16px',
          cursor: 'pointer',
          fontSize: 13,
          borderRadius: 'var(--dr-radius-sm)',
          background: active ? 'var(--dr-accent-soft)' : 'transparent',
          color: active ? 'var(--dr-accent)' : 'var(--dr-text-2)',
          fontWeight: active ? 600 : 400,
          userSelect: 'none',
        }}
      >
        {MODE_LABELS[item]}
      </div>
    );
  };

  return (
    <Modal
      open={open}
      title="日期筛选"
      width={880}
      okText="确定"
      cancelText="取消"
      okButtonProps={{ disabled: !complete }}
      onOk={() => onOk({ value, granularity, weekStart, asFilter, filterLabel: displayLabel })}
      onCancel={onCancel}
      destroyOnHidden
      styles={{
        body: { padding: 0, display: 'flex', flexWrap: 'wrap', minHeight: 460 },
      }}
    >
      <div
        style={{
          width: 190,
          flex: '0 0 190px',
          borderRight: '1px solid var(--dr-border)',
          padding: '12px 8px',
          display: 'flex',
          flexDirection: 'column',
          gap: 4,
        }}
      >
        <Select
          style={{ width: '100%', marginBottom: 8 }}
          value={granularity}
          data-testid="date-filter-granularity"
          options={
            withTime ? [...GRANULARITY_OPTIONS, HOUR_GRANULARITY_OPTION] : [...GRANULARITY_OPTIONS]
          }
          onChange={(next) => {
            setGranularity(next);
            // 粒度切换会换掉整套快捷选项：原选项在新粒度里不存在时退回「未选择」，
            // 而不是留一个下拉里高亮不出来的幽灵选中态（预览与选项会对不上）。
            setValues((previous) => {
              const current = previous.dynamic;
              if (current.kind !== 'dynamic' || current.preset === undefined) return previous;
              if (presetsForGranularity(next).includes(current.preset)) return previous;
              return {
                ...previous,
                dynamic: { kind: 'dynamic', includeEmpty: current.includeEmpty },
              };
            });
          }}
        />
        <Text type="secondary" style={{ fontSize: 12, padding: '0 16px' }}>
          范围
        </Text>
        {RANGE_MODES.map(renderNavItem)}
        <Text type="secondary" style={{ fontSize: 12, padding: '8px 16px 0' }}>
          单个日期
        </Text>
        {SINGLE_MODES.map(renderNavItem)}
      </div>

      <div style={{ flex: 1, padding: '16px 20px', overflow: 'auto' }}>
        <DateFilterEditor
          value={value}
          settings={settings}
          withTime={withTime}
          availableDates={availableDates}
          onChange={handleValueChange}
          onSettingsChange={(patch) => {
            if (patch.weekStart !== undefined) setWeekStart(patch.weekStart);
            if (patch.granularity !== undefined) setGranularity(patch.granularity);
          }}
        />
      </div>

      <div
        style={{
          flexBasis: '100%',
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          borderTop: '1px solid var(--dr-border)',
          padding: '12px 20px',
        }}
      >
        <Checkbox
          checked={asFilter}
          onChange={(event) => setAsFilter(event.target.checked)}
          data-testid="date-filter-as-filter"
        >
          作为时间范围筛选器
        </Checkbox>
        <Text type="secondary">显示名称：</Text>
        <Input
          style={{ width: 200 }}
          placeholder={fieldName}
          value={filterLabel}
          onChange={(event) => setFilterLabel(event.target.value)}
          data-testid="date-filter-label"
        />
      </div>
    </Modal>
  );
};

export default DateFilterModal;
