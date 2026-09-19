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
