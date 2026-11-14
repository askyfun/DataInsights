/**
 * 图型 × 方言能力门控（R-78）——纯逻辑，零 UI 依赖。
 *
 * 门控口径的唯一来源是后端 `Capabilities()` 探针投影：只有 `boxplot` 是硬门控
 * （`executor.executeBoxplot` 在 `PercentileStrategy === "unsupported"` 时直接拒绝取数）；
 * `pivot` 缺 GROUPING SETS 会自动回退 UNION ALL、仍能出图，故不门控。
 *
 * 拿不到能力（请求失败 / 未选数据集）时一律返回 undefined —— 按现状放开，
 * 不因探测缺失误伤本可用的图型。
 */
import type { DatasourceCapabilities } from '../api';
import type { ChartType } from './chartConfigSchema';

const DIALECT_LABELS: Record<string, string> = {
  postgresql: 'PostgreSQL',
  mysql: 'MySQL',
  clickhouse: 'ClickHouse',
  starrocks: 'StarRocks',
};

/** 方言展示名；未知方言原样回显，避免编造名字。 */
export function dialectLabel(dialect: string): string {
  return DIALECT_LABELS[dialect] ?? dialect;
}

/** 需要百分位能力的图型（与 `CheckPercentileSupport` 同一门控面）。 */
const PERCENTILE_CHART_TYPES: ReadonlySet<ChartType> = new Set(['boxplot']);

/**
 * 返回该图型在当前方言下不可用的原因；undefined = 可用（或能力未知，按现状放开）。
 */
export function chartUnavailableReason(
  chartType: ChartType,
  capabilities: DatasourceCapabilities | null | undefined
): string | undefined {
  if (!capabilities || !PERCENTILE_CHART_TYPES.has(chartType)) {
    return undefined;
  }
  if (capabilities.percentile_strategy !== 'unsupported') {
    return undefined;
  }
  return `${dialectLabel(capabilities.dialect)} 暂不支持箱线图（缺少百分位计算能力）`;
}
