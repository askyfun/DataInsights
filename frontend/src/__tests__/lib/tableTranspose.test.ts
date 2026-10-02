import { describe, expect, it } from 'vitest';
import { type TransposedTable, transposeTable } from '@/lib/tableTranspose';

/**
 * 行列转置（issue #156 AC2）的纯逻辑契约：维度与指标的角色互换**只发生在渲染层**，
 * 不改查询契约。转置后：每个指标变成一行，每个原始数据行变成一列（列标题 = 该行
 * 维度取值 join）。没有维度或没有数据时返回 null —— 没有行标题可当列名，转置无从
 * 谈起，调用方回落普通渲染。
 */
const rows = [
  { region: '华东', revenue: 100, cost: 40 },
  { region: '华北', revenue: 80, cost: 30 },
];

/** 期望转置成功的断言辅助：为 null 时显式失败（不用非空断言）。 */
function mustTranspose(
  data: Record<string, unknown>[],
  dimensionNames: string[],
  opts?: Parameters<typeof transposeTable>[2]
): TransposedTable {
  const t = transposeTable(data, dimensionNames, opts);
  if (!t) {
    throw new Error('expected transposeTable to return a table');
  }
  return t;
}

describe('transposeTable', () => {
  it('指标变行、原始数据行变列，首列为指标标签列', () => {
    const t = mustTranspose(rows, ['region']);
    expect(t.columns).toEqual([
      { key: '__metric__', title: '指标' },
      { key: 'c0', title: '华东' },
      { key: 'c1', title: '华北' },
    ]);
    expect(t.rows.map((r) => r.metricKey)).toEqual(['revenue', 'cost']);
    expect(t.rows[0].values).toEqual({ c0: 100, c1: 80 });
    expect(t.rows[1].values).toEqual({ c0: 40, c1: 30 });
  });

  it('多维度时列标题按维度顺序 join；指标行标签可用 metricLabels 换展示名', () => {
    const t = mustTranspose(
      [
        { region: '华东', city: '上海', revenue: 1 },
        { region: '华东', city: '杭州', revenue: 2 },
      ],
      ['region', 'city'],
      { metricLabels: { revenue: '销售额' } }
    );
    expect(t.columns[1].title).toBe('华东/上海');
    expect(t.columns[2].title).toBe('华东/杭州');
    expect(t.rows[0].metricLabel).toBe('销售额');
  });

  it('空值不进列标题 join；全空时回落「行 N」', () => {
    const t = mustTranspose(
      [
        { region: null, revenue: 5 },
        { region: '华东', revenue: 6 },
      ],
      ['region']
    );
    expect(t.columns[1].title).toBe('行 1');
    expect(t.columns[2].title).toBe('华东');
  });

  it('maxColumns 截断数据列并置 truncated，防 N 行 × M 指标爆列', () => {
    const many = Array.from({ length: 8 }, (_, i) => ({ region: `r${i}`, revenue: i }));
    const t = mustTranspose(many, ['region'], { maxColumns: 3 });
    expect(t.columns).toHaveLength(4); // 标签列 + 3 个数据列
    expect(t.truncated).toBe(true);
    expect(t.rows[0].values).toEqual({ c0: 0, c1: 1, c2: 2 });
  });

  it('没有维度或没有数据时返回 null（调用方回落普通渲染）', () => {
    expect(transposeTable(rows, [])).toBeNull();
    expect(transposeTable([], ['region'])).toBeNull();
  });
});
