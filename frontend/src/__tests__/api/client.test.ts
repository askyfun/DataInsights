import { describe, expect, it } from 'vitest';
import { isHistogramPayload, isRadarPayload } from '@/api';
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

describe('isHistogramPayload', () => {
  it('按形状判别：bins 数组 true；table/裸数组/null false', () => {
    expect(isHistogramPayload({ bins: [] })).toBe(true);
    expect(isHistogramPayload({ bins: [{ bin_start: 0, bin_end: 1, count: 2 }] })).toBe(true);
    expect(isHistogramPayload({ columns: ['a'], data: [] })).toBe(false);
    expect(isHistogramPayload([])).toBe(false);
    expect(isHistogramPayload(null)).toBe(false);
  });
});

describe('isRadarPayload', () => {
  it('按形状判别：indicators+series 双数组 true；单 indicators/histogram/pie/裸数组/null false', () => {
    // 正例：完整 radar 负载
    expect(
      isRadarPayload({
        indicators: [{ name: 'A', max: 10 }],
        series: [{ name: 'p1', values: [5] }],
      })
    ).toBe(true);
    // 空数组仍是合法形状（processor 兜底会产出空切片）
    expect(isRadarPayload({ indicators: [], series: [] })).toBe(true);
    // 反例：单 indicators 无 series（形状不完整）
    expect(isRadarPayload({ indicators: [{ name: 'A', max: 1 }] })).toBe(false);
    // 反例：histogram {bins} / pie {data} / table {columns,data} / 裸数组 / null
    expect(isRadarPayload({ bins: [] })).toBe(false);
    expect(isRadarPayload({ data: [] })).toBe(false);
    expect(isRadarPayload([])).toBe(false);
    expect(isRadarPayload(null)).toBe(false);
  });
});
