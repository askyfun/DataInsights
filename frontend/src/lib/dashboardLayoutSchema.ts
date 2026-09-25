/**
 * bi_dashboard.layout_json 的 v1 文档 schema 与迁移函数（纯逻辑，不依赖 UI/store 运行时）。
 *
 * 与 bi_chart.config 的关系：两者是**独立**的版本化文档，各自带 version 与迁移函数。
 * 仪表盘文档只**引用**图表（存 chartId），不内嵌图表 config——图表配置的演进归
 * `chartConfigSchema.migrateChartConfig` 管，本文件不复制一份会漂移的类型定义。
 *
 * v1 约定（本版本）：
 * - `grid.cols` 恒定 12。**注意**：若将来引入 >12 列的文档，本函数必须改为"按原
 *   grid.cols 缩放后再归一到渲染列数"，否则会静默位移既有块。v1 不存在这种文档，
 *   故此处直接归一到 12 并在超界时收敛。
 * - 每个 widget 持有**盘内唯一**的 `widgetId`。同一 chartId 允许在同一盘出现多次，
 *   各自独立 x/y/w/h——这是"同图复用"能力的实现基础，故 widgetId 绝不等于 chartId。
 * - 三种 widget：`chart`（引用）/ `text`（Markdown）/ `filter`（绑定数据集字段）。
 *   筛选器绑定的是 `(datasetId, column)` 二元组而非纯列名：因允许跨数据集混搭，
 *   纯列名会让不同数据集里的同名列互相误伤。
 * - 占位与错误是**运行期渲染行为**，不进文档：图表被软删后 widget 仍保留原位，
 *   由取数响应里的 status 决定渲染占位块。文档层不做任何级联删除。
 *
 * 迁移是全覆盖函数：任何输入（空串、损坏 JSON、非对象、字段缺失/类型错误）都返回
 * 合法 v1 文档，绝不抛异常。无法修复的 widget 被丢弃（例如 chart 块没有合法 chartId
 * ——它取不到数），可修复的则就地修复（缺失 widgetId 按位置补确定性 id、位置超界收敛）。
 */

import type { FilterOperator } from '../store';
import { normalizeDataType } from './dataTypes';

/** 渲染列数。与后端 layout_json 的 grid.cols 保持同一口径。 */
export const DASHBOARD_GRID_COLS = 12;

/** widget 最小尺寸（栅格单位）。对齐 R-04.2 的"不能缩到 0"。 */
export const DASHBOARD_MIN_W = 2;
export const DASHBOARD_MIN_H = 2;

/** 各类型 widget 的默认尺寸（新建时使用）。 */
export const DASHBOARD_DEFAULT_SIZE: Record<DashboardWidgetType, { w: number; h: number }> = {
  chart: { w: 6, h: 8 },
  text: { w: 6, h: 4 },
  filter: { w: 3, h: 3 },
};

export type DashboardWidgetType = 'chart' | 'text' | 'filter';

/** 栅格位置与尺寸（react-grid-layout 的四轴）。 */
export interface DashboardPlacement {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface DashboardChartWidget extends DashboardPlacement {
  widgetId: string;
  type: 'chart';
  /** 引用既有 bi_chart.id；不存快照。 */
  chartId: number;
  /** 仅覆盖盘内显示标题，不写回图表；null/缺省表示用图表自身标题。 */
  titleOverride?: string | null;
}

export interface DashboardTextWidget extends DashboardPlacement {
  widgetId: string;
  type: 'text';
  markdown: string;
}

/** 筛选器绑定：数据集 + 列名。两者共同构成字段标识（DatasetColumn 没有稳定 id，列名即标识）。 */
export interface DashboardFilterBinding {
  datasetId: number;
  column: string;
}

export interface DashboardFilterWidget extends DashboardPlacement {
  widgetId: string;
  type: 'filter';
  binding: DashboardFilterBinding;
  label: string;
  /** 后端 StandardDataType 原样透传，用于选控件族（日期/数值/字符串）。 */
  dataType: string;
  operator: FilterOperator;
  multi: boolean;
  /** 未选择（undefined/null/空数组）表示"未激活"，不参与筛选合并。 */
  defaultValue?: unknown;
}

export type DashboardWidget = DashboardChartWidget | DashboardTextWidget | DashboardFilterWidget;

export interface DashboardLayoutDocument {
  version: 1;
  grid: { cols: number };
  widgets: DashboardWidget[];
}

const WIDGET_TYPES: readonly DashboardWidgetType[] = ['chart', 'text', 'filter'];

const OPERATORS: readonly FilterOperator[] = [
  'eq',
  'neq',
  'gt',
  'gte',
  'lt',
  'lte',
  'like',
  'in',
  'notIn',
  'between',
  'isNull',
  'isNotNull',
];

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value !== '';
}

/** 正整数（含 0 视为非法：chartId / datasetId / id 均从 1 起）。 */
function isPositiveInt(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0;
}

/** 有限非负整数，用于栅格四轴。 */
function isNonNegativeInt(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0;
}

export function emptyDashboardLayout(): DashboardLayoutDocument {
  return { version: 1, grid: { cols: DASHBOARD_GRID_COLS }, widgets: [] };
}

/**
 * 生成盘内 widget id。优先 crypto.randomUUID，回退时间戳 + 随机后缀
 * （对齐 store/index.ts 的 createFilterId 范式）。
 */
