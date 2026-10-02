/**
 * 表格维度列纵向合并（issue #156 AC3）的纯逻辑：把「同值相邻」的维度单元格并成一条。
 * 零 React/antd 依赖，TableChart 与测试共用同一实现。
 *
 * 语义（issue 里「排序语义评审时定稿」，这里取最直白的一种，且只作用于展示层）：
 * - 只合并**相邻**且取值相同的单元格；中间插入任何不同取值即断成新的一组。
 *   于是合并结果与行顺序强相关——排序一变，分组随之重算（这正是"相邻"的含义）。
 * - 一段同值组里，首个单元格 rowSpan = 组长度，组内其余单元格 rowSpan = 0
 *   （antd 的合并语义：0 表示被上一格吞并、本行不再渲染该格）。
 * - **空值（null/undefined/空串）不合并、也不参与同值判定**：一「同值」指确有相同取值，
 *   二把空值并成一大块会让行与行之间再也分不清边界。空值单元格各自占一行，且会打断同值组。
 * - 取值按归一后的字符串比较，故 1 与 '1' 视为同值；非有限数值（NaN/Infinity）
 *   与对象/数组/日期等不作合并（"看起来一样"与"同一个值"无从界定）。
 * - 只在**当前渲染的这批行**上计算（服务端分页下即当前页）：跨页本就不相邻，
 *   强行跨页合并会把两页的连接处算成一段。
 */

/**
 * 归一化的同值判定键；不参与合并的取值返回 null。
 * 数字/布尔转字符串后比较，覆盖接口返回的 mixed 类型维度。
 */
function mergeKeyOf(value: unknown): string | null {
  if (value === null || value === undefined) return null;
  if (typeof value === 'string') return value === '' ? null : value;
  if (typeof value === 'number') return Number.isFinite(value) ? String(value) : null;
  if (typeof value === 'boolean') return String(value);
  return null;
}

/**
 * 计算某一列按行顺序的 rowSpan 数组。
 * @param values 该维度列在当前渲染行上的取值（行顺序）
 * @returns 每行的 rowSpan：同值组首为组长度、组内其余为 0；单独一格与不参与合并的取值恒为 1
 */
export function computeRowSpans(values: unknown[]): number[] {
  const n = values.length;
  const spans = new Array<number>(n).fill(1);
  let i = 0;
  while (i < n) {
    const key = mergeKeyOf(values[i]);
    if (key === null) {
      i += 1;
      continue;
    }
    let j = i + 1;
    while (j < n && mergeKeyOf(values[j]) === key) {
      j += 1;
    }
    if (j - i > 1) {
      spans[i] = j - i;
      for (let k = i + 1; k < j; k += 1) {
        spans[k] = 0;
      }
    }
    i = j;
  }
  return spans;
}
