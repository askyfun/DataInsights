import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { datasetsApi } from '../../api';
import FilterConfigModal, {
  classifyFieldKind,
} from '../../components/ChartBuilder/FilterConfigModal';
import type { ChartField, FilterCondition } from '../../store';

/**
 * 过滤配置弹窗（弹窗式过滤交互的核心）：按字段数据类型分流控件，
 * 枚举模式从数据集实查候选值（数值从大到小），手输文本框与勾选合并。
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

const FIELD: ChartField = {
  id: 'retail_sales',
  name: 'retail_sales',
  type: 'metric',
  dataType: 'double',
};

const STRING_FIELD: ChartField = {
  id: 'brand_name',
  name: 'brand_name',
  type: 'dimension',
  dataType: 'character varying',
};

const DATE_FIELD: ChartField = {
  id: 'sale_date',
  name: 'sale_date',
  type: 'dimension',
  dataType: 'timestamp without time zone',
};

const makeInitial = (patch: Partial<FilterCondition>): FilterCondition => ({
  id: 'filter-1',
  field: FIELD.name,
  operator: 'gte',
  value: '',
  logic: 'and',
  ...patch,
});

type Props = Parameters<typeof FilterConfigModal>[0];

const renderModal = (props: Partial<Props> = {}) => {
  const onOk = vi.fn();
  const onCancel = vi.fn();
  render(
    <FilterConfigModal
      open
      field={FIELD}
      datasetId={1}
      onOk={onOk}
      onCancel={onCancel}
      {...props}
    />
  );
  return { onOk, onCancel };
};

describe('classifyFieldKind', () => {
  it('按数据类型三分类', () => {
    expect(classifyFieldKind('timestamp without time zone')).toBe('date');
    expect(classifyFieldKind('date')).toBe('date');
    expect(classifyFieldKind('double')).toBe('number');
    expect(classifyFieldKind('bigint')).toBe('number');
    expect(classifyFieldKind('character varying')).toBe('string');
    expect(classifyFieldKind('text')).toBe('string');
  });
});

describe('FilterConfigModal', () => {
  beforeEach(() => {
    mockedQuery.mockReset();
  });

  it('数值字段：枚举模式拉取候选值并按数值从大到小排序', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ retail_sales: 100 }, { retail_sales: 50 }, { retail_sales: 80 }] },
    } as never);

    renderModal();

    fireEvent.click(screen.getByText('枚举'));

    // 候选值下拉应包含实查回来的去重值（80 在 100 与 50 之间，验证降序由下拉选项顺序体现）
    await waitFor(() => {
      expect(mockedQuery).toHaveBeenCalledWith(1, 'retail_sales');
    });
  });

  it('数值字段：手输文本框一行一个值与勾选合并、去重、忽略空行', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ retail_sales: 100 }] },
    } as never);

    const onOk = vi.fn();
    renderModal({ onOk });

    fireEvent.click(screen.getByText('枚举'));
    fireEvent.change(screen.getByTestId('filter-modal-extra'), {
      target: { value: '200\n\n 300\n200' },
    });
    fireEvent.click(screen.getByRole('button', { name: '确 定' }));

    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({ operator: 'in', value: ['200', '300'] });
    });
  });

  it('数值字段：比较模式默认大于等于，区间模式上抛 between', () => {
    const onOk = vi.fn();
    renderModal({ onOk });

    fireEvent.change(screen.getByTestId('filter-modal-number'), { target: { value: '1000' } });
    fireEvent.click(screen.getByRole('button', { name: '确 定' }));
    expect(onOk).toHaveBeenCalledWith({ operator: 'gte', value: 1000 });

    fireEvent.click(screen.getByText('区间'));
    fireEvent.change(screen.getByTestId('filter-modal-min'), { target: { value: '10' } });
    fireEvent.change(screen.getByTestId('filter-modal-max'), { target: { value: '20' } });
    fireEvent.click(screen.getByRole('button', { name: '确 定' }));
    expect(onOk).toHaveBeenLastCalledWith({ operator: 'between', value: 10, valueEnd: 20 });
  });

  it('字符串字段：枚举模式实查候选值，文本框输入上抛 in 数组', async () => {
    mockedQuery.mockResolvedValue({
      data: {
        code: 0,
        data: [{ brand_name: '比亚迪' }, { brand_name: '奥迪' }, { brand_name: '本田' }],
      },
    } as never);

    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    // 字符串字段默认进枚举模式并实查候选值
    await waitFor(() => {
      expect(mockedQuery).toHaveBeenCalledWith(1, 'brand_name');
    });

    fireEvent.change(screen.getByTestId('filter-modal-extra'), {
      target: { value: '比亚迪' },
    });
    fireEvent.click(screen.getByRole('button', { name: '确 定' }));
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({ operator: 'in', value: ['比亚迪'] });
    });
  });

  it('日期字段：timestamp 用带时间的日期选择，上抛格式化字符串', () => {
    const onOk = vi.fn();
    renderModal({ field: DATE_FIELD, onOk });

    // 无值时确定按钮禁用（防半成品条件写回）
    expect(screen.getByRole('button', { name: '确 定' }).hasAttribute('disabled')).toBe(true);
  });

  it('数值字段：勾选反选后上抛 notIn，摘要显示「将排除」', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ retail_sales: 100 }] },
    } as never);

    const onOk = vi.fn();
    renderModal({ onOk });

    fireEvent.click(screen.getByText('枚举'));
    fireEvent.change(screen.getByTestId('filter-modal-extra'), {
      target: { value: '100' },
    });
    // 未勾选反选：保留
    fireEvent.click(screen.getByRole('button', { name: '确 定' }));
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({ operator: 'in', value: ['100'] });
    });

    // 勾选反选：排除
    fireEvent.click(screen.getByTestId('filter-modal-notin'));
    // antd Checkbox 的 testid 落在根 label，勾选状态看内部 input
    expect(screen.getByRole('checkbox')).toBeChecked();
    expect(screen.getByTestId('filter-modal-summary')).toHaveTextContent('将排除 1 个值');
    fireEvent.click(screen.getByRole('button', { name: '确 定' }));
    await waitFor(() => {
      expect(onOk).toHaveBeenLastCalledWith({ operator: 'notIn', value: ['100'] });
    });
  });

  it('编辑 notIn 条件时反选勾选框回显', () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ retail_sales: 100 }] },
    } as never);

    renderModal({
      initial: makeInitial({ operator: 'notIn', value: ['100', '200'] }),
    });

    expect(screen.getByRole('checkbox')).toBeChecked();
    expect(screen.getByTestId('filter-modal-summary')).toHaveTextContent('将排除 2 个值');
  });

  it('编辑已有条件时按原值初始化', () => {
    renderModal({
      initial: makeInitial({ operator: 'between', value: 10, valueEnd: 99 }),
    });

    expect(screen.getByTestId('filter-modal-min')).toHaveValue('10');
    expect(screen.getByTestId('filter-modal-max')).toHaveValue('99');
  });
});
