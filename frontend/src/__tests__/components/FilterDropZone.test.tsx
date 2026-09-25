import { DndContext } from '@dnd-kit/core';
import { fireEvent, render, screen } from '@testing-library/react';
import type { ComponentProps } from 'react';
import { describe, expect, it, vi } from 'vitest';
import FilterDropZone from '../../components/ChartBuilder/FilterDropZone';
import type { ChartField, FilterCondition } from '../../store';

/**
 * 过滤字段组：与维度/指标字段组同构的拖入式过滤交互（弹窗式配置）。
 *
 * 这里只覆盖组件自身的渲染与交互契约（拖入落点由 ChartBuilder 的拖拽测试覆盖）：
 * 空态提示、条件行 = 字段芯片 + 摘要、点击行上抛 onEdit（进配置弹窗）、删除。
 * 行内不再直接编辑操作符/值——收集过滤条件的职责已移交 FilterConfigModal。
 */

const FIELDS: ChartField[] = [
  { id: 'brand_name', name: 'brand_name', type: 'dimension', dataType: 'string' },
  { id: 'sale_count', name: 'sale_count', type: 'metric', dataType: 'integer' },
];

const makeFilter = (patch: Partial<FilterCondition>): FilterCondition => ({
  id: 'filter-1',
  fieldId: 'brand_name',
  operator: 'eq',
  value: '',
  logic: 'and',
  ...patch,
});

type ZoneProps = ComponentProps<typeof FilterDropZone>;

const renderZone = (props: Partial<ZoneProps> = {}) => {
  const onAdd = vi.fn();
  const onEdit = vi.fn();
  const onRemove = vi.fn();

  render(
    <DndContext>
      <FilterDropZone
        filters={[]}
        availableFields={FIELDS}
        onAdd={onAdd}
        onEdit={onEdit}
        onRemove={onRemove}
        {...props}
      />
    </DndContext>
  );

  return { onAdd, onEdit, onRemove };
};

describe('FilterDropZone', () => {
  it('空态显示拖入提示，不渲染任何条件行', () => {
    renderZone();

    expect(screen.getByTestId('filter-drop-zone')).toHaveTextContent('拖拽字段到此添加筛选');
    expect(screen.getByTestId('filter-drop-zone')).toHaveTextContent('「且」');
    expect(screen.queryByTestId('filter-row-filter-1')).not.toBeInTheDocument();
  });

  it('通过 + 下拉添加字段', async () => {
    const { onAdd } = renderZone();

    fireEvent.click(screen.getByRole('button'));
    fireEvent.click(await screen.findByText('sale_count'));

    expect(onAdd).toHaveBeenCalledWith(FIELDS[1]);
  });

  it('渲染条件行的字段名与摘要（比较符）', () => {
    renderZone({
      filters: [makeFilter({ operator: 'gte', value: '100' })],
    });

    const row = screen.getByTestId('filter-row-filter-1');
    expect(row).toHaveTextContent('brand_name');
    expect(screen.getByTestId('filter-summary-filter-1')).toHaveTextContent('大于等于 100');
  });

  it('摘要按操作符分流：区间 / 属于 / 为空', () => {
    renderZone({
      filters: [
        makeFilter({ id: 'f-between', operator: 'between', value: '10', valueEnd: '20' }),
        makeFilter({ id: 'f-in', operator: 'in', value: ['北京', '上海'] }),
        makeFilter({ id: 'f-null', operator: 'isNull' }),
      ],
    });

    expect(screen.getByTestId('filter-summary-f-between')).toHaveTextContent('区间 10 ~ 20');
    expect(screen.getByTestId('filter-summary-f-in')).toHaveTextContent('属于 2 个值');
    expect(screen.getByTestId('filter-summary-f-null')).toHaveTextContent('为空');
  });

  it('点击条件行上抛 onEdit（携带条件与字段对象）', () => {
    const { onEdit } = renderZone({
      filters: [makeFilter({})],
    });

    fireEvent.click(screen.getByTestId('filter-row-filter-1'));

    expect(onEdit).toHaveBeenCalledTimes(1);
    expect(onEdit.mock.calls[0]?.[0]).toMatchObject({ id: 'filter-1', fieldId: 'brand_name' });
    expect(onEdit.mock.calls[0]?.[1]).toEqual(FIELDS[0]);
  });

  it('条件按列 ID 反查字段：列 id 与列名不同时芯片仍显示列名', () => {
    // 现实里 DatasetColumn.id（"0000i529"）与 name（"date"）并不相同，
    // 按列名反查会让芯片退化成裸 id、并丢掉字段颜色。
    const fields: ChartField[] = [
      { id: '0000i529', name: 'date', type: 'dimension', dataType: 'date' },
    ];
    const { onEdit } = renderZone({
      availableFields: fields,
      filters: [makeFilter({ fieldId: '0000i529' })],
    });

    expect(screen.getByTestId('filter-row-filter-1')).toHaveTextContent('date');
    expect(screen.getByTestId('filter-row-filter-1')).not.toHaveTextContent('0000i529');

    fireEvent.click(screen.getByTestId('filter-row-filter-1'));
    expect(onEdit.mock.calls[0]?.[1]).toEqual(fields[0]);
  });

  it('日期筛选条件展示「意图」摘要，而不是兜底的 operator/value', () => {
    renderZone({
      filters: [
        makeFilter({
          fieldId: 'brand_name',
          operator: 'between',
          value: '2026-09-18',
          valueEnd: '2026-09-24',
          date: {
            value: { kind: 'dynamic', preset: 'last7d' },
            granularity: 'day',
            weekStart: 1,
            asFilter: true,
            label: '交易日期',
          },
        }),
      ],
    });

    expect(screen.getByTestId('filter-summary-filter-1')).toHaveTextContent('最近 7 天');
  });

  it('点击删除按钮只上抛 onRemove，不触发 onEdit', () => {
    const { onRemove, onEdit } = renderZone({ filters: [makeFilter({})] });

    fireEvent.click(screen.getByTestId('filter-remove-filter-1'));

    expect(onRemove).toHaveBeenCalledWith('filter-1');
    expect(onEdit).not.toHaveBeenCalled();
  });

  it('只按 availableFields 过滤：没有可拖入字段时不渲染 + 按钮', () => {
    renderZone({ availableFields: [] });

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
