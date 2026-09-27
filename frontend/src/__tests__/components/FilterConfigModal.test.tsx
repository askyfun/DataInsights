import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { datasetsApi } from '../../api';
import FilterConfigModal, {
  classifyFieldKind,
} from '../../components/ChartBuilder/FilterConfigModal';
import type { ChartField, FilterCondition } from '../../store';

/**
 * 筛选配置弹窗（模式体系版）：字符串/数值字段呈现 精确筛选/条件筛选/手动输入/子查询
 * 四个模式 Tab；各 Tab 状态独立，切换往返不丢；「确定」以当前停留 Tab 为准写回。
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
  fieldId: FIELD.name,
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

const clickOk = () => fireEvent.click(screen.getByRole('button', { name: '确 定' }));

const switchTab = (label: string) => fireEvent.click(screen.getByRole('tab', { name: label }));

/** 点击 EnumPanel 里指定文案的选项勾选框（testid 落在 input 上，经 label 反查文案）。 */
const clickEnumItem = (label: string) => {
  const input = screen
    .getAllByTestId('filter-modal-enum-item')
    .find((el) => el.closest('label')?.textContent === label);
  if (!input) throw new Error(`未找到候选项 ${label}`);
  fireEvent.click(input);
};

describe('classifyFieldKind', () => {
  it('按规范数据类型三分类（历史词同步归一）', () => {
    // 规范词表
    expect(classifyFieldKind('date')).toBe('date');
    expect(classifyFieldKind('datetime')).toBe('date');
    expect(classifyFieldKind('integer')).toBe('number');
    expect(classifyFieldKind('float')).toBe('number');
    expect(classifyFieldKind('string')).toBe('string');
    expect(classifyFieldKind('boolean')).toBe('string');
    // 历史词/原始列类型归一
    expect(classifyFieldKind('timestamp')).toBe('date');
    expect(classifyFieldKind('timestamp without time zone')).toBe('date');
    expect(classifyFieldKind('number')).toBe('number');
    expect(classifyFieldKind('bigint')).toBe('number');
    expect(classifyFieldKind('double')).toBe('number');
    expect(classifyFieldKind('character varying')).toBe('string');
    expect(classifyFieldKind('text')).toBe('string');
  });
});

describe('FilterConfigModal · 模式体系', () => {
  beforeEach(() => {
    mockedQuery.mockReset();
  });

  it('字符串/数值字段渲染 精确筛选/条件筛选/手动输入 三个可用模式，子查询置灰', () => {
    mockedQuery.mockResolvedValue({ data: { code: 0, data: [] } } as never);
    renderModal({ field: STRING_FIELD });

    expect(screen.getByRole('tab', { name: '精确筛选' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '条件筛选' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '手动输入' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '子查询' })).toHaveAttribute('aria-disabled', 'true');
  });

  it('数值字段新建默认条件筛选-比较，不拉候选值', () => {
    renderModal();

    expect(screen.getByTestId('filter-modal-number')).toBeInTheDocument();
    expect(mockedQuery).not.toHaveBeenCalled();
  });

  it('字符串字段新建默认精确筛选并实查候选值', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ brand_name: '比亚迪' }, { brand_name: '奥迪' }] },
    } as never);

    renderModal({ field: STRING_FIELD });

    await waitFor(() => {
      expect(screen.getAllByTestId('filter-modal-enum-item')).toHaveLength(2);
    });
  });

  it('模式切换往返不丢已配内容：精确勾选 → 条件填值 → 切回精确，选中值仍在', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ brand_name: '比亚迪' }, { brand_name: '奥迪' }] },
    } as never);
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    await waitFor(() => {
      expect(screen.getAllByTestId('filter-modal-enum-item')).toHaveLength(2);
    });
    clickEnumItem('比亚迪');

    switchTab('条件筛选');
    fireEvent.change(screen.getByTestId('filter-modal-text'), { target: { value: '奥' } });

    switchTab('精确筛选');
    // 精确面板回显此前的勾选（摘要 + 选中态）
    expect(screen.getByTestId('filter-modal-enum-summary')).toHaveTextContent(
      '将保留 1 个值：比亚迪'
    );

    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'in',
        value: ['比亚迪'],
        asFilter: false,
        filterLabel: '',
      });
    });
  });
});

