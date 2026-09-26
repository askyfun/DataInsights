import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { datasetsApi } from '../../api';
import FilterValueControl from '../../components/ChartBuilder/FilterValueControl';
import type { ChartField, FilterCondition } from '../../store';

/**
 * 「作为筛选器」的字符串/数值行内控件（图表预览区上方）：
 * 算子词表与值编辑器按族分流，枚举模式实查候选值（与 FilterConfigModal 同一取数）。
 */

vi.mock('../../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api')>();
  return {
    ...actual,
    datasetsApi: {
      ...actual.datasetsApi,
      queryDistinct: vi.fn(),
    },
  };
});

const mockedQuery = vi.mocked(datasetsApi.queryDistinct);

const STRING_FIELD: ChartField = {
  id: 'brand_name',
  name: 'brand_name',
  type: 'dimension',
  dataType: 'character varying',
};

const NUMBER_FIELD: ChartField = {
  id: 'retail_sales',
  name: 'retail_sales',
  type: 'metric',
  dataType: 'double',
};

const makeFilter = (patch: Partial<FilterCondition>): FilterCondition => ({
  id: 'filter-1',
  fieldId: 'brand_name',
  operator: 'eq',
  value: '',
  logic: 'and',
  ...patch,
});

const renderControl = (props: Partial<Parameters<typeof FilterValueControl>[0]> = {}) => {
  const onChange = vi.fn();
  render(
    <FilterValueControl
      field={STRING_FIELD}
      filter={makeFilter({})}
      datasetId={1}
      onChange={onChange}
      {...props}
    />
  );
  return { onChange };
};

describe('FilterValueControl', () => {
  beforeEach(() => {
    mockedQuery.mockReset();
  });

  it('字符串 in 条件：实查候选值并渲染多选，勾选回写 value 数组', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ brand_name: '比亚迪' }, { brand_name: '奥迪' }] },
    } as never);

    const { onChange } = renderControl({
      filter: makeFilter({ operator: 'in', value: [] }),
    });

    await waitFor(() => {
      expect(mockedQuery).toHaveBeenCalledWith(1, 'brand_name');
      expect(screen.getByTestId('chart-filter-enum-filter-1')).toBeInTheDocument();
    });

    // antd 多选下拉：打开后勾选候选值
    fireEvent.mouseDown(screen.getByTestId('chart-filter-enum-filter-1'));
    fireEvent.click(await screen.findByTitle('比亚迪'));
    await waitFor(() => {
      expect(onChange).toHaveBeenCalledWith({ value: ['比亚迪'] });
    });
  });

  it('数值 gt 条件：InputNumber 输入回写 value', () => {
    const { onChange } = renderControl({
      field: NUMBER_FIELD,
      filter: makeFilter({ fieldId: 'retail_sales', operator: 'gt', value: 10 }),
    });

    const input = screen.getByTestId('chart-filter-value-filter-1');
    fireEvent.change(input, { target: { value: '42' } });
    expect(onChange).toHaveBeenCalledWith({ value: 42 });
  });

  it('数值 between 条件：两格输入分别回写 value / valueEnd', () => {
    const { onChange } = renderControl({
      field: NUMBER_FIELD,
      filter: makeFilter({ fieldId: 'retail_sales', operator: 'between', value: 1, valueEnd: 9 }),
    });

    expect(screen.getByTestId('chart-filter-min-filter-1')).toBeInTheDocument();
    expect(screen.getByTestId('chart-filter-max-filter-1')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('chart-filter-max-filter-1'), {
      target: { value: '99' },
    });
    expect(onChange).toHaveBeenCalledWith({ valueEnd: 99 });
  });

  it('切换为无值算子时清空值区并上抛 value: null', () => {
    const { onChange } = renderControl({
      field: NUMBER_FIELD,
      filter: makeFilter({ fieldId: 'retail_sales', operator: 'gt', value: 10 }),
    });

    fireEvent.mouseDown(screen.getByTestId('chart-filter-operator-filter-1'));
    fireEvent.click(screen.getByTitle('为空'));
    expect(onChange).toHaveBeenCalledWith({ operator: 'isNull', value: null, valueEnd: undefined });
  });

  it('无值算子下不渲染值编辑器', () => {
    renderControl({
      filter: makeFilter({ operator: 'isNotNull', value: null }),
    });

    expect(screen.queryByTestId('chart-filter-value-filter-1')).not.toBeInTheDocument();
  });
});
