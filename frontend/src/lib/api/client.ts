import * as Sentry from '@sentry/react';
import { message } from 'antd';
import axios, { AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios';

export const API_CODE = {
  SUCCESS: 20000,
  BAD_REQUEST: 20100,
  UNAUTHORIZED: 20200,
  NOT_FOUND: 20300,
  BUSINESS_ERROR: 20400,
  THIRD_PARTY_ERROR: 20500,
  INTERNAL_ERROR: 50000,
} as const;

export type ApiCode = (typeof API_CODE)[keyof typeof API_CODE];

export interface ApiResponse<T = unknown> {
  code: ApiCode;
  msg: string;
  trace: string;
  data: T;
}

export interface PageResult<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

const CODE_MSG_MAP: Record<ApiCode, string> = {
  [API_CODE.SUCCESS]: 'success',
  [API_CODE.BAD_REQUEST]: '请求参数有误，请检查输入',
  [API_CODE.UNAUTHORIZED]: '登录状态已失效，请重新登录',
  [API_CODE.NOT_FOUND]: '请求的资源不存在',
  [API_CODE.BUSINESS_ERROR]: '操作失败，请稍后重试',
  [API_CODE.THIRD_PARTY_ERROR]: '服务暂时不可用',
  [API_CODE.INTERNAL_ERROR]: '服务器内部错误',
};

function generateRequestId(): string {
  return `req_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
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
      const requestId = generateRequestId();
      config.headers.set('X-Request-ID', requestId);
      return config;
    },
    (error) => {
      return Promise.reject(error);
    }
  );

  client.interceptors.response.use(
    (response: AxiosResponse<ApiResponse>) => {
      const { code, msg } = response.data;

      if (code !== API_CODE.SUCCESS) {
        const displayMsg = CODE_MSG_MAP[code] || msg;
        message.error(displayMsg);

        if (code === API_CODE.UNAUTHORIZED) {
          // Handle logout logic here
        }

        Sentry.captureMessage(`API Error: ${code} - ${msg}`, {
          extra: {
            url: response.config.url,
            method: response.config.method,
            code,
          },
        });

        return Promise.reject(new Error(msg));
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

      if (status && status >= 400) {
        Sentry.captureMessage(
          `API Error: ${error.response?.config?.method?.toUpperCase()} ${error.response?.config?.url} returned ${status}: ${errorMsg}`,
          {
            extra: {
              url: error.response?.config?.url,
              method: error.response?.config?.method,
              status,
            },
          }
        );
      }

      return Promise.reject(error);
    }
  );

  return client;
}

export const apiClient = createApiClient(`http://${window.location.hostname || 'localhost'}:8080`);

export function get<T>(url: string, config?: InternalAxiosRequestConfig): Promise<ApiResponse<T>> {
  return apiClient.get<ApiResponse<T>>(url, config).then((res) => res.data);
}

export function post<T>(
  url: string,
  data?: unknown,
  config?: InternalAxiosRequestConfig
): Promise<ApiResponse<T>> {
  return apiClient.post<ApiResponse<T>>(url, data, config).then((res) => res.data);
}

export function put<T>(
  url: string,
  data?: unknown,
  config?: InternalAxiosRequestConfig
): Promise<ApiResponse<T>> {
  return apiClient.put<ApiResponse<T>>(url, data, config).then((res) => res.data);
}

export function del<T>(url: string, config?: InternalAxiosRequestConfig): Promise<ApiResponse<T>> {
  return apiClient.delete<ApiResponse<T>>(url, config).then((res) => res.data);
}
