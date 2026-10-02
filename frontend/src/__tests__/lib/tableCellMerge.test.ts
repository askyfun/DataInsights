import { describe, expect, it } from 'vitest';
import { computeRowSpans } from '../../lib/tableCellMerge';

/**
 * 表格维度列纵向合并（issue #156 AC3）的纯逻辑。
 * TableChart 只负责把这里算出的 rowSpan 挂到单元格上，语义判定全在本模块。
 */
describe('computeRowSpans', () => {
  it('相邻同值合并成一组：组首拿组长度，组内其余为 0', () => {
    expect(computeRowSpans(['A', 'A', 'B'])).toEqual([2, 0, 1]);
  });

  it('整列同值合并为一个跨全列的单元格', () => {
    expect(computeRowSpans(['A', 'A', 'A'])).toEqual([3, 0, 0]);
  });

  it('同值但不相邻不合并（中间隔着别的取值即断开）', () => {
    expect(computeRowSpans(['A', 'B', 'A'])).toEqual([1, 1, 1]);
  });

  it('空值不参与合并：null/undefined/空串各自占一行', () => {
    expect(computeRowSpans([null, null, undefined, '', ''])).toEqual([1, 1, 1, 1, 1]);
  });

  it('空值把同值组打断：A 与 A 之间夹着 null 就不再合并', () => {
    expect(computeRowSpans(['A', null, 'A'])).toEqual([1, 1, 1]);
  });

  it("数字与字符串按归一化取值判定同值（1 与 '1' 视为同值）", () => {
    expect(computeRowSpans([1, '1', 1])).toEqual([3, 0, 0]);
  });

  it('布尔值可合并', () => {
    expect(computeRowSpans([true, true, false])).toEqual([2, 0, 1]);
  });

  it('非有限数值（NaN/Infinity）不合并', () => {
    expect(computeRowSpans([Number.NaN, Number.NaN])).toEqual([1, 1]);
    expect(computeRowSpans([Number.POSITIVE_INFINITY, Number.POSITIVE_INFINITY])).toEqual([1, 1]);
  });

  it('空数组返回空跨度', () => {
    expect(computeRowSpans([])).toEqual([]);
  });

  it('多组交替：每组各自算跨度', () => {
    expect(computeRowSpans(['A', 'A', 'B', 'B', 'B', 'C'])).toEqual([2, 0, 3, 0, 0, 1]);
  });
});
