import dayjs from 'dayjs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ACTIVATION_PLACEHOLDER,
  dashboardDateFilterQueryValue,
  dashboardFiltersPayload,
  FAMILY_OPERATORS,
  filterWidgetFamily,
  filterWidgetQueryValue,
  initialFilterWidgetValues,
} from '../../lib/dashboardFilterValue';
import type { DateFilterSettings, DateFilterValue } from '../../lib/dateFilter';

/**
 * 仪表盘盘级筛选器的取值下发（三族共用一条链路）。
 *
 * 形状契约的另一半在后端 `service/dashboard/query.go` 的 `buildOverrides`（有同名对照用例）：
 * `in`/`notIn` 要数组、`between` 要两元素、其余标量算子要**一个**元素、`isNull`/`isNotNull`
 * 只靠「数组非空」表示已激活。改动这里必须同步改那边。
 * now 注入固定时刻：2026-09-25（周五）。
 */

const NOW = dayjs('2026-09-25 14:30:45');
const DAY: DateFilterSettings = { granularity: 'day' };

const dateWidget = (extra = {}) => ({
  widgetId: 'w-date',
  dataType: 'date',
  operator: 'between',
  multi: false,
  ...extra,
});

const textWidget = (operator: string, extra = {}) => ({
  widgetId: 'w-text',
  dataType: 'string',
  operator,
  multi: operator === 'in' || operator === 'notIn',
  ...extra,
});

const numberWidget = (operator: string, extra = {}) => ({
  widgetId: 'w-num',
  dataType: 'float',
  operator,
  multi: false,
  ...extra,
});

describe('filterWidgetFamily：按数据类型分族', () => {
  it('三族各归其位（含历史脏词）', () => {
    expect(filterWidgetFamily({ dataType: 'date' })).toBe('date');
    expect(filterWidgetFamily({ dataType: 'timestamp' })).toBe('date');
    expect(filterWidgetFamily({ dataType: 'timestamp without time zone' })).toBe('date');
    expect(filterWidgetFamily({ dataType: 'int' })).toBe('number');
    expect(filterWidgetFamily({ dataType: 'number' })).toBe('number');
    expect(filterWidgetFamily({ dataType: 'float' })).toBe('number');
    expect(filterWidgetFamily({ dataType: 'string' })).toBe('string');
    expect(filterWidgetFamily({ dataType: 'varchar' })).toBe('string');
  });

  it('每族的算子词表互不串（number 不该出现 in，string 不该出现 between）', () => {
    expect(FAMILY_OPERATORS.number).not.toContain('in');
    expect(FAMILY_OPERATORS.string).not.toContain('between');
    expect(FAMILY_OPERATORS.date).toContain('isNull');
  });
});

describe('日期族：意图在下发这一刻现算', () => {
  it('动态区间 → 两元素数组', () => {
    expect(dashboardDateFilterQueryValue({ kind: 'dynamic', preset: 'last7d' }, DAY, NOW)).toEqual([
      '2026-09-18',
      '2026-09-24',
    ]);
  });

  it('高级·单侧无限制 → 一元素数组', () => {
    expect(
      filterWidgetQueryValue(
        dateWidget(),
        {
          kind: 'advanced',
          start: { type: 'fixed', value: '2023-06-14' },
          end: { type: 'unlimited' },
        },
        NOW
      )
    ).toEqual(['2023-06-14']);
  });

  it('特殊值 → 一个占位元素（算子已表达语义，占位只为标记「已激活」）', () => {
    expect(filterWidgetQueryValue(dateWidget(), { kind: 'special', value: 'empty' }, NOW)).toEqual([
      ACTIVATION_PLACEHOLDER,
    ]);
  });

  it('「所有日期」/「未选择」/ 脏值 → 空数组（未激活）', () => {
    expect(filterWidgetQueryValue(dateWidget(), { kind: 'special', value: 'all' }, NOW)).toEqual(
      []
    );
    expect(filterWidgetQueryValue(dateWidget(), { kind: 'dynamic' }, NOW)).toEqual([]);
    expect(filterWidgetQueryValue(dateWidget(), 'last7d', NOW)).toEqual([]);
    expect(filterWidgetQueryValue(dateWidget(), undefined, NOW)).toEqual([]);
  });

  it('粒度按布局里的 date 配置求值', () => {
    expect(
      filterWidgetQueryValue(
        dateWidget({ date: { granularity: 'month', weekStart: 1 } }),
        { kind: 'single', date: '2026-05-15' },
        NOW
      )
    ).toEqual(['2026-05-01', '2026-05-31']);
  });
});

describe('字符串族：按算子收形', () => {
  it('eq → 一个元素（后端取 value[0] 当标量绑定参数）', () => {
    expect(filterWidgetQueryValue(textWidget('eq'), ['华东'])).toEqual(['华东']);
  });

  it('eq 拿到多值时只取第一个（不越权把它变成 in）', () => {
    expect(filterWidgetQueryValue(textWidget('eq'), ['华东', '华南'])).toEqual(['华东']);
  });

  it('in / notIn → 原样数组（后端按元素展开）', () => {
    expect(filterWidgetQueryValue(textWidget('in'), ['华东', '华南'])).toEqual(['华东', '华南']);
    expect(filterWidgetQueryValue(textWidget('notIn'), ['华东'])).toEqual(['华东']);
  });

  it('like → 一个元素', () => {
    expect(filterWidgetQueryValue(textWidget('like'), ['华'])).toEqual(['华']);
  });

  it('空值 / 空串 / null → 未激活', () => {
    expect(filterWidgetQueryValue(textWidget('in'), [])).toEqual([]);
    expect(filterWidgetQueryValue(textWidget('in'), ['', null, undefined])).toEqual([]);
    expect(filterWidgetQueryValue(textWidget('eq'), undefined)).toEqual([]);
    expect(filterWidgetQueryValue(textWidget('eq'), '华东')).toEqual([]);
  });
});

