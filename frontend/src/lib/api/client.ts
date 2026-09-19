import * as Sentry from '@sentry/react';
import axios, { AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import type { components } from '../../idls/gen_types';

// 业务成功码（后端 response 信封约定）。其余非 2xx/业务码由拦截器统一按"非此即错"处理。
const API_SUCCESS_CODE = 20000;

// Envelope from the OpenAPI schema with data re-tightened per call site.
// G['Envelope']['code'] (ResponseCode) is the same 7-literal union as ApiCode.
export type ApiResponse<T = unknown> = Omit<components['schemas']['Envelope'], 'data'> & {
  data: T;
};

// X-Request-ID 的取值：crypto.randomUUID 只在安全上下文（HTTPS / localhost）可用，
// 经局域网 IP 明文访问时它是 undefined —— 那时必须退回时间戳方案，否则每个请求都会
// 在请求拦截器里抛错。
function generateRequestId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `req_${Date.now()}_${Math.random().toString(36).slice(2, 11)}`;
}

// 全前端唯一的 axios 实例（别再在别处 axios.create）。baseURL 只有这一个来源：
//   1. VITE_API_BASE_URL 已设置 → 用它；空字符串 = 同源相对路径，生产部署把
//      /api 交给 nginx 反代到后端，前端因此不需要 CORS 白名单。
//   2. 未设置 → 开发默认 http://<当前访问主机名>:23352（后端 [CORS] AllowedOrigins
//      必须包含该主机名的 23351 来源，否则预检不返回 CORS 头）。
export function resolveApiBaseURL(configured: string | undefined): string {
  if (typeof configured === 'string') {
    return configured;
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

  client.interceptors.request.use(
    (config: InternalAxiosRequestConfig) => {
      config.headers.set('X-Request-ID', generateRequestId());
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
        return Promise.reject(new Error(msg || `API Error: ${code}`));
      }

      return response;
    },
    (error) => {
      const status = error.response?.status;
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

      return Promise.reject(error);
    }
  );

  return client;
}

const configuredBaseURL = import.meta.env.VITE_API_BASE_URL;

export const apiClient: AxiosInstance = createApiClient(resolveApiBaseURL(configuredBaseURL));
