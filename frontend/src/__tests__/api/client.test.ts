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
  it('显式配置时直接采用（含空串 = 同源，生产走 nginx 反代的 /api）', () => {
    expect(resolveApiBaseURL('https://api.example.com')).toBe('https://api.example.com');
    expect(resolveApiBaseURL('')).toBe('');
  });

  it('未配置时回退到当前访问主机名的 23352 开发默认', () => {
    expect(resolveApiBaseURL(undefined)).toBe(
      `http://${window.location.hostname || 'localhost'}:23352`
    );
  });
});
