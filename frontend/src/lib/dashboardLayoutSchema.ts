/**
 * bi_dashboard.layout_json 的文档 schema 与迁移函数（纯逻辑，不依赖 UI/store 运行时）。
 *
 * 与 bi_chart.config 的关系：两者是**独立**的版本化文档，各自带 version 与迁移函数。
 * 仪表盘文档只**引用**图表（存 chartId），不内嵌图表 config——图表配置的演进归
 * `chartConfigSchema.migrateChartConfig` 管，本文件不复制一份会漂移的类型定义。
 *
 * v2 约定（本版本，v1 是「单页」）：
 * - `grid.cols` 恒定 12。**注意**：若将来引入 >12 列的文档，本函数必须改为"按原
 *   grid.cols 缩放后再归一到渲染列数"，否则会静默位移既有块。不存在这种文档，
 *   故此处直接归一到 12 并在超界时收敛。
 * - **多页面**：一个盘有 `pages[]`（至少一页），每个 widget 用 `pageId` 归属恰好一页。
 *   页面顺序 **就是 `pages` 数组顺序**（拖拽排序即重排数组）。
 * - 页面归属用**扁平** `widgets[] + pageId` 表达，而不是把 widgets 嵌进 page 里。
 *   这是刻意的：后端 `service/dashboard/query.go` 的投影与
 *   `impl.go` 的引用计数 jsonpath（`$.widgets[*] ? (@.chartId == …)`）都建立在这个
 *   扁平形状上，嵌套会让那两个「按块」的读路径全部失效；扁平则只需多读一个属性。
 *   代价是 `widgetId` 的唯一域是**整盘**而非单页（与 v1 的 D1 不变量一致）。
 * - 每个 widget 持有**盘内唯一**的 `widgetId`。同一 chartId 允许在同一盘出现多次，
 *   各自独立 x/y/w/h——这是"同图复用"能力的实现基础，故 widgetId 绝不等于 chartId。
 * - 三类**内容** widget：`chart`（引用）/ `text`（Markdown）/ `filter`（绑定数据集字段）；
 *   另有一类**分组** widget：`container`（查询容器，issue #154）——它本身是一块栅格，
 *   只做「把若干筛选器归拢到一处并支持置顶」的**视觉分组**，不参与取数：它没有 binding /
 *   chartId，后端 `projectLayout` 的 switch 天然跳过它，容器里的筛选器仍靠自身的
 *   `pageId` / `scope` 走原路下发。被归拢的筛选器带一个可选 `containerId`（指向所属容器的
 *   widgetId，与 `pageId` 同为**扁平跨块引用**，不嵌套数组，以免破坏 `$.widgets[*]` 的读路径）。
 *   筛选器绑定的是 `(datasetId, column)` 二元组而非纯列名：因允许跨数据集混搭，
 *   纯列名会让不同数据集里的同名列互相误伤。
 * - 筛选器另有 `scope`：缺省 `'page'` 只作用于**所在页**；`'all'` 作用于**所有页**
 *   （取数时任意页都下发它）。作用于哪一页是这条筛选器自身的属性，故存在 widget
 *   上而不是盘级设置里。
 * - 占位与错误是**运行期渲染行为**，不进文档：图表被软删后 widget 仍保留原位，
 *   由取数响应里的 status 决定渲染占位块。文档层不做任何级联删除。
 *
 * 迁移是全覆盖函数：任何输入（空串、损坏 JSON、非对象、字段缺失/类型错误）都返回
 * 合法 v2 文档，绝不抛异常。无法修复的 widget 被丢弃（例如 chart 块没有合法 chartId
 * ——它取不到数），可修复的则就地修复（缺失 widgetId 按位置补确定性 id、位置超界收敛、
 * 页面引用悬空/重复时归到首个页面）。**v1 → v2 是无损的**：v1 的 widgets 全部落到
 * 迁移合成的那一页上。
 *
 * 两个默认名（`页面 1` / `未命名页面`）是**数据层**取值而非 UI 文案：它们会成为落库
 * 内容、由用户改名，不随界面语言切换（与 `w-recovered-<n>` 这类修复产物同一性质）。
 */

import type { FilterOperator } from '../store';
import { normalizeDataType } from './dataTypes';
import type { DateGranularity, WeekStart } from './dateFilter';
import { isDateGranularity, isWeekStart } from './dateFilter';

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
  // 查询容器默认横贯整行、只占两行高——它是控件条，不是内容块。
  container: { w: 12, h: 2 },
};

export type DashboardWidgetType = 'chart' | 'text' | 'filter' | 'container';

