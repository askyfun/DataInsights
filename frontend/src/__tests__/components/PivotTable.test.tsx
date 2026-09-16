import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { isPivotV2Payload, type PivotResponseV2 } from '@/api';
import PivotTable from '@/components/ChartBuilder/PivotTable';

/**
 * PivotTable（R-53 透视表 v2 交叉表）渲染契约测试。
 * fixture 键严格遵循后端 query.PivotValueKey 契约（processor_pivot.go）：
 * 明细单元格 "<colHeader>|<metricAlias>"、小计/合计单元格 "__subtotal__|<metricAlias>"。
 * 若组件把分隔符 "|" 或哨兵 "__subtotal__" 写错，取值全为 undefined → 以下值断言必红。
 */
const fixture: PivotResponseV2 = {
  row_headers: ['region'],
  col_headers: ['A', 'B'],
  metric_names: ['total'],
  cells: [
    { row_key: ['East'], is_subtotal: false, values: { 'A|total': 100, 'B|total': 50 } },
    { row_key: ['East'], is_subtotal: true, values: { '__subtotal__|total': 150 } },
    { row_key: ['West'], is_subtotal: false, values: { 'A|total': 30 } },
  ],
  grand_total: { row_key: [], is_subtotal: true, values: { '__subtotal__|total': 180 } },
};

const tdTexts = (row: Element | null | undefined): string[] =>
  Array.from(row?.querySelectorAll('td') ?? []).map((td) => td.textContent ?? '');

describe('PivotTable', () => {
  it('渲染交叉表表头并按 "<colHeader>|<metric>" 键取明细单元格值', () => {
    const { container } = render(<PivotTable data={fixture} />);

    // 表头：行维度列名、列维度值分组头、指标叶子头（A/B/小计 三组）、小计分组列。
    // antd 的隐藏测宽节点会复制表头文本，表头断言限定在 thead th 上。
    const headerTexts = Array.from(container.querySelectorAll('thead th')).map(
      (th) => th.textContent ?? ''
    );
    expect(headerTexts).toContain('region');
    expect(headerTexts).toContain('A');
    expect(headerTexts).toContain('B');
    expect(headerTexts).toContain('小计');
    expect(headerTexts.filter((t) => t === 'total')).toHaveLength(3);

    // East 明细行 td 顺序 = [region, A|total, B|total, 小计]：
    // 100/50 必须按键取值落在对应列（分隔符或键序写错则全为空串）。
    const eastDetailRow = screen.getAllByText('East')[0].closest('tr');
    expect(tdTexts(eastDetailRow)).toEqual(['East', '100', '50', '']);

    // West 明细行：values 缺 "B|total" 键 → 渲染空串，不漏 undefined/NaN 字样。
    const westRow = screen.getByText('West').closest('tr');
    expect(tdTexts(westRow)).toEqual(['West', '30', '', '']);
    expect(screen.queryByText(/undefined|NaN/)).toBeNull();
  });

  it('小计/合计按哨兵键 "__subtotal__|<metric>" 取值（键契约锁定）', () => {
    render(<PivotTable data={fixture} />);

    // East 小计行：交叉体列留空，小计列渲染 150（来自 "__subtotal__|total"）。
    const eastSubtotalRow = screen.getAllByText('East')[1].closest('tr');
    expect(tdTexts(eastSubtotalRow)).toEqual(['East', '', '', '150']);

    // 合计行渲染 180（来自 grand_total.values["__subtotal__|total"]）。
    expect(screen.getByText('180')).toBeInTheDocument();
  });

  it('小计行与合计行带区分类名，明细行没有', () => {
    const { container } = render(<PivotTable data={fixture} />);

    expect(container.querySelectorAll('.pivot-subtotal-row')).toHaveLength(1);
    expect(container.querySelectorAll('.pivot-grand-total-row')).toHaveLength(1);

    const eastDetailRow = screen.getAllByText('East')[0].closest('tr');
    expect(eastDetailRow?.classList.contains('pivot-subtotal-row')).toBe(false);
    expect(eastDetailRow?.classList.contains('pivot-grand-total-row')).toBe(false);
  });

  it('grand_total 作为最后一行追加，左列显示"合计"标签', () => {
    render(<PivotTable data={fixture} />);

    const grandRow = screen.getByText('合计').closest('tr');
    expect(grandRow).not.toBeNull();
    expect(grandRow?.classList.contains('pivot-grand-total-row')).toBe(true);
    expect(tdTexts(grandRow)).toEqual(['合计', '', '', '180']);

    // 合计行是表格最后一行
    const rows = grandRow?.parentElement?.querySelectorAll('tr');
    expect(rows?.[rows.length - 1]).toBe(grandRow);
  });

  it('cells 为空时渲染 Empty（暂无数据）', () => {
    render(
      <PivotTable
        data={{
          row_headers: ['region'],
          col_headers: ['A'],
          metric_names: ['total'],
          cells: [],
          grand_total: null,
        }}
      />
    );

    expect(screen.getByText('暂无数据')).toBeInTheDocument();
  });

  it('isPivotV2Payload 按形状判别：v2 交叉形状 true，v1 平铺/裸数组/null false', () => {
    expect(isPivotV2Payload(fixture)).toBe(true);
    expect(isPivotV2Payload({ columns: ['region'], data: [] })).toBe(false);
    expect(isPivotV2Payload([])).toBe(false);
    expect(isPivotV2Payload(null)).toBe(false);
  });
});
