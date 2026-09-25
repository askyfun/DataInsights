import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import DateFilterControl from '../../components/DateFilter/DateFilterControl';
import DateFilterModal, {
  type DateFilterModalPayload,
} from '../../components/DateFilter/DateFilterModal';
import type { DateFilterValue } from '../../lib/dateFilter';

/**
 * 日期筛选器两个外壳（完整弹窗 / 行内控件）的交互。
 *
 * 语义本身（区间怎么算、粒度怎么影响展示）已由 lib/dateFilter.test.ts 钉死，
 * 这里只验「外壳把用户的操作翻译成了什么筛选值」，不做重复断言。
 */

const renderModal = (props: Partial<Parameters<typeof DateFilterModal>[0]> = {}) => {
  const onOk = vi.fn<(payload: DateFilterModalPayload) => void>();
  const onCancel = vi.fn();
  render(<DateFilterModal open fieldName="交易日期" onOk={onOk} onCancel={onCancel} {...props} />);
  return { onOk, onCancel };
};

const clickOk = () => fireEvent.click(screen.getByRole('button', { name: /确\s*定/ }));
const okDisabled = () =>
  (screen.getByRole('button', { name: /确\s*定/ }) as HTMLButtonElement).disabled;

describe('DateFilterModal 布局与出参', () => {
  it('渲染五模式导航、粒度下拉与「作为时间范围筛选器」', () => {
    renderModal();

    expect(screen.getByText('日期筛选')).toBeInTheDocument();
    for (const [mode, label] of [
      ['dynamic', '动态日期'],
      ['fixed', '固定日期'],
      ['advanced', '高级'],
      ['special', '特殊值'],
      ['single', '单个日期'],
    ] as const) {
      const item = screen.getByTestId(`date-filter-mode-${mode}`);
      expect(item.textContent).toBe(label);
    }
    expect(screen.getByText('范围')).toBeInTheDocument();
    expect(screen.getByTestId('date-filter-granularity')).toBeInTheDocument();
    expect(screen.getByTestId('date-filter-as-filter')).toBeInTheDocument();
    expect(screen.getByTestId('date-filter-label')).toBeInTheDocument();
  });

  it('未选择日期时「确定」禁用；点一个快捷选项后可用并带上该选项', async () => {
    const { onOk } = renderModal();

    expect(okDisabled()).toBe(true);

    fireEvent.click(screen.getByTestId('date-filter-preset-last7d'));
    expect(okDisabled()).toBe(false);
    // 时间预览换成具体区间（可读性），不是选项名
    expect(screen.getByTestId('date-filter-preview').textContent).not.toBe('请选择');

    clickOk();
    await waitFor(() => {
      expect(onOk).toHaveBeenCalledTimes(1);
    });
    const payload = onOk.mock.calls[0][0];
    expect(payload.value).toEqual({ kind: 'dynamic', preset: 'last7d', includeEmpty: undefined });
    expect(payload.granularity).toBe('day');
    expect(payload.weekStart).toBe(1);
  });

  it('「作为时间范围筛选器」与「显示名称」进 payload；名称留空时回落字段名', () => {
    const { onOk } = renderModal();

    fireEvent.click(screen.getByTestId('date-filter-preset-last30d'));
    fireEvent.click(screen.getByTestId('date-filter-as-filter'));
    clickOk();

    expect(onOk.mock.calls[0][0]).toMatchObject({ asFilter: true, filterLabel: '交易日期' });
  });

  it('自定义动态日期：操作符/数值/单位与「包含今天」一起进筛选值', () => {
    const { onOk } = renderModal();

    fireEvent.click(screen.getByTestId('date-filter-preset-custom'));
    expect(screen.getByTestId('date-filter-custom')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('date-filter-custom-n'), { target: { value: '10' } });
    clickOk();

    expect(onOk.mock.calls[0][0].value).toMatchObject({
      kind: 'dynamic',
      custom: { op: 'last', n: 10, unit: 'day', includeToday: true },
    });
  });

  it('先勾「包含空日期」再开「自定义」，勾选不被静默清掉', () => {
    const { onOk } = renderModal();

    fireEvent.click(screen.getByTestId('date-filter-include-empty'));
    fireEvent.click(screen.getByTestId('date-filter-preset-custom'));
    clickOk();

    expect(onOk.mock.calls[0][0].value).toMatchObject({ kind: 'dynamic', includeEmpty: true });
  });

  it('粒度决定快捷选项集合：月粒度给到「最近 12 个月」，日粒度没有', () => {
    const { unmount } = render(
      <DateFilterModal
        open
        fieldName="交易日期"
        initialGranularity="month"
        onOk={vi.fn()}
        onCancel={vi.fn()}
      />
    );
    expect(screen.getByTestId('date-filter-preset-last12m')).toBeInTheDocument();
    expect(screen.queryByTestId('date-filter-preset-last7d')).not.toBeInTheDocument();
    // 周粒度的专属项（周计算逻辑）在月粒度下不出现
    expect(screen.queryByTestId('date-filter-week-start')).not.toBeInTheDocument();
    unmount();
  });

  it('周粒度展示「周计算逻辑」下拉', () => {
    renderModal({ initialGranularity: 'week' });
    expect(screen.getByTestId('date-filter-week-start')).toBeInTheDocument();
  });

  it('固定日期：区间回显并可提交', () => {
    const { onOk } = renderModal({
      initial: { kind: 'fixed', start: '2026-08-08', end: '2026-08-12' },
    });

    expect(okDisabled()).toBe(false);
    clickOk();
    expect(onOk.mock.calls[0][0].value).toEqual({
      kind: 'fixed',
      start: '2026-08-08',
      end: '2026-08-12',
    });
  });

  it('高级：起止同时「无限制」给出冲突提示并禁用「确定」', () => {
    renderModal({
      initial: {
        kind: 'advanced',
        start: { type: 'unlimited' },
        end: { type: 'unlimited' },
      },
    });

    expect(screen.getByTestId('date-filter-advanced-conflict')).toBeInTheDocument();
    expect(okDisabled()).toBe(true);
  });

  it('高级：单侧无限制是合法配置', () => {
    const { onOk } = renderModal({
      initial: {
        kind: 'advanced',
        start: { type: 'fixed', value: '2023-06-14' },
        end: { type: 'unlimited' },
      },
    });

    expect(okDisabled()).toBe(false);
    clickOk();
    expect(onOk.mock.calls[0][0].value).toEqual({
      kind: 'advanced',
      start: { type: 'fixed', value: '2023-06-14' },
      end: { type: 'unlimited' },
    });
  });

  it('特殊值：三个单选项直接可提交', () => {
    const { onOk } = renderModal({ initial: { kind: 'special', value: 'empty' } });

    expect(okDisabled()).toBe(false);
    clickOk();
    expect(onOk.mock.calls[0][0].value).toEqual({ kind: 'special', value: 'empty' });
  });

  it('单个日期：未选日期时禁用，回显后可用', () => {
    renderModal({ initial: { kind: 'single', date: '' } });
    expect(okDisabled()).toBe(true);
  });

  it('切换模式后各模式的已填内容互不覆盖；切回仍保留', () => {
    const { onOk } = renderModal();

    fireEvent.click(screen.getByTestId('date-filter-preset-last14d'));
    fireEvent.click(screen.getByTestId('date-filter-mode-special'));
    expect(screen.getByTestId('date-filter-special')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('date-filter-mode-dynamic'));

    clickOk();
    expect(onOk.mock.calls[0][0].value).toMatchObject({ kind: 'dynamic', preset: 'last14d' });
  });
});

