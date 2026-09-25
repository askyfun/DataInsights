import dayjs, { type Dayjs } from 'dayjs';
import { describe, expect, it } from 'vitest';
import {
  type DateFilterIntent,
  type DateFilterSettings,
  type DateFilterValue,
  dateFilterValueFromLegacy,
  defaultDateFilterValue,
  expandDateFilterIntent,
  formatDateFilterPreview,
  formatDateFilterSummary,
  hasDateFilterSelection,
  isDateFilterComplete,
  isDateFilterValue,
  materializeDateFilterSnapshot,
  PRESET_LABELS,
  presetsForGranularity,
  resolveDateFilter,
} from '../../lib/dateFilter';

/**
 * 日期筛选器核心语义（对齐火山引擎智能数据洞察「日期筛选」文档）。
 *
 * 所有用例注入固定 now，避免依赖运行时钟：
 *   now = 2026-09-25（周五），本周一 = 2026-09-21，本周日 = 2026-09-20。
 * 文档口径统一以「昨天」为数据上界（`本周=本周一~昨天`、`本月=本月 1 日~昨天`），
 * 因此绝大多数区间的 end 都是 2026-09-24。
 */

const NOW: Dayjs = dayjs('2026-09-25 14:30:45');

const DAY: DateFilterSettings = { granularity: 'day' };
const MONTH: DateFilterSettings = { granularity: 'month' };
const WEEK: DateFilterSettings = { granularity: 'week' };

/** 解析出 `start ~ end` 的速记，断言时只看这段。 */
const range = (value: DateFilterValue, settings: DateFilterSettings = DAY, now: Dayjs = NOW) => {
  const resolved = resolveDateFilter(value, settings, now);
  return `${resolved.start} ~ ${resolved.end}`;
};

describe('presetsForGranularity', () => {
  it('按粒度给三套不同的快捷选项（日 / 周 / 月）', () => {
    const day = presetsForGranularity('day');
    const week = presetsForGranularity('week');
    const month = presetsForGranularity('month');

    expect(day).toContain('last7d');
    expect(day).toContain('last365d');
    expect(day).not.toContain('last4w');
    expect(day).not.toContain('last12m');

    expect(week).toContain('last4w');
    expect(week).toContain('last52w');
    expect(week).not.toContain('last7d');

    expect(month).toContain('last12m');
    expect(month).not.toContain('last7d');
    expect(month).not.toContain('last4w');
  });

  it('三套都含跨越粒度的公共项，且周期专属项只出现在对应粒度', () => {
    for (const granularity of ['day', 'week', 'month'] as const) {
      const presets = presetsForGranularity(granularity);
      expect(presets).toContain('thisMonth');
      expect(presets).toContain('thisQuarter');
      expect(presets).toContain('thisYear');
    }
    // 「去年」只在周/月粒度出现：日粒度用「最近 2 年」表达同义区间（对齐文档的日粒度选项集）
    expect(presetsForGranularity('day')).not.toContain('lastYear');
    expect(presetsForGranularity('day')).toContain('last2y');
    expect(presetsForGranularity('week')).toContain('lastYear');
    expect(presetsForGranularity('month')).toContain('lastYear');
  });

  it('每个快捷选项都有中文标签', () => {
    for (const granularity of ['day', 'week', 'month'] as const) {
      for (const key of presetsForGranularity(granularity)) {
        expect(PRESET_LABELS[key], key).toBeTruthy();
      }
    }
  });
});

