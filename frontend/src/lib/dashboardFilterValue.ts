/**
 * 仪表盘盘级筛选器的取值下发（纯逻辑，不依赖组件运行时）。
 *
 * 为什么单独一个模块：控件负责「用户选了什么」，这里负责「这个选择要怎么变成
 * `POST /api/dashboards/{id}/query` 的请求载荷」。后端**只接受筛选器的当前取值**、
 * 由它单点完成与图表自身条件的合并（PRD §6.3），所以前端必须把值翻成后端认得的
 * **数组形状**，翻错就是 SQL 层的错。
 *
 * 形状规则来自后端 `service/dashboard/query.go` 的 `buildOverrides`（取值形状按算子分流）：
 *   - `in` / `notIn` 要**数组**（后端按元素展开成 `IN (?, ?, …)`）；
 *   - `between` 要**恰好两个**元素 `[下界, 上界]`（后端拆成 Value + ValueEnd）；
 *   - 其余**标量算子**（eq/neq/gt/gte/lt/lte/like）要**一个**元素（后端取 `value[0]` 当绑定参数）；
 *   - `isNull` / `isNotNull` 不看值，但**必须非空**——契约里「空数组 = 未激活」，
 *     所以放一个 `null` 占位，把这条筛选器标记成已激活（算子自身已表达全部语义）；
 *   - 「所有日期」「未选择」→ 空数组 = 未激活，不参与合并。
 *
 * 三族（date / string / number）的分流点只有两处：**控件形态**与**算子词表**；
 * 取值下发这条链路对三族是同一条，故共用本模块。
 *
 * ⚠️ 一处**已接受的能力缺口**：盘级筛选每次查询、每个筛选器最多产生**一条**合并条件
 * （请求的 `values` 以 widgetId 为键），因此「包含空日期」那种「区间 OR IS NULL」在这里
 * **表达不出来**。图表查询页不受影响（那边一条意图可以展开成两条条件）。
 */

import dayjs, { type Dayjs } from 'dayjs';
import { classifyFieldKind, normalizeDataType } from './dataTypes';
import {
  type DateFilterSettings,
  type DateFilterValue,
  type DateGranularity,
  isDateFilterValue,
  resolveDateFilter,
  type WeekStart,
} from './dateFilter';

/** 筛选器族：决定控件形态与算子词表（与 `classifyFieldKind` 一一对应）。 */
export type FilterWidgetFamily = 'date' | 'string' | 'number';

/**
 * 盘级筛选器块的最小结构。
 * 用结构化类型而不是 import `DashboardFilterWidget`，避免 lib 之间的循环依赖。
 */
export interface DashboardFilterWidgetRef {
  widgetId: string;
  dataType: string;
  operator: string;
  multi: boolean;
  date?: { granularity: DateGranularity; weekStart: WeekStart };
}

/** 各族可用的算子（控件据此渲染下拉；也用来校验布局里被手改过的算子）。 */
export const FAMILY_OPERATORS: Record<FilterWidgetFamily, readonly string[]> = {
  date: ['between', 'gte', 'lte', 'isNull', 'isNotNull'],
  string: ['eq', 'neq', 'like', 'in', 'notIn'],
  number: ['eq', 'neq', 'gt', 'gte', 'lt', 'lte', 'between'],
};

export function filterWidgetFamily(widget: { dataType: string }): FilterWidgetFamily {
  return classifyFieldKind(widget.dataType);
}

/**
 * 单个筛选器块的求值设置：粒度与周计算逻辑来自布局，`withTime` 由字段类型决定。
 * 控件（行内浮层与完整弹窗）与下发载荷必须共用这一份口径，否则会「显示一种、查另一种」。
 */
export function dateFilterWidgetSettings(widget: DashboardFilterWidgetRef): DateFilterSettings {
  return {
    granularity: widget.date?.granularity ?? 'day',
    weekStart: widget.date?.weekStart ?? 1,
    withTime: normalizeDataType(widget.dataType) === 'datetime',
  };
}

/** `POST /api/dashboards/{id}/query` 里的单个筛选器取值（对齐 openapi 的 DashboardQueryFilter）。 */
export interface DashboardQueryFilterValue {
  widgetId: string;
  /** 空数组 = 未激活（不参与合并）。 */
  value: unknown[];
}

/**
 * 「已激活」的占位值：`isNull` / `isNotNull` 的语义全在算子里，但契约用「数组非空」判定激活，
 * 所以必须放一个元素。用 `null` 而不是 `''`——它不会被误当成一个真实的比较值。
 */
