/**
 * 日期筛选器核心语义 —— 前端单一事实源（对齐火山引擎智能数据洞察「日期筛选」）。
 *
 * 设计要点：
 * 1. **纯逻辑、零 UI 依赖**：解析、快捷选项、文案格式化都在这里，弹窗组件只做摆放。
 *    这样同一个筛选值既能被图表查询页消费，也能被仪表盘盘级筛选器复用。
 * 2. **动态日期在「下发请求时」才解析**：用户存的是 `最近 7 天` 这个意图，
 *    不是 `2026-09-18 ~ 2026-09-24` 这个快照。每次查询现算，页面放着不动跨天也不会失真。
 * 3. **区间端点一律输出 `YYYY-MM-DD`（datetime/小时粒度输出到秒）**，不输出
 *    `YYYY-MM` / 周序号这类展示形态——SQL 比较用的是真实日期，展示形态只活在 UI 层。
 * 4. 全部区间以「昨天」为数据上界（文档口径）：`本周`=本周首日~昨天、`本月`=本月 1 日~昨天。
 *    这也是产品侧「T+1 落数」的默认假设；需要含今天时用「自定义 + 包含今天」显式打开。
 */

import dayjs, { type Dayjs } from 'dayjs';

// ---------------------------------------------------------------------------
// 类型
// ---------------------------------------------------------------------------

/** 日期筛选粒度。`hour` 仅对带时分秒的 datetime 字段开放。 */
export type DateGranularity = 'hour' | 'day' | 'week' | 'month';

/** 周计算逻辑：一周从星期几开始（0=周日 … 6=周六）。默认 1（周一）。 */
export type WeekStart = 0 | 1 | 2 | 3 | 4 | 5 | 6;

/** 自定义动态日期的单位。 */
export type DynamicUnit = 'hour' | 'day' | 'week' | 'month' | 'year';

/**
 * 自定义动态日期的操作符：
 * - `last`   最近 N 个周期（区间，止于昨天/上一整点）
 * - `before` N 个周期之前（只有上界）
 * - `after`  N 个周期之后（只有下界）
 */
export type DynamicOp = 'last' | 'before' | 'after';

/** 快捷选项键。三套粒度各用其一子集，见 `presetsForGranularity`。 */
export type DynamicPresetKey =
  // 日单位
  | 'last1d'
  | 'last7d'
  | 'last14d'
  | 'last30d'
  | 'last365d'
  // 周单位
  | 'last1w'
  | 'last4w'
  | 'last13w'
  | 'last52w'
  // 月单位
  | 'last1m'
  | 'last3m'
  | 'last6m'
  | 'last12m'
  // 自然周期
  | 'thisWeek'
  | 'lastWeek'
  | 'thisMonth'
  | 'lastMonth'
  | 'thisBiMonth'
  | 'lastBiMonth'
  | 'thisQuarter'
  | 'lastQuarter'
  | 'thisYear'
  | 'last2y'
  | 'lastYear';

export interface DynamicCustomValue {
  op: DynamicOp;
  n: number;
  unit: DynamicUnit;
  /** 仅 `op === 'last'`：把今天算进区间（默认算到昨天）。 */
  includeToday?: boolean;
  /** 仅 `unit === 'hour'`：起止对齐到整点边界（默认对齐）；显式 false 为相对当前时刻的滚动窗口。 */
  onTheHour?: boolean;
  /** 仅 `unit === 'hour'` 且对齐时：结束时间包含当前所在小时（默认不含）。 */
  includeCurrentHour?: boolean;
}

export interface DynamicDateFilterValue {
  kind: 'dynamic';
  /** 与 `custom` 二选一；两者都缺省 = 未选择。 */
  preset?: DynamicPresetKey;
  custom?: DynamicCustomValue;
  /** 包含空日期：额外放行该列为 NULL 的行（OR 连接）。 */
  includeEmpty?: boolean;
}

export interface FixedDateFilterValue {
  kind: 'fixed';
  start: string;
  end: string;
  includeEmpty?: boolean;
}

/** 高级模式的一侧端点。 */
export type DateBound =
  | { type: 'fixed'; value: string }
  | { type: 'dynamic'; op: Exclude<DynamicOp, 'last'>; n: number; unit: DynamicUnit }
  | { type: 'unlimited' };

export interface AdvancedDateFilterValue {
  kind: 'advanced';
  start: DateBound;
  end: DateBound;
  includeEmpty?: boolean;
}

export interface SpecialDateFilterValue {
  kind: 'special';
  value: 'empty' | 'notEmpty' | 'all';
}

export interface SingleDateFilterValue {
  kind: 'single';
  date: string;
}

export type DateFilterValue =
  | DynamicDateFilterValue
  | FixedDateFilterValue
  | AdvancedDateFilterValue
  | SpecialDateFilterValue
  | SingleDateFilterValue;