describe('动态日期：快捷选项（文档口径，now=2026-09-25）', () => {
  const dynamic = (preset: string): DateFilterValue => ({
    kind: 'dynamic',
    preset: preset as never,
  });

  it('最近 N 天 = 昨天-(N-1) ~ 昨天', () => {
    expect(range(dynamic('last1d'))).toBe('2026-09-24 ~ 2026-09-24');
    expect(range(dynamic('last7d'))).toBe('2026-09-18 ~ 2026-09-24');
    expect(range(dynamic('last14d'))).toBe('2026-09-11 ~ 2026-09-24');
    expect(range(dynamic('last30d'))).toBe('2026-08-26 ~ 2026-09-24');
    expect(range(dynamic('last365d'))).toBe('2025-09-25 ~ 2026-09-24');
  });

  it('本周 / 上周（本周止于昨天）', () => {
    expect(range(dynamic('thisWeek'))).toBe('2026-09-21 ~ 2026-09-24');
    expect(range(dynamic('lastWeek'))).toBe('2026-09-14 ~ 2026-09-20');
  });

  it('月 / 双月 / 季度 / 年（本 X 止于昨天，上 X 取完整区间）', () => {
    expect(range(dynamic('thisMonth'))).toBe('2026-09-01 ~ 2026-09-24');
    expect(range(dynamic('lastMonth'))).toBe('2026-08-01 ~ 2026-08-31');
    // 双月以 1 月为起点两两分组：9~10 月 / 7~8 月
    expect(range(dynamic('thisBiMonth'))).toBe('2026-09-01 ~ 2026-09-24');
    expect(range(dynamic('lastBiMonth'))).toBe('2026-07-01 ~ 2026-08-31');
    // 季度：Q3 = 7~9 月
    expect(range(dynamic('thisQuarter'))).toBe('2026-07-01 ~ 2026-09-24');
    expect(range(dynamic('lastQuarter'))).toBe('2026-04-01 ~ 2026-06-30');
    expect(range(dynamic('thisYear'))).toBe('2026-01-01 ~ 2026-09-24');
    expect(range(dynamic('lastYear'))).toBe('2025-01-01 ~ 2025-12-31');
  });

  it('最近 N 月 / 最近 N 年（自本月/本年回溯 N-1 个周期的首日 ~ 昨天）', () => {
    expect(range(dynamic('last3m'))).toBe('2026-07-01 ~ 2026-09-24');
    expect(range(dynamic('last6m'))).toBe('2026-04-01 ~ 2026-09-24');
    expect(range(dynamic('last2y'))).toBe('2025-01-01 ~ 2026-09-24');
  });

  it('周粒度专属：最近 N 周', () => {
    expect(range(dynamic('last1w'), WEEK)).toBe('2026-09-21 ~ 2026-09-24');
    expect(range(dynamic('last4w'), WEEK)).toBe('2026-08-31 ~ 2026-09-24');
    expect(range(dynamic('last13w'), WEEK)).toBe('2026-06-29 ~ 2026-09-24');
  });

  it('月粒度专属：最近 N 个月', () => {
    expect(range(dynamic('last1m'), MONTH)).toBe('2026-09-01 ~ 2026-09-24');
    expect(range(dynamic('last12m'), MONTH)).toBe('2025-10-01 ~ 2026-09-24');
  });
});

describe('周计算逻辑（weekStart）', () => {
  const thisWeek: DateFilterValue = { kind: 'dynamic', preset: 'thisWeek' };

  it('默认周一起算，可切换为周日起算', () => {
    expect(range(thisWeek, { granularity: 'day' })).toBe('2026-09-21 ~ 2026-09-24');
    expect(range(thisWeek, { granularity: 'day', weekStart: 0 })).toBe('2026-09-20 ~ 2026-09-24');
    expect(range(thisWeek, { granularity: 'day', weekStart: 6 })).toBe('2026-09-19 ~ 2026-09-24');
  });

  it('上周整体随之平移', () => {
    const lastWeek: DateFilterValue = { kind: 'dynamic', preset: 'lastWeek' };
    expect(range(lastWeek, { granularity: 'day', weekStart: 0 })).toBe('2026-09-13 ~ 2026-09-19');
  });
});

describe('动态日期：自定义（最近 / 前 / 后）', () => {
  it('最近 N 天默认不含今天', () => {
    expect(range({ kind: 'dynamic', custom: { op: 'last', n: 3, unit: 'day' } })).toBe(
      '2026-09-22 ~ 2026-09-24'
    );
  });

  it('勾选「包含今天」后区间延到今天', () => {
    expect(
      range({ kind: 'dynamic', custom: { op: 'last', n: 3, unit: 'day', includeToday: true } })
    ).toBe('2026-09-22 ~ 2026-09-25');
  });

  it('最近 N 周 / N 月 / N 年按自然周期对齐', () => {
    expect(range({ kind: 'dynamic', custom: { op: 'last', n: 2, unit: 'week' } })).toBe(
      '2026-09-14 ~ 2026-09-24'
    );
    expect(range({ kind: 'dynamic', custom: { op: 'last', n: 2, unit: 'month' } })).toBe(
      '2026-08-01 ~ 2026-09-24'
    );
    expect(range({ kind: 'dynamic', custom: { op: 'last', n: 2, unit: 'year' } })).toBe(
      '2025-01-01 ~ 2026-09-24'
    );
  });

  it('「前 N 天」= 上界为 N 天前，下界无限；「后 N 天」= 下界为 N 天后，上界无限', () => {
    expect(
      resolveDateFilter({ kind: 'dynamic', custom: { op: 'before', n: 1, unit: 'day' } }, DAY, NOW)
    ).toEqual({ operator: 'lte', end: '2026-09-24', includeEmpty: false });
    expect(
      resolveDateFilter({ kind: 'dynamic', custom: { op: 'after', n: 1, unit: 'day' } }, DAY, NOW)
    ).toEqual({ operator: 'gte', start: '2026-09-26', includeEmpty: false });
  });
});