describe('DateFilterControl 行内控件', () => {
  const settings = { granularity: 'day' as const };

  it('按钮显示「名称：当前筛选值」，点开浮层可换筛选条件', () => {
    const onChange = vi.fn<(next: DateFilterValue) => void>();
    render(
      <DateFilterControl
        label="交易日期"
        value={{ kind: 'dynamic', preset: 'last7d' }}
        settings={settings}
        onChange={onChange}
      />
    );

    const button = screen.getByTestId('date-filter-control');
    expect(button.textContent).toContain('交易日期');
    expect(button.textContent).toContain('最近 7 天');

    fireEvent.click(button);
    fireEvent.click(screen.getByTestId('date-filter-preset-last30d'));
    expect(onChange).toHaveBeenCalledWith({
      kind: 'dynamic',
      preset: 'last30d',
      includeEmpty: undefined,
    });
  });

  it('allowModeSwitch 才给模式切换栏', () => {
    const { unmount } = render(
      <DateFilterControl
        label="交易日期"
        value={{ kind: 'dynamic', preset: 'last7d' }}
        settings={settings}
        onChange={vi.fn()}
      />
    );
    fireEvent.click(screen.getByTestId('date-filter-control'));
    expect(screen.queryByTestId('date-filter-control-mode-fixed')).not.toBeInTheDocument();
    unmount();

    render(
      <DateFilterControl
        label="交易日期"
        value={{ kind: 'dynamic', preset: 'last7d' }}
        settings={settings}
        allowModeSwitch
        onChange={vi.fn()}
      />
    );
    fireEvent.click(screen.getByTestId('date-filter-control'));
    expect(screen.getByTestId('date-filter-control-mode-fixed')).toBeInTheDocument();
  });

  it('单个日期模式直接给日期选择器，不套浮层', () => {
    render(
      <DateFilterControl
        label="交易日期"
        value={{ kind: 'single', date: '2026-05-15' }}
        settings={settings}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByTestId('date-filter-control-single')).toBeInTheDocument();
    expect(screen.queryByTestId('date-filter-control')).not.toBeInTheDocument();
  });

  it('未选择时摘要显示「请选择」；清除按钮只在已激活时出现', () => {
    const onClear = vi.fn();
    const { unmount } = render(
      <DateFilterControl
        label="交易日期"
        value={{ kind: 'dynamic' }}
        settings={settings}
        allowClear
        onClear={onClear}
        onChange={vi.fn()}
      />
    );
    expect(screen.getByTestId('date-filter-control').textContent).toContain('请选择');
    expect(screen.queryByTestId('date-filter-control-clear')).not.toBeInTheDocument();
    unmount();

    render(
      <DateFilterControl
        label="交易日期"
        value={{ kind: 'dynamic', preset: 'last7d' }}
        settings={settings}
        allowClear
        onClear={onClear}
        onChange={vi.fn()}
      />
    );
    fireEvent.click(screen.getByTestId('date-filter-control-clear'));
    expect(onClear).toHaveBeenCalledTimes(1);
  });
});