/** 栅格位置与尺寸（react-grid-layout 的四轴）。 */
export interface DashboardPlacement {
  x: number;
  y: number;
  w: number;
  h: number;
}

/**
 * 图表联动的一条去向（issue #143）：这块图被点击时，把所选维度值下发到哪个目标块、
 * 落在目标数据集的哪一列上。
 *
 * `column` 是**目标数据集**的列 ID（`DatasetColumn.id`）：同数据集联动时它就是来源那条
 * 维度列，跨数据集联动则由用户在联动设置里显式选一列。把落点写死在布局里（而不是让请求
 * 方指定），联动就与盘级筛选器共享同一条信任边界——列标识只来自已落库文档。
 */
export interface DashboardLinkageTarget {
  /** 目标图表块的 widgetId（盘内唯一，不存 chartId：同图复用要分别联动）。 */
  widgetId: string;
  /** 目标数据集上的列 ID。 */
  column: string;
}

export interface DashboardChartLinkage {
  /** 勾选为联动目标的图表块。空数组等同未配置联动。 */
  targets: DashboardLinkageTarget[];
}

/** 一块 widget 的公共标识：盘内唯一 id + 所属页面。 */
export interface DashboardWidgetBase extends DashboardPlacement {
  widgetId: string;
  /**
   * 所属页面 id。迁移保证它**恒指向 `pages` 里存在的页**：指向不存在页面的块在
   * 渲染上会整块消失（不像文件夹的悬空引用还能退化成根级），所以这里选择归到首页
   * 而不是原样保留。
   */
  pageId: string;
}

export interface DashboardChartWidget extends DashboardWidgetBase {
  type: 'chart';
  /** 引用既有 bi_chart.id；不存快照。 */
  chartId: number;
  /** 仅覆盖盘内显示标题，不写回图表；null/缺省表示用图表自身标题。 */
  titleOverride?: string | null;
  /** 联动设置（issue #143）：缺省 = 不发起联动。 */
  linkage?: DashboardChartLinkage;
}

export interface DashboardTextWidget extends DashboardWidgetBase {
  type: 'text';
  markdown: string;
}

/**
 * 筛选器绑定：数据集 + **列 ID**（`DatasetColumn.id`）。
 *
 * ⚠️ 取值必须是列 ID 而不是列名：盘级条件最终以 `entity.Filter.Field` 传给图表取数，
 * 而「哪些字段被盘级条件覆盖」的判定（`overriddenFields`）拿的是图表自身过滤条件里的
 * **列 ID** 去求交集 —— 存列名会让这个可见标识永远匹配不上。列名可变，只用于展示。
 */
export interface DashboardFilterBinding {
  datasetId: number;
  column: string;
}

/**
 * 筛选器的作用范围：`'page'`（缺省）= 只作用于所在页；`'all'` = 作用于所有页。
 * 缺省值刻意用「键不存在」表达（而不是显式 `'page'`），省得把默认值写进每一份布局。
 */
export type DashboardFilterScope = 'page' | 'all';

export interface DashboardFilterWidget extends DashboardWidgetBase {
  type: 'filter';
  binding: DashboardFilterBinding;
  label: string;
  /** 后端 StandardDataType 原样透传，用于选控件族（日期/数值/字符串）。 */
  dataType: string;
  operator: FilterOperator;
  multi: boolean;
  /** 未选择（undefined/null/空数组）表示"未激活"，不参与筛选合并。 */
  defaultValue?: unknown;
  /** 只有 `'all'` 会被显式写进文档；缺省即 `'page'`。 */
  scope?: DashboardFilterScope;
  /**
   * 日期型筛选器的粒度与周计算逻辑。只对 `dataType` 是 date/datetime 的筛选器有意义：
   * 控件据它决定快捷选项集合、周起始日与展示格式（见 `lib/dateFilter.ts`）。
   * 其余类型的筛选器带上它会被忽略，不影响行为。
   */
  date?: { granularity: DateGranularity; weekStart: WeekStart };
  /**
   * 所属查询容器的 widgetId（issue #154）。缺省 = 这块筛选器是独立栅格块；非空 = 它被归拢进
   * 那个容器、只在容器里渲染，**不再占据顶层栅格**。与 `pageId` 同为扁平跨块引用：迁移时若
   * 指向不存在的容器会被剥掉（退回独立块），以免筛选器凭空消失。
   */
  containerId?: string;
}

