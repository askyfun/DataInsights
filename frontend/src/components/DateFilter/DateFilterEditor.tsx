import { Alert, Button, Checkbox, DatePicker, InputNumber, Radio, Select, Typography } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import React, { useMemo } from 'react';
import {
  type DateBound,
  type DateFilterMode,
  type DateFilterSettings,
  type DateFilterValue,
  type DateGranularity,
  type DynamicCustomValue,
  type DynamicOp,
  type DynamicPresetKey,
  type DynamicUnit,
  formatDateFilterPreview,
  PRESET_LABELS,
  presetsForGranularity,
  resolveDateFilter,
  WEEK_START_OPTIONS,
} from '@/lib/dateFilter';

const { Text } = Typography;

const DATE_FORMAT = 'YYYY-MM-DD';
const DATETIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

const OP_OPTIONS: { value: DynamicOp; label: string }[] = [
  { value: 'last', label: '最近' },
  { value: 'before', label: '前' },
  { value: 'after', label: '后' },
];

const UNIT_OPTIONS: { value: DynamicUnit; label: string }[] = [
  { value: 'hour', label: '小时' },
  { value: 'day', label: '天' },
  { value: 'week', label: '周' },
  { value: 'month', label: '月' },
  { value: 'year', label: '年' },
];

const BOUND_TYPE_OPTIONS: { value: DateBound['type']; label: string }[] = [
  { value: 'fixed', label: '固定' },
  { value: 'dynamic', label: '动态' },
  { value: 'unlimited', label: '无限制' },
];

export interface DateFilterEditorProps {
  /** 当前筛选值；其 `kind` 决定渲染哪一支编辑体。 */
  value: DateFilterValue;
  settings: DateFilterSettings;
  /** 字段带时分秒 → 开放时间选择与小时粒度。 */
  withTime?: boolean;
  /** 数据集里真实存在的日期（YYYY-MM-DD）；给了就在日历上把其余日期置灰。 */
  availableDates?: string[];
  /** 注入当前时刻，便于测试；缺省取运行时钟。 */
  now?: Dayjs;
  onChange: (next: DateFilterValue) => void;
  onSettingsChange: (patch: Partial<DateFilterSettings>) => void;
}

function readIncludeEmpty(value: DateFilterValue): boolean {
  if (value.kind === 'special' || value.kind === 'single') return false;
  return value.includeEmpty === true;
}

function withIncludeEmpty(value: DateFilterValue, includeEmpty: boolean): DateFilterValue {
  // 特殊值与单个日期没有「区间」概念，不接受这个开关。
  if (value.kind === 'special' || value.kind === 'single') return value;
  return { ...value, includeEmpty };
}

/** 自定义动态日期的缺省值：与文档截图的默认勾选一致（含今天 / 整点 / 包含本小时）。 */
export function defaultCustomDynamic(unit: DynamicUnit = 'day'): DynamicCustomValue {
  return {
    op: 'last',
    n: 1,
    unit,
    includeToday: true,
    ...(unit === 'hour' ? { onTheHour: true, includeCurrentHour: true } : {}),
  };
}

/** 某个模式的空白初值（切模式时用，保证不残留上一个模式的选择）。 */
export function blankValueForMode(mode: DateFilterMode): DateFilterValue {
  switch (mode) {
    case 'dynamic':
      return { kind: 'dynamic' };
    case 'fixed':
      return { kind: 'fixed', start: '', end: '' };
    case 'advanced':
      return {
        kind: 'advanced',
        start: { type: 'unlimited' },
        end: { type: 'fixed', value: '' },
      };
    case 'special':
      return { kind: 'special', value: 'all' };
    case 'single':
      return { kind: 'single', date: '' };
  }
}

/**
 * 日期筛选器的编辑体（形态无关的深模块）。
 *
 * 为什么单独抽一层：图表查询页的弹窗与仪表盘的行内控件外壳完全不同，
 * 但「选中什么日期」这件事必须只有一份实现——两份实现必然会漂移。
 * 因此这里只负责「渲染某个筛选值 + 把它改掉」，不关心自己长在弹窗里还是浮层里。
 */
