// issue #99：API 客户端增强 —— GET 自动重试、GET 微缓存/在途去重、细粒度 ApiClientError。
// 传输层用 mock adapter 替换（defaults.adapter），不走真实网络。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiClientError, apiClient, createCachingAdapter } from '@/lib/api/client';

function mockAdapterOnce(
  impl: (config: Record<string, unknown>) => Promise<unknown>,
  calls: Array<Record<string, unknown>> = []
) {
  const adapter = vi.fn((config: Record<string, unknown>) => {
    calls.push(config);
    return impl(config);
  });
  (apiClient.defaults as { adapter: unknown }).adapter = adapter;
  return adapter;
}

const okEnvelope = (data: unknown = {}) => ({
  status: 200,
  statusText: 'OK',
  headers: {},
  config: {} as never,
  data: { code: 20000, msg: 'success', trace: '', data },
});

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('GET 自动重试（issue #99）', () => {
  it('5xx 后重试，第二次成功则返回成功', async () => {
    const calls: Array<Record<string, unknown>> = [];
    let n = 0;
    const adapter = mockAdapterOnce(
      (config) =>
        ++n === 1
          ? Promise.reject({
              code: 'ERR_BAD_RESPONSE',
              response: { status: 502, data: {} },
              config,
            })
          : Promise.resolve({ ...okEnvelope(), config }),
      calls
    );
    const resp = await apiClient.get('/x', { skipCache: true });
    expect(resp.data.code).toBe(20000);
    expect(adapter).toHaveBeenCalledTimes(2);
    expect((calls[1] as { _retryCount?: number })._retryCount).toBe(1);
  });

  it('4xx 不重试，直接抛 ApiClientError 且带 status', async () => {
    const calls: Array<Record<string, unknown>> = [];
    const adapter = mockAdapterOnce(
      (config) =>
        Promise.reject({
          code: 'ERR_BAD_REQUEST',
          response: { status: 404, data: { msg: 'not found' } },
          config,
        }),
      calls
    );
    await expect(apiClient.get('/x', { skipCache: true })).rejects.toMatchObject({
      name: 'ApiClientError',
      status: 404,
      message: 'not found',
    });
    expect(adapter).toHaveBeenCalledTimes(1);
  });

  it('POST 不重试', async () => {
    const calls: Array<Record<string, unknown>> = [];
    const adapter = mockAdapterOnce(
      (config) => Promise.reject({ code: 'ECONNABORTED', config }),
      calls
    );
    await expect(apiClient.post('/x')).rejects.toBeInstanceOf(ApiClientError);
    expect(adapter).toHaveBeenCalledTimes(1);
  });

  it('重试次数封顶（默认 2 次 + 首次 = 3 个请求）', async () => {
    const calls: Array<Record<string, unknown>> = [];
    const adapter = mockAdapterOnce(
      (config) => Promise.reject({ code: 'ECONNABORTED', config }),
      calls
    );
    await expect(apiClient.get('/x', { skipCache: true })).rejects.toMatchObject({
      timedOut: true,
    });
    expect(adapter).toHaveBeenCalledTimes(3);
  });

  it('skipRetry 跳过重试', async () => {
    const calls: Array<Record<string, unknown>> = [];
    const adapter = mockAdapterOnce(
      (config) => Promise.reject({ code: 'ECONNABORTED', config }),
      calls
    );
    await expect(apiClient.get('/x', { skipCache: true, skipRetry: true })).rejects.toBeInstanceOf(
      ApiClientError
    );
    expect(adapter).toHaveBeenCalledTimes(1);
  });
});

describe('GET 微缓存与在途去重（issue #99）', () => {
  it('TTL 内重复 GET 返回缓存，不再发包', async () => {
    const transport = vi.fn((config: Record<string, unknown>) =>
      Promise.resolve({ ...okEnvelope({ n: 1 }), config })
    );
    const adapter = createCachingAdapter(transport as never);
    const cfg = { method: 'get', url: '/a', params: { q: 1 } } as never;
    const r1 = await (adapter as (c: never) => Promise<{ data: { data: { n: number } } }>)(cfg);
    const r2 = await (adapter as (c: never) => Promise<{ data: { data: { n: number } } }>)(cfg);
    expect(r1.data.data.n).toBe(1);
    expect(r2.data.data.n).toBe(1);
    expect(transport).toHaveBeenCalledTimes(1);
  });

  it('不同参数不共享缓存', async () => {
    const transport = vi.fn((config: Record<string, unknown>) =>
      Promise.resolve({ ...okEnvelope(), config })
    );
    const adapter = createCachingAdapter(transport as never);
    const run = (params: unknown) =>
      (adapter as (c: never) => Promise<unknown>)({ method: 'get', url: '/a', params } as never);
    await run({ q: 1 });
    await run({ q: 2 });
    expect(transport).toHaveBeenCalledTimes(2);
  });

  it('skipCache 与非 GET 不缓存', async () => {
    const transport = vi.fn((config: Record<string, unknown>) =>
      Promise.resolve({ ...okEnvelope(), config })
    );
    const adapter = createCachingAdapter(transport as never);
    const run = (cfg: unknown) => (adapter as (c: never) => Promise<unknown>)(cfg as never);
    await run({ method: 'get', url: '/a', skipCache: true } as never);
    await run({ method: 'get', url: '/a', skipCache: true } as never);
    await run({ method: 'post', url: '/a' } as never);
    await run({ method: 'post', url: '/a' } as never);
    expect(transport).toHaveBeenCalledTimes(4);
  });

  it('TTL 过期后重新发包', async () => {
    vi.useFakeTimers();
    try {
      const transport = vi.fn((config: Record<string, unknown>) =>
        Promise.resolve({ ...okEnvelope(), config })
      );
      const adapter = createCachingAdapter(transport as never);
      const run = () =>
        (adapter as (c: never) => Promise<unknown>)({ method: 'get', url: '/a' } as never);
      await run();
      await vi.advanceTimersByTimeAsync(6000);
      await run();
      expect(transport).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('细粒度错误 ApiClientError（issue #99）', () => {
  it('业务码错误：code 有值、status 为 undefined，且是 Error 子类', async () => {
    mockAdapterOnce(() =>
      Promise.resolve({
        ...okEnvelope(),
        data: { code: 40001, msg: '参数错误', trace: '', data: null },
      })
    );
    const err = await apiClient.get('/x', { skipCache: true }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiClientError);
    expect(err).toBeInstanceOf(Error);
    const apiErr = err as ApiClientError;
    expect(apiErr.code).toBe(40001);
    expect(apiErr.status).toBeUndefined();
    expect(apiErr.message).toBe('参数错误');
  });

  it('网络层错误：timedOut 标记超时', async () => {
    const calls: Array<Record<string, unknown>> = [];
    mockAdapterOnce((config) => Promise.reject({ code: 'ECONNABORTED', config }), calls);
    const err = await apiClient
      .get('/x', { skipCache: true, skipRetry: true })
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiClientError);
    expect((err as ApiClientError).timedOut).toBe(true);
  });
});
