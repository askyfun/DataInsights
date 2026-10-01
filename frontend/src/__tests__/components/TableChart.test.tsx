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

/**
 * 合计行（issue #131）。
 *
 * 契约：合计值只来自后端负载的 `total`（在过滤后的完整数据集上重算），前端只做展示。
 * 关键红线是**不得**在前端拿当前页明细相加——服务端分页下那只是某一页的和，
 * AVG / COUNT(DISTINCT) 还会算成「平均数的平均数」。
 */
describe('TableChart 合计行', () => {
  const summaryRows = (container: HTMLElement) =>
    Array.from(container.querySelectorAll('.ant-table-summary tr'));
  const summaryCells = (container: HTMLElement) =>
    Array.from(summaryRows(container)[0]?.querySelectorAll('td') ?? []).map(
      (td) => td.textContent ?? ''
    );

  it('传入 totalRow 时在表尾渲染一行：首列为「合计」标签，指标列取后端值', () => {
    const { container } = renderTable({
      totalRow: { revenue: 58 },
      dimensionNames: ['region'],
      metricNames: ['revenue'],
    });
    expect(summaryRows(container)).toHaveLength(1);
    expect(summaryCells(container)).toEqual(['合计', '58']);
    // 明细行不受影响：合计不是往 dataSource 里塞一行
    expect(container.querySelectorAll('.ant-table-row')).toHaveLength(2);
  });

  it('不传 totalRow 时不渲染合计行（默认关闭，行为与本任务前一致）', () => {
    const { container } = renderTable();
    expect(summaryRows(container)).toHaveLength(0);
  });

  it('合计值套用该列的「格式」，与明细行同口径', () => {
    const { container } = renderTable({
      totalRow: { revenue: 58 },
      dimensionNames: ['region'],
      metricNames: ['revenue'],
      metricFormats: { revenue: '0,0.00' },
    });
    expect(summaryCells(container)).toEqual(['合计', '58.00']);
  });

  it('序号列 + 维度列：序号列留空，「合计」标签落在维度列', () => {
    const { container } = render(
      <TableChart
        data={[{ region: 'East', revenue: 42 }]}
        columns={['region', 'revenue']}
        loading={false}
        showIndex
        dimensionNames={['region']}
        metricNames={['revenue']}
        totalRow={{ revenue: 42 }}
      />
    );
    expect(summaryCells(container)).toEqual(['', '合计', '42']);
  });

  it('只有指标列时，「合计」标签落到序号列', () => {
    const { container } = render(
      <TableChart
        data={[{ revenue: 42 }]}
        columns={['revenue']}
        loading={false}
        showIndex
        metricNames={['revenue']}
        totalRow={{ revenue: 42 }}
      />
    );
    expect(summaryCells(container)).toEqual(['合计', '42']);
  });

  it('明细为空但有合计：不显示空态，合计行仍在（过滤后 0 行、全集仍有数值）', () => {
    const { container } = render(
      <TableChart
        data={[]}
        columns={['region', 'revenue']}
        loading={false}
        dimensionNames={['region']}
        metricNames={['revenue']}
        totalRow={{ revenue: 58 }}
      />
    );
    expect(screen.queryByText('暂无数据')).toBeNull();
    expect(summaryCells(container)).toEqual(['合计', '58']);
  });
});

/**
 * 指标占比列（issue #132）：格式串带 `%` 后缀的列按 value/全集合计 显示。
 *
 * 红线是分母必须来自后端重算的完整数据集合计（grandTotal）。服务端分页下拿当前页
 * 明细相加当分母，同一行在第 1 页和第 2 页会显示不同占比 —— 所以拿不到分母时
 * 宁可留空，也不能"看起来算出来了"。
 */
describe('TableChart 占比列', () => {
  const percentRows = [
    { region: 'East', share: 30 },
    { region: 'West', share: 10 },
  ];

  const renderPercent = (props: Partial<React.ComponentProps<typeof TableChart>> = {}) =>
    render(
      <TableChart
        data={percentRows}
        columns={['region', 'share']}
        loading={false}
        dimensionNames={['region']}
        metricNames={['share']}
        metricFormats={{ share: '0,0.00%' }}
        metricPercentOfTotal={['share']}
        grandTotal={{ share: 120 }}
        {...props}
      />
    );

  const columnValues = (container: HTMLElement, columnIndex: number) =>
    Array.from(container.querySelectorAll(`.ant-table-row td:nth-child(${columnIndex})`)).map(
      (td) => td.textContent ?? ''
    );

  it('占比列显示 value/全集合计，`%` 作为后缀拼在格式化结果之后', () => {
    const { container } = renderPercent();
    expect(columnValues(container, 2)).toEqual(['25.00%', '8.33%']);
  });

  it('未配占比的指标列仍按原格式显示（不受分母影响）', () => {
    const { container } = renderPercent({ metricPercentOfTotal: [] });
    expect(columnValues(container, 2)).toEqual(['30.00', '10.00']);
  });

  it('拿不到全集合计时占比列留空，不回落到本页合计', () => {
    const { container } = renderPercent({ grandTotal: undefined });
    // 本页相加会得到 100% / 33.33% —— 那正是必须避免的错数
    expect(columnValues(container, 2)).toEqual(['', '']);
  });

  it('合计为 0（除零）时留空而不是 Infinity%', () => {
    const { container } = renderPercent({ grandTotal: { share: 0 } });
    expect(columnValues(container, 2)).toEqual(['', '']);
  });

  it('合计行里的占比列按同一分母算出 100%', () => {
    const { container } = renderPercent({ totalRow: { share: 120 } });
    const cells = Array.from(container.querySelectorAll('.ant-table-summary td')).map(
      (td) => td.textContent ?? ''
    );
    expect(cells).toEqual(['合计', '100.00%']);
  });
});

/**
 * 条件格式（issue #156 AC1）：规则命中时单元格 onCell 背景色生效；
 * 语义解析本身在 lib/tableConditionalFormat.ts，这里只验证渲染接线。
 */
describe('TableChart 条件格式', () => {
  it('阈值命中的单元格上背景色，未命中与维度列不上色', () => {
    const { container } = renderTable({
      conditionalFormat: [
        { metric: 'revenue', kind: 'threshold', op: '>', value: 10, color: '#fa8c16' },
      ],
    });
    const cells = Array.from(container.querySelectorAll('.ant-table-tbody td'));
    const colored = cells.filter((td) => (td as HTMLElement).style.backgroundColor !== '');
    // 42 命中；7 不命中；region 列永不命中
    expect(colored.length).toBe(1);
    expect((colored[0] as HTMLElement).style.backgroundColor).toBe('rgb(250, 140, 22)');
    expect(colored[0].textContent).toBe('42');
  });

  it('涨跌色：正负值分别取涨/跌色，0 不着色', () => {
    const { container } = renderTable({
      data: [
        { region: 'East', revenue: 5 },
        { region: 'West', revenue: -3 },
        { region: 'North', revenue: 0 },
      ],
      conditionalFormat: [{ metric: 'revenue', kind: 'diff' }],
    });
    const cells = Array.from(container.querySelectorAll('.ant-table-tbody td'));
    const colored = cells.filter((td) => (td as HTMLElement).style.backgroundColor !== '');
    // 5 → 涨红、-3 → 跌绿；0 与维度列不着色
    expect(colored.map((td) => td.textContent)).toEqual(['5', '-3']);
    expect((colored[0] as HTMLElement).style.backgroundColor).toBe('rgb(207, 19, 34)');
    expect((colored[1] as HTMLElement).style.backgroundColor).toBe('rgb(56, 158, 13)');
  });
});