export type DateFilterMode = DateFilterValue['kind'];

export interface DateFilterSettings {
  granularity: DateGranularity;
  /** 周计算逻辑；缺省周一。 */
  weekStart?: WeekStart;
  /** 字段带时分秒 → 输出到秒。`hour` 粒度隐式开启。 */
  withTime?: boolean;
}

/** 解析后的下发语义。`operator` 为 null 表示这条日期条件不下发任何约束。 */
export interface DateFilterResolution {
  operator: 'between' | 'gte' | 'lte' | 'isNull' | 'isNotNull' | null;
  start?: string;
  end?: string;
  /** 需要追加一条 `字段 IS NULL` 并用 OR 连接。 */
  includeEmpty: boolean;
}

// ---------------------------------------------------------------------------
// 常量
// ---------------------------------------------------------------------------

export const PRESET_LABELS: Record<DynamicPresetKey, string> = {
  last1d: '最近 1 天',
  last7d: '最近 7 天',
  last14d: '最近 14 天',
  last30d: '最近 30 天',
  last365d: '最近 365 天',
  last1w: '最近 1 周',
  last4w: '最近 4 周',
  last13w: '最近 13 周',
  last52w: '最近 52 周',
  last1m: '最近 1 个月',
  last3m: '最近 3 个月',
  last6m: '最近 6 个月',
  last12m: '最近 12 个月',
  thisWeek: '本周',
  lastWeek: '上周',
  thisMonth: '本月',
  lastMonth: '上月',
  thisBiMonth: '本双月',
  lastBiMonth: '上双月',
  thisQuarter: '本季度',
  lastQuarter: '上季度',
  thisYear: '今年',
  last2y: '最近 2 年',
  lastYear: '去年',
};

/**
 * 快捷选项按粒度分流。日单位与周单位/月单位的选项语义并不互换
 * （月粒度下「最近 1 个月」≠「最近 30 天」），所以不做并集，而是各给一套。
 * `hour` 复用日粒度这一套：小时级的窗口交给「自定义」（最近 N 小时）。
 */
const DAY_PRESETS: readonly DynamicPresetKey[] = [
  'last1d',
  'last7d',
  'last14d',
  'last30d',
  'thisWeek',
  'lastWeek',
  'thisMonth',
  'thisBiMonth',
  'lastBiMonth',
  'thisQuarter',
  'lastQuarter',
  'last3m',
  'last6m',
  'thisYear',
  'last2y',
  'last365d',
];

const PRESETS_BY_GRANULARITY: Record<DateGranularity, readonly DynamicPresetKey[]> = {
  hour: DAY_PRESETS,
  day: DAY_PRESETS,
  week: [
    'last1w',
    'last4w',
    'last13w',
    'last52w',
    'thisWeek',
    'lastWeek',
    'thisMonth',
    'lastMonth',
    'thisQuarter',
    'lastQuarter',
    'thisYear',
    'lastYear',
  ],
  month: [
    'last1m',
    'last3m',
    'last6m',
    'last12m',
    'thisMonth',
    'lastMonth',
    'thisQuarter',
    'lastQuarter',
    'thisYear',
    'lastYear',
  ],
};

/** 周计算逻辑下拉的 7 档（与文档截图一致）。 */
export const WEEK_START_OPTIONS: readonly { value: WeekStart; label: string }[] = [
  { value: 1, label: '周一 ~ 周日' },
  { value: 2, label: '周二 ~ 周一' },
  { value: 3, label: '周三 ~ 周二' },
  { value: 4, label: '周四 ~ 周三' },
  { value: 5, label: '周五 ~ 周四' },
  { value: 6, label: '周六 ~ 周五' },
  { value: 0, label: '周日 ~ 周六' },
];

const DATE_FORMAT = 'YYYY-MM-DD';
const DATETIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

const SPECIAL_LABELS: Record<SpecialDateFilterValue['value'], string> = {
  empty: '空日期',
  notEmpty: '非空日期',
  all: '所有日期',
};

const OP_LABELS: Record<DynamicOp, string> = {
  last: '最近',
  before: '前',
  after: '后',
};

const UNIT_LABELS: Record<DynamicUnit, string> = {
  hour: '小时',
  day: '天',
  week: '周',
  month: '个月',
  year: '年',
};

/** 粒度下拉的展示名（与文档一致）。 */
export const GRANULARITY_OPTIONS: readonly { value: DateGranularity; label: string }[] = [
  { value: 'day', label: '年-月-日' },
  { value: 'month', label: '年-月' },
  { value: 'week', label: '年-周' },
];

export const HOUR_GRANULARITY_OPTION = { value: 'hour' as const, label: '年-月-日 时' };

export const MODE_LABELS: Record<DateFilterMode, string> = {
  dynamic: '动态日期',
  fixed: '固定日期',
  advanced: '高级',
  special: '特殊值',
  single: '单个日期',
};

