import { describe, expect, it } from 'vitest';
import { apiClient, resolveApiBaseURL } from '@/lib/api/client';

describe('API Client', () => {
  it('should build baseURL without stray characters', () => {
    // Regression guard: the template literal once ended with a stray `}`.
    expect(apiClient.defaults.baseURL).toBe(
      `http://${window.location.hostname || 'localhost'}:23352`
    );
    expect(apiClient.defaults.baseURL).not.toContain('}');
  });
});

describe('resolveApiBaseURL', () => {
  it('配置了非空绝对地址时直接采用（前后端分开部署）', () => {
    expect(resolveApiBaseURL('https://api.example.com')).toBe('https://api.example.com');
  });

  it('生产构建未配置时走同源（baseURL 空串，请求路径自带 /api）', () => {
    expect(resolveApiBaseURL(undefined, true)).toBe('');
    expect(resolveApiBaseURL('', true)).toBe('');
  });

  it('开发未配置时回退到当前访问主机名的 23352', () => {
    expect(resolveApiBaseURL(undefined, false)).toBe(
      `http://${window.location.hostname || 'localhost'}:23352`
    );
  });
});
