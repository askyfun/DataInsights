// #268 分页「每页条数」应为只读选择、不进入搜索模式（避免密码管理器把可输入框当凭据框）。
import { fireEvent, render, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import PageSizeChanger from '../../../components/ui/PageSizeChanger';

describe('ui/PageSizeChanger（#268）', () => {
  it('不进入搜索模式，输入框只读', () => {
    const { container } = render(<PageSizeChanger value={100} />);
    // 内置 SizeChanger 是 showSearch:true → 带 ant-select-show-search；修复后不应出现
    expect(container.querySelector('.ant-select-show-search')).toBeNull();
    const input = container.querySelector('input.ant-select-input') as HTMLInputElement | null;
    // 非搜索态的 rc-select 输入框为 readonly（不可键入）
    if (input) expect(input.readOnly || input.hasAttribute('readonly')).toBe(true);
  });

  it('从固定选项里选择后以数字回调 onChange', async () => {
    const onChange = vi.fn();
    const { container } = render(<PageSizeChanger value={100} onChange={onChange} />);
    fireEvent.mouseDown(container.querySelector('.ant-select') as HTMLElement);
    const option = await waitFor(() => {
      const found = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (n) => n.textContent === '50 条/页'
      );
      expect(found).toBeTruthy();
      return found as HTMLElement;
    });
    fireEvent.click(option);
    expect(onChange).toHaveBeenCalledWith(50);
  });
});