/**
 * 查询容器（issue #154）：一块栅格，做筛选器的**视觉分组 + 置顶**，不参与取数（无 binding /
 * chartId，后端投影跳过）。容器成员靠筛选器身上的 `containerId` 反向指过来，而不是把子块嵌进
 * 容器对象里——保持 `widgets[]` 扁平，`$.widgets[*] ? (@.chartId == …)` 与 `projectLayout` 不破。
 */
export interface DashboardContainerWidget extends DashboardWidgetBase {
  type: 'container';
  label: string;
  /** 置顶：渲染时吸附到栅格顶行（y=0）。缺省不置顶。 */
  pinned?: boolean;
}

export type DashboardWidget =
  | DashboardChartWidget
  | DashboardTextWidget
  | DashboardFilterWidget
  | DashboardContainerWidget;

/** 一个页面（独立画布）。顺序即标签顺序。 */
export interface DashboardPage {
  id: string;
  name: string;
}

export interface DashboardLayoutDocument {
  version: 2;
  grid: { cols: number };
  /** 至少一页（迁移保证非空）。 */
  pages: DashboardPage[];
  /** 全部页面的块，用 `pageId` 归属；widgetId 的唯一域是整盘。 */
  widgets: DashboardWidget[];
}

/** 迁移合成页面时用的默认名（数据层取值，不是 UI 文案，见文件头）。 */
export const DASHBOARD_DEFAULT_PAGE_NAME = '页面 1';

/** 页面名为空/非法时的兜底名（数据层取值，见文件头）。 */
export const DASHBOARD_UNTITLED_PAGE_NAME = '未命名页面';

/** 查询容器名为空/非法时的兜底名（数据层取值，与页面默认名同一性质）。 */
export const DASHBOARD_DEFAULT_CONTAINER_LABEL = '查询容器';

const WIDGET_TYPES: readonly DashboardWidgetType[] = ['chart', 'text', 'filter', 'container'];

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
  return {
    version: 2,
    grid: { cols: DASHBOARD_GRID_COLS },
    pages: [{ id: 'p-recovered-0', name: DASHBOARD_DEFAULT_PAGE_NAME }],
    widgets: [],
  };
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

