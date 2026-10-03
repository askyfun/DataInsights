import * as Sentry from '@sentry/react';
import axios, {
  AxiosAdapter,
  AxiosInstance,
  AxiosResponse,
  InternalAxiosRequestConfig,
} from 'axios';
import type { components } from '../../idls/gen_types';

// 业务成功码（后端 response 信封约定）。其余非 2xx/业务码由拦截器统一按"非此即错"处理。
const API_SUCCESS_CODE = 20000;

// 会话 token 只存内存（#184 存储规则：人类会话 token 不得 localStorage 长期明文驻留）。
let bearerToken = '';
export function setBearerToken(token: string): void {
  bearerToken = token;
}
export function getBearerToken(): string {
  return bearerToken;
}

// Envelope from the OpenAPI schema with data re-tightened per call site.
// G['Envelope']['code'] (ResponseCode) is the same 7-literal union as ApiCode.
export type ApiResponse<T = unknown> = Omit<components['schemas']['Envelope'], 'data'> & {
  data: T;
};

// ---- 细粒度错误（issue #99）：业务码 / HTTP 状态 / 请求 id 一处可查 ----
export class ApiClientError extends Error {
  /** 后端业务码（信封 code），HTTP 层错误时为 undefined */
  code?: number;
  /** HTTP 状态码，业务码错误 / 网络层错误时为 undefined */
  status?: number;
  /** 本次请求的 X-Request-ID（与后端日志/trace 对账用） */
  requestId?: string;
  /** 是否因超时失败（error.code === 'ECONNABORTED'） */
  timedOut?: boolean;

  constructor(
    message: string,
    opts: { code?: number; status?: number; requestId?: string; timedOut?: boolean } = {}
  ) {
    super(message);
    this.name = 'ApiClientError';
    this.code = opts.code;
    this.status = opts.status;
    this.requestId = opts.requestId;
    this.timedOut = opts.timedOut;
  }
}

// 自定义 per-request 开关（axios declaration merging；调用方可 opt-out 缓存/重试）
declare module 'axios' {
  export interface AxiosRequestConfig {
    /** 跳过 GET 微缓存（如轮询、强实时查询） */
    skipCache?: boolean;
    /** 跳过网络层自动重试 */
    skipRetry?: boolean;
  }
}

// ---- GET 微缓存 + 在途去重（issue #99）----
// 只缓存幂等的 GET；TTL 很短，目的不是"离线缓存"，而是消灭同帧多组件挂载触发的
// 重复请求（列表页 + 统计卡片等场景）。写操作（POST/PUT/DELETE）永不缓存。
const GET_CACHE_TTL_MS = 5000;
const GET_CACHE_MAX_ENTRIES = 200;

interface CacheEntry {
  expiresAt: number;
  response: AxiosResponse;
}

function cacheKeyOf(config: InternalAxiosRequestConfig): string | undefined {
  if (config.method?.toLowerCase() !== 'get' || config.skipCache) {
    return undefined;
  }
  return [config.baseURL ?? '', config.url ?? '', JSON.stringify(config.params ?? null)].join('|');
}

export function createCachingAdapter(defaultAdapter: AxiosAdapter): AxiosAdapter {
  const cache = new Map<string, CacheEntry>();
  const inflight = new Map<string, Promise<AxiosResponse>>();

  return (config) => {
    const key = cacheKeyOf(config as InternalAxiosRequestConfig);
    if (!key) {
      return defaultAdapter(config);
    }
    const hit = cache.get(key);
    const now = Date.now();
    if (hit && hit.expiresAt > now) {
      // 返回浅拷贝：调用方改写 response 不影响其他命中方
      return Promise.resolve({ ...hit.response, config });
    }
    const pending = inflight.get(key);
    if (pending) {
      return pending.then((response) => ({ ...response, config }));
    }
    const request = defaultAdapter(config)
      .then((response) => {
        if (cache.size >= GET_CACHE_MAX_ENTRIES) {
          cache.clear();
        }
        cache.set(key, { expiresAt: Date.now() + GET_CACHE_TTL_MS, response });
        return response;
      })
      .finally(() => {
        inflight.delete(key);
      });
    inflight.set(key, request);
    return request;
  };
}

// ---- GET 自动重试（issue #99）----
// 只重试幂等 GET，且只重试"值得重试"的失败：网络层错误 / 超时 / 5xx。
// 4xx（参数/权限问题）重试必然同果，直接抛。调用方可用 skipRetry 关闭。
const GET_RETRY_LIMIT = 2;
const GET_RETRY_BASE_DELAY_MS = 300;

interface RetryState {
  _retryCount?: number;
}

function sleepWithAbort(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new Error('aborted'));
      return;
    }
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener(
      'abort',
      () => {
        clearTimeout(timer);
        reject(new Error('aborted'));
      },
      { once: true }
    );
  });
}