describe('FilterConfigModal · 精确筛选面板', () => {
  beforeEach(() => {
    mockedQuery.mockReset();
  });

  it('搜索过滤后可全选当前搜索结果，上抛 in 数组', async () => {
    mockedQuery.mockResolvedValue({
      data: {
        code: 0,
        data: [{ brand_name: '比亚迪' }, { brand_name: '比亚迪经销' }, { brand_name: '奥迪' }],
      },
    } as never);
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    await waitFor(() => {
      expect(screen.getAllByTestId('filter-modal-enum-item')).toHaveLength(3);
    });
    fireEvent.change(screen.getByTestId('filter-modal-enum-search'), {
      target: { value: '比亚迪' },
    });
    expect(screen.getByTestId('filter-modal-enum-select-all')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('filter-modal-enum-select-all'));

    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'in',
        value: expect.arrayContaining(['比亚迪', '比亚迪经销']),
        asFilter: false,
        filterLabel: '',
      });
    });
    const payload = onOk.mock.calls[0][0];
    expect(payload.value).toHaveLength(2);
  });

  it('「查看已选项」视图只列选中值', async () => {
    mockedQuery.mockResolvedValue({
      data: {
        code: 0,
        data: [{ brand_name: '比亚迪' }, { brand_name: '奥迪' }, { brand_name: '本田' }],
      },
    } as never);
    renderModal({ field: STRING_FIELD });

    await waitFor(() => {
      expect(screen.getAllByTestId('filter-modal-enum-item')).toHaveLength(3);
    });
    clickEnumItem('奥迪');

    fireEvent.click(screen.getByText('已选(1)'));
    const visible = screen.getAllByTestId('filter-modal-enum-item');
    expect(visible).toHaveLength(1);
    expect(visible[0].closest('label')?.textContent).toBe('奥迪');
  });

  it('候选值超过抽样上限时只渲染前 50 条，「更多数据项」展开全部', async () => {
    const many = Array.from({ length: 60 }, (_, i) => ({
      brand_name: `v${String(i).padStart(2, '0')}`,
    }));
    mockedQuery.mockResolvedValue({ data: { code: 0, data: many } } as never);

    renderModal({ field: STRING_FIELD });

    await waitFor(() => {
      expect(screen.getByText('更多数据项（剩余 10）')).toBeInTheDocument();
    });
    expect(screen.getAllByTestId('filter-modal-enum-item')).toHaveLength(50);
    fireEvent.click(screen.getByTestId('filter-modal-enum-expand'));
    expect(screen.getAllByTestId('filter-modal-enum-item')).toHaveLength(60);
  });

  it('搜索无结果可「加入选项」，新值成为候选并被选中', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ brand_name: '比亚迪' }] },
    } as never);
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    await waitFor(() => {
      expect(screen.getAllByTestId('filter-modal-enum-item')).toHaveLength(1);
    });
    fireEvent.change(screen.getByTestId('filter-modal-enum-search'), { target: { value: '蔚来' } });
    fireEvent.click(screen.getByTestId('filter-modal-enum-add-option'));

    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'in',
        value: ['蔚来'],
        asFilter: false,
        filterLabel: '',
      });
    });
  });

  it('反选后上抛 notIn，摘要显示「将排除」', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ brand_name: '比亚迪' }, { brand_name: '奥迪' }] },
    } as never);
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    await waitFor(() => {
      expect(screen.getAllByTestId('filter-modal-enum-item')).toHaveLength(2);
    });
    clickEnumItem('奥迪');
    fireEvent.click(screen.getByTestId('filter-modal-enum-notin'));
    expect(screen.getByTestId('filter-modal-enum-notin')).toBeChecked();
    expect(screen.getByTestId('filter-modal-enum-summary')).toHaveTextContent('将排除 1 个值');

    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'notIn',
        value: ['奥迪'],
        asFilter: false,
        filterLabel: '',
      });
    });
  });

  it('编辑 in/notIn 条件回显进精确筛选：勾选态与选中值都在', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ retail_sales: 100 }] },
    } as never);

    renderModal({ initial: makeInitial({ operator: 'notIn', value: ['100', '200'] }) });

    expect(screen.getByTestId('filter-modal-enum-notin')).toBeChecked();
    expect(screen.getByTestId('filter-modal-enum-summary')).toHaveTextContent('将排除 2 个值');
  });
});