/** 生成页面 id（与 widgetId 同一范式，但前缀区分，便于日志里一眼认出）。 */
export function createPageId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `p-${crypto.randomUUID()}`;
  }
  return `p-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

/**
 * 归一页面数组：丢弃非对象项、按位置补齐缺失 id、**首个同 id 胜出**。
 * 结果保证非空——空文档也要有一页可编辑，否则「加图表」无处可放。
 */
export function normalizePages(raw: unknown): DashboardPage[] {
  const pages: DashboardPage[] = [];
  const seen = new Set<string>();
  if (Array.isArray(raw)) {
    raw.forEach((entry, index) => {
      if (!isPlainObject(entry)) {
        return;
      }
      const id = isNonEmptyString(entry.id) ? entry.id : `p-recovered-${index}`;
      if (seen.has(id)) {
        return;
      }
      seen.add(id);
      pages.push({
        id,
        name: isNonEmptyString(entry.name) ? entry.name : DASHBOARD_UNTITLED_PAGE_NAME,
      });
    });
  }
  if (pages.length === 0) {
    pages.push({ id: 'p-recovered-0', name: DASHBOARD_DEFAULT_PAGE_NAME });
  }
  return pages;
}

/**
 * 把 `fromId` 页移动到 `toId` 页**原来的位置**（拖拽落点的语义）。
 * 纯函数；任一 id 不存在或两者相同则原样返回（复制一份，不泄漏引用）。
 */
export function reorderPages(
  pages: readonly DashboardPage[],
  fromId: string,
  toId: string
): DashboardPage[] {
  const from = pages.findIndex((page) => page.id === fromId);
  const to = pages.findIndex((page) => page.id === toId);
  if (from < 0 || to < 0 || from === to) {
    return [...pages];
  }
  const next = [...pages];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved);
  return next;
}

/** 挑出布局里的筛选器块（类型守卫版本，多处复用）。 */
export function filterWidgetsOf(widgets: readonly DashboardWidget[]): DashboardFilterWidget[] {
  return widgets.filter((widget): widget is DashboardFilterWidget => widget.type === 'filter');
}

/** 挑出布局里的查询容器块（issue #154）。 */
export function containerWidgetsOf(
  widgets: readonly DashboardWidget[]
): DashboardContainerWidget[] {
  return widgets.filter(
    (widget): widget is DashboardContainerWidget => widget.type === 'container'
  );
}

/**
 * 某一页**渲染在顶层栅格**上的块：该页的 chart / text / container，加上**尚未被归拢**的筛选器
 * （`containerId` 为空者）。被归拢进容器的筛选器由容器自行渲染，不占顶层栅格，故在此排除。
 */
export function gridWidgetsOfPage(
  widgets: readonly DashboardWidget[],
  pageId: string
): DashboardWidget[] {
  return widgetsOfPage(widgets, pageId).filter(
    (widget) => !(widget.type === 'filter' && widget.containerId)
  );
}

/**
 * 某容器的成员筛选器（`containerId` 命中且归属同一页，保持文档内顺序）。
 * 只取该容器所在页的筛选器——容器与成员本就同页（成员 `containerId` 由编辑器在同页内设置）。
 */
export function childrenOfContainer(
  widgets: readonly DashboardWidget[],
  container: DashboardContainerWidget
): DashboardFilterWidget[] {
  return filterWidgetsOf(widgets).filter(
    (widget) => widget.containerId === container.widgetId && widget.pageId === container.pageId
  );
}

/** 挑出某一页的全部块（保持文档内顺序）。 */
export function widgetsOfPage(
  widgets: readonly DashboardWidget[],
  pageId: string
): DashboardWidget[] {
  return widgets.filter((widget) => widget.pageId === pageId);
}

/**
 * 某一页取数时真正该下发的筛选器：**本页的** + **任意页里 `scope: 'all'` 的**。
 *
 * 与后端 `projectLayout` 的按页收窄口径一致（那里按 `pageId` 与 `scope` 两条判定），
 * 两边一起改才不会出现「前端不发、后端也认不出」的死筛选器。
 */
export function applicableFilterWidgets(
  widgets: readonly DashboardWidget[],
  pageId: string
): DashboardFilterWidget[] {
  return filterWidgetsOf(widgets).filter(
    (widget) => widget.pageId === pageId || widget.scope === 'all'
  );
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

/**
 * 在栅格上找首个能容纳 `size` 的空位（自上而下、自左而右）。
 *
 * 替代原先「新块一律 `x: 0`、落在最底边之下」的硬编码：12 列画布上默认 6 列宽的块
 * 会因此永远堆在左半边、右半边长期空置（浏览器验收 D-3）。vertical compactor 只做
 * **纵向**吸附，不会把块横向挪进空位，所以这个选择必须在建块时就做对。
 *
 * 纯函数、不修改入参；`widgets` 被填满时回落到最底边之下（与旧行为一致）。
 */
export function findFreePlacement(
  widgets: readonly DashboardPlacement[],
  size: { w: number; h: number },
  cols: number = DASHBOARD_GRID_COLS
): { x: number; y: number } {
  const w = Math.min(Math.max(size.w, DASHBOARD_MIN_W), cols);
  const h = Math.max(size.h, DASHBOARD_MIN_H);
  const bottom = widgets.reduce((max, widget) => Math.max(max, widget.y + widget.h), 0);
  const overlaps = (x: number, y: number) =>
    widgets.some(
      (widget) =>
        x < widget.x + widget.w && widget.x < x + w && y < widget.y + widget.h && widget.y < y + h
    );
  for (let y = 0; y <= bottom; y += 1) {
    for (let x = 0; x + w <= cols; x += 1) {
      if (!overlaps(x, y)) {
        return { x, y };
      }
    }
  }
  return { x: 0, y: bottom };
}

/**
 * 归一 chart 块的联动配置：丢掉声明不了条件的去向（缺 widgetId / 缺列 ID——与后端
 * `projectLinkageTargets` 同一口径，两侧都丢才不会出现「前端显示已联动、后端不生效」），
 * 同一目标只保留首条。一个去向都不剩时整个 linkage 键不落盘。
 */
function normalizeLinkage(raw: unknown): DashboardChartLinkage | undefined {
  if (!isPlainObject(raw) || !Array.isArray(raw.targets)) {
    return undefined;
  }
  const targets: DashboardLinkageTarget[] = [];
  const seen = new Set<string>();
  for (const entry of raw.targets) {
    if (!isPlainObject(entry)) {
      continue;
    }
    if (!isNonEmptyString(entry.widgetId) || !isNonEmptyString(entry.column)) {
      continue;
    }
    if (seen.has(entry.widgetId)) {
      continue;
    }
    seen.add(entry.widgetId);
    targets.push({ widgetId: entry.widgetId, column: entry.column });
  }
  return targets.length > 0 ? { targets } : undefined;
}

/** 归一单个 widget；无法修复时返回 null（调用方负责丢弃）。 */
function normalizeWidget(
  raw: unknown,
  index: number,
  pageIds: ReadonlySet<string>,
  fallbackPageId: string
): DashboardWidget | null {
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

  // 页面归属：悬空（指向不存在的页）或缺省一律归到首页。归首页而不是丢弃，
  // 是因为「块在渲染上凭空消失」比「块换了位置」难懂得多。
  const pageId =
    isNonEmptyString(raw.pageId) && pageIds.has(raw.pageId) ? raw.pageId : fallbackPageId;

  const placement = normalizePlacement(raw, DASHBOARD_DEFAULT_SIZE[widgetType]);

  if (widgetType === 'chart') {
    // 没有合法 chartId 的图表块取不到数，且无从修复 → 丢弃。
    if (!isPositiveInt(raw.chartId)) {
      return null;
    }
    const widget: DashboardChartWidget = {
      widgetId,
      pageId,
      type: 'chart',
      chartId: raw.chartId,
      ...placement,
    };
    if (typeof raw.titleOverride === 'string') {
      widget.titleOverride = raw.titleOverride;
    } else if (raw.titleOverride === null) {
      widget.titleOverride = null;
    }
    const linkage = normalizeLinkage(raw.linkage);
    if (linkage) {
      widget.linkage = linkage;
    }
    return widget;
  }

  if (widgetType === 'text') {
    if (typeof raw.markdown !== 'string') {
      return null;
    }
    return { widgetId, pageId, type: 'text', markdown: raw.markdown, ...placement };
  }

  if (widgetType === 'container') {
    // 容器只承载 label / pinned，缺 label 兜底为默认名（可修复，不丢弃）。
    const widget: DashboardContainerWidget = {
      widgetId,
      pageId,
      type: 'container',
      label: isNonEmptyString(raw.label) ? raw.label : DASHBOARD_DEFAULT_CONTAINER_LABEL,
      ...placement,
    };
    if (raw.pinned === true) {
      widget.pinned = true;
    }
    return widget;
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
    pageId,
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
  // 只有显式 'all' 才写进文档；其余（含 'page' / 脏值）都按缺省处理，省得把默认值
  // 散落进每一份布局里。
  if (raw.scope === 'all') {
    widget.scope = 'all';
  }
  // 日期粒度 / 周计算逻辑：两个字段都合法才采纳。半截配置当没有——宁可退回默认口径，
  // 也不要拿一个「粒度来自布局、周起始日来自默认值」的混合配置去求值。
  const dateConfig = raw.date;
  if (
    isPlainObject(dateConfig) &&
    isDateGranularity(dateConfig.granularity) &&
    isWeekStart(dateConfig.weekStart)
  ) {
    widget.date = { granularity: dateConfig.granularity, weekStart: dateConfig.weekStart };
  }
  // containerId 的悬空校验（指向不存在的容器就退回独立块）在 migrateDashboardLayout
  // 建完全盘容器索引后做后置剥离——这里只做类型层面的采纳。
  if (isNonEmptyString(raw.containerId)) {
    widget.containerId = raw.containerId;
  }
  return widget;
}

/**
 * 把任意 bi_dashboard.layout_json 字符串转为合法 v2 文档。
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

  // 恒定 12 列：原文档的 grid.cols 一律归一到渲染列数（见文件头关于 >12 列的说明）。
  const cols = DASHBOARD_GRID_COLS;
  const pages = normalizePages(parsed.pages);
  const pageIds = new Set(pages.map((page) => page.id));
  const fallbackPageId = pages[0].id;
  const source = parsed.widgets;
  if (!Array.isArray(source)) {
    return { version: 2, grid: { cols }, pages, widgets: [] };
  }

  const widgets: DashboardWidget[] = [];
  source.forEach((entry, index) => {
    const widget = normalizeWidget(entry, index, pageIds, fallbackPageId);
    if (widget) {
      widgets.push(widget);
    }
  });

  // 后置剥离悬空 containerId：全盘容器索引建好后才能判定成员指向是否存在。指向不存在
  // 容器的筛选器退回独立栅格块（与悬空 pageId 归首页同理——宁可回到可见状态，也不要让
  // 筛选器凭空消失）。
  const containerIds = new Set(
    widgets
      .filter((w): w is DashboardContainerWidget => w.type === 'container')
      .map((w) => w.widgetId)
  );
  for (const widget of widgets) {
    if (widget.type === 'filter' && widget.containerId && !containerIds.has(widget.containerId)) {
      delete widget.containerId;
    }
  }

  return { version: 2, grid: { cols }, pages, widgets };
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
