/**
 * 首启向导第③步「出第一张图」的配置生成（#226 AC6）。
 *
 * 现场实测：载入示例后把用户送到 `/chart-builder` 的空白画布，图的形状完全靠他自己拖
 * 维度与指标——「5 分钟出第一张图」实测 9 分钟仍无图。本模块用示例数据集的列**直接配好
 * 一份查询文档**：日期维度 + 求和指标，向导把这文档落成一条查询记录，点开
 * `/chart-builder?q=<短码>` 即刻有图（复用 #181 的直链还原路径，不新造入口）。
 *
 * 挑选规则刻意保守：配不出来就返回 null，由调用方降级成「打开空画布」——
 * 宁可不给图，也不给一张查不出数的图。
 */
import type { DatasetColumn } from '../api';
import { buildQuerySpecDocument, type QuerySpecDocument } from './querySpec';

/** 能当数值用（可聚合）的类型。 */
const NUMERIC_TYPES = new Set<string>(['integer', 'float']);
/** 能当时间轴用的类型。 */
const TEMPORAL_TYPES = new Set<string>(['date', 'datetime']);

export interface FirstChartBuild {
  chartType: 'line' | 'bar';
  title: string;
  dimension: DatasetColumn;
  metric: DatasetColumn;
  /** 可直接提交给 POST /api/queries 的记录信封。 */
  spec: QuerySpecDocument;
}

/**
 * 维度列挑选：先找时间列（折线的横轴最需要它），再找维度角色的文本列，
 * 最后才退回任意维度列。刻意**不**优先整数列——示例数据里的整数维度多是
 * `order_id` 这类逐行唯一的值，画出来是一条锯齿噪声，不像图。
 */
function pickDimension(columns: readonly DatasetColumn[]): DatasetColumn | null {
  const dimensions = columns.filter((c) => c.role !== 'metric');
  const byType = (types: Set<string>) => dimensions.find((c) => types.has(c.type)) ?? null;
  return (
    byType(TEMPORAL_TYPES) ??
    byType(new Set(['string'])) ??
    dimensions[0] ??
    columns.find((c) => TEMPORAL_TYPES.has(c.type)) ??
    null
  );
}

/** 指标列挑选：优先维度角色明确的数值列，其次任意数值列。 */
function pickMetric(
  columns: readonly DatasetColumn[],
  dimension: DatasetColumn
): DatasetColumn | null {
  const numeric = columns.filter((c) => c !== dimension && NUMERIC_TYPES.has(c.type));
  return numeric.find((c) => c.role === 'metric') ?? numeric[0] ?? null;
}

/**
 * 用数据集的列配一张示例图。配不出来（没有可用指标）返回 null。
 *
 * 槽位 id 用图型定义里的 `x_axis` / `values`：写成别的 id 会被
 * `normalizeQueryConfigForChartType` 当作溢出绑定搬走，直链还原出来就是空图。
 */
export function buildFirstChartSpec(
  columns: readonly DatasetColumn[],
  title: string
): FirstChartBuild | null {
  if (columns.length === 0) {
    return null;
  }
  const dimension = pickDimension(columns);
  if (!dimension) {
    return null;
  }
  const metric = pickMetric(columns, dimension);
  if (!metric) {
    return null;
  }

  // 有日期维度就用折线（趋势），否则用柱状（分类对比）——两种都是「一眼能看懂」的默认。
  const chartType = TEMPORAL_TYPES.has(dimension.type) ? 'line' : 'bar';
  const dimensionGroups = [
    { id: 'x_axis', bindings: [{ bindingId: 'b-0', fieldId: dimension.id }] },
  ];
  const metricGroups = [{ id: 'values', bindings: [{ bindingId: 'b-1', fieldId: metric.id }] }];

  const spec = buildQuerySpecDocument({
    chartType,
    title,
    queryConfig: {
      dimensionGroups,
      metricGroups,
      filters: [],
    },
    activeQueryConfig: { dimensionGroups, metricGroups },
    dimensionLabels: {},
    metricAggregations: { 'b-1': 'sum' },
    metricAliases: {},
    metricUnits: {},
    metricFormats: {},
    // 与产品默认样式逐字段一致（store 初始值 / chartOptions.DEFAULT_CHART_STYLE 同形）：
    // 首启就冒出「另一套配色/字号」比用默认值更奇怪。空 colors 即走内置调色板。
    chartStyle: { colors: [], smooth: false, tableRowSize: 'small' },
    chartQueryOptions: {},
  });

  return { chartType, title, dimension, metric, spec };
}

/** 示例数据集的识别：后端给它打了 `sample` 标签。 */
export function isSampleDataset(dataset: { tags?: string | null; name?: string }): boolean {
  if (dataset.tags?.includes('sample')) {
    return true;
  }
  // 标签被用户清掉时按名字兜底：示例数据集名字固定为「示例：…」。
  return typeof dataset.name === 'string' && dataset.name.startsWith('示例：');
}

/** 在数据集列表里挑「电商订单」（有日期 + 金额，最适合出示例图），否则取任意示例集。 */
export function pickSampleDataset<T extends { tags?: string | null; name?: string }>(
  datasets: readonly T[]
): T | null {
  const samples = datasets.filter(isSampleDataset);
  return samples.find((d) => d.name?.includes('订单')) ?? samples[0] ?? null;
}