// ---------------------------------------------------------------------------
// 基础日期运算（不引入 dayjs 插件，避免为一个能力拉一个全局副作用）
// ---------------------------------------------------------------------------

/** 解析 `YYYY-MM-DD[ HH:mm[:ss]]`（也接受 `T` 分隔）。无法解析返回 null。 */
function parseDateValue(input: string | undefined | null): Dayjs | null {
  if (typeof input !== 'string') return null;
  const normalized = input.trim().replace('T', ' ');
  if (normalized === '') return null;
  const matched = /^(\d{4})-(\d{2})-(\d{2})(?:[ ](\d{2}):(\d{2})(?::(\d{2}))?)?$/.exec(normalized);
  if (matched) {
    // 按本地时间构造：这里的日期代表「数据里的那一天的本地零点」，不是 UTC 时刻。
    const value = dayjs(
      new Date(
        Number(matched[1]),
        Number(matched[2]) - 1,
        Number(matched[3]),
        Number(matched[4] ?? 0),
        Number(matched[5] ?? 0),
        Number(matched[6] ?? 0)
      )
    );
    return value.isValid() ? value : null;
  }
  const loose = dayjs(normalized);
  return loose.isValid() ? loose : null;
}

/** 周计算逻辑归一：非法值退回周一。 */
function normalizeWeekStart(weekStart: number | undefined): WeekStart {
  return typeof weekStart === 'number' && weekStart >= 0 && weekStart <= 6
    ? (weekStart as WeekStart)
    : 1;
}

function startOfWeek(value: Dayjs, weekStart: WeekStart): Dayjs {
  const shift = (value.day() - weekStart + 7) % 7;
  return value.subtract(shift, 'day').startOf('day');
}

function endOfWeek(value: Dayjs, weekStart: WeekStart): Dayjs {
  return startOfWeek(value, weekStart).add(6, 'day').endOf('day');
}

/** 双月分组以 1 月为起点两两成组（1~2 月、3~4 月 …），与文档示例一致。 */
function startOfBiMonth(value: Dayjs): Dayjs {
  return value.month(Math.floor(value.month() / 2) * 2).startOf('month');
}

function endOfBiMonth(value: Dayjs): Dayjs {
  return startOfBiMonth(value).add(1, 'month').endOf('month');
}

function startOfQuarter(value: Dayjs): Dayjs {
  return value.month(Math.floor(value.month() / 3) * 3).startOf('month');
}

function endOfQuarter(value: Dayjs): Dayjs {
  return startOfQuarter(value).add(2, 'month').endOf('month');
}

/** 按单位取一个时刻所在周期的起点 / 终点。`week` 需要周计算逻辑。 */
function startOfUnit(value: Dayjs, unit: DynamicUnit, weekStart: WeekStart): Dayjs {
  switch (unit) {
    case 'hour':
      return value.startOf('hour');
    case 'day':
      return value.startOf('day');
    case 'week':
      return startOfWeek(value, weekStart);
    case 'month':
      return value.startOf('month');
    case 'year':
      return value.startOf('year');
  }
}

function endOfUnit(value: Dayjs, unit: DynamicUnit, weekStart: WeekStart): Dayjs {
  switch (unit) {
    case 'hour':
      return value.endOf('hour');
    case 'day':
      return value.endOf('day');
    case 'week':
      return endOfWeek(value, weekStart);
    case 'month':
      return value.endOf('month');
    case 'year':
      return value.endOf('year');
  }
}

interface DateRange {
  start: Dayjs;
  end: Dayjs;
}

