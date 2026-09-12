import { describe, expect, it } from 'vitest';
import { API_CODE, apiClient, del, get, post, put } from '@/lib/api/client';

describe('API Client', () => {
  it('should export API_CODE constants', () => {
    expect(API_CODE.SUCCESS).toBe(20000);
    expect(API_CODE.BAD_REQUEST).toBe(20100);
    expect(API_CODE.UNAUTHORIZED).toBe(20200);
    expect(API_CODE.NOT_FOUND).toBe(20300);
    expect(API_CODE.BUSINESS_ERROR).toBe(20400);
    expect(API_CODE.THIRD_PARTY_ERROR).toBe(20500);
    expect(API_CODE.INTERNAL_ERROR).toBe(50000);
  });

  it('should export get, post, put, del functions', () => {
    expect(typeof get).toBe('function');
    expect(typeof post).toBe('function');
    expect(typeof put).toBe('function');
    expect(typeof del).toBe('function');
  });

  it('should build baseURL without stray characters', () => {
    // Regression guard: the template literal once ended with a stray `}`.
    expect(apiClient.defaults.baseURL).toBe(
      `http://${window.location.hostname || 'localhost'}:8080`
    );
    expect(apiClient.defaults.baseURL).not.toContain('}');
  });
});
