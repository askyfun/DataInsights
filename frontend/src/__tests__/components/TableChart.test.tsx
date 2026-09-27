import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import TableChart from '@/components/ChartBuilder/TableChart';

/**
 * TableChart 的服务端排序契约。
 *
 * 表头排序是「受控」组件：箭头状态只来自 sortField/sortOrder（父组件用 queryConfig.sort
 * 驱动），点击只负责回调。历史上表头挂的是非受控 sorter，箭头自己会动、数据却不一定动，
 * 而且 onChange 里分页回调无条件触发，会额外发一次不带 sort 的查询与排序请求抢结果。
 */
const rows = [
  { region: 'East', revenue: 42 },
  { region: 'West', revenue: 7 },
];
const columns = ['region', 'revenue'];

const renderTable = (props: Partial<React.ComponentProps<typeof TableChart>> = {}) =>
  render(<TableChart data={rows} columns={columns} loading={false} {...props} />);

/** 取某一列表头里的 sorter 图标激活方向（up/down/无）。 */
const activeSortDirection = (headerName: string): 'up' | 'down' | null => {
  const header = screen.getByRole('columnheader', { name: new RegExp(headerName) });
  if (header.querySelector('.ant-table-column-sorter-up.active')) return 'up';
  if (header.querySelector('.ant-table-column-sorter-down.active')) return 'down';
  return null;
};

describe('TableChart 表头排序', () => {
  it('点击表头回调一次 asc，并且不把这次点击当成翻页', () => {
    const onSortChange = vi.fn();
    const onPageChange = vi.fn();
    renderTable({
      onSortChange,
      onPageChange,
      pagination: { page: 1, pageSize: 10, total: 30 },
    });

    fireEvent.click(screen.getByRole('columnheader', { name: /revenue/ }));

    expect(onSortChange).toHaveBeenCalledTimes(1);
    expect(onSortChange).toHaveBeenCalledWith({ field: 'revenue', order: 'asc' });
    // 页码/每页条数都没变：不能再发一次不带 sort 的查询
    expect(onPageChange).not.toHaveBeenCalled();
  });

  it('受控排序：箭头方向只由 sortField/sortOrder 决定', () => {
    const { unmount } = renderTable({
      onSortChange: vi.fn(),
      sortField: 'revenue',
      sortOrder: 'desc',
    });
    expect(activeSortDirection('revenue')).toBe('down');
    expect(activeSortDirection('region')).toBeNull();
    unmount();

    // 换成升序 + 另一个列：箭头必须跟着走，而不是留在上一列
    renderTable({ onSortChange: vi.fn(), sortField: 'region', sortOrder: 'asc' });
    expect(activeSortDirection('region')).toBe('up');
    expect(activeSortDirection('revenue')).toBeNull();
  });

  it('已按该列降序时再次点击回调 null（用户取消排序）', () => {
    const onSortChange = vi.fn();
    renderTable({ onSortChange, sortField: 'revenue', sortOrder: 'desc' });

    fireEvent.click(screen.getByRole('columnheader', { name: /revenue/ }));

    expect(onSortChange).toHaveBeenCalledWith(null);
  });

  it('与当前受控状态相同的点击不下发（避免重复查询）', () => {
    const onSortChange = vi.fn();
    renderTable({ onSortChange, sortField: 'revenue', sortOrder: 'asc' });

    // antd 计算出的下一次方向是 descend，与受控状态不同 → 仍然要下发
    fireEvent.click(screen.getByRole('columnheader', { name: /revenue/ }));
    expect(onSortChange).toHaveBeenCalledTimes(1);
    expect(onSortChange).toHaveBeenCalledWith({ field: 'revenue', order: 'desc' });
  });

  it('未提供 onSortChange 时不渲染排序箭头（分享页只读表格）', () => {
    const { container } = renderTable();
    expect(container.querySelector('.ant-table-column-sorter')).toBeNull();
  });

  it('翻页回调只在页码/每页条数真的变化时触发', () => {
    const onPageChange = vi.fn();
    renderTable({
      onPageChange,
      pagination: { page: 1, pageSize: 10, total: 30 },
    });

    // 点击第 2 页
    fireEvent.click(screen.getByTitle('2'));

    expect(onPageChange).toHaveBeenCalledWith(2, 10);
  });
});