/** 快捷选项 → 具体区间。全部以「昨天」为数据上界。 */
function resolvePreset(preset: DynamicPresetKey, now: Dayjs, weekStart: WeekStart): DateRange {
  const yesterdayEnd = now.subtract(1, 'day').endOf('day');
  const yesterdayStart = now.subtract(1, 'day').startOf('day');
  const monthsAgoStart = (months: number) => now.startOf('month').subtract(months, 'month');

  switch (preset) {
    case 'last1d':
      return { start: yesterdayStart, end: yesterdayEnd };
    case 'last7d':
      return { start: now.subtract(7, 'day').startOf('day'), end: yesterdayEnd };
    case 'last14d':
      return { start: now.subtract(14, 'day').startOf('day'), end: yesterdayEnd };
    case 'last30d':
      return { start: now.subtract(30, 'day').startOf('day'), end: yesterdayEnd };
    case 'last365d':
      return { start: now.subtract(365, 'day').startOf('day'), end: yesterdayEnd };

    case 'last1w':
      return { start: startOfWeek(now, weekStart), end: yesterdayEnd };
    case 'last4w':
      return { start: startOfWeek(now, weekStart).subtract(3, 'week'), end: yesterdayEnd };
    case 'last13w':
      return { start: startOfWeek(now, weekStart).subtract(12, 'week'), end: yesterdayEnd };
    case 'last52w':
      return { start: startOfWeek(now, weekStart).subtract(51, 'week'), end: yesterdayEnd };

    case 'last1m':
      return { start: monthsAgoStart(0), end: yesterdayEnd };
    case 'last3m':
      return { start: monthsAgoStart(2), end: yesterdayEnd };
    case 'last6m':
      return { start: monthsAgoStart(5), end: yesterdayEnd };
    case 'last12m':
      return { start: monthsAgoStart(11), end: yesterdayEnd };

    case 'thisWeek':
      return { start: startOfWeek(now, weekStart), end: yesterdayEnd };
    case 'lastWeek': {
      const lastWeekDay = now.subtract(7, 'day');
      return { start: startOfWeek(lastWeekDay, weekStart), end: endOfWeek(lastWeekDay, weekStart) };
    }

    case 'thisMonth':
      return { start: now.startOf('month'), end: yesterdayEnd };
    case 'lastMonth': {
      const lastMonth = now.subtract(1, 'month');
      return { start: lastMonth.startOf('month'), end: lastMonth.endOf('month') };
    }

    case 'thisBiMonth':
      return { start: startOfBiMonth(now), end: yesterdayEnd };
    case 'lastBiMonth': {
      const previous = now.subtract(2, 'month');
      return { start: startOfBiMonth(previous), end: endOfBiMonth(previous) };
    }

    case 'thisQuarter':
      return { start: startOfQuarter(now), end: yesterdayEnd };
    case 'lastQuarter': {
      const previous = now.subtract(3, 'month');
      return { start: startOfQuarter(previous), end: endOfQuarter(previous) };
    }

    case 'thisYear':
      return { start: now.startOf('year'), end: yesterdayEnd };
    case 'last2y':
      return { start: now.subtract(1, 'year').startOf('year'), end: yesterdayEnd };
    case 'lastYear': {
      const lastYear = now.subtract(1, 'year');
      return { start: lastYear.startOf('year'), end: lastYear.endOf('year') };
    }
  }
}

/** 自定义动态日期 → 区间（或单侧边界）。 */
function resolveCustom(
  custom: DynamicCustomValue,
  now: Dayjs,
  weekStart: WeekStart
): { range?: DateRange; upperOnly?: Dayjs; lowerOnly?: Dayjs } {
  const n = Number.isFinite(custom.n) ? Math.max(0, Math.trunc(custom.n)) : 0;
  const { op, unit } = custom;

  if (unit === 'hour') {
    // 整点开关默认「对齐」；显式 false 时退化为相对当前时刻的滚动窗口。
    const aligned = custom.onTheHour !== false;
    if (op !== 'last') {
      const anchor = aligned ? startOfUnit(now, unit, weekStart) : now;
      const shifted = op === 'before' ? anchor.subtract(n, 'hour') : anchor.add(n, 'hour');
      return op === 'before'
        ? { upperOnly: aligned ? shifted.endOf('hour') : shifted }
        : { lowerOnly: shifted };
    }
    if (!aligned) {
      return { range: { start: now.subtract(n, 'hour'), end: now } };
    }
    const end = custom.includeCurrentHour
      ? now.endOf('hour')
      : now.subtract(1, 'hour').endOf('hour');
    return { range: { start: now.subtract(n, 'hour').startOf('hour'), end } };
  }

  if (op === 'last') {
    const start = startOfUnit(now, unit, weekStart).subtract(
      unit === 'week' || unit === 'month' || unit === 'year' ? n - 1 : n,
      unit
    );
    const end = custom.includeToday ? now.endOf('day') : now.subtract(1, 'day').endOf('day');
    return { range: { start, end } };
  }

  const anchor = startOfUnit(now, unit, weekStart);
  if (op === 'before') {
    return { upperOnly: endOfUnit(anchor.subtract(n, unit), unit, weekStart) };
  }
  return { lowerOnly: startOfUnit(anchor.add(n, unit), unit, weekStart) };
}

// ---------------------------------------------------------------------------
// 解析
// ---------------------------------------------------------------------------

/** 输出格式：小时粒度或字段带时分秒 → 到秒；否则到日。 */
function usesTime(settings: DateFilterSettings): boolean {
  return settings.granularity === 'hour' || settings.withTime === true;
}

function formatMoment(value: Dayjs, settings: DateFilterSettings): string {
  return value.format(usesTime(settings) ? DATETIME_FORMAT : DATE_FORMAT);
}

/** 单一日期收敛到该粒度的完整边界（月粒度选一天 = 覆盖整月）。 */
function snapSingleDate(date: Dayjs, settings: DateFilterSettings): DateRange {
  const weekStart = normalizeWeekStart(settings.weekStart);
  switch (settings.granularity) {
    case 'month':
      return { start: date.startOf('month'), end: date.endOf('month') };
    case 'week':
      return { start: startOfWeek(date, weekStart), end: endOfWeek(date, weekStart) };
    case 'hour':
      return { start: date.startOf('hour'), end: date.endOf('hour') };
    default:
      return { start: date.startOf('day'), end: date.endOf('day') };
  }
}