describe('小时粒度与整点 / 包含本小时（datetime 字段）', () => {
  const HOUR: DateFilterSettings = { granularity: 'hour', withTime: true };

  it('最近 N 小时默认对齐整点、止于上一整点末（不含本小时）', () => {
    expect(
      resolveDateFilter({ kind: 'dynamic', custom: { op: 'last', n: 1, unit: 'hour' } }, HOUR, NOW)
    ).toEqual({
      operator: 'between',
      start: '2026-09-25 13:00:00',
      end: '2026-09-25 13:59:59',
      includeEmpty: false,
    });
  });

  it('勾选「包含本小时」后结束延到本小时末', () => {
    expect(
      resolveDateFilter(
        { kind: 'dynamic', custom: { op: 'last', n: 1, unit: 'hour', includeCurrentHour: true } },
        HOUR,
        NOW
      )
    ).toEqual({
      operator: 'between',
      start: '2026-09-25 13:00:00',
      end: '2026-09-25 14:59:59',
      includeEmpty: false,
    });
  });

  it('显式关闭「整点」时退化为滚动窗口（保留分秒）', () => {
    expect(
      resolveDateFilter({ kind: 'dynamic', custom: { op: 'last', n: 1, unit: 'hour' } }, HOUR, NOW)
        .start
    ).toBe('2026-09-25 13:00:00');
    // 整点开关只影响是否对齐到小时边界；显式关闭时保留原始时刻
    const rolling = resolveDateFilter(
      { kind: 'dynamic', custom: { op: 'last', n: 1, unit: 'hour', onTheHour: false } },
      { granularity: 'hour', withTime: true },
      NOW
    );
    expect(rolling.start).toBe('2026-09-25 13:30:45');
  });
});

describe('固定日期', () => {
  it('区间原样解析', () => {
    expect(range({ kind: 'fixed', start: '2026-08-08', end: '2026-08-12' })).toBe(
      '2026-08-08 ~ 2026-08-12'
    );
  });

  it('带时分秒的字段保留时间部分', () => {
    expect(
      range(
        { kind: 'fixed', start: '2026-08-08 09:00:00', end: '2026-08-12 18:30:00' },
        { granularity: 'day', withTime: true }
      )
    ).toBe('2026-08-08 09:00:00 ~ 2026-08-12 18:30:00');
  });
});

describe('高级：起止各取固定 / 动态 / 无限制（文档示例）', () => {
  it('开始固定 + 结束「1 天前」= 2020-01-04 ~ 2020-02-12（文档原例）', () => {
    const now = dayjs('2020-02-13');
    expect(
      resolveDateFilter(
        {
          kind: 'advanced',
          start: { type: 'fixed', value: '2020-01-04' },
          end: { type: 'dynamic', op: 'before', n: 1, unit: 'day' },
        },
        DAY,
        now
      )
    ).toEqual({
      operator: 'between',
      start: '2020-01-04',
      end: '2020-02-12',
      includeEmpty: false,
    });
  });

  it('结束无限制 → 只下发下界（gte）', () => {
    expect(
      resolveDateFilter(
        {
          kind: 'advanced',
          start: { type: 'fixed', value: '2023-06-14' },
          end: { type: 'unlimited' },
        },
        DAY,
        NOW
      )
    ).toEqual({ operator: 'gte', start: '2023-06-14', includeEmpty: false });
  });

  it('开始无限制 → 只下发上界（lte）', () => {
    expect(
      resolveDateFilter(
        {
          kind: 'advanced',
          start: { type: 'unlimited' },
          end: { type: 'fixed', value: '2023-06-14' },
        },
        DAY,
        NOW
      )
    ).toEqual({ operator: 'lte', end: '2023-06-14', includeEmpty: false });
  });

  it('起止同时无限制视为非法 → 不收窄任何范围（operator=null）', () => {
    expect(
      resolveDateFilter(
        { kind: 'advanced', start: { type: 'unlimited' }, end: { type: 'unlimited' } },
        DAY,
        NOW
      )
    ).toEqual({ operator: null, includeEmpty: false });
  });

  it('两端都用动态', () => {
    expect(
      range({
        kind: 'advanced',
        start: { type: 'dynamic', op: 'before', n: 30, unit: 'day' },
        end: { type: 'dynamic', op: 'before', n: 1, unit: 'day' },
      })
    ).toBe('2026-08-26 ~ 2026-09-24');
  });
});

