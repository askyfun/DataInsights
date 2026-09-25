import { describe, expect, it } from 'vitest';
import {
  collectChartIds,
  createWidgetId,
  DASHBOARD_GRID_COLS,
  DASHBOARD_MIN_H,
  DASHBOARD_MIN_W,
  type DashboardFilterWidget,
  emptyDashboardLayout,
  findFreePlacement,
  isFilterActive,
  migrateDashboardLayout,
  normalizePlacement,
  serializeDashboardLayout,
} from '@/lib/dashboardLayoutSchema';

/**
 * dashboardLayoutSchema v1 迁移测试。
 *
 * 契约：migrateDashboardLayout 是**全覆盖**函数——任何输入都返回合法 v1 文档、绝不抛异常。
 * 三条与产品决策直接绑定的断言（回归防线）：
 *   1. 同一 chartId 出现多次时，各块必须保有**各自独立**的 widgetId 与四轴（D1 同图复用）；
 *   2. 图表块只存 chartId、**不内嵌 config 快照**（D1 引用而非拷贝）；
 *   3. 缺失 widgetId 按位置补**确定性** id，保证纯函数语义（同输入同输出）。
 */

describe('migrateDashboardLayout：非法输入一律降级为空文档', () => {
  const invalidInputs: Array<[string, string]> = [
    ['空串', ''],
    ['损坏的 JSON', '{bad'],
    ['JSON 数组', '[]'],
    ['JSON 标量', '42'],
    ['null', 'null'],
    ['字符串字面量', '"hello"'],
  ];

  for (const [name, raw] of invalidInputs) {
    it(`${name} → 合法空 v1 文档且不抛异常`, () => {
      let doc: ReturnType<typeof migrateDashboardLayout> | undefined;
      expect(() => {
        doc = migrateDashboardLayout(raw);
      }).not.toThrow();
      expect(doc).toEqual(emptyDashboardLayout());
    });
  }

  it('合法对象但缺 widgets → 保留 grid，widgets 为空', () => {
    const doc = migrateDashboardLayout(JSON.stringify({ version: 1, grid: { cols: 12 } }));
    expect(doc.widgets).toEqual([]);
    expect(doc.grid.cols).toBe(DASHBOARD_GRID_COLS);
  });
});