export const ACTIVATION_PLACEHOLDER = null;

/**
 * 单个日期筛选值 → 请求载荷里的 `value` 数组。
 * 返回空数组表示「未激活」。
 *
 * `resolveDateFilter` 的契约保证了 operator 与端点一一对应
 * （`between` 恒有 start+end、`gte` 恒有 start、`lte` 恒有 end），所以这里不需要兜底猜测。
 */
export function dashboardDateFilterQueryValue(
  value: DateFilterValue,
  settings: DateFilterSettings,
  now: Dayjs = dayjs()
): unknown[] {
  const resolved = resolveDateFilter(value, settings, now);
  switch (resolved.operator) {
    case 'between':
      return [resolved.start, resolved.end];
    case 'gte':
      return [resolved.start];
    case 'lte':
      return [resolved.end];
    case 'isNull':
    case 'isNotNull':
      return [ACTIVATION_PLACEHOLDER];
    default:
      return [];
  }
}

/** 去掉空值后的已选值；空串/null/undefined 都不算一个有效选择。 */
function meaningfulValues(value: unknown): unknown[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item) => item !== undefined && item !== null && item !== '');
}

/**
 * 字符串 / 数值族的取值 → 请求载荷里的 `value` 数组。
 *
 * ⚠️ 形状必须按算子分流（见文件头）：标量算子只给**一个**元素，给多了后端会把它降级成 `in`；
 * 区间缺一端时**整条不下发**（退回未激活）而不是补一个空值——补空值会变成恒假区间，
 * 用户看到「明明填了却一条数据都没有」比「还没生效」难懂得多。
 */
function scalarFilterQueryValue(operator: string, value: unknown): unknown[] {
  const values = meaningfulValues(value);
  if (values.length === 0) {
    return [];
  }
  switch (operator) {
    case 'between':
      return values.length >= 2 ? [values[0], values[1]] : [];
    case 'in':
    case 'notIn':
      return values;
    default:
      // 标量算子：后端 `buildOverrides` 取 value[0] 当绑定参数。
      return [values[0]];
  }
}

/**
 * 单个筛选器块的当前取值 → 请求载荷里的 `value` 数组（空数组 = 未激活）。
 *
 * 日期族走意图解析（`最近 7 天` 在这里现算成具体区间），其余族按算子收形。
 */
export function filterWidgetQueryValue(
  widget: DashboardFilterWidgetRef,
  value: unknown,
  now: Dayjs = dayjs()
): unknown[] {
  if (filterWidgetFamily(widget) === 'date') {
    return isDateFilterValue(value)
      ? dashboardDateFilterQueryValue(value, dateFilterWidgetSettings(widget), now)
      : [];
  }
  return scalarFilterQueryValue(widget.operator, value);
}

/**
 * 把盘里所有筛选块的当前值翻成请求载荷。
 *
 * 只下发**已激活**的块：未激活的块不出现在数组里（而不是出现且值为空数组）——
 * 两种写法后端行为一致，但少发一条更省事，也让「请求是取值的唯一真相源」更好读。
 */
export function dashboardFiltersPayload(
  widgets: readonly DashboardFilterWidgetRef[],
  values: Readonly<Record<string, unknown>>,
  now: Dayjs = dayjs()
): DashboardQueryFilterValue[] {
  const payload: DashboardQueryFilterValue[] = [];
  for (const widget of widgets) {
    if (!(widget.widgetId in values)) continue;
    const value = filterWidgetQueryValue(widget, values[widget.widgetId], now);
    if (value.length === 0) continue;
    payload.push({ widgetId: widget.widgetId, value });
  }
  return payload;
}

/**
 * 用布局里落库的默认值给编辑器做初值。
 *
 * 各族只接受自己认得的形状：日期族要过 `isDateFilterValue`，其余族要是数组。
 * 布局是手改得动的 JSON，读到脏数据时宁可退回「未选择」，也不要拿畸形值去求值。
 */
export function initialFilterWidgetValues(
  widgets: readonly (DashboardFilterWidgetRef & { defaultValue?: unknown })[]
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const widget of widgets) {
    if (filterWidgetFamily(widget) === 'date') {
      if (isDateFilterValue(widget.defaultValue)) {
        out[widget.widgetId] = widget.defaultValue;
      }
      continue;
    }
    if (Array.isArray(widget.defaultValue)) {
      out[widget.widgetId] = widget.defaultValue;
    }
  }
  return out;
}