/** 高级模式单侧端点 → 时刻；`unlimited` 返回 null 表示该侧不设界。 */
function resolveBound(
  bound: DateBound,
  role: 'start' | 'end',
  now: Dayjs,
  weekStart: WeekStart
): Dayjs | null {
  if (bound.type === 'unlimited') return null;
  if (bound.type === 'fixed') return parseDateValue(bound.value);
  const n = Number.isFinite(bound.n) ? Math.max(0, Math.trunc(bound.n)) : 0;
  const anchor = startOfUnit(now, bound.unit, weekStart);
  const shifted =
    bound.op === 'before' ? anchor.subtract(n, bound.unit) : anchor.add(n, bound.unit);
  return role === 'start'
    ? startOfUnit(shifted, bound.unit, weekStart)
    : endOfUnit(shifted, bound.unit, weekStart);
}

/**
 * 把日期筛选值解析成下发的过滤语义。`now` 可注入，便于测试与「同一时刻多块复用」。
 *
 * 返回 `{operator: null}` 的两种情况（都表示「不产生过滤条件」）：
 * - 未选择（动态模式既无 preset 也无 custom）
 * - 特殊值「所有日期」、起止同时「无限制」
 * 畸形输入同样收敛到 null：宁可不过滤，也不凭空造一个区间出来。
 */
export function resolveDateFilter(
  value: DateFilterValue,
  settings: DateFilterSettings,
  now: Dayjs = dayjs()
): DateFilterResolution {
  const weekStart = normalizeWeekStart(settings.weekStart);
  const none: DateFilterResolution = { operator: null, includeEmpty: false };

  switch (value.kind) {
    case 'dynamic': {
      const includeEmpty = value.includeEmpty === true;
      if (value.custom) {
        const custom = resolveCustom(value.custom, now, weekStart);
        if (custom.range) {
          return {
            operator: 'between',
            start: formatMoment(custom.range.start, settings),
            end: formatMoment(custom.range.end, settings),
            includeEmpty,
          };
        }
        if (custom.upperOnly) {
          return { operator: 'lte', end: formatMoment(custom.upperOnly, settings), includeEmpty };
        }
        if (custom.lowerOnly) {
          return { operator: 'gte', start: formatMoment(custom.lowerOnly, settings), includeEmpty };
        }
        return none;
      }
      if (!value.preset) return none;
      const range = resolvePreset(value.preset, now, weekStart);
      return {
        operator: 'between',
        start: formatMoment(range.start, settings),
        end: formatMoment(range.end, settings),
        includeEmpty,
      };
    }

    case 'fixed': {
      const start = parseDateValue(value.start);
      const end = parseDateValue(value.end);
      if (!start || !end) return none;
      return {
        operator: 'between',
        start: formatMoment(start, settings),
        end: formatMoment(end, settings),
        includeEmpty: value.includeEmpty === true,
      };
    }

    case 'advanced': {
      const start = resolveBound(value.start, 'start', now, weekStart);
      const end = resolveBound(value.end, 'end', now, weekStart);
      const includeEmpty = value.includeEmpty === true;
      if (start && end) {
        return {
          operator: 'between',
          start: formatMoment(start, settings),
          end: formatMoment(end, settings),
          includeEmpty,
        };
      }
      if (start) return { operator: 'gte', start: formatMoment(start, settings), includeEmpty };
      if (end) return { operator: 'lte', end: formatMoment(end, settings), includeEmpty };
      return none;
    }

    case 'special':
      if (value.value === 'empty') return { operator: 'isNull', includeEmpty: false };
      if (value.value === 'notEmpty') return { operator: 'isNotNull', includeEmpty: false };
      return none;

    case 'single': {
      const date = parseDateValue(value.date);
      if (!date) return none;
      const range = snapSingleDate(date, settings);
      return {
        operator: 'between',
        start: formatMoment(range.start, settings),
        end: formatMoment(range.end, settings),
        includeEmpty: false,
      };
    }
  }
}

/**
 * 日期筛选意图 —— 随筛选条件一起持久化的「用户想怎么筛」。
 *
 * 关键区别：这里的 `value` 是意图（`最近 7 天`），不是快照（`2026-09-18 ~ 2026-09-24`）。
 * **两端都会解析它**：图表查询页在每次下发请求时现算；后端读 config 的路径
 * （分享页 / 仪表盘）由 `internal/query/datefilter.go` 现算。所以动态日期在所有路径上都是动态的。
 */