describe('migrateDashboardLayout：三类 widget 的保留与修复', () => {
  it('三类 widget 全部保留，四轴与载荷逐字段一致', () => {
    const raw = JSON.stringify({
      version: 1,
      grid: { cols: 12 },
      widgets: [
        { widgetId: 'w-1', type: 'chart', x: 0, y: 0, w: 6, h: 8, chartId: 42 },
        { widgetId: 'w-2', type: 'text', x: 6, y: 0, w: 6, h: 4, markdown: '# 口径' },
        {
          widgetId: 'w-3',
          type: 'filter',
          x: 0,
          y: 8,
          w: 3,
          h: 3,
          binding: { datasetId: 7, column: 'region' },
          label: '区域',
          dataType: 'string',
          operator: 'in',
          multi: true,
          defaultValue: ['华东'],
        },
      ],
    });

    const doc = migrateDashboardLayout(raw);

    expect(doc.widgets).toHaveLength(3);
    expect(doc.widgets[0]).toEqual({
      widgetId: 'w-1',
      type: 'chart',
      chartId: 42,
      x: 0,
      y: 0,
      w: 6,
      h: 8,
    });
    expect(doc.widgets[1]).toEqual({
      widgetId: 'w-2',
      type: 'text',
      markdown: '# 口径',
      x: 6,
      y: 0,
      w: 6,
      h: 4,
    });
    expect(doc.widgets[2]).toEqual({
      widgetId: 'w-3',
      type: 'filter',
      binding: { datasetId: 7, column: 'region' },
      label: '区域',
      dataType: 'string',
      operator: 'in',
      multi: true,
      defaultValue: ['华东'],
      x: 0,
      y: 8,
      w: 3,
      h: 3,
    });
  });

  it('图表块不含 config 快照字段（引用而非拷贝）', () => {
    const raw = JSON.stringify({
      version: 1,
      widgets: [
        {
          widgetId: 'w-1',
          type: 'chart',
          chartId: 42,
          config: '{"version":2}',
          chartType: 'bar',
          datasetId: 7,
        },
      ],
    });

    const [widget] = migrateDashboardLayout(raw).widgets;
    expect(widget).not.toHaveProperty('config');
    expect(widget).not.toHaveProperty('chartType');
    expect(widget).not.toHaveProperty('datasetId');
  });

  it('同一 chartId 出现两次 → 两个独立 widgetId 且四轴互不影响（D1）', () => {
    const raw = JSON.stringify({
      version: 1,
      widgets: [
        { widgetId: 'w-top', type: 'chart', chartId: 42, x: 0, y: 0, w: 12, h: 6 },
        { widgetId: 'w-bottom', type: 'chart', chartId: 42, x: 0, y: 6, w: 6, h: 4 },
      ],
    });

    const doc = migrateDashboardLayout(raw);

    expect(doc.widgets).toHaveLength(2);
    expect(doc.widgets[0].widgetId).not.toBe(doc.widgets[1].widgetId);
    expect(doc.widgets[0]).toMatchObject({ chartId: 42, y: 0, w: 12, h: 6 });
    expect(doc.widgets[1]).toMatchObject({ chartId: 42, y: 6, w: 6, h: 4 });
  });

  it('缺 widgetId → 按位置补确定性 id，且同输入同输出', () => {
    const raw = JSON.stringify({
      version: 1,
      widgets: [
        { type: 'text', markdown: 'a' },
        { type: 'text', markdown: 'b' },
      ],
    });

    const first = migrateDashboardLayout(raw);
    const second = migrateDashboardLayout(raw);

    expect(first.widgets.map((w) => w.widgetId)).toEqual(['w-recovered-0', 'w-recovered-1']);
    expect(second).toEqual(first);
  });

  it('无法修复的块被丢弃：chart 缺 chartId / chartId 非正 / 未知 type / 非对象', () => {
    const raw = JSON.stringify({
      version: 1,
      widgets: [
        { widgetId: 'w-a', type: 'chart', x: 0, y: 0, w: 6, h: 6 },
        { widgetId: 'w-b', type: 'chart', chartId: 0, x: 0, y: 0, w: 6, h: 6 },
        { widgetId: 'w-c', type: 'chart', chartId: -3, x: 0, y: 0, w: 6, h: 6 },
        { widgetId: 'w-d', type: 'unknown', x: 0, y: 0, w: 6, h: 6 },
        'not-an-object',
        { widgetId: 'w-e', type: 'text', markdown: 'survivor' },
      ],
    });

    const doc = migrateDashboardLayout(raw);

    expect(doc.widgets).toHaveLength(1);
    expect(doc.widgets[0]).toMatchObject({ widgetId: 'w-e', markdown: 'survivor' });
  });

  it('filter 块缺 binding 或缺列名 → 丢弃', () => {
    const raw = JSON.stringify({
      version: 1,
      widgets: [
        { widgetId: 'w-a', type: 'filter' },
        { widgetId: 'w-b', type: 'filter', binding: { datasetId: 0, column: 'region' } },
        { widgetId: 'w-c', type: 'filter', binding: { datasetId: 7, column: '' } },
        { widgetId: 'w-d', type: 'filter', binding: { datasetId: 7, column: 'region' } },
      ],
    });

    const doc = migrateDashboardLayout(raw);

    expect(doc.widgets).toHaveLength(1);
    expect(doc.widgets[0]).toMatchObject({
      widgetId: 'w-d',
      binding: { datasetId: 7, column: 'region' },
    });
  });

  it('filter 的非法 operator 回退 in，label 缺省用列名，dataType 缺省 string', () => {
    const raw = JSON.stringify({
      version: 1,
      widgets: [
        {
          widgetId: 'w-a',
          type: 'filter',
          binding: { datasetId: 7, column: 'amount' },
          operator: 'drop table',
        },
      ],
    });

    const doc = migrateDashboardLayout(raw);
    const widget = doc.widgets[0] as DashboardFilterWidget;

    expect(widget.operator).toBe('in');
    expect(widget.label).toBe('amount');
    expect(widget.dataType).toBe('string');
    expect(widget.multi).toBe(false);
  });
});

