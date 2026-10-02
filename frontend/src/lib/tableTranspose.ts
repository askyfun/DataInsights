/**
 * 行列转置（issue #156 AC2）的纯逻辑：维度与指标的角色互换**只发生在渲染层**，
 * 不改查询契约 —— 查询仍按原样发，响应行数组在这里换位。
 *
 * 转置语义：
 * - 每个指标变成一行（行标签 = 指标展示名，缺省列名）；
 * - 每个原始数据行变成一列（列标题 = 该行各维度取值按维度顺序 join，`/` 分隔；
 *   空值不进 join，全空时回落「行 N」）；
 * - 数据行数超过 maxColumns（缺省 50）时截断并置 truncated —— 服务端分页一页可达
 *   100 行，不截断会渲出上百列。
 *
 * 没有维度或没有数据时返回 null：没有行标题可当列名，转置无从谈起，调用方应回落
 * 普通渲染而不是渲染一张空表。
 */

export interface TransposedColumn {
  key: string;
  title: string;
}

export interface TransposedRow {
  /** 指标的输出列名（原始命名空间） */
  metricKey: string;
  /** 指标展示名（metricLabels 优先，缺省列名） */
  metricLabel: string;
  /** 键为数据列 key（c0/c1/…），值即原始单元格值（格式化留给渲染层） */
  values: Record<string, unknown>;
}

export interface TransposedTable {
  /** 第 0 列固定是指标标签列（key = __metric__），其余每列对应一条原始数据行 */
  columns: TransposedColumn[];
  rows: TransposedRow[];
  /** 数据行数超过 maxColumns 被截断时为 true，调用方可提示「仅展示前 N 列」 */
  truncated: boolean;
}

/** 指标标签列的常量 key（不与真实数据列冲突的占位命名空间） */
export const TRANSPOSE_METRIC_KEY = '__metric__';

const isBlank = (v: unknown): boolean => v === null || v === undefined || v === '';

export function transposeTable(
  data: Record<string, unknown>[],
  dimensionNames: string[],
  opts?: {
    /** 指标输出列名 → 展示名（缺省列名） */
    metricLabels?: Record<string, string>;
    /** 数据列上限，缺省 50 */
    maxColumns?: number;
  }
): TransposedTable | null {
  if (!data || data.length === 0 || !dimensionNames || dimensionNames.length === 0) {
    return null;
  }
  const maxColumns = opts?.maxColumns ?? 50;

  // 指标列 = 全部数据列中不属于维度的（按首行出现序；后续行新出现的键追加在尾）。
  const dimSet = new Set(dimensionNames);
  const metricKeys: string[] = [];
  const seen = new Set<string>();
  for (const row of data) {
    for (const key of Object.keys(row)) {
      if (!seen.has(key)) {
        seen.add(key);
        if (!dimSet.has(key)) {
          metricKeys.push(key);
        }
      }
    }
  }
  if (metricKeys.length === 0) {
    return null;
  }

  const kept = data.slice(0, maxColumns);
  const columns: TransposedColumn[] = [
    { key: TRANSPOSE_METRIC_KEY, title: '指标' },
    ...kept.map((row, i) => ({
      key: `c${i}`,
      title: columnTitle(row, dimensionNames, i),
    })),
  ];
  const rows: TransposedRow[] = metricKeys.map((metricKey) => ({
    metricKey,
    metricLabel: opts?.metricLabels?.[metricKey] || metricKey,
    values: Object.fromEntries(kept.map((row, i) => [`c${i}`, row[metricKey]])),
  }));
  return { columns, rows, truncated: data.length > kept.length };
}

/** 数据列标题：各维度取值按维度顺序 join；全空回落「行 N」（1 起）。 */
function columnTitle(
  row: Record<string, unknown>,
  dimensionNames: string[],
  index: number
): string {
  const parts = dimensionNames
    .map((d) => row?.[d])
    .filter((v) => !isBlank(v))
    .map((v) => String(v));
  return parts.length > 0 ? parts.join('/') : `行 ${index + 1}`;
}