// X-Request-ID 的取值：crypto.randomUUID 只在安全上下文（HTTPS / localhost）可用，
// 经局域网 IP 明文访问时它是 undefined —— 那时必须退回时间戳方案，否则每个请求都会
// 在请求拦截器里抛错。
function generateRequestId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `req_${Date.now()}_${Math.random().toString(36).slice(2, 11)}`;
}

// 全前端唯一的 axios 实例（别再在别处 axios.create）。baseURL 只有这一个来源，两档语义：
//   1. VITE_API_BASE_URL 配置了非空绝对地址 → 直接采用（前后端分开部署时才需要）。
//   2. 未配置 → 生产构建走同源（页面与 /api 由同一个 Go 进程提供，请求路径自带 /api
//      前缀，所以 baseURL 留空串）；开发回退 http://<当前访问主机名>:23352（后端 CORS 默认放开）。
export function resolveApiBaseURL(
  configured: string | undefined,
  isProd: boolean = import.meta.env.PROD
): string {
  if (configured) {
    return configured;
  }
  if (isProd) {
    return '';
  }
  const host =
    typeof window === 'undefined' ? 'localhost' : window.location.hostname || 'localhost';
  return `http://${host}:23352`;
}

function createApiClient(baseURL: string): AxiosInstance {
  const client = axios.create({
    baseURL,
    timeout: 30000,
    headers: {
      'Content-Type': 'application/json',
    },
  });

  // GET 微缓存 + 在途去重挂在 adapter 层：比拦截器早于真正发包，又能拿到完整响应，
  // 且不干扰拦截器链（业务码判定等仍按原路径走）。
  const defaultAdapter =
    typeof client.defaults?.adapter === 'function'
      ? (client.defaults.adapter as AxiosAdapter)
      : undefined;
  if (defaultAdapter) {
    client.defaults.adapter = createCachingAdapter(defaultAdapter);
  }

  client.interceptors.request.use(
    (config: InternalAxiosRequestConfig) => {
      config.headers.set('X-Request-ID', generateRequestId());
      // Session token (#183/#184) is held in-memory only — never localStorage
      // long-term plaintext (that is the documented storage rule for the human
      // token). setBearerToken wires it in after login/register.
      if (bearerToken) {
        config.headers.set('Authorization', `Bearer ${bearerToken}`);
      }
      return config;
    },
    (error) => {
      return Promise.reject(error);
    }
  );

  // 业务错误只记录并抛出，不在这里弹 toast：调用方（页面/store）自己决定提示文案，
  // 统一弹窗会导致同一失败被提示两次。
  client.interceptors.response.use(
    (response: AxiosResponse<ApiResponse>) => {
      const { code, msg } = response.data || {};

      if (code !== undefined && code !== API_SUCCESS_CODE) {
        console.error(`API Error [${code}]: ${msg}`);
        throw new ApiClientError(msg || `API Error: ${code}`, {
          code,
          requestId: response.config.headers?.['X-Request-ID'] as string | undefined,
        });
      }

      return response;
    },
    async (error) => {
      const config = error.config as (InternalAxiosRequestConfig & RetryState) | undefined;
      const status: number | undefined = error.response?.status;
      const isGet = config?.method?.toLowerCase() === 'get';
      const retryCount = config?._retryCount ?? 0;
      const retriable =
        !!config &&
        isGet &&
        !config.skipRetry &&
        retryCount < GET_RETRY_LIMIT &&
        error.code !== 'ERR_CANCELED' &&
        (!error.response || status === undefined || status >= 500 || error.code === 'ECONNABORTED');

      if (retriable && config) {
        try {
          await sleepWithAbort(
            GET_RETRY_BASE_DELAY_MS * 2 ** retryCount,
            (config.signal as AbortSignal | undefined) ?? undefined
          );
        } catch {
          // 等待期间被取消：按取消收场，不再重试
          throw error;
        }
        config._retryCount = retryCount + 1;
        return client.request(config);
      }

      const errorMsg =
        error.response?.data?.msg ||
        error.response?.data?.message ||
        error.message ||
        'An error occurred';
      console.error('API Error:', errorMsg);

      // Report non-2xx responses to Sentry
      if (status && status >= 400) {
        Sentry.captureMessage(
          `API Error: ${error.response?.config?.method?.toUpperCase()} ${error.response?.config?.url} returned ${status}: ${errorMsg}`
        );
      }

      // 细粒度错误（issue #99）：统一包成 ApiClientError，保留 status/业务码/请求 id。
      // 调用方普遍只读 error.message（instanceof Error），包一层不破坏既有判断。
      throw new ApiClientError(errorMsg, {
        status,
        requestId: config?.headers?.['X-Request-ID'] as string | undefined,
        timedOut: error.code === 'ECONNABORTED',
      });
    }
  );

  return client;
}

const configuredBaseURL = import.meta.env.VITE_API_BASE_URL;

export const apiClient: AxiosInstance = createApiClient(resolveApiBaseURL(configuredBaseURL));
