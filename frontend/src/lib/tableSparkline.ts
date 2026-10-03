/**
 * 迷你图列（issue #156 AC4）的纯逻辑：表格查询只回扁平行（一行 = 维度组合 × 一个
 * 日期点），行内 sparkline 的序列由**前端在渲染层分组**得到 —— 按「非日期维度」
 * 分组，组内按日期列升序取指标值。零 React/ECharts 依赖，TableChart 与测试共用。
 *
 * 日期维度列由**数据形状**探测（YYYY-MM-DD 前缀），不依赖列类型元数据 —— 分享页/
 * 仪表盘链路拿不到列 dataType，靠传元数据会把两个外壳再分叉。样本中混入任何非日期
 * 值即判定不命中（宁可不用也不猜错）。
 *
 * 已知边界：服务端分页下分组序列只含当前页的行 —— 翻页后序列会变。这是扁平负载
 * 的固有限制（完整序列需要按维度整组取数），在此如实不做假象。
 */

type Row = Record<string, unknown>;

/** YYYY-MM-DD 前缀（含带时间的 datetime）；月 1-12、日 1-31 才算真日期（2026-13-40 不是） */
const DATE_RE = /^(\d{4})-(\d{2})-(\d{2})/;
const SAMPLE_SIZE = 20;

const isDateLike = (v: unknown): boolean => {
  const m = String(v).match(DATE_RE);
  if (m === null) return false;
  const month = Number(m[2]);
  const day = Number(m[3]);
  return month >= 1 && month <= 12 && day >= 1 && day <= 31;
};

/** 按 dimensionNames 顺序探测第一个「取值是日期形状」的维度列；探测不到返回 null。 */
export function detectDateDimension(rows: Row[], dimensionNames: string[]): string | null {
  for (const dim of dimensionNames) {
    const samples: unknown[] = [];
    for (const row of rows || []) {
      const v = row?.[dim];
      if (v !== null && v !== undefined && v !== '') {
        samples.push(v);
        if (samples.length >= SAMPLE_SIZE) break;
      }
    }
    if (samples.length > 0 && samples.every(isDateLike)) {
      return dim;
    }
  }
  return null;
}

/** 一行数据的分组键：非日期维度取值按顺序 join（控制字符分隔，避免取值粘连歧义）。 */
export function rowGroupKey(row: Row, groupDims: string[]): string {
  return groupDims.map((d) => String(row?.[d] ?? '')).join('\u0001');
}

/**
 * 分组序列：groupDims 各组一条，组内按 dateColumn 升序（YYYY-MM-DD 字典序即时间序）
 * 取 metric 的有限数值点；null/空/NaN/Infinity 剔除。
 */
export function buildSparklineSeries(
  rows: Row[],
  groupDims: string[],
  dateColumn: string,
  metric: string
): Map<string, number[]> {
  const series = new Map<string, { date: string; value: number }[]>();
  for (const row of rows || []) {
    const raw = row?.[metric];
    const value =
      typeof raw === 'number'
        ? raw
        : raw === null || raw === undefined || raw === ''
          ? NaN
          : Number(raw);
    if (!Number.isFinite(value)) continue;
    const date = String(row?.[dateColumn] ?? '');
    if (!isDateLike(date)) continue;
    const key = rowGroupKey(row, groupDims);
    const points = series.get(key) ?? [];
    points.push({ date, value });
    series.set(key, points);
  }
  const result = new Map<string, number[]>();
  for (const [key, points] of series) {
    points.sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0));
    result.set(
      key,
      points.map((p) => p.value)
    );
  }
  return result;
}

/**
 * 数值序列 → SVG polyline 的 points 串（viewBox 坐标系，四周留 1px 边距）。
 * 少于 2 个点返回空串（不画）；全等值画水平中线。
 */
export function sparklinePoints(values: number[], width: number, height: number): string {
  if (!values || values.length < 2) return '';
  const pad = 1;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min;
  const innerW = width - pad * 2;
  const innerH = height - pad * 2;
  return values
    .map((v, i) => {
      const x = pad + (i / (values.length - 1)) * innerW;
      const y = span === 0 ? pad + innerH / 2 : pad + innerH * (1 - (v - min) / span);
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(' ');
}
