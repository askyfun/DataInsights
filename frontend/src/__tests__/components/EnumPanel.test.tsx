import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import EnumPanel from '../../components/ChartBuilder/EnumPanel';

/**
 * 精确筛选面板（EnumPanel）单测：搜索、全选（含搜索过滤结果）、
 * 查看已选项视图、排序三态、抽样 +「更多数据项」、加入选项。
 */

const CANDIDATES = ['b2', 'A1', 'b10', 'a3'];

const renderPanel = (props: Partial<Parameters<typeof EnumPanel>[0]> = {}) => {
  const onChange = vi.fn();
  const utils = render(
    <EnumPanel
      candidates={CANDIDATES}
      loading={false}
      value={[]}
      onChange={onChange}
      testIdPrefix="ep"
      {...props}
    />
  );
  return { onChange, ...utils };
};

const itemLabels = () =>
  screen.getAllByTestId('ep-item').map((el) => el.closest('label')?.textContent ?? '');

const setSort = async (label: string) => {
  fireEvent.mouseDown(screen.getByTestId('ep-sort'));
  fireEvent.click(await screen.findByTitle(label));
};

describe('EnumPanel', () => {
  it('默认升序（数值感知）：A1 < a3 < b2 < b10', () => {
    renderPanel();
    expect(itemLabels()).toEqual(['A1', 'a3', 'b2', 'b10']);
  });

  it('降序反转顺序', async () => {
    renderPanel();
    await setSort('降序');
    expect(itemLabels()).toEqual(['b10', 'b2', 'a3', 'A1']);
  });

  it('手动排序按选中先后展示', async () => {
    let value: string[] = [];
    const onChange = vi.fn((next: string[]) => {
      value = next;
      panel.rerender(
        <EnumPanel
          candidates={CANDIDATES}
          loading={false}
          value={value}
          onChange={onChange}
          testIdPrefix="ep"
        />
      );
    });
    const panel = render(
      <EnumPanel
        candidates={CANDIDATES}
        loading={false}
        value={value}
        onChange={onChange}
        testIdPrefix="ep"
      />
    );
    // 升序 [A1, a3, b2, b10]：先点 b2（第 3 项），再点 a3（第 2 项）
    fireEvent.click(screen.getAllByTestId('ep-item')[2]);
    fireEvent.click(screen.getAllByTestId('ep-item')[1]);
    expect(value).toEqual(['b2', 'a3']);
    await setSort('手动');
    // 手动模式：已选在前（按选中先后），未选恒在后
    expect(itemLabels()).toEqual(['b2', 'a3', 'A1', 'b10']);
  });

  it('搜索过滤只列匹配项，全选作用于搜索结果', () => {
    const { onChange } = renderPanel();
    fireEvent.change(screen.getByTestId('ep-search'), { target: { value: 'b' } });
    expect(itemLabels()).toEqual(['b2', 'b10']);
    fireEvent.click(screen.getByTestId('ep-select-all'));
    expect(onChange).toHaveBeenCalledWith(['b2', 'b10']);
  });

  it('全选当前可见项（含 locale 排序，不断言顺序）', () => {
    const { onChange } = renderPanel();
    fireEvent.click(screen.getByTestId('ep-select-all'));
    const payload = onChange.mock.calls[0][0] as string[];
    expect(payload).toHaveLength(CANDIDATES.length);
    expect(payload.sort()).toEqual([...CANDIDATES].sort());
  });

  it('查看已选项视图只列选中值', () => {
    renderPanel({ value: ['b2', 'a3'] });
    fireEvent.click(screen.getByText('已选(2)'));
    expect(itemLabels()).toEqual(['a3', 'b2']);
  });

  it('超过抽样上限只渲染 50 条，展开后全量', () => {
    const many = Array.from({ length: 80 }, (_, i) => `v${String(i).padStart(2, '0')}`);
    renderPanel({ candidates: many });
    expect(screen.getAllByTestId('ep-item')).toHaveLength(50);
    fireEvent.click(screen.getByTestId('ep-expand'));
    expect(screen.getAllByTestId('ep-item')).toHaveLength(80);
  });

  it('搜索无结果出现「加入选项」，回调同时带入选中与新候选', () => {
    const onChange = vi.fn();
    const onAddOption = vi.fn();
    render(
      <EnumPanel
        candidates={CANDIDATES}
        loading={false}
        value={[]}
        onChange={onChange}
        onAddOption={onAddOption}
        testIdPrefix="ep"
      />
    );
    fireEvent.change(screen.getByTestId('ep-search'), { target: { value: 'zz' } });
    fireEvent.click(screen.getByTestId('ep-add-option'));
    expect(onChange).toHaveBeenCalledWith(['zz']);
    expect(onAddOption).toHaveBeenCalledWith('zz');
  });

  it('加载失败的空候选给出「手动输入」指引', () => {
    renderPanel({ candidates: [] });
    expect(screen.getByText(/手动输入/)).toBeInTheDocument();
  });
});