/**
 * 表格展示增强（#121 第 1 批）：序号列 / 空值显示 / 自动换行 / 冻结维度列。
 * 全部是渲染层行为，不涉及服务端契约。
 */
describe('TableChart 展示增强', () => {
  const tdWithText = (text: string): HTMLElement | null => {
    const node = screen.getAllByText(text)[0]?.closest('td') ?? null;
    return node;
  };

  // 首列单元格文本，仅保留纯数字（跳过 antd v6 固定列拆分出的表头 '#' 度量行）。
  const indexColumnValues = (container: HTMLElement): string[] =>
    Array.from(container.querySelectorAll('.ant-table-tbody tr td:first-child'))
      .map((td) => td.textContent ?? '')
      .filter((text) => /^\d+$/.test(text));

  it('序号列：无分页时从 1 起连续编号', () => {
    const { container } = renderTable({ showIndex: true });
    expect(screen.getByRole('columnheader', { name: '#' })).toBeTruthy();
    expect(indexColumnValues(container)).toEqual(['1', '2']);
  });

  it('序号列：跨服务端分页连续编号（第 2 页从 11 起）', () => {
    const { container } = renderTable({
      showIndex: true,
      pagination: { page: 2, pageSize: 10, total: 30 },
    });
    expect(indexColumnValues(container)).toEqual(['11', '12']);
  });

  it("空值显示 'dash'：NULL 与空串都渲染为 --", () => {
    renderTable({
      data: [{ region: null, revenue: '' }],
      columns: ['region', 'revenue'],
      dimensionNames: ['region'],
      metricNames: ['revenue'],
      nullDisplay: 'dash',
    });
    expect(screen.getAllByText('--').length).toBe(2);
  });

  it("空值显示 'zero'：NULL 渲染为 0", () => {
    renderTable({
      data: [{ region: null, revenue: 42 }],
      columns: ['region', 'revenue'],
      dimensionNames: ['region'],
      metricNames: ['revenue'],
      nullDisplay: 'zero',
    });
    expect(screen.getByText('0')).toBeTruthy();
    expect(screen.getByText('42')).toBeTruthy();
  });

  it('空值显示缺省(raw)：不产生 -- 占位', () => {
    renderTable({
      data: [{ region: null, revenue: '' }],
      columns: ['region', 'revenue'],
      dimensionNames: ['region'],
      metricNames: ['revenue'],
    });
    expect(screen.queryByText('--')).toBeNull();
  });

  it('自动换行：默认单元格省略号截断，开启后取消', () => {
    const { container, unmount } = renderTable();
    expect(container.querySelector('.ant-table-tbody .ant-table-cell-ellipsis')).toBeTruthy();
    unmount();

    renderTable({ wordWrap: true });
    expect(container.querySelector('.ant-table-tbody .ant-table-cell-ellipsis')).toBeNull();
  });

  it('冻结维度列：维度列固定左侧，指标列不固定', () => {
    renderTable({
      dimensionNames: ['region'],
      metricNames: ['revenue'],
      freezeDimensions: true,
    });
    expect(tdWithText('East')?.className).toContain('ant-table-cell-fix-start');
    expect(tdWithText('42')?.className).not.toContain('ant-table-cell-fix-start');
  });

  it('未开启冻结时维度列不带 fix-start', () => {
    renderTable({ dimensionNames: ['region'], metricNames: ['revenue'] });
    expect(tdWithText('East')?.className).not.toContain('ant-table-cell-fix-start');
  });
});
