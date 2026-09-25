import { CalendarOutlined, CloseCircleFilled } from '@ant-design/icons';
import { Button, DatePicker, Popover, Typography } from 'antd';
import dayjs from 'dayjs';
import React, { useState } from 'react';
import {
  type DateFilterMode,
  type DateFilterSettings,
  type DateFilterValue,
  formatDateFilterPreview,
  formatDateFilterSummary,
  hasDateFilterSelection,
  MODE_LABELS,
} from '@/lib/dateFilter';
import DateFilterEditor, { blankValueForMode } from './DateFilterEditor';

const { Text } = Typography;

/** 行内控件里可切换的模式：只有「范围」组四种（单个日期是另一种控件形态）。 */
const SWITCHABLE_MODES: DateFilterMode[] = ['dynamic', 'fixed', 'advanced', 'special'];

const DATE_FORMAT = 'YYYY-MM-DD';
const DATETIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

export interface DateFilterControlProps {
  /** 筛选器显示名称（如「交易日期」）。 */
  label: string;
  value: DateFilterValue;
  settings: DateFilterSettings;
  /** 字段带时分秒 → 开放时间选择与小时粒度。 */
  withTime?: boolean;
  /** 允许在动态/固定/高级/特殊值之间切换；仅当用户勾选过「作为时间范围筛选器」。 */
  allowModeSwitch?: boolean;
  /** 数据集里真实存在的日期（YYYY-MM-DD），用于日历置灰。 */
  availableDates?: string[];
  /** 是否显示清除按钮（清空 = 未激活，不参与筛选合并）。 */
  allowClear?: boolean;
  onChange: (next: DateFilterValue) => void;
  onClear?: () => void;
}

/**
 * 日期筛选器的行内控件（图表查询页与仪表盘查询页共用）。
 *
 * 形态对齐火山引擎智能数据洞察：
 * - 范围类模式 → 一枚显示「名称：当前值」的按钮，点击弹出浮层改筛选条件；
 * - 单个日期模式 → 直接是一枚日期选择器，没有浮层（文档 §3.10 的口径）。
 *
 * 浮层内即时生效（点一个快捷选项就换一次筛选），不设「确定」——行内控件是高频切换的
 * 场景，多一步确认反而碍事；需要精细配置时用户走弹窗那条路。
 */
const DateFilterControl: React.FC<DateFilterControlProps> = ({
  label,
  value,
  settings,
  withTime,
  allowModeSwitch = false,
  availableDates,
  allowClear = false,
  onChange,
  onClear,
}) => {
  const [open, setOpen] = useState(false);
  const active = hasDateFilterSelection(value);
  const showTime = withTime === true || settings.granularity === 'hour';

  if (value.kind === 'single') {
    return (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
        <Text type="secondary">{label}</Text>
        <DatePicker
          size="small"
          showTime={showTime}
          format={showTime ? DATETIME_FORMAT : DATE_FORMAT}
          value={value.date ? dayjs(value.date) : null}
          onChange={(next) =>
            onChange({
              kind: 'single',
              date: next ? next.format(showTime ? DATETIME_FORMAT : DATE_FORMAT) : '',
            })
          }
          data-testid="date-filter-control-single"
        />
      </span>
    );
  }

  const switchMode = (mode: DateFilterMode) => {
    // 浮层里切模式不做跨模式记忆：空间小、用户预期是「换一种筛法」，
    // 留一个上一个模式的残值反而更容易误判当前生效的是什么。
    onChange(blankValueForMode(mode));
  };

  const content = (
    <div style={{ display: 'flex', gap: 12, minWidth: 520 }}>
      {allowModeSwitch && (
        <div
          style={{
            width: 96,
            flex: '0 0 96px',
            borderRight: '1px solid var(--dr-border)',
            paddingRight: 8,
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
          }}
        >
          {SWITCHABLE_MODES.map((mode) => (
            <div
              key={mode}
              role="tab"
              aria-selected={value.kind === mode}
              tabIndex={0}
              data-testid={`date-filter-control-mode-${mode}`}
              onClick={() => switchMode(mode)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') switchMode(mode);
              }}
              style={{
                padding: '6px 10px',
                borderRadius: 'var(--dr-radius-sm)',
                cursor: 'pointer',
                fontSize: 13,
                background: value.kind === mode ? 'var(--dr-accent-soft)' : 'transparent',
                color: value.kind === mode ? 'var(--dr-accent)' : 'var(--dr-text-2)',
                fontWeight: value.kind === mode ? 600 : 400,
                userSelect: 'none',
              }}
            >
              {MODE_LABELS[mode]}
            </div>
          ))}
        </div>
      )}
      <div style={{ flex: 1, minWidth: 0 }}>
        <DateFilterEditor
          value={value}
          settings={settings}
          withTime={withTime}
          availableDates={availableDates}
          onChange={onChange}
          onSettingsChange={() => {
            /* 浮层里不提供粒度/周计算逻辑的编辑入口——那是弹窗的职责 */
          }}
        />
      </div>
    </div>
  );

  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      trigger="click"
      placement="bottomLeft"
      content={content}
      data-testid="date-filter-control-popover"
    >
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
        <Button
          size="small"
          icon={<CalendarOutlined />}
          data-testid="date-filter-control"
          title={formatDateFilterPreview(value, settings)}
        >
          {label}：{formatDateFilterSummary(value, settings)}
        </Button>
        {allowClear && active && onClear && (
          <Button
            size="small"
            type="text"
            icon={<CloseCircleFilled />}
            onClick={onClear}
            data-testid="date-filter-control-clear"
          />
        )}
      </span>
    </Popover>
  );
};

export default DateFilterControl;