describe('数值族：按算子收形', () => {
  it('between → 两元素', () => {
    expect(filterWidgetQueryValue(numberWidget('between'), [10, 20])).toEqual([10, 20]);
  });

  it('between 缺一端 → 整条不下发（不补空值造出恒假区间）', () => {
    expect(filterWidgetQueryValue(numberWidget('between'), [10])).toEqual([]);
    expect(filterWidgetQueryValue(numberWidget('between'), [10, ''])).toEqual([]);
  });

  it('单侧比较 → 一个元素', () => {
    expect(filterWidgetQueryValue(numberWidget('gt'), [100])).toEqual([100]);
    expect(filterWidgetQueryValue(numberWidget('gte'), [100])).toEqual([100]);
    expect(filterWidgetQueryValue(numberWidget('lte'), [100])).toEqual([100]);
  });

  it('0 是有效值（不能被当成空值丢掉）', () => {
    expect(filterWidgetQueryValue(numberWidget('gt'), [0])).toEqual([0]);
    expect(filterWidgetQueryValue(numberWidget('between'), [0, 0])).toEqual([0, 0]);
  });
});

describe('dashboardFiltersPayload：批量下发', () => {
  const widgets = [dateWidget(), textWidget('in'), numberWidget('gt')];

  it('只下发已激活的块，未激活的整体缺席', () => {
    const payload = dashboardFiltersPayload(
      widgets,
      {
        'w-date': { kind: 'dynamic', preset: 'last1d' },
        'w-text': [],
        'w-num': [100],
      },
      NOW
    );

    expect(payload).toEqual([
      { widgetId: 'w-date', value: ['2026-09-24', '2026-09-24'] },
      { widgetId: 'w-num', value: [100] },
    ]);
  });

  it('请求里没有值的块不发（状态可能还没初始化）', () => {
    expect(dashboardFiltersPayload(widgets, {}, NOW)).toEqual([]);
  });

  it('三族可以同盘共存，顺序与 widgets 顺序一致', () => {
    const payload = dashboardFiltersPayload(
      [numberWidget('gt'), dateWidget(), textWidget('eq')],
      {
        'w-num': [1],
        'w-date': { kind: 'special', value: 'empty' },
        'w-text': ['华东'],
      },
      NOW
    );
    expect(payload.map((item) => item.widgetId)).toEqual(['w-num', 'w-date', 'w-text']);
    expect(payload[1].value).toEqual([ACTIVATION_PLACEHOLDER]);
  });

  describe('动态日期按「下发这一刻」解析', () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    it('同一块筛选器在不同时刻下发不同区间', () => {
      const form = [dateWidget()];
      const values = { 'w-date': { kind: 'dynamic', preset: 'last7d' } as DateFilterValue };

      vi.setSystemTime(new Date(2026, 8, 25, 9, 0, 0));
      expect(dashboardFiltersPayload(form, values)[0].value).toEqual(['2026-09-18', '2026-09-24']);

      vi.setSystemTime(new Date(2026, 9, 2, 9, 0, 0));
      expect(dashboardFiltersPayload(form, values)[0].value).toEqual(['2026-09-25', '2026-10-01']);
    });
  });
});

describe('initialFilterWidgetValues：从布局默认值做初值', () => {
  it('日期族要过守卫才采纳', () => {
    expect(
      initialFilterWidgetValues([
        { ...dateWidget(), defaultValue: { kind: 'dynamic', preset: 'last7d' } },
      ])
    ).toEqual({ 'w-date': { kind: 'dynamic', preset: 'last7d' } });
    expect(
      initialFilterWidgetValues([{ ...dateWidget(), defaultValue: { kind: '未知' } }])
    ).toEqual({});
    expect(initialFilterWidgetValues([{ ...dateWidget(), defaultValue: 'last7d' }])).toEqual({});
  });

  it('字符串 / 数值族要是数组才采纳', () => {
    expect(initialFilterWidgetValues([{ ...textWidget('in'), defaultValue: ['华东'] }])).toEqual({
      'w-text': ['华东'],
    });
    expect(initialFilterWidgetValues([{ ...numberWidget('gt'), defaultValue: [10] }])).toEqual({
      'w-num': [10],
    });
  });

  it('空数组是合法默认值（= 未激活），要保留成键', () => {
    expect(initialFilterWidgetValues([{ ...textWidget('in'), defaultValue: [] }])).toEqual({
      'w-text': [],
    });
  });

  it('脏默认值一律跳过，退回「未选择」', () => {
    expect(
      initialFilterWidgetValues([
        { ...textWidget('in'), defaultValue: '华东' },
        { ...numberWidget('gt'), defaultValue: 10 },
        { ...numberWidget('gt', { widgetId: 'w-num2' }), defaultValue: null },
        { ...numberWidget('gt', { widgetId: 'w-num3' }) },
      ])
    ).toEqual({});
  });
});