describe('FilterConfigModal · 条件筛选', () => {
  beforeEach(() => {
    mockedQuery.mockReset();
  });

  it('数值：比较模式默认大于等于，区间子模式上抛 between', () => {
    const onOk = vi.fn();
    renderModal({ onOk });

    fireEvent.change(screen.getByTestId('filter-modal-number'), { target: { value: '1000' } });
    clickOk();
    expect(onOk).toHaveBeenCalledWith({
      operator: 'gte',
      value: 1000,
      asFilter: false,
      filterLabel: '',
    });
    fireEvent.click(screen.getByText('区间'));
    fireEvent.change(screen.getByTestId('filter-modal-min'), { target: { value: '10' } });
    fireEvent.change(screen.getByTestId('filter-modal-max'), { target: { value: '20' } });
    clickOk();
    expect(onOk).toHaveBeenLastCalledWith({
      operator: 'between',
      value: 10,
      valueEnd: 20,
      asFilter: false,
      filterLabel: '',
    });
  });

  it('数值：枚举子模式用精确面板，上抛 in', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ retail_sales: 100 }, { retail_sales: 50 }, { retail_sales: 80 }] },
    } as never);
    const onOk = vi.fn();
    renderModal({ onOk });

    switchTab('条件筛选');
    fireEvent.click(screen.getByText('枚举'));
    await waitFor(() => {
      expect(mockedQuery).toHaveBeenCalledWith(1, 'retail_sales');
    });
    fireEvent.click(screen.getByTestId('filter-modal-condition-enum-select-all'));
    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'in',
        value: expect.arrayContaining(['100', '80', '50']),
        asFilter: false,
        filterLabel: '',
      });
    });
  });

  it('字符串：新增 开头为/结尾为 算子可选可上抛', async () => {
    mockedQuery.mockResolvedValue({ data: { code: 0, data: [] } } as never);
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    switchTab('条件筛选');
    fireEvent.mouseDown(screen.getByTestId('filter-modal-operator'));
    fireEvent.click(await screen.findByTitle('开头为'));
    fireEvent.change(screen.getByTestId('filter-modal-text'), { target: { value: '比' } });
    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'startsWith',
        value: '比',
        asFilter: false,
        filterLabel: '',
      });
    });
  });

  it('字符串：为空字符串/不为空字符串是无值算子，确定不带值', async () => {
    mockedQuery.mockResolvedValue({ data: { code: 0, data: [] } } as never);
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    switchTab('条件筛选');
    fireEvent.mouseDown(screen.getByTestId('filter-modal-operator'));
    fireEvent.click(await screen.findByTitle('不为空字符串'));
    expect(screen.queryByTestId('filter-modal-text')).not.toBeInTheDocument();
    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'isNotEmptyString',
        value: null,
        asFilter: false,
        filterLabel: '',
      });
    });
  });

  it('编辑 between 条件回显进区间子模式', () => {
    renderModal({ initial: makeInitial({ operator: 'between', value: 10, valueEnd: 99 }) });

    expect(screen.getByTestId('filter-modal-min')).toHaveValue('10');
    expect(screen.getByTestId('filter-modal-max')).toHaveValue('99');
  });
});