describe('normalizePlacement：收敛保证不越界', () => {
  it('x + w 超出 cols 时先收 w 再收 x', () => {
    const placement = normalizePlacement({ x: 11, y: 0, w: 6, h: 8 }, { w: 6, h: 8 });
    expect(placement).toEqual({ x: 6, y: 0, w: 6, h: 8 });
  });

  it('w/h 低于最小值时提到最小值', () => {
    const placement = normalizePlacement({ x: 0, y: 0, w: 0, h: 1 }, { w: 6, h: 8 });
    expect(placement.w).toBe(DASHBOARD_MIN_W);
    expect(placement.h).toBe(DASHBOARD_MIN_H);
  });

  it('负 x/y 归零，缺省 y 归零', () => {
    expect(normalizePlacement({ x: -5, y: -2, w: 6, h: 6 }, { w: 6, h: 6 })).toEqual({
      x: 0,
      y: 0,
      w: 6,
      h: 6,
    });
    expect(normalizePlacement({ w: 6, h: 6 }, { w: 6, h: 6 }).y).toBe(0);
  });

  it('w 超过 cols 时收到 cols，且 x 归零', () => {
    const placement = normalizePlacement({ x: 4, y: 0, w: 99, h: 6 }, { w: 6, h: 6 });
    expect(placement.w).toBe(DASHBOARD_GRID_COLS);
    expect(placement.x).toBe(0);
  });

  it('任意输入下恒有 0 <= x 且 x + w <= cols 且 h >= MIN_H', () => {
    const cases: Array<Partial<{ x: number; y: number; w: number; h: number }>> = [
      {},
      { x: 0, y: 0, w: 0, h: 0 },
      { x: 999, y: 999, w: 999, h: 999 },
      { x: -1, y: -1, w: -1, h: -1 },
      { x: 5.5, y: 2.2, w: 3.3, h: 4.4 },
    ];

    for (const input of cases) {
      const { x, y, w, h } = normalizePlacement(input, { w: 6, h: 6 });
      expect(x).toBeGreaterThanOrEqual(0);
      expect(y).toBeGreaterThanOrEqual(0);
      expect(w).toBeGreaterThanOrEqual(DASHBOARD_MIN_W);
      expect(x + w).toBeLessThanOrEqual(DASHBOARD_GRID_COLS);
      expect(h).toBeGreaterThanOrEqual(DASHBOARD_MIN_H);
    }
  });
});

describe('文档级工具函数', () => {
  it('collectChartIds 去重且保持首次出现顺序', () => {
    const doc = migrateDashboardLayout(
      JSON.stringify({
        version: 1,
        widgets: [
          { widgetId: 'a', type: 'chart', chartId: 9, w: 6, h: 6 },
          { widgetId: 'b', type: 'text', markdown: 'x', w: 6, h: 4 },
          { widgetId: 'c', type: 'chart', chartId: 3, w: 6, h: 6 },
          { widgetId: 'd', type: 'chart', chartId: 9, w: 6, h: 6 },
        ],
      })
    );

    expect(collectChartIds(doc)).toEqual([9, 3]);
  });

  it('serializeDashboardLayout 与 migrateDashboardLayout 往返一致', () => {
    const raw = JSON.stringify({
      version: 1,
      widgets: [{ widgetId: 'w-1', type: 'chart', chartId: 5, x: 0, y: 0, w: 6, h: 6 }],
    });

    const doc = migrateDashboardLayout(raw);
    expect(migrateDashboardLayout(serializeDashboardLayout(doc))).toEqual(doc);
  });

  it('createWidgetId 带有 w- 前缀且互不相同', () => {
    const ids = new Set(Array.from({ length: 50 }, () => createWidgetId()));
    expect(ids.size).toBe(50);
    for (const id of ids) {
      expect(id.startsWith('w-')).toBe(true);
    }
  });
});