describe('特殊值与单个日期', () => {
  it('空日期 / 非空日期 / 所有日期', () => {
    expect(resolveDateFilter({ kind: 'special', value: 'empty' }, DAY, NOW)).toEqual({
      operator: 'isNull',
      includeEmpty: false,
    });
    expect(resolveDateFilter({ kind: 'special', value: 'notEmpty' }, DAY, NOW)).toEqual({
      operator: 'isNotNull',
      includeEmpty: false,
    });
    expect(resolveDateFilter({ kind: 'special', value: 'all' }, DAY, NOW)).toEqual({
      operator: null,
      includeEmpty: false,
    });
  });

  it('单个日期收敛到该粒度的完整边界', () => {
    expect(range({ kind: 'single', date: '2026-05-15' })).toBe('2026-05-15 ~ 2026-05-15');
    expect(range({ kind: 'single', date: '2026-05-15' }, MONTH)).toBe('2026-05-01 ~ 2026-05-31');
  });
});

describe('包含空日期', () => {
  it('只在区间模式下追加标记，特殊值模式不会重复追加', () => {
    expect(
      resolveDateFilter(
        { kind: 'fixed', start: '2026-08-01', end: '2026-08-31', includeEmpty: true },
        DAY,
        NOW
      ).includeEmpty
    ).toBe(true);
    expect(
      resolveDateFilter(
        { kind: 'special', value: 'empty', includeEmpty: true } as DateFilterValue,
        DAY,
        NOW
      ).includeEmpty
    ).toBe(false);
  });
});

describe('展示文案：芯片摘要 vs 时间预览', () => {
  it('快捷选项摘要用选项名，预览用具体区间', () => {
    const value: DateFilterValue = { kind: 'dynamic', preset: 'last7d' };
    expect(formatDateFilterSummary(value)).toBe('最近 7 天');
    expect(formatDateFilterPreview(value, DAY, NOW)).toBe('2026-09-18 ~ 2026-09-24');
  });

  it('未选择时给出「请选择」', () => {
    const empty = defaultDateFilterValue();
    expect(hasDateFilterSelection(empty)).toBe(false);
    expect(formatDateFilterSummary(empty)).toBe('请选择');
    expect(formatDateFilterPreview(empty, DAY, NOW)).toBe('请选择');
  });

  it('粒度决定预览的日期格式（日 / 月）', () => {
    const value: DateFilterValue = { kind: 'dynamic', preset: 'last3m' };
    expect(formatDateFilterPreview(value, DAY, NOW)).toBe('2026-07-01 ~ 2026-09-24');
    expect(formatDateFilterPreview(value, MONTH, NOW)).toBe('2026-07 ~ 2026-09');
  });

  it('周粒度的预览带周序号与周内日期段', () => {
    const value: DateFilterValue = { kind: 'dynamic', preset: 'lastWeek' };
    expect(formatDateFilterPreview(value, WEEK, NOW)).toBe('2026-38周(0914-0920)');
  });

  it('高级模式的无限制端显示为「无限制」', () => {
    expect(
      formatDateFilterPreview(
        {
          kind: 'advanced',
          start: { type: 'fixed', value: '2023-06-14' },
          end: { type: 'unlimited' },
        },
        DAY,
        NOW
      )
    ).toBe('2023-06-14 ~ 无限制');
  });

  it('特殊值 / 单个日期的摘要', () => {
    expect(formatDateFilterSummary({ kind: 'special', value: 'empty' })).toBe('空日期');
    expect(formatDateFilterSummary({ kind: 'special', value: 'all' })).toBe('所有日期');
    expect(formatDateFilterSummary({ kind: 'single', date: '2026-05-15' })).toBe('2026-05-15');
  });
});

