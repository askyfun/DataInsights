import { fireEvent, render, screen } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { describe, expect, it, vi } from 'vitest';
import { messages } from '../../i18n/useLocale';
import SampleDatasetCard from '../SampleDatasetCard';

const renderCard = (props: React.ComponentProps<typeof SampleDatasetCard>) =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <SampleDatasetCard {...props} />
    </IntlProvider>
  );

const noop = () => {};

describe('SampleDatasetCard', () => {
  it('空列表且未忽略时展示引导，点击加载 / 忽略回调对应动作', () => {
    const onLoad = vi.fn();
    const onDismiss = vi.fn();
    renderCard({
      status: { loaded: false, dismissed: false },
      empty: true,
      onLoad,
      onDismiss,
      onRemove: noop,
    });

    expect(screen.getByTestId('sample-dataset-onboarding')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /加\s*载\s*示\s*例\s*数\s*据\s*集/ }));
    fireEvent.click(screen.getByRole('button', { name: /忽\s*略/ }));
    expect(onLoad).toHaveBeenCalledTimes(1);
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it('已忽略或已有数据时不出现引导', () => {
    const { rerender } = renderCard({
      status: { loaded: false, dismissed: true },
      empty: true,
      onLoad: noop,
      onDismiss: noop,
      onRemove: noop,
    });
    expect(screen.queryByTestId('sample-dataset-onboarding')).not.toBeInTheDocument();

    rerender(
      <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
        <SampleDatasetCard
          status={{ loaded: false, dismissed: false }}
          empty={false}
          onLoad={noop}
          onDismiss={noop}
          onRemove={noop}
        />
      </IntlProvider>
    );
    expect(screen.queryByTestId('sample-dataset-onboarding')).not.toBeInTheDocument();
  });

  it('已加载时展示移除入口并可一键移除', () => {
    const onRemove = vi.fn();
    renderCard({
      status: { loaded: true, dismissed: false },
      empty: false,
      onLoad: noop,
      onDismiss: noop,
      onRemove,
    });

    expect(screen.getByTestId('sample-dataset-loaded')).toBeInTheDocument();
    expect(screen.queryByTestId('sample-dataset-onboarding')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /移\s*除\s*示\s*例\s*数\s*据/ }));
    expect(onRemove).toHaveBeenCalledTimes(1);
  });

  it('状态未就绪时不渲染任何内容', () => {
    const { container } = renderCard({
      status: null,
      empty: true,
      onLoad: noop,
      onDismiss: noop,
      onRemove: noop,
    });
    expect(container).toBeEmptyDOMElement();
  });
});