export interface DateFilterIntent {
  value: DateFilterValue;
  granularity: DateGranularity;
  weekStart: WeekStart;
  /** 勾选了「作为时间范围筛选器」→ 页面上额外出一个行内控件。 */
  asFilter: boolean;
  /** 行内控件的显示名称。 */
  label: string;
  /** 字段带时分秒；固化下来是为了让快照解析与页面解析取同一口径。 */
  withTime?: boolean;
}

/** 一条下发给 `POST /api/charts/query` 的过滤项（与 api 层的 ChartQueryFilter 同形）。 */
export interface WireFilterCondition {
  field: string;
  operator:
    | 'eq'
    | 'neq'
    | 'gt'
    | 'gte'
    | 'lt'
    | 'lte'
    | 'like'
    | 'in'
    | 'notIn'
    | 'between'
    | 'isNull'
    | 'isNotNull';
  value: unknown;
  value_end?: unknown;
  logic: 'and' | 'or';
}

/** 带（可选）日期意图的筛选条件；`date` 存在时它才是权威，operator/value 只是守旧的占位。 */
export interface DateFilterableCondition {
  operator: string;
  value: unknown;
  valueEnd?: unknown;
  logic: 'and' | 'or';
  date?: DateFilterIntent;
}

/**
 * 把日期意图展开成一条或多条下发条件。
 *
 * 展开规则：
 * - 区间 → `between`；只有单侧（高级模式的「无限制」）→ `gte` / `lte`；
 * - 特殊值 → `isNull` / `isNotNull`；「所有日期」→ 不产生任何条件；
 * - 「包含空日期」→ 追加一条 `IS NULL` 并用 **OR** 连接（与前面的区间构成
 *   `区间 OR IS NULL`，正是文档里「休息日/空值也放行」的语义）。
 */
export function expandDateFilterIntent(
  intent: DateFilterIntent,
  field: string,
  now: Dayjs = dayjs()
): WireFilterCondition[] {
  const settings: DateFilterSettings = {
    granularity: intent.granularity,
    weekStart: intent.weekStart,
    withTime: intent.withTime,
  };
  const resolved = resolveDateFilter(intent.value, settings, now);
  if (resolved.operator === null) return [];

  const condition: WireFilterCondition =
    resolved.operator === 'between'
      ? {
          field,
          operator: 'between',
          value: resolved.start,
          value_end: resolved.end,
          logic: 'and',
        }
      : resolved.operator === 'gte' || resolved.operator === 'lte'
        ? {
            field,
            operator: resolved.operator,
            value: resolved.start ?? resolved.end,
            logic: 'and',
          }
        : { field, operator: resolved.operator, value: '', logic: 'and' };

  if (!resolved.includeEmpty) return [condition];
  return [condition, { field, operator: 'isNull', value: '', logic: 'or' }];
}

/**
 * 保存路径的物化：把日期意图就地换成**具体区间快照**。
 *
 * 现在两端都会解析意图（前端 `expandDateFilterIntent`、后端 `internal/query/datefilter.go`，
 * 后者是权威路径），所以这份快照**不再是取数依据**，退化成两件事：
 *   1. 「不认识 `date` 的下游」的兜底值（例如手工查库、或将来新增的消费方）；
 *   2. 让人直接读 `config` 时能看懂这条条件大致落在哪段时间。
 * 取数时后端只要看到 `date` 就用它现算，看不到才退回这里写下的 operator/value。
 *
 * 返回数组：日期意图可能一条都产生不出来（`所有日期`、未选择），此时返回空数组，
 * 由调用方丢弃这条条件 —— 它本来就等价于「没有这条筛选」，而不是等价于某个能写进 SQL 的谓词。
 *
 * ⚠️ 快照仍是有损的：一条 FilterCondition 只能表达一个 SQL 条件，所以「包含空日期」在
 * 快照里丢失（取数不受影响——那条语义由意图现算时展开成两条条件）。
 */
export function materializeDateFilterSnapshot<T extends DateFilterableCondition>(
  condition: T,
  now: Dayjs = dayjs()
): T[] {
  if (!condition.date) return [condition];
  // 主条件恒为第一条：`包含空日期` 追加的那条 IS NULL 排在其后。别用「跳过 isNull」来找主条件
  //——特殊值「空日期」的主条件本身就是 isNull。
  const primary = expandDateFilterIntent(condition.date, '', now)[0];
  if (!primary) return [];
  return [
    { ...condition, operator: primary.operator, value: primary.value, valueEnd: primary.value_end },
  ];
}

/**
 * 把「旧式日期条件」（操作符 + 两个日期串）识别回日期筛选值。
 * 用于让历史图表里的日期条件也能进新的日期筛选弹窗；表达不了时返回 null（仍走通用弹窗）。
 */