describe('FilterConfigModal · 手动输入', () => {
  beforeEach(() => {
    mockedQuery.mockResolvedValue({ data: { code: 0, data: [] } } as never);
  });

  it('分隔符分词 + 去重 + 忽略空项，上抛 in', async () => {
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    switchTab('手动输入');
    fireEvent.change(screen.getByTestId('filter-modal-manual'), {
      target: { value: '北京\n上海\n 北京\n\n广州' },
    });
    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'in',
        value: ['北京', '上海', '广州'],
        asFilter: false,
        filterLabel: '',
      });
    });
  });

  it('逗号分隔模式同样分词', async () => {
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    switchTab('手动输入');
    fireEvent.click(screen.getByText('逗号'));
    fireEvent.change(screen.getByTestId('filter-modal-manual'), {
      target: { value: '比亚迪, 蔚来，理想' },
    });
    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'in',
        value: ['比亚迪', '蔚来', '理想'],
        asFilter: false,
        filterLabel: '',
      });
    });
  });

  it('候选值加载成功时，不在取值域内的值标红且不进条件', async () => {
    mockedQuery.mockResolvedValue({
      data: { code: 0, data: [{ brand_name: '比亚迪' }, { brand_name: '奥迪' }] },
    } as never);
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    await waitFor(() => {
      expect(mockedQuery).toHaveBeenCalled();
    });
    switchTab('手动输入');
    fireEvent.change(screen.getByTestId('filter-modal-manual'), {
      target: { value: '比亚迪\n不存在的品牌' },
    });
    expect(screen.getByTestId('filter-modal-manual-invalid')).toHaveTextContent(
      '1 个非法值将被忽略'
    );
    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'in',
        value: ['比亚迪'],
        asFilter: false,
        filterLabel: '',
      });
    });
  });

  it('反选后手动输入上抛 notIn', async () => {
    const onOk = vi.fn();
    renderModal({ field: STRING_FIELD, onOk });

    switchTab('手动输入');
    fireEvent.change(screen.getByTestId('filter-modal-manual'), { target: { value: '某值' } });
    fireEvent.click(screen.getByTestId('filter-modal-notin'));
    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledWith({
        operator: 'notIn',
        value: ['某值'],
        asFilter: false,
        filterLabel: '',
      });
    });
  });
});

describe('FilterConfigModal · 日期兜底分支', () => {
  beforeEach(() => {
    mockedQuery.mockReset();
  });

  it('日期字段无值时确定禁用（防半成品条件写回）', () => {
    renderModal({ field: DATE_FIELD });

    expect(screen.getByRole('button', { name: '确 定' }).hasAttribute('disabled')).toBe(true);
  });

  it('日期字段：回显已有 isNull 时算子不被改写（不得静默变成 eq）', () => {
    renderModal({ field: DATE_FIELD, initial: makeInitial({ operator: 'isNull', value: null }) });

    expect(screen.getByTestId('filter-modal-operator')).toHaveTextContent('为空');
  });
});

/**
 * #111 回归：`isNull` / `isNotNull` 是无值算子，编辑已存条件不得被静默改写；
 * 回显入口是条件筛选 Tab（in/notIn 例外，归精确筛选）。
 */