export function createWidgetId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `w-${crypto.randomUUID()}`;
  }
  return `w-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

/**
 * 把位置收敛进合法栅格：w/h 不小于最小值，x/w 不越界，y 不为负。
 * 越界时的收敛**保证 x + w <= cols**（先收 w 再收 x），避免渲染时被挤出画布。
 */
export function normalizePlacement(
  input: Partial<DashboardPlacement> | undefined,
  fallback: { w: number; h: number },
  cols: number = DASHBOARD_GRID_COLS
): DashboardPlacement {
  const rawW = isNonNegativeInt(input?.w) ? input.w : fallback.w;
  const rawH = isNonNegativeInt(input?.h) ? input.h : fallback.h;
  const w = Math.min(Math.max(rawW, DASHBOARD_MIN_W), cols);
  const h = Math.max(rawH, DASHBOARD_MIN_H);
  const rawX = isNonNegativeInt(input?.x) ? input.x : 0;
  const x = Math.min(rawX, cols - w);
  const y = isNonNegativeInt(input?.y) ? input.y : 0;
  return { x, y, w, h };
}

/** 归一单个 widget；无法修复时返回 null（调用方负责丢弃）。 */
function normalizeWidget(raw: unknown, index: number): DashboardWidget | null {
  if (!isPlainObject(raw)) {
    return null;
  }
  const type = raw.type;
  if (typeof type !== 'string' || !WIDGET_TYPES.includes(type as DashboardWidgetType)) {
    return null;
  }
  const widgetType = type as DashboardWidgetType;

  // 缺 widgetId 时按位置补一个**确定性** id：迁移函数保持纯函数语义（同输入同输出），
  // 且不丢失这块内容——比直接丢弃更保守。
  const widgetId = isNonEmptyString(raw.widgetId) ? raw.widgetId : `w-recovered-${index}`;

  const placement = normalizePlacement(raw, DASHBOARD_DEFAULT_SIZE[widgetType]);

  if (widgetType === 'chart') {
    // 没有合法 chartId 的图表块取不到数，且无从修复 → 丢弃。
    if (!isPositiveInt(raw.chartId)) {
      return null;
    }
    const widget: DashboardChartWidget = {
      widgetId,
      type: 'chart',
      chartId: raw.chartId,
      ...placement,
    };
    if (typeof raw.titleOverride === 'string') {
      widget.titleOverride = raw.titleOverride;
    } else if (raw.titleOverride === null) {
      widget.titleOverride = null;
    }
    return widget;
  }

  if (widgetType === 'text') {
    if (typeof raw.markdown !== 'string') {
      return null;
    }
    return { widgetId, type: 'text', markdown: raw.markdown, ...placement };
  }

  // filter
  const binding = raw.binding;
  if (
    !isPlainObject(binding) ||
    !isPositiveInt(binding.datasetId) ||
    !isNonEmptyString(binding.column)
  ) {
    return null;
  }
  const widget: DashboardFilterWidget = {
    widgetId,
    type: 'filter',
    binding: { datasetId: binding.datasetId, column: binding.column },
    label: isNonEmptyString(raw.label) ? raw.label : binding.column,
    // 历史布局可能存有 number/json/timestamp 等旧词，读取时归一为规范词表
    dataType: normalizeDataType(typeof raw.dataType === 'string' ? raw.dataType : 'string'),
    operator: OPERATORS.includes(raw.operator as FilterOperator)
      ? (raw.operator as FilterOperator)
      : 'in',
    multi: raw.multi === true,
    ...placement,
  };
  if (raw.defaultValue !== undefined) {
    widget.defaultValue = raw.defaultValue;
  }
  return widget;
}

/**
 * 把任意 bi_dashboard.layout_json 字符串转为合法 v1 文档。
 *
 * @param raw layout_json 的 JSON 字符串（可能是空串、损坏内容、旧结构或未来版本）
 */
export function migrateDashboardLayout(raw: string): DashboardLayoutDocument {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return emptyDashboardLayout();
  }
  if (!isPlainObject(parsed)) {
    return emptyDashboardLayout();
  }

  // v1 恒定 12 列：原文档的 grid.cols 一律归一到渲染列数（见文件头关于 >12 列的说明）。
  const cols = DASHBOARD_GRID_COLS;
  const source = parsed.widgets;
  if (!Array.isArray(source)) {
    return { version: 1, grid: { cols }, widgets: [] };
  }

  const widgets: DashboardWidget[] = [];
  source.forEach((entry, index) => {
    const widget = normalizeWidget(entry, index);
    if (widget) {
      widgets.push(widget);
    }
  });

  return { version: 1, grid: { cols }, widgets };
}

/** 序列化为落库字符串。布局全量覆写，故 PUT 时直接替换整串。 */
export function serializeDashboardLayout(doc: DashboardLayoutDocument): string {
  return JSON.stringify(doc);
}

/** 盘内被引用的 chartId 去重集合（供取数/引用校验使用，保持首次出现顺序）。 */
export function collectChartIds(doc: DashboardLayoutDocument): number[] {
  const seen = new Set<number>();
  const ids: number[] = [];
  for (const widget of doc.widgets) {
    if (widget.type === 'chart' && !seen.has(widget.chartId)) {
      seen.add(widget.chartId);
      ids.push(widget.chartId);
    }
  }
  return ids;
}

/** 筛选器块是否"已激活"（有值才参与筛选合并，见 PRD §8.3 步骤 2）。 */
export function isFilterActive(widget: DashboardFilterWidget): boolean {
  if (widget.operator === 'isNull' || widget.operator === 'isNotNull') {
    return true;
  }
  const value = widget.defaultValue;
  if (value === undefined || value === null || value === '') {
    return false;
  }
  if (Array.isArray(value)) {
    return value.length > 0;
  }
  return true;
}