describe('isFilterActive：只有"有值"的筛选器才参与合并', () => {
  const base: DashboardFilterWidget = {
    widgetId: 'w-1',
    type: 'filter',
    binding: { datasetId: 1, column: 'region' },
    label: '区域',
    dataType: 'string',
    operator: 'in',
    multi: true,
    x: 0,
    y: 0,
    w: 3,
    h: 3,
  };

  it('无值 / null / 空串 / 空数组 → 未激活', () => {
    expect(isFilterActive(base)).toBe(false);
    expect(isFilterActive({ ...base, defaultValue: null })).toBe(false);
    expect(isFilterActive({ ...base, defaultValue: '' })).toBe(false);
    expect(isFilterActive({ ...base, defaultValue: [] })).toBe(false);
  });

  it('有标量或非空数组 → 激活', () => {
    expect(isFilterActive({ ...base, defaultValue: '华东' })).toBe(true);
    expect(isFilterActive({ ...base, defaultValue: ['华东'] })).toBe(true);
    expect(isFilterActive({ ...base, defaultValue: 0 })).toBe(true);
  });

  it('无值算子（isNull / isNotNull）恒为激活——它们靠算子本身表达，不需要值', () => {
    expect(isFilterActive({ ...base, operator: 'isNull', defaultValue: undefined })).toBe(true);
    expect(isFilterActive({ ...base, operator: 'isNotNull', defaultValue: undefined })).toBe(true);
  });
});

/**
 * 新增块的落点选择。
 *
 * 回归（浏览器验收 D-3）：旧实现硬编码 `x: 0, y: 最底边`，12 列画布上默认 6 列宽的块会
 * 全部堆在左半边、右半边长期空置。RGL 的 vertical compactor 只做纵向吸附，不会横向填空，
 * 所以这个选择必须在建块时就做对。
 */
