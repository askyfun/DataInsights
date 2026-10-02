/**
 * 表格条件格式（issue #156 AC1）的纯逻辑：规则 → 单元格背景色。
 * 零 React/antd 依赖，TableChart 与测试共用同一实现。
 *
 * 三种模式：
 * - threshold：算子 + 阈值，命中着色（常用于「超线标红」）；
 * - scale：按列内数值 min/max 归一，minColor→maxColor 插值（色阶）；
 * - diff：值 >0 涨色 / <0 跌色 / 0 不着色，缺省遵循国内惯例涨红跌绿，
 *   可通过 upColor/downColor 反转。
 *
 * 颜色一律 hex 字符串（#rgb / #rrggbb），输出归一为 #rrggbb。
 */

export type ConditionalFormatKind = 'threshold' | 'scale' | 'diff';

export interface ConditionalFormat {
  /** 目标指标列的**输出列名**（与 TableChart 的 dataIndex 同一命名空间）。 */
  metric: string;
  kind: ConditionalFormatKind;
  /** threshold 算子；缺省视为规则不完整（不着色）。 */
  op?: '>' | '<' | '>=' | '<=' | '=';
  /** threshold 阈值。 */
  value?: number;
  /** threshold 命中色；缺省琥珀。 */
  color?: string;
  /** scale 下限色；缺省白。 */
  minColor?: string;
  /** scale 上限色；缺省主蓝。 */
  maxColor?: string;
  /** diff 涨色；缺省红（国内惯例涨红跌绿，反转由调用方对调两色实现）。 */
  upColor?: string;
  /** diff 跌色；缺省绿。 */
  downColor?: string;
}

export const DEFAULT_THRESHOLD_COLOR = '#fa8c16';
export const DEFAULT_SCALE_MIN_COLOR = '#ffffff';
export const DEFAULT_SCALE_MAX_COLOR = '#1677ff';
export const DEFAULT_UP_COLOR = '#cf1322';
export const DEFAULT_DOWN_COLOR = '#389e0d';

/** hex → [r,g,b]；非法输入返回 null（规则里的脏颜色按缺省处理，不抛错）。 */
export function hexToRgb(hex: string): [number, number, number] | null {
  let h = hex.trim().replace(/^#/, '');
  if (h.length === 3) {
    h = h
      .split('')
      .map((c) => c + c)
      .join('');
  }
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return null;
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}

const toHex2 = (n: number) => n.toString(16).padStart(2, '0');

/** 线性插值两 hex 色，t 钳到 [0,1]，输出 #rrggbb。任一端非法返回 null。 */
export function interpolateColor(a: string, b: string, t: number): string | null {
  const ra = hexToRgb(a);
  const rb = hexToRgb(b);
  if (!ra || !rb) return null;
  const k = Math.min(1, Math.max(0, t));
  const mix = ra.map((v, i) => Math.round(v + (rb[i] - v) * k));
  return `#${toHex2(mix[0])}${toHex2(mix[1])}${toHex2(mix[2])}`;
}

export function thresholdHit(
  value: number,
  op: '>' | '<' | '>=' | '<=' | '=',
  threshold: number
): boolean {
  switch (op) {
    case '>':
      return value > threshold;
    case '<':
      return value < threshold;
    case '>=':
      return value >= threshold;
    case '<=':
      return value <= threshold;
    case '=':
      return value === threshold;
  }
}

/** 单元格取数值；非有限数值（字符串/空值/NaN）返回 null。 */
function numericOf(v: unknown): number | null {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    return Number.isFinite(n) ? n : null;
  }
  return null;
}

/**
 * 解析某单元格的背景色；不着色返回 undefined。
 * @param rule        条件格式规则
 * @param cellValue   当前单元格原始值
 * @param columnValues 该指标列在**当前渲染数据**里的全部取值（scale 归一分母）
 * @param cellMetric  当前单元格所属列名；传入且与 rule.metric 不符时直接不着色
 */
export function resolveCellBackground(
  rule: ConditionalFormat,
  cellValue: unknown,
  columnValues: unknown[],
  cellMetric?: string
): string | undefined {
  if (cellMetric !== undefined && cellMetric !== rule.metric) return undefined;

  if (rule.kind === 'threshold') {
    const v = numericOf(cellValue);
    if (
      v === null ||
      rule.op === undefined ||
      rule.value === undefined ||
      !Number.isFinite(rule.value)
    ) {
      return undefined;
    }
    return thresholdHit(v, rule.op, rule.value)
      ? (rule.color ?? DEFAULT_THRESHOLD_COLOR)
      : undefined;
  }

  if (rule.kind === 'scale') {
    const v = numericOf(cellValue);
    if (v === null) return undefined;
    const nums = columnValues.map(numericOf).filter((n): n is number => n !== null);
    if (nums.length === 0) return undefined;
    const min = Math.min(...nums);
    const max = Math.max(...nums);
    const t = max > min ? (v - min) / (max - min) : 0;
    return (
      interpolateColor(
        rule.minColor ?? DEFAULT_SCALE_MIN_COLOR,
        rule.maxColor ?? DEFAULT_SCALE_MAX_COLOR,
        t
      ) ?? undefined
    );
  }

  // diff
  const v = numericOf(cellValue);
  if (v === null || v === 0) return undefined;
  return v > 0 ? (rule.upColor ?? DEFAULT_UP_COLOR) : (rule.downColor ?? DEFAULT_DOWN_COLOR);
}
