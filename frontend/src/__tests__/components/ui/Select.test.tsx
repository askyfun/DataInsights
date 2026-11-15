// #264 统一 Select 封装：默认开启关键词搜索（大小写不敏感、按 label 过滤），
// allowSearch={false} 可显式关闭；无命中给可读 notFoundContent。
// 注：antd v6 的 Select DOM 为 .ant-select > .ant-select-content > input.ant-select-input。
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { describe, expect, it } from 'vitest';
import Select from '../../../components/ui/Select';

const renderSelect = (ui: React.ReactNode) =>
  render(
    <IntlProvider locale="en" messages={{ 'common.noMatch': 'No matches' }}>
      {ui}
    </IntlProvider>
  );

const visibleOptionTexts = () =>
  Array.from(document.querySelectorAll('.ant-select-item-option'))
    .filter((n) => !n.closest('.ant-select-dropdown-hidden'))
    .map((n) => n.textContent);

describe('ui/Select 统一封装（#264）', () => {
  it('默认开启搜索：输入关键词按 label 大小写不敏感过滤', async () => {
    const { container } = renderSelect(
      <Select
        placeholder="pick"
        options={[
          { value: 'apple', label: 'Apple' },
          { value: 'apricot', label: 'Apricot' },
          { value: 'banana', label: 'Banana' },
        ]}
      />
    );
    // 默认开启搜索 → 根节点带 ant-select-show-search，且有可输入 input
    expect(container.querySelector('.ant-select-show-search')).toBeTruthy();
    const input = container.querySelector('input.ant-select-input') as HTMLInputElement;
    expect(input).toBeTruthy();

    fireEvent.mouseDown(container.querySelector('.ant-select') as HTMLElement);
    await waitFor(() => expect(visibleOptionTexts()).toContain('Apple'));
    // 'AP' 大写，命中大小写不敏感的 Apple / Apricot，排除 Banana
    fireEvent.change(input, { target: { value: 'AP' } });
    await waitFor(() => {
      const texts = visibleOptionTexts();
      expect(texts).toContain('Apple');
      expect(texts).toContain('Apricot');
      expect(texts).not.toContain('Banana');
    });
  });

  it('allowSearch={false} 关闭搜索（不进搜索模式）', () => {
    const { container } = renderSelect(
      <Select
        allowSearch={false}
        placeholder="pick"
        options={[
          { value: 'a', label: 'A' },
          { value: 'b', label: 'B' },
        ]}
      />
    );
    expect(container.querySelector('.ant-select-show-search')).toBeNull();
  });

  it('无命中时展示 notFoundContent（默认多语言键 common.noMatch）', async () => {
    const { container } = renderSelect(
      <Select placeholder="pick" options={[{ value: 'apple', label: 'Apple' }]} />
    );
    const input = container.querySelector('input.ant-select-input') as HTMLInputElement;
    fireEvent.mouseDown(container.querySelector('.ant-select') as HTMLElement);
    fireEvent.change(input, { target: { value: 'zzz' } });
    await waitFor(() => {
      expect(screen.getByText('No matches')).toBeTruthy();
    });
  });
});
