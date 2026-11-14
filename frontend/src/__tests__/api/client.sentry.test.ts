// R-98（issue #223）验收 7：API 客户端失败统一上报。
// 业务码 ≠20000 与 HTTP ≥400 都必须 captureMessage（带 biz_code/request_id/
// error_id/路由），且 error_id 透传到 ApiClientError。Sentry 用 mock 替换，
// 传输层用 mock adapter 替换（同 client.retryCache.test.ts 的做法）。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { captureMessage } = vi.hoisted(() => ({ captureMessage: vi.fn() }));
vi.mock('@sentry/react', () => ({ captureMessage }));

import { ApiClientError, apiClient } from '@/lib/api/client';

function mockAdapter(impl: (config: Record<string, unknown>) => Promise<unknown>) {
  (apiClient.defaults as { adapter: unknown }).adapter = vi.fn((config: Record<string, unknown>) =>
    impl(config)
  );
}

beforeEach(() => {
  captureMessage.mockClear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('API 客户端失败上报（R-98 验收 7）', () => {
  it('业务码 50000 上报一次，tags 带 biz_code/error_id/路由，errorId 透传', async () => {
    mockAdapter((config) =>
      Promise.resolve({
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
        data: {
          code: 50000,
          msg: 'db exploded',
          trace: 'req-1',
          data: {},
          error_id: 'DI-50000-ABCDEF12',
        },
      })
    );

    const err = await apiClient.post('/x', { a: 1 }).catch((e: ApiClientError) => e);
    expect(err).toBeInstanceOf(ApiClientError);
    expect((err as ApiClientError).errorId).toBe('DI-50000-ABCDEF12');

    expect(captureMessage).toHaveBeenCalledTimes(1);
    const [message, opts] = captureMessage.mock.calls[0] as [
      string,
      { level: string; tags: Record<string, string> },
    ];
    expect(message).toContain('API Error [50000]');
    expect(opts.level).toBe('error');
    expect(opts.tags.biz_code).toBe('50000');
    expect(opts.tags.error_id).toBe('DI-50000-ABCDEF12');
    expect(opts.tags.route).toBe(window.location.pathname);
  });

  it('业务码 20100（4xx 族）同样上报——前端口径与后端降噪口径不同（验收 7 原文：≠20000）', async () => {
    mockAdapter((config) =>
      Promise.resolve({
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
        data: { code: 20100, msg: 'bad input', trace: 'req-2', data: {} },
      })
    );

    await apiClient.post('/x', {}).catch(() => undefined);
    expect(captureMessage).toHaveBeenCalledTimes(1);
    const [, opts] = captureMessage.mock.calls[0] as [string, { tags: Record<string, string> }];
    expect(opts.tags.biz_code).toBe('20100');
    // 信封没带 error_id 时 tag 为空串（占位），不许出现 undefined
    expect(opts.tags.error_id).toBe('');
  });

  it('HTTP 500 上报且带 error_id；panic 信封是 HTTP 200 + 50000，不会走进这个分支', async () => {
    mockAdapter((config) =>
      Promise.reject({
        code: 'ERR_BAD_RESPONSE',
        response: { status: 500, data: { msg: 'boom', error_id: 'DI-50000-11223344' } },
        config,
      })
    );

    const err = await apiClient
      .get('/x', { skipCache: true, skipRetry: true })
      .catch((e: ApiClientError) => e);
    expect((err as ApiClientError).errorId).toBe('DI-50000-11223344');
    expect(captureMessage).toHaveBeenCalledTimes(1);
    const [, opts] = captureMessage.mock.calls[0] as [string, { tags: Record<string, string> }];
    expect(opts.tags.error_id).toBe('DI-50000-11223344');
  });

  it('成功响应不产生任何上报', async () => {
    mockAdapter((config) =>
      Promise.resolve({
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
        data: { code: 20000, msg: 'success', trace: 'req-3', data: {} },
      })
    );

    await apiClient.get('/x', { skipCache: true });
    expect(captureMessage).not.toHaveBeenCalled();
  });
});