describe('FilterConfigModal · 无值算子（isNull / isNotNull）', () => {
  beforeEach(() => {
    mockedQuery.mockReset();
    mockedQuery.mockResolvedValue({ data: { code: 0, data: [] } } as never);
  });

  it('数值字段：可创建「为空」，值区不渲染输入控件', async () => {
    const onOk = vi.fn();
    renderModal({ onOk });

    expect(screen.getByTestId('filter-modal-number')).toBeInTheDocument();
    fireEvent.mouseDown(screen.getByTestId('filter-modal-operator'));
    fireEvent.click(await screen.findByTitle('为空'));

    await waitFor(() => {
      expect(screen.queryByTestId('filter-modal-number')).not.toBeInTheDocument();
    });

    clickOk();
    expect(onOk).toHaveBeenCalledWith({
      operator: 'isNull',
      value: null,
      asFilter: false,
      filterLabel: '',
    });
  });

  it('数值字段：回显已有 isNull 时算子不被改写，确定即上抛 isNull', () => {
    const onOk = vi.fn();
    renderModal({ onOk, initial: makeInitial({ operator: 'isNull', value: null }) });

    expect(screen.getByTestId('filter-modal-operator')).toHaveTextContent('为空');
    expect(screen.queryByTestId('filter-modal-number')).not.toBeInTheDocument();

    clickOk();
    expect(onOk).toHaveBeenCalledWith({
      operator: 'isNull',
      value: null,
      asFilter: false,
      filterLabel: '',
    });
  });

  it('数值字段：回显已有 isNotNull 时确定即上抛 isNotNull', () => {
    const onOk = vi.fn();
    renderModal({ onOk, initial: makeInitial({ operator: 'isNotNull', value: null }) });

    expect(screen.getByTestId('filter-modal-operator')).toHaveTextContent('不为空');

    clickOk();
    expect(onOk).toHaveBeenCalledWith({
      operator: 'isNotNull',
      value: null,
      asFilter: false,
      filterLabel: '',
    });
  });

  it('字符串字段：回显已有 isNull 时不回落到精确筛选，确定即上抛 isNull', () => {
    const onOk = vi.fn();
    renderModal({
      field: STRING_FIELD,
      onOk,
      initial: makeInitial({ fieldId: STRING_FIELD.name, operator: 'isNull', value: null }),
    });

    // 条件筛选 Tab 停驻：精确面板不渲染
    expect(screen.queryByTestId('filter-modal-enum-search')).not.toBeInTheDocument();
    expect(screen.getByTestId('filter-modal-operator')).toHaveTextContent('为空');

    clickOk();
    expect(onOk).toHaveBeenCalledWith({
      operator: 'isNull',
      value: null,
      asFilter: false,
      filterLabel: '',
    });
  });
});

/**
 * 「作为筛选器」（全族通用）：勾选后条件顶层带 asFilter/filterLabel，
 * 图表预览区上方渲染行内筛选控件（渲染侧见 FilterValueControl / ChartBuilder）。
 */
describe('FilterConfigModal · 作为筛选器', () => {
  beforeEach(() => {
    mockedQuery.mockReset();
    mockedQuery.mockResolvedValue({ data: { code: 0, data: [] } } as never);
  });

  it('默认不勾选，patch 上抛 asFilter: false', () => {
    const onOk = vi.fn();
    renderModal({ onOk });

    expect(screen.getByTestId('filter-modal-as-filter')).not.toBeChecked();
    expect(screen.queryByTestId('filter-modal-filter-label')).not.toBeInTheDocument();

    fireEvent.change(screen.getByTestId('filter-modal-number'), { target: { value: '5' } });
    clickOk();
    expect(onOk).toHaveBeenCalledWith({
      operator: 'gte',
      value: 5,
      asFilter: false,
      filterLabel: '',
    });
  });

  it('勾选后出显示名称输入，patch 上抛 asFilter: true 与自定义名称', async () => {
    const onOk = vi.fn();
    renderModal({ onOk, field: STRING_FIELD });

    fireEvent.click(screen.getByTestId('filter-modal-as-filter'));
    expect(screen.getByTestId('filter-modal-filter-label')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('filter-modal-filter-label'), {
      target: { value: '品牌筛选' },
    });
    switchTab('手动输入');
    fireEvent.change(screen.getByTestId('filter-modal-manual'), { target: { value: '比亚迪' } });
    clickOk();
    expect(onOk).toHaveBeenCalledWith({
      operator: 'in',
      value: ['比亚迪'],
      asFilter: true,
      filterLabel: '品牌筛选',
    });
  });

  it('编辑已勾选条件时勾选态与显示名称回显，取消勾选上抛 asFilter: false', () => {
    const onOk = vi.fn();
    renderModal({
      onOk,
      initial: makeInitial({ operator: 'gte', value: 5, asFilter: true, filterLabel: '销售额' }),
    });

    expect(screen.getByTestId('filter-modal-as-filter')).toBeChecked();
    expect(screen.getByTestId('filter-modal-filter-label')).toHaveValue('销售额');

    fireEvent.click(screen.getByTestId('filter-modal-as-filter'));
    clickOk();
    expect(onOk).toHaveBeenCalledWith({
      operator: 'gte',
      value: 5,
      asFilter: false,
      filterLabel: '销售额',
    });
  });
});
