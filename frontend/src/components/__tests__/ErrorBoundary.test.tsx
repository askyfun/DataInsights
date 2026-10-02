import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { messages } from '../../i18n/useLocale';
import ErrorBoundary from '../ErrorBoundary';

function Bomb({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) throw new Error('boom');
  return <div>fine</div>;
}

const renderWithIntl = (ui: React.ReactElement) =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      {ui}
    </IntlProvider>
  );

describe('ErrorBoundary', () => {
  let errorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    errorSpy = vi.spyOn(console, 'error');
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('子树正常时不渲染 fallback', () => {
    renderWithIntl(
      <ErrorBoundary>
        <Bomb shouldThrow={false} />
      </ErrorBoundary>
    );
    expect(screen.getByText('fine')).toBeInTheDocument();
  });

  it('子树抛错时渲染错误页而非白屏，重试后恢复', async () => {
    const { rerender } = renderWithIntl(
      <ErrorBoundary>
        <Bomb shouldThrow />
      </ErrorBoundary>
    );
    await waitFor(() => {
      expect(screen.getByText('页面出错了')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /重\s*试/ })).toBeInTheDocument();
    });
    expect(errorSpy).toHaveBeenCalled();

    rerender(
      <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
        <ErrorBoundary>
          <Bomb shouldThrow={false} />
        </ErrorBoundary>
      </IntlProvider>
    );
    fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }));
    expect(screen.getByText('fine')).toBeInTheDocument();
  });
});