const DateFilterEditor: React.FC<DateFilterEditorProps> = ({
  value,
  settings,
  withTime,
  availableDates,
  now,
  onChange,
  onSettingsChange,
}) => {
  const current = now ?? dayjs();
  const granularity = settings.granularity;
  const showTime = withTime === true || granularity === 'hour';

  // 日历置灰：只在日粒度上做（月/周粒度的选择单位不是「某一天」）。
  const available = useMemo(
    () => (availableDates && availableDates.length > 0 ? new Set(availableDates) : null),
    [availableDates]
  );
  const disabledDate =
    available && granularity === 'day'
      ? (candidate: Dayjs) => !available.has(candidate.format(DATE_FORMAT))
      : undefined;

  const preview = formatDateFilterPreview(value, settings, current);
  const resolved = resolveDateFilter(value, settings, current);

  const renderIncludeEmpty = () => (
    <Checkbox
      checked={readIncludeEmpty(value)}
      onChange={(event) => onChange(withIncludeEmpty(value, event.target.checked))}
      data-testid="date-filter-include-empty"
    >
      包含空日期
    </Checkbox>
  );

  const renderDynamic = () => {
    const dynamic = value.kind === 'dynamic' ? value : { kind: 'dynamic' as const };
    const custom = dynamic.custom;
    const presets = presetsForGranularity(granularity);
    const unitOptions = unitOptionsFor(granularity);

    const patchCustom = (patch: Partial<DynamicCustomValue>) => {
      const base = custom ?? defaultCustomDynamic(granularity === 'hour' ? 'hour' : 'day');
      onChange({ kind: 'dynamic', custom: { ...base, ...patch } });
    };

    return (
      <>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
            gap: 8,
          }}
          data-testid="date-filter-presets"
        >
          {presets.map((key: DynamicPresetKey) => (
            <Button
              key={key}
              size="middle"
              type={dynamic.preset === key ? 'primary' : 'default'}
              data-testid={`date-filter-preset-${key}`}
              onClick={() =>
                onChange({ kind: 'dynamic', preset: key, includeEmpty: dynamic.includeEmpty })
              }
            >
              {PRESET_LABELS[key]}
            </Button>
          ))}
          <Button
            size="middle"
            type={custom ? 'primary' : 'default'}
            data-testid="date-filter-preset-custom"
            onClick={() => {
              if (custom) {
                onChange({ kind: 'dynamic', includeEmpty: dynamic.includeEmpty });
                return;
              }
              onChange({
                kind: 'dynamic',
                includeEmpty: dynamic.includeEmpty,
                custom: defaultCustomDynamic(granularity === 'hour' ? 'hour' : 'day'),
              });
            }}
          >
            自定义
          </Button>
        </div>

        {custom && (
          <div
            style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}
            data-testid="date-filter-custom"
          >
            <Select
              style={{ width: 96 }}
              value={custom.op}
              options={OP_OPTIONS}
              onChange={(op) => patchCustom({ op })}
              data-testid="date-filter-custom-op"
            />
            <InputNumber
              min={0}
              max={999}
              style={{ width: 88 }}
              value={custom.n}
              onChange={(n) => patchCustom({ n: n ?? 0 })}
              data-testid="date-filter-custom-n"
            />
            <Select
              style={{ width: 112 }}
              value={custom.unit}
              options={unitOptions}
              onChange={(unit) => patchCustom({ unit })}
              data-testid="date-filter-custom-unit"
            />
            {custom.op === 'last' && (
              <Checkbox
                checked={custom.includeToday === true}
                onChange={(event) => patchCustom({ includeToday: event.target.checked })}
                data-testid="date-filter-custom-include-today"
              >
                包含今天
              </Checkbox>
            )}
            {custom.unit === 'hour' && custom.op === 'last' && (
              <>
                <Checkbox
                  checked={custom.onTheHour !== false}
                  onChange={(event) => patchCustom({ onTheHour: event.target.checked })}
                  data-testid="date-filter-custom-on-the-hour"
                >
                  整点
                </Checkbox>
                <Checkbox
                  checked={custom.includeCurrentHour === true}
                  onChange={(event) => patchCustom({ includeCurrentHour: event.target.checked })}
                  data-testid="date-filter-custom-include-current-hour"
                >
                  包含本小时
                </Checkbox>
              </>
            )}
          </div>
        )}

        {granularity === 'week' && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Text type="secondary">周计算逻辑：</Text>
            <Select
              style={{ width: 160 }}
              value={settings.weekStart ?? 1}
              options={[...WEEK_START_OPTIONS]}
              onChange={(weekStart) => onSettingsChange({ weekStart })}
              data-testid="date-filter-week-start"
            />
          </div>
        )}

        {renderIncludeEmpty()}
      </>
    );
  };

  const renderFixed = () => {
    const fixed = value.kind === 'fixed' ? value : { kind: 'fixed' as const, start: '', end: '' };
    const range: [Dayjs | null, Dayjs | null] =
      fixed.start && fixed.end ? [dayjs(fixed.start), dayjs(fixed.end)] : [null, null];
    return (
      <>
        <DatePicker.RangePicker
          style={{ width: '100%' }}
          showTime={showTime}
          format={showTime ? DATETIME_FORMAT : DATE_FORMAT}
          value={range}
          disabledDate={disabledDate}
          onChange={(next) =>
            onChange({
              kind: 'fixed',
              start: next?.[0] ? next[0].format(showTime ? DATETIME_FORMAT : DATE_FORMAT) : '',
              end: next?.[1] ? next[1].format(showTime ? DATETIME_FORMAT : DATE_FORMAT) : '',
              includeEmpty: fixed.includeEmpty,
            })
          }
          data-testid="date-filter-fixed-range"
        />
        {renderIncludeEmpty()}
      </>
    );
  };

  const renderAdvanced = () => {
    const advanced =
      value.kind === 'advanced'
        ? value
        : ({
            kind: 'advanced' as const,
            start: { type: 'unlimited' },
            end: { type: 'fixed', value: '' },
          } as const);
    const bothUnlimited = advanced.start.type === 'unlimited' && advanced.end.type === 'unlimited';

    const renderBound = (role: 'start' | 'end') => {
      const bound = advanced[role];
      const patch = (next: DateBound) => onChange({ ...advanced, [role]: next } as DateFilterValue);
      return (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <Select
            style={{ width: 112 }}
            value={bound.type}
            options={BOUND_TYPE_OPTIONS}
            data-testid={`date-filter-advanced-${role}-type`}
            onChange={(type) => {
              if (type === 'unlimited') {
                patch({ type: 'unlimited' });
                return;
              }
              if (type === 'fixed') {
                patch({ type: 'fixed', value: bound.type === 'fixed' ? bound.value : '' });
                return;
              }
              patch({
                type: 'dynamic',
                op: bound.type === 'dynamic' ? bound.op : 'before',
                n: bound.type === 'dynamic' ? bound.n : 1,
                unit: bound.type === 'dynamic' ? bound.unit : 'day',
              });
            }}
          />
          {bound.type === 'fixed' && (
            <DatePicker
              style={{ width: 200 }}
              showTime={showTime}
              format={showTime ? DATETIME_FORMAT : DATE_FORMAT}
              value={bound.value ? dayjs(bound.value) : null}
              disabledDate={disabledDate}
              onChange={(next) =>
                patch({
                  type: 'fixed',
                  value: next ? next.format(showTime ? DATETIME_FORMAT : DATE_FORMAT) : '',
                })
              }
              data-testid={`date-filter-advanced-${role}-value`}
            />
          )}
          {bound.type === 'dynamic' && (
            <>
              <InputNumber
                min={0}
                max={999}
                style={{ width: 88 }}
                value={bound.n}
                onChange={(n) => patch({ ...bound, n: n ?? 0 })}
                data-testid={`date-filter-advanced-${role}-n`}
              />
              <Select
                style={{ width: 100 }}
                value={bound.unit}
                options={unitOptionsFor(granularity)}
                onChange={(unit) => patch({ ...bound, unit })}
                data-testid={`date-filter-advanced-${role}-unit`}
              />
              <Select
                style={{ width: 88 }}
                value={bound.op}
                options={[
                  { value: 'before', label: '前' },
                  { value: 'after', label: '后' },
                ]}
                onChange={(op) => patch({ ...bound, op })}
                data-testid={`date-filter-advanced-${role}-op`}
              />
            </>
          )}
        </div>
      );
    };

    return (
      <>
        <div>
          <Text type="secondary" style={{ display: 'block', marginBottom: 6 }}>
            开始时间
          </Text>
          {renderBound('start')}
        </div>
        <div>
          <Text type="secondary" style={{ display: 'block', marginBottom: 6 }}>
            结束时间
          </Text>
          {renderBound('end')}
        </div>
        {bothUnlimited && (
          <Alert
            type="warning"
            showIcon
            title="开始时间与结束时间不能同时为「无限制」，请至少限定一端。"
            data-testid="date-filter-advanced-conflict"
          />
        )}
        {renderIncludeEmpty()}
      </>
    );
  };

  const renderSpecial = () => {
    const special =
      value.kind === 'special' ? value : { kind: 'special' as const, value: 'all' as const };
    return (
      <Radio.Group
        value={special.value}
        options={[
          { value: 'empty', label: '空日期' },
          { value: 'notEmpty', label: '非空日期' },
          { value: 'all', label: '所有日期' },
        ]}
        onChange={(event) => onChange({ kind: 'special', value: event.target.value })}
        data-testid="date-filter-special"
      />
    );
  };

  const renderSingle = () => {
    const single = value.kind === 'single' ? value : { kind: 'single' as const, date: '' };
    return (
      <DatePicker
        style={{ width: 260 }}
        showTime={showTime}
        format={showTime ? DATETIME_FORMAT : DATE_FORMAT}
        value={single.date ? dayjs(single.date) : null}
        disabledDate={disabledDate}
        onChange={(next) =>
          onChange({
            kind: 'single',
            date: next ? next.format(showTime ? DATETIME_FORMAT : DATE_FORMAT) : '',
          })
        }
        data-testid="date-filter-single"
      />
    );
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div>
        <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>
          时间预览：
        </Text>
        <Text strong style={{ fontSize: 16 }} data-testid="date-filter-preview">
          {preview}
        </Text>
      </div>

      {value.kind === 'dynamic' && renderDynamic()}
      {value.kind === 'fixed' && renderFixed()}
      {value.kind === 'advanced' && renderAdvanced()}
      {value.kind === 'special' && renderSpecial()}
      {value.kind === 'single' && renderSingle()}

      {resolved.includeEmpty && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          已开启「包含空日期」：区间条件之外会额外放行该列为 NULL 的行。
        </Text>
      )}
    </div>
  );
};

/** 小时只能出现在「小时粒度」；其余粒度把小时单位从下拉里摘掉。 */
function unitOptionsFor(granularity: DateGranularity) {
  return granularity === 'hour'
    ? UNIT_OPTIONS
    : UNIT_OPTIONS.filter((option) => option.value !== 'hour');
}

export default DateFilterEditor;
