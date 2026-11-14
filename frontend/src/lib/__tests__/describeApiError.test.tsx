import { render, screen } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { messages } from '../../i18n/useLocale';
import { ApiClientError, resolveErrorKind } from '../api/client';
import { buildReportText, describeApiError, notifyApiError } from '../describeApiError';

vi.mock('antd', async (importOriginal) => {
  const actual = await importOriginal<typeof import('antd')>();
  return {
    ...actual,
    message: {
      open: vi.fn(),
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
    },
  };
});

import { message } from 'antd';

// 极简 intl 替身：formatMessage 直接查 zh-CN 词条并做 {var} 插值，
// 让 describeApiError 的输出可断言。
function makeIntl() {
  const dict = messages['zh-CN'];
  return {
    formatMessage: (descriptor: { id: string }, vals?: Record<string, string>) => {
      let out: string = dict[descriptor.id] ?? descriptor.id;
      if (vals) {
        for (const [k, v] of Object.entries(vals)) out = out.split(`{${k}}`).join(v);
      }
      return out;
    },
  } as never as Parameters<typeof describeApiError>[1];
}

const renderWithIntl = (ui: React.ReactElement) =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      {ui}
    </IntlProvider>
  );

describe('resolveErrorKind（六档表驱动，issue #222 验收 4）', () => {
  const cases: Array<{
    name: string;
    input: Parameters<typeof resolveErrorKind>[0];
    want: string;
  }> = [
    { name: '取消', input: { axiosCode: 'ERR_CANCELED' }, want: 'canceled' },
    {
      name: '断网优先于超时',
      input: { axiosCode: 'ECONNABORTED', timedOut: true, online: false },
      want: 'offline',
    },
    { name: '断网', input: { axiosCode: 'ERR_NETWORK', online: false }, want: 'offline' },
    {
      name: '超时',
      input: { axiosCode: 'ECONNABORTED', timedOut: true, online: true },
      want: 'timeout',
    },
    { name: '网络层', input: { axiosCode: 'ERR_NETWORK' }, want: 'network' },
    { name: '业务码', input: { code: 20300, online: true }, want: 'business' },
    { name: 'HTTP 5xx', input: { status: 502, online: true }, want: 'server' },
    { name: 'HTTP 4xx 无信封码', input: { status: 404, online: true }, want: 'server' },
    { name: '无任何信号兜底', input: {}, want: 'network' },
  ];
  for (const c of cases) {
    it(c.name, () => {
      expect(resolveErrorKind(c.input)).toBe(c.want);
    });
  }
});

describe('describeApiError（issue #222 验收 3/5）', () => {
  const intl = makeIntl();

  it('断网：给断网文案且不带编号（没有请求就没有 id）', () => {
    const err = new ApiClientError('Network Error', { axiosCode: 'ERR_NETWORK', online: false });
    const d = describeApiError(err, intl);
    expect(d.kind).toBe('offline');
    expect(d.text).toBe('网络已断开，请检查连接后重试');
    expect(d.errorId).toBeUndefined();
  });

  it('服务端错误：带编号且报障文本包含编号 / 请求 id / 路由 / 版本', () => {
    const err = new ApiClientError('boom', {
      status: 500,
      errorId: 'DI-50000-3F7A2C9B',
      requestId: 'req-1',
      online: true,
    });
    const d = describeApiError(err, intl);
    expect(d.kind).toBe('server');
    expect(d.text).toBe('服务暂时不可用，请稍后重试');
    expect(d.errorId).toBe('DI-50000-3F7A2C9B');
    expect(d.reportText).toContain('Error ID: DI-50000-3F7A2C9B');
    expect(d.reportText).toContain('Request ID: req-1');
    expect(d.reportText).toContain('HTTP status: 500');
    expect(d.reportText).toContain('Route: /');
    expect(d.reportText).toContain('Version: dev');
  });

  it('业务错误：优先后端 msg 原样展示（面向用户，有 Dashboards 测试钉死此口径）', () => {
    const err = new ApiClientError('数据源不存在', { code: 20300, online: true });
    const d = describeApiError(err, intl);
    expect(d.kind).toBe('business');
    expect(d.text).toBe('数据源不存在');
  });

  it('未知错误按 server 档处理，无编号', () => {
    const d = describeApiError(new Error('plain'), intl);
    expect(d.kind).toBe('server');
    expect(d.errorId).toBeUndefined();
  });
});

describe('notifyApiError（toast 统一出口）', () => {
  const intl = makeIntl();

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('canceled 静默：不弹任何提示', () => {
    const err = new ApiClientError('canceled', { axiosCode: 'ERR_CANCELED' });
    notifyApiError(err, intl);
    expect(message.open).not.toHaveBeenCalled();
  });

  it('断网：弹「网络已断开」且内容不含编号', () => {
    const err = new ApiClientError('Network Error', { axiosCode: 'ERR_NETWORK', online: false });
    notifyApiError(err, intl);
    expect(message.open).toHaveBeenCalledTimes(1);
    const call = vi.mocked(message.open).mock.calls[0][0];
    expect(String(call.content)).toContain('网络已断开');
    expect(String(call.content)).not.toContain('DI-');
  });

  it('服务端错误：内容带错误编号', () => {
    const err = new ApiClientError('boom', {
      status: 500,
      errorId: 'DI-50000-ABCDEF01',
      online: true,
    });
    notifyApiError(err, intl);
    const call = vi.mocked(message.open).mock.calls[0][0];
    expect(String(call.content)).toContain('DI-50000-ABCDEF01');
  });
});

describe('buildReportText', () => {
  it('componentStack 超长截断', () => {
    const text = buildReportText({
      errorId: 'DI-50000-3F7A2C9B',
      route: '/charts',
      componentStack: 'x'.repeat(2000),
    });
    expect(text).toContain('…(truncated)');
    expect(text.length).toBeLessThan(1200);
  });
});

describe('notifyApiError 渲染冒烟（IntlProvider 下不抛错）', () => {
  it('服务端错误 toast 可渲染', () => {
    const intl = makeIntl();
    expect(() =>
      renderWithIntl(
        <button type="button" onClick={() => notifyApiError(new Error('x'), intl)}>
          go
        </button>
      )
    ).not.toThrow();
    expect(screen.getByText('go')).toBeTruthy();
  });
});