export function dateFilterValueFromLegacy(
  operator: string,
  value: unknown,
  valueEnd: unknown
): DateFilterValue | null {
  const asText = (input: unknown) => (typeof input === 'string' ? input.trim() : '');
  if (operator === 'between') {
    const start = asText(value);
    const end = asText(valueEnd);
    return start !== '' && end !== '' ? { kind: 'fixed', start, end } : null;
  }
  if (operator === 'isNull') return { kind: 'special', value: 'empty' };
  if (operator === 'isNotNull') return { kind: 'special', value: 'notEmpty' };
  return null;
}

// ---------------------------------------------------------------------------
// 展示
// ---------------------------------------------------------------------------

export function presetsForGranularity(granularity: DateGranularity): readonly DynamicPresetKey[] {
  return PRESETS_BY_GRANULARITY[granularity] ?? PRESETS_BY_GRANULARITY.day;
}

/** 是否已做出选择（未选择时「确定」应禁用）。 */
export function hasDateFilterSelection(value: DateFilterValue): boolean {
  if (value.kind !== 'dynamic') return true;
  return value.preset !== undefined || value.custom !== undefined;
}

/** 单侧端点是否填好；`unlimited` 天然算填好。 */
function isBoundComplete(bound: DateBound): boolean {
  return bound.type !== 'fixed' || bound.value.trim() !== '';
}

/**
 * 配置是否完整 —— 决定弹窗「确定」与行内控件「应用」的可用性。
 * 与 `resolveDateFilter` 的分工：这里管「能不能提交」，那里管「提交后下发什么」。
 */
export function isDateFilterComplete(value: DateFilterValue): boolean {
  switch (value.kind) {
    case 'dynamic':
      return hasDateFilterSelection(value);
    case 'fixed':
      return value.start.trim() !== '' && value.end.trim() !== '';
    case 'advanced':
      // 文档明确禁止起止同时「无限制」——那等于不过滤，属于配置错误而非有效输入。
      if (value.start.type === 'unlimited' && value.end.type === 'unlimited') return false;
      return isBoundComplete(value.start) && isBoundComplete(value.end);
    case 'special':
      return true;
    case 'single':
      return value.date.trim() !== '';
  }
}

/** 粒度守卫：布局 JSON 是手改得动的，读取时一律过守卫而不是直接断言。 */
export function isDateGranularity(input: unknown): input is DateGranularity {
  return input === 'hour' || input === 'day' || input === 'week' || input === 'month';
}

/** 周计算逻辑守卫（0=周日 … 6=周六）。 */
export function isWeekStart(input: unknown): input is WeekStart {
  return typeof input === 'number' && Number.isInteger(input) && input >= 0 && input <= 6;
}

export function isDateFilterValue(input: unknown): input is DateFilterValue {
  if (typeof input !== 'object' || input === null || Array.isArray(input)) return false;
  const candidate = input as Record<string, unknown>;
  switch (candidate.kind) {
    case 'dynamic': {
      const preset = candidate.preset;
      if (preset !== undefined && !isPresetKey(preset)) return false;
      const custom = candidate.custom;
      if (custom === undefined) return true;
      if (typeof custom !== 'object' || custom === null) return false;
      const c = custom as Record<string, unknown>;
      return (
        (c.op === 'last' || c.op === 'before' || c.op === 'after') &&
        typeof c.n === 'number' &&
        (c.unit === 'hour' ||
          c.unit === 'day' ||
          c.unit === 'week' ||
          c.unit === 'month' ||
          c.unit === 'year')
      );
    }
    case 'fixed':
      return isNonEmptyString(candidate.start) && isNonEmptyString(candidate.end);
    case 'advanced':
      return isBound(candidate.start) && isBound(candidate.end);
    case 'special':
      return (
        candidate.value === 'empty' || candidate.value === 'notEmpty' || candidate.value === 'all'
      );
    case 'single':
      return isNonEmptyString(candidate.date);
    default:
      return false;
  }
}

function isNonEmptyString(input: unknown): input is string {
  return typeof input === 'string' && input.trim() !== '';
}

/** 快捷选项键集合：用 Set 判存在，避免 `Object.hasOwn`（ES2022）超出 lib 版本。 */
const PRESET_KEYS: ReadonlySet<string> = new Set(Object.keys(PRESET_LABELS));

function isPresetKey(input: unknown): input is DynamicPresetKey {
  return typeof input === 'string' && PRESET_KEYS.has(input);
}

function isBound(input: unknown): input is DateBound {
  if (typeof input !== 'object' || input === null) return false;
  const bound = input as Record<string, unknown>;
  if (bound.type === 'unlimited') return true;
  if (bound.type === 'fixed') return isNonEmptyString(bound.value);
  return (
    bound.type === 'dynamic' &&
    (bound.op === 'before' || bound.op === 'after') &&
    typeof bound.n === 'number' &&
    typeof bound.unit === 'string'
  );
}

