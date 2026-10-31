import { act, render, screen, waitFor } from '@testing-library/react';
import { App } from 'antd';
import { IntlProvider } from 'react-intl';
import { describe, expect, it } from 'vitest';
import { messages } from '../../i18n/useLocale';
import { ApiClientError } from '../../lib/api/client';
import { notifyApiError } from '../../lib/describeApiError';

// 渲染探针（jsdom）：页面走 App.useApp() 实例 + notifyApiError 第三参注入，
// 业务文案必须真的渲染进 DOM（静态 message 在 App 上下文外渲染不出来，见 lib 注释）。
describe('notifyApiError 渲染探针', () => {
  it('App.useApp 实例注入后，业务文案渲染进 DOM', async () => {
    let fire: ((err: unknown) => void) | null = null;
    const Probe = () => {
      const intl = {
        formatMessage: ({ id }: { id: string }) => messages['zh-CN'][id] ?? id,
      } as never as Parameters<typeof notifyApiError>[1];
      const { message } = App.useApp();
      fire = (err: unknown) => notifyApiError(err, intl, message);
      return <div />;
    };
    render(
      <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
        <App>
          <Probe />
        </App>
      </IntlProvider>
    );
    act(() => {
      fire?.(new ApiClientError('业务原文ABC', { code: 20400, online: true }));
    });
    await waitFor(() => expect(screen.getByText(/业务原文ABC/)).toBeInTheDocument(), {
      timeout: 3000,
    });
  });
});
