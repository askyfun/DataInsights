/**
 * 仪表盘图表联动的纯逻辑（issue #143，不依赖组件运行时）。
 *
 * 「联动」= 把某块图的**被点击数据项**变成一个条件，追加到另一些图表块的查询上，
 * 复用盘级筛选器那条取值下发链路（`POST /api/dashboards/{id}/query` 的 `linkages`）。
 *
 * 职责边界与 `dashboardFilterValue` 一致：控件负责「用户点了什么」，这里负责
 * 「这个点击要怎么变成请求载荷 / 这块图现在被谁筛着」。落点（打到哪块图的哪一列）
 * **不在载荷里**，只存在布局文档的 `linkage.targets` 上，由后端读取——请求方只能
 * 表达「点了什么值」，表达不了「往哪一列注入」。
 *
 * 为什么不复用 `filters` 数组：那条链路的取值以**筛选器 widgetId** 为键、并与布局里
 * 已落库的 filter 块绑定校验，联动没有对应的 filter 块，硬塞进去会被后端当作未知键忽略。
 */

import type { DashboardQueryLinkage } from '../api';
import type { ChartConfigDocument, ChartType } from './chartConfigSchema';
import { migrateChartConfig } from './chartConfigSchema';
import type { DashboardChartWidget, DashboardWidget } from './dashboardLayoutSchema';

/**
 * 一次已激活的联动取值。键是**来源图表块**的 widgetId。
 *
 * `column` 只用于展示（「按 省份 = 华东 联动」），不进请求：落点在布局里。
 */
export interface ActiveLinkage {
  /** 被点击的维度列 ID（`DatasetColumn.id`），仅用于展示。 */
  column: string;
  /** 被点击的维度取值。 */
  value: unknown;
}

export type ActiveLinkageMap = Record<string, ActiveLinkage>;

/** 一个取值算不算「激活」：空串/null/undefined 都不算（维度取值 0 是真值）。 */
function isActiveValue(value: unknown): boolean {
  return value !== undefined && value !== null && value !== '';
}

/**
 * 已激活的联动 → 请求载荷。
 *
 * 单值恒为 `[value]`：与盘级筛选器同一条「按值个数收形」的规则（后端单值 → eq、
 * 多值 → in），所以将来支持多选时这里只需放多个元素。
 */
export function linkageQueryPayload(active: ActiveLinkageMap): DashboardQueryLinkage[] {
  const payload: DashboardQueryLinkage[] = [];
  for (const [sourceWidgetId, entry] of Object.entries(active)) {
    if (!isActiveValue(entry.value)) {
      continue;
    }
    payload.push({ sourceWidgetId, value: [entry.value] });
  }
  return payload;
}

/** 某块图当前被某个来源筛着的一条记录（供块上「联动中」提示与逐条清除）。 */
export interface IncomingLinkage extends ActiveLinkage {
  /** 来源图表块的 widgetId（清除时要按它撤掉联动）。 */
  sourceWidgetId: string;
}

/**
 * 某块图当前被哪些来源筛着（供块上的「联动中」提示与逐条清除）。
 *
 * 只看已激活的来源，且只认**确实把这块图勾成了目标**的那些——来源自己点击自己
 * （布局里没这么配）不会出现在这里。
 */
export function incomingLinkages(
  widgets: readonly DashboardWidget[],
  active: ActiveLinkageMap,
  targetWidgetId: string
): IncomingLinkage[] {
  const out: IncomingLinkage[] = [];
  for (const widget of widgets) {
    if (widget.type !== 'chart') {
      continue;
    }
    const entry = active[widget.widgetId];
    if (!entry || !isActiveValue(entry.value)) {
      continue;
    }
    const hit = widget.linkage?.targets.some((target) => target.widgetId === targetWidgetId);
    if (hit) {
      out.push({ sourceWidgetId: widget.widgetId, column: entry.column, value: entry.value });
    }
  }
  return out;
}

/**
 * 没有「点数据项 → 拿到某个维度取值」语义的图型：
 *   - `kpi`：标量卡，压根没有可点的数据项；
 *   - `pivot`：交叉表，行列都可展开，一个单元格对应两个维度，点不出唯一条件；
 *   - `radar`：雷达图的分类轴是**指标**（维度落成了 series 名），点击拿到的不是维度值。
 * 其余图型（含 ECharts 各臂与 table）的分类轴都是维度，可作联动来源。
 */
const NON_CLICKABLE_CHART_TYPES: readonly ChartType[] = ['kpi', 'pivot', 'radar'];

/**
 * 来源图的**联动键列**：它被点击时能把哪个维度值发出去。
 *
 * 两个条件同时成立才有键列：图型本身有「点数据项拿维度值」的语义，且恰好一个维度绑定
 * ——一块图有多个维度时，「点了哪个维度」在点击事件里分不出来（ECharts 只给一个分类值），
 * 硬猜会给出错误的条件。返回 null = 这块图不可作为联动来源（前端据此关闭点击）。
 */
export function linkageKeyColumnOf(doc: ChartConfigDocument): string | null {
  if (NON_CLICKABLE_CHART_TYPES.includes(doc.chartType)) {
    return null;
  }
  const bindings = doc.query.dimensionGroups.flatMap((group) => group.bindings);
  return bindings.length === 1 ? bindings[0].fieldId : null;
}

/** `linkageKeyColumnOf` 的入口形态：直接吃库里的 config 串（内部先跑迁移）。 */
export function linkageKeyColumn(rawConfig: string | undefined, chartType: string): string | null {
  return linkageKeyColumnOf(migrateChartConfig(rawConfig ?? '', chartType as ChartType));
}

/**
 * 勾选一个联动目标时的默认落点列：
 *   - 同数据集 → 沿用来源的联动键列（最常见的「省份点省份」）；
 *   - 跨数据集 → 无从推断，返回空串，要求用户显式选一列。
 */
export function linkageDefaultColumn(
  keyColumn: string | null,
  sourceDatasetId: number | undefined,
  targetDatasetId: number | undefined
): string {
  if (keyColumn === null || sourceDatasetId === undefined || targetDatasetId === undefined) {
    return '';
  }
  return sourceDatasetId === targetDatasetId ? keyColumn : '';
}

/** 可作为某来源联动目标的图表块：盘内其它 chart 块（同一块图不能联动自己）。 */
export function linkageCandidates(
  widgets: readonly DashboardWidget[],
  sourceWidgetId: string
): DashboardChartWidget[] {
  return widgets.filter(
    (widget): widget is DashboardChartWidget =>
      widget.type === 'chart' && widget.widgetId !== sourceWidgetId
  );
}