/** 未选择态：动态日期模式、既无快捷选项也无自定义。 */
export function defaultDateFilterValue(): DateFilterValue {
  return { kind: 'dynamic' };
}

/** 自定义动态日期的摘要文案，如 `最近 1 天`、`前 3 周`。 */
function describeCustom(custom: DynamicCustomValue): string {
  const { op, n, unit } = custom;
  const suffix = custom.includeToday && op === 'last' ? '（含今天）' : '';
  return `${OP_LABELS[op]} ${n} ${UNIT_LABELS[unit]}${suffix}`;
}

function describeBound(
  bound: DateBound,
  role: 'start' | 'end',
  settings: DateFilterSettings,
  now: Dayjs
): string {
  if (bound.type === 'unlimited') return '无限制';
  if (bound.type === 'fixed') {
    const parsed = parseDateValue(bound.value);
    return parsed ? formatMoment(parsed, settings) : bound.value;
  }
  const resolved = resolveBound(bound, role, now, normalizeWeekStart(settings.weekStart));
  return resolved
    ? formatMoment(resolved, settings)
    : `${OP_LABELS[bound.op]} ${bound.n} ${UNIT_LABELS[bound.unit]}`;
}

/** 芯片 / 筛选器控件上的一行摘要（求短）。 */
export function formatDateFilterSummary(
  value: DateFilterValue,
  settings: DateFilterSettings = { granularity: 'day' }
): string {
  switch (value.kind) {
    case 'dynamic':
      if (value.preset) return PRESET_LABELS[value.preset];
      if (value.custom) return describeCustom(value.custom);
      return '请选择';
    case 'fixed': {
      const start = parseDateValue(value.start);
      const end = parseDateValue(value.end);
      if (!start || !end) return '请选择';
      return `${formatMoment(start, settings)} ~ ${formatMoment(end, settings)}`;
    }
    case 'advanced': {
      const now = dayjs();
      return `${describeBound(value.start, 'start', settings, now)} ~ ${describeBound(value.end, 'end', settings, now)}`;
    }
    case 'special':
      return SPECIAL_LABELS[value.value];
    case 'single': {
      const date = parseDateValue(value.date);
      return date ? formatMoment(date, settings) : '请选择';
    }
  }
}

/** 月粒度展示成 `YYYY-MM`。 */
function formatForGranularity(value: Dayjs, settings: DateFilterSettings): string {
  if (settings.granularity === 'month') return value.format('YYYY-MM');
  return formatMoment(value, settings);
}

/**
 * 周序号（以配置的周起始日为准，自本年首周起数）。
 * 刻意不是 ISO 周：用户改了「周计算逻辑」后序号必须跟着变，否则预览会对不上日历。
 */
function weekOrdinal(value: Dayjs, weekStart: WeekStart): { year: number; week: number } {
  const firstWeekStart = startOfWeek(value.startOf('year'), weekStart);
  const days = value.startOf('day').diff(firstWeekStart, 'day');
  return { year: value.year(), week: Math.floor(days / 7) + 1 };
}

function formatWeekToken(value: Dayjs, weekStart: WeekStart): string {
  const { year, week } = weekOrdinal(value, weekStart);
  return `${year}-${String(week).padStart(2, '0')}周(${value.format('MMDD')}-${value.add(6, 'day').format('MMDD')})`;
}

/**
 * 「时间预览」里的具体区间。
 * 粒度决定形态：日 → `2026-09-18 ~ 2026-09-24`；月 → `2026-07 ~ 2026-09`；
 * 周 → `2026-38周(0914-0920)`，跨周时给两段。
 */
export function formatDateFilterPreview(
  value: DateFilterValue,
  settings: DateFilterSettings,
  now: Dayjs = dayjs()
): string {
  if (!hasDateFilterSelection(value)) return '请选择';

  if (value.kind === 'special') return SPECIAL_LABELS[value.value];
  if (value.kind === 'advanced') {
    return `${describeBound(value.start, 'start', settings, now)} ~ ${describeBound(value.end, 'end', settings, now)}`;
  }

  const resolved = resolveDateFilter(value, settings, now);
  if (!resolved.start) return resolved.end ?? '请选择';
  if (!resolved.end) return `${resolved.start} ~ 无限制`;

  const start = parseDateValue(resolved.start);
  const end = parseDateValue(resolved.end);
  if (!start || !end) return `${resolved.start} ~ ${resolved.end}`;

  if (settings.granularity === 'week') {
    const weekStart = normalizeWeekStart(settings.weekStart);
    const sameWeek = startOfWeek(start, weekStart).isSame(startOfWeek(end, weekStart), 'day');
    return sameWeek
      ? formatWeekToken(start, weekStart)
      : `${formatWeekToken(start, weekStart)} ~ ${formatWeekToken(end, weekStart)}`;
  }

  return `${formatForGranularity(start, settings)} ~ ${formatForGranularity(end, settings)}`;
}