describe('findFreePlacement：新块放进首个空位（自上而下、自左而右）', () => {
  const six = { w: 6, h: 8 };

  it('空盘 → 左上角', () => {
    expect(findFreePlacement([], six)).toEqual({ x: 0, y: 0 });
  });

  it('已有一块 6 列块 → 落在同一行右侧，而不是下一行', () => {
    const spot = findFreePlacement([{ x: 0, y: 0, w: 6, h: 8 }], six);
    expect(spot).toEqual({ x: 6, y: 0 });
  });

  it('左右各一块 → 回落到下一行左侧（这一行放不下 6 列）', () => {
    const spot = findFreePlacement(
      [
        { x: 0, y: 0, w: 6, h: 8 },
        { x: 6, y: 0, w: 6, h: 8 },
      ],
      six
    );
    expect(spot).toEqual({ x: 0, y: 8 });
  });

  it('上方有空洞时优先填空洞，而不是继续往下堆', () => {
    const spot = findFreePlacement(
      [
        { x: 0, y: 0, w: 6, h: 8 },
        { x: 0, y: 8, w: 6, h: 8 },
      ],
      six
    );
    expect(spot).toEqual({ x: 6, y: 0 });
  });

  it('3 列块填得更紧：4 块之后占满前两行', () => {
    const three = { w: 3, h: 3 };
    const placed: { x: number; y: number; w: number; h: number }[] = [];
    for (let i = 0; i < 5; i += 1) {
      const spot = findFreePlacement(placed, three);
      placed.push({ ...spot, ...three });
    }
    expect(placed).toEqual([
      { x: 0, y: 0, w: 3, h: 3 },
      { x: 3, y: 0, w: 3, h: 3 },
      { x: 6, y: 0, w: 3, h: 3 },
      { x: 9, y: 0, w: 3, h: 3 },
      { x: 0, y: 3, w: 3, h: 3 },
    ]);
  });

  it('返回的位置恒不与既有块重叠，且 x + w 不越界', () => {
    const placed: { x: number; y: number; w: number; h: number }[] = [];
    for (let i = 0; i < 12; i += 1) {
      const spot = findFreePlacement(placed, { w: 5, h: 4 });
      expect(spot.x).toBeGreaterThanOrEqual(0);
      expect(spot.x + 5).toBeLessThanOrEqual(DASHBOARD_GRID_COLS);
      expect(spot.y).toBeGreaterThanOrEqual(0);
      for (const other of placed) {
        const disjoint =
          spot.x >= other.x + other.w ||
          other.x >= spot.x + 5 ||
          spot.y >= other.y + other.h ||
          other.y >= spot.y + 4;
        expect(disjoint).toBe(true);
      }
      placed.push({ ...spot, w: 5, h: 4 });
    }
  });

  it('尺寸先按最小尺寸与列数收敛（w 超过列数时按满列处理）', () => {
    expect(findFreePlacement([], { w: 999, h: 1 })).toEqual({ x: 0, y: 0 });
    // 满列块放不进任何一行 → 回落到最底边之下（与旧行为一致）。
    expect(findFreePlacement([{ x: 0, y: 0, w: 12, h: 2 }], { w: 12, h: 2 })).toEqual({
      x: 0,
      y: 2,
    });
  });

  it('不修改入参', () => {
    const widgets = [{ x: 0, y: 0, w: 6, h: 8 }];
    const snapshot = JSON.stringify(widgets);
    findFreePlacement(widgets, six);
    expect(JSON.stringify(widgets)).toBe(snapshot);
  });
});

describe('筛选器块的 date 配置（粒度 / 周计算逻辑）', () => {
  const layoutWith = (date: unknown) =>
    JSON.stringify({
      version: 1,
      grid: { cols: 12 },
      widgets: [
        {
          widgetId: 'w-f',
          type: 'filter',
          binding: { datasetId: 3, column: '0000i529' },
          label: '交易日期',
          dataType: 'date',
          operator: 'between',
          multi: false,
          date,
          x: 0,
          y: 0,
          w: 3,
          h: 3,
        },
      ],
    });

  const dateOf = (date: unknown) => {
    const widget = migrateDashboardLayout(layoutWith(date)).widgets[0];
    return widget.type === 'filter' ? widget.date : undefined;
  };

  it('合法的粒度 + 周计算逻辑被采纳', () => {
    expect(dateOf({ granularity: 'week', weekStart: 0 })).toEqual({
      granularity: 'week',
      weekStart: 0,
    });
    expect(dateOf({ granularity: 'month', weekStart: 6 })).toEqual({
      granularity: 'month',
      weekStart: 6,
    });
  });

  it('半截 / 非法配置一律丢弃（宁可退回默认口径，也不要混合配置）', () => {
    expect(dateOf({ granularity: 'week' })).toBeUndefined();
    expect(dateOf({ weekStart: 0 })).toBeUndefined();
    expect(dateOf({ granularity: 'nope', weekStart: 0 })).toBeUndefined();
    // weekStart 合法域是 0..6，7 越界
    expect(dateOf({ granularity: 'day', weekStart: 7 })).toBeUndefined();
    expect(dateOf('day')).toBeUndefined();
    expect(dateOf(null)).toBeUndefined();
    expect(dateOf(undefined)).toBeUndefined();
  });
});
