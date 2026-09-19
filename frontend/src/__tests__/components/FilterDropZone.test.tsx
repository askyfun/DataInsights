import { DndContext } from '@dnd-kit/core';
import { fireEvent, render, screen } from '@testing-library/react';
import type { ComponentProps } from 'react';
import { describe, expect, it, vi } from 'vitest';
import FilterDropZone from '../../components/ChartBuilder/FilterDropZone';
import type { ChartField, FilterCondition } from '../../store';

/**
 * 过滤字段组：与维度/指标字段组同构的拖入式过滤交互。
 *
 * 这里只覆盖组件自身的渲染与交互契约（拖入落点由 ChartBuilder 的拖拽测试覆盖）：
 * 空态提示、条件行三件套（字段/操作符/值）、按操作符切换值输入形态、删除。
 * 多个条件之间恒为「且」——组件不提供 AND/OR 切换，故没有 logic 相关用例。
 */

const FIELDS: ChartField[] = [
  { id: 'brand_name', name: 'brand_name', type: 'dimension', dataType: 'string' },
  { id: 'sale_count', name: 'sale_count', type: 'metric', dataType: 'integer' },
];

const makeFilter = (patch: Partial<FilterCondition>): FilterCondition => ({
  id: 'filter-1',
  field: 'brand_name',
  operator: 'eq',
  value: '',
  logic: 'and',
  ...patch,
});

type ZoneProps = ComponentProps<typeof FilterDropZone>;

const renderZone = (props: Partial<ZoneProps> = {}) => {
  const onAdd = vi.fn();
  const onRemove = vi.fn();
  const onUpdate = vi.fn();

  render(
    <DndContext>
      <FilterDropZone
        filters={[]}
        availableFields={FIELDS}
        onAdd={onAdd}
        onRemove={onRemove}
        onUpdate={onUpdate}
        {...props}
      />
    </DndContext>
  );

  return { onAdd, onRemove, onUpdate };
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

  it('渲染条件行的字段名、操作符与值', () => {
    renderZone({
      filters: [makeFilter({ operator: 'gte', value: '100' })],
    });

    const row = screen.getByTestId('filter-row-filter-1');
    expect(row).toHaveTextContent('brand_name');
    expect(row).toHaveTextContent('>=');
    expect(screen.getByTestId('filter-value-filter-1')).toHaveValue('100');
  });

  it('同一字段可重复加入：>= 与 <= 两条条件各自一行', () => {
    renderZone({
      filters: [
        makeFilter({ id: 'filter-1', operator: 'gte', value: '100' }),
        makeFilter({ id: 'filter-2', operator: 'lte', value: '500' }),
      ],
    });

    expect(screen.getByTestId('filter-row-filter-1')).toHaveTextContent('>=');
    expect(screen.getByTestId('filter-row-filter-2')).toHaveTextContent('<=');
    expect(screen.getByTestId('filter-value-filter-1')).toHaveValue('100');
    expect(screen.getByTestId('filter-value-filter-2')).toHaveValue('500');
  });

  it('值输入变更上抛 onUpdate', () => {
    const { onUpdate } = renderZone({ filters: [makeFilter({})] });

    fireEvent.change(screen.getByTestId('filter-value-filter-1'), {
      target: { value: 'BYD' },
    });

    expect(onUpdate).toHaveBeenCalledWith('filter-1', { value: 'BYD' });
  });

  it('between 操作符渲染最小/最大值两个输入框', () => {
    renderZone({
      filters: [makeFilter({ operator: 'between', value: '10', valueEnd: '20' })],
    });

    expect(screen.getByTestId('filter-value-filter-1')).toHaveValue('10');
    expect(screen.getByTestId('filter-value-end-filter-1')).toHaveValue('20');
  });

  it('in 操作符复用单输入框并提示多值写法', () => {
    renderZone({ filters: [makeFilter({ operator: 'in' })] });

    expect(screen.getByTestId('filter-value-filter-1')).toHaveAttribute(
      'placeholder',
      '值1, 值2, 值3'
    );
    expect(screen.queryByTestId('filter-value-end-filter-1')).not.toBeInTheDocument();
  });

  it('isNull 这类无值操作符不渲染值输入框', () => {
    renderZone({ filters: [makeFilter({ operator: 'isNull' })] });

    expect(screen.queryByTestId('filter-value-filter-1')).not.toBeInTheDocument();
  });

  it('点击删除按钮上抛 onRemove', () => {
    const { onRemove } = renderZone({ filters: [makeFilter({})] });

    fireEvent.click(screen.getByTestId('filter-remove-filter-1'));

    expect(onRemove).toHaveBeenCalledWith('filter-1');
  });

  it('只按 availableFields 过滤：没有可拖入字段时不渲染 + 按钮', () => {
    renderZone({ availableFields: [] });

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