describe('isDateFilterComplete（决定「确定」是否可用）', () => {
  it('未选择 / 半填 / 双无限制都不算完整', () => {
    expect(isDateFilterComplete(defaultDateFilterValue())).toBe(false);
    expect(isDateFilterComplete({ kind: 'fixed', start: '2026-01-01', end: '' })).toBe(false);
    expect(
      isDateFilterComplete({
        kind: 'advanced',
        start: { type: 'unlimited' },
        end: { type: 'unlimited' },
      })
    ).toBe(false);
    expect(
      isDateFilterComplete({
        kind: 'advanced',
        start: { type: 'fixed', value: '' },
        end: { type: 'unlimited' },
      })
    ).toBe(false);
    expect(isDateFilterComplete({ kind: 'single', date: '' })).toBe(false);
  });

  it('选中快捷选项 / 填好区间 / 特殊值都算完整', () => {
    expect(isDateFilterComplete({ kind: 'dynamic', preset: 'last7d' })).toBe(true);
    expect(
      isDateFilterComplete({
        kind: 'dynamic',
        custom: { op: 'last', n: 3, unit: 'day' },
      })
    ).toBe(true);
    expect(isDateFilterComplete({ kind: 'fixed', start: '2026-01-01', end: '2026-01-31' })).toBe(
      true
    );
    expect(isDateFilterComplete({ kind: 'special', value: 'all' })).toBe(true);
    expect(isDateFilterComplete({ kind: 'single', date: '2026-05-15' })).toBe(true);
    // 单侧无限制是合法配置（文档 §3.5）
    expect(
      isDateFilterComplete({
        kind: 'advanced',
        start: { type: 'unlimited' },
        end: { type: 'fixed', value: '2023-06-14' },
      })
    ).toBe(true);
  });
});

describe('日期意图 → 下发条件', () => {
  const intent = (value: DateFilterValue): DateFilterIntent => ({
    value,
    granularity: 'day',
    weekStart: 1,
    asFilter: false,
    label: '交易日期',
  });

  it('区间意图展开成 between（含下发的列标识）', () => {
    expect(
      expandDateFilterIntent(intent({ kind: 'dynamic', preset: 'last7d' }), '0000i529', NOW)
    ).toEqual([
      {
        field: '0000i529',
        operator: 'between',
        value: '2026-09-18',
        value_end: '2026-09-24',
        logic: 'and',
      },
    ]);
  });

  it('特殊值 → isNull / isNotNull；「所有日期」不下发任何条件', () => {
    expect(expandDateFilterIntent(intent({ kind: 'special', value: 'empty' }), 'c1', NOW)).toEqual([
      { field: 'c1', operator: 'isNull', value: '', logic: 'and' },
    ]);
    expect(
      expandDateFilterIntent(intent({ kind: 'special', value: 'notEmpty' }), 'c1', NOW)
    ).toEqual([{ field: 'c1', operator: 'isNotNull', value: '', logic: 'and' }]);
    expect(expandDateFilterIntent(intent({ kind: 'special', value: 'all' }), 'c1', NOW)).toEqual(
      []
    );
  });

  it('高级模式的单侧无限制落成 gte / lte', () => {
    expect(
      expandDateFilterIntent(
        intent({
          kind: 'advanced',
          start: { type: 'fixed', value: '2023-06-14' },
          end: { type: 'unlimited' },
        }),
        'c1',
        NOW
      )
    ).toEqual([{ field: 'c1', operator: 'gte', value: '2023-06-14', logic: 'and' }]);
    expect(
      expandDateFilterIntent(
        intent({
          kind: 'advanced',
          start: { type: 'unlimited' },
          end: { type: 'fixed', value: '2023-06-14' },
        }),
        'c1',
        NOW
      )
    ).toEqual([{ field: 'c1', operator: 'lte', value: '2023-06-14', logic: 'and' }]);
  });

  it('「包含空日期」追加一条 IS NULL 并用 OR 连接', () => {
    expect(
      expandDateFilterIntent(
        intent({ kind: 'fixed', start: '2026-08-01', end: '2026-08-31', includeEmpty: true }),
        'c1',
        NOW
      )
    ).toEqual([
      {
        field: 'c1',
        operator: 'between',
        value: '2026-08-01',
        value_end: '2026-08-31',
        logic: 'and',
      },
      { field: 'c1', operator: 'isNull', value: '', logic: 'or' },
    ]);
  });
});

describe('保存路径的物化（materializeDateFilterSnapshot）', () => {
  it('无日期意图的条件原样返回（不该被动过）', () => {
    const condition = { operator: 'gte', value: 100, logic: 'and' as const };
    expect(materializeDateFilterSnapshot(condition)).toEqual([condition]);
  });

  it('日期意图被换成保存时刻的区间快照', () => {
    const [materialized] = materializeDateFilterSnapshot(
      {
        operator: 'eq',
        value: '',
        logic: 'and' as const,
        date: {
          value: { kind: 'dynamic', preset: 'last7d' },
          granularity: 'day',
          weekStart: 1,
          asFilter: false,
          label: '交易日期',
        },
      },
      NOW
    );
    expect(materialized).toMatchObject({
      operator: 'between',
      value: '2026-09-18',
      valueEnd: '2026-09-24',
    });
    // 意图本身保留：图表查询页仍要靠它做「动态」解析
    expect(materialized.date).toBeDefined();
  });

  it('空日期落成 isNull，「所有日期」一条都产生不出来（等价于没有这条筛选）', () => {
    const make = (value: DateFilterValue) =>
      materializeDateFilterSnapshot(
        {
          operator: 'eq',
          value: '',
          logic: 'and' as const,
          date: { value, granularity: 'day', weekStart: 1, asFilter: false, label: 'x' },
        },
        NOW
      );
    expect(make({ kind: 'special', value: 'empty' })[0]?.operator).toBe('isNull');
    expect(make({ kind: 'special', value: 'all' })).toEqual([]);
  });
});

describe('dateFilterValueFromLegacy（历史日期条件回填进新弹窗）', () => {
  it('between → 固定日期区间', () => {
    expect(dateFilterValueFromLegacy('between', '2026-08-08', '2026-08-12')).toEqual({
      kind: 'fixed',
      start: '2026-08-08',
      end: '2026-08-12',
    });
  });

  it('isNull / isNotNull → 特殊值', () => {
    expect(dateFilterValueFromLegacy('isNull', '', undefined)).toEqual({
      kind: 'special',
      value: 'empty',
    });
    expect(dateFilterValueFromLegacy('isNotNull', '', undefined)).toEqual({
      kind: 'special',
      value: 'notEmpty',
    });
  });

  it('表达不了的旧条件返回 null（仍走通用筛选弹窗）', () => {
    expect(dateFilterValueFromLegacy('between', '2026-08-08', '')).toBeNull();
    expect(dateFilterValueFromLegacy('gte', '2026-08-08', undefined)).toBeNull();
    expect(dateFilterValueFromLegacy('eq', '', undefined)).toBeNull();
  });
});

describe('isDateFilterValue 守卫（chart config 里是 unknown）', () => {
  it('接受五种合法模式', () => {
    expect(isDateFilterValue({ kind: 'dynamic', preset: 'last7d' })).toBe(true);
    expect(isDateFilterValue({ kind: 'fixed', start: '2026-01-01', end: '2026-01-31' })).toBe(true);
    expect(
      isDateFilterValue({
        kind: 'advanced',
        start: { type: 'unlimited' },
        end: { type: 'unlimited' },
      })
    ).toBe(true);
    expect(isDateFilterValue({ kind: 'special', value: 'all' })).toBe(true);
    expect(isDateFilterValue({ kind: 'single', date: '2026-05-15' })).toBe(true);
  });

  it('拒绝未知模式与畸形输入', () => {
    expect(isDateFilterValue(null)).toBe(false);
    expect(isDateFilterValue(undefined)).toBe(false);
    expect(isDateFilterValue('last7d')).toBe(false);
    expect(isDateFilterValue({})).toBe(false);
    expect(isDateFilterValue({ kind: 'unknown' })).toBe(false);
    expect(isDateFilterValue({ kind: 'dynamic', preset: 'nope' })).toBe(false);
    expect(isDateFilterValue({ kind: 'fixed', start: '2026-01-01' })).toBe(false);
    expect(isDateFilterValue({ kind: 'special', value: 'nope' })).toBe(false);
  });
});
