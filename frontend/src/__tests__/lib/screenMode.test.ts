import { describe, expect, it } from 'vitest';
import {
  fitScreenScale,
  parseIntervalSeconds,
  parsePlaybackConfig,
  SCREEN_DESIGN_ASPECT,
  SCREEN_DESIGN_HEIGHT,
  SCREEN_DESIGN_WIDTH,
} from '../../lib/screenMode';

describe('fitScreenScale', () => {
  it('按设计画布 1920×1080 等比缩放', () => {
    expect(SCREEN_DESIGN_WIDTH).toBe(1920);
    expect(SCREEN_DESIGN_HEIGHT).toBe(1080);
    expect(SCREEN_DESIGN_ASPECT).toBeCloseTo(16 / 9);
  });

  it('同比例视口缩放为 1，无偏移', () => {
    const r = fitScreenScale(1920, 1080);
    expect(r.scale).toBe(1);
    expect(r.width).toBe(1920);
    expect(r.height).toBe(1080);
    expect(r.offsetX).toBe(0);
    expect(r.offsetY).toBe(0);
  });

  it('4K 视口（3840×2160）等比放大 2 倍', () => {
    const r = fitScreenScale(3840, 2160);
    expect(r.scale).toBe(2);
    expect(r.width).toBe(3840);
    expect(r.height).toBe(2160);
  });

  it('宽视口（超宽）按高度约束，左右黑边', () => {
    const r = fitScreenScale(2560, 1080);
    expect(r.scale).toBeCloseTo(1);
    expect(r.height).toBe(1080);
    expect(r.width).toBe(1920);
    expect(r.offsetX).toBeCloseTo((2560 - 1920) / 2);
    expect(r.offsetY).toBe(0);
  });

  it('窄视口（超高）按宽度约束，上下黑边', () => {
    const r = fitScreenScale(1920, 1400);
    expect(r.scale).toBeCloseTo(1);
    expect(r.width).toBe(1920);
    expect(r.height).toBe(1080);
    expect(r.offsetY).toBeCloseTo((1400 - 1080) / 2);
    expect(r.offsetX).toBe(0);
  });

  it('小视口等比缩小', () => {
    const r = fitScreenScale(960, 540);
    expect(r.scale).toBeCloseTo(0.5);
    expect(r.width).toBe(960);
    expect(r.height).toBe(540);
  });

  it('宽高比不一致时保持 16:9 不变形（letterbox）', () => {
    // 1280×800：宽比 1280/1920=0.667，高比 800/1080=0.741 → 取宽的约束
    const r = fitScreenScale(1280, 800);
    expect(r.scale).toBeCloseTo(1280 / 1920);
    expect(r.width / r.height).toBeCloseTo(16 / 9);
  });

  it('视口为 0（首帧未测量）返回 scale 0', () => {
    const r = fitScreenScale(0, 1080);
    expect(r.scale).toBe(0);
    expect(r.width).toBe(0);
    expect(r.height).toBe(0);
  });
});

describe('parseIntervalSeconds', () => {
  it('空/undefined/null → 0（关闭）', () => {
    expect(parseIntervalSeconds(null)).toBe(0);
    expect(parseIntervalSeconds(undefined)).toBe(0);
    expect(parseIntervalSeconds('')).toBe(0);
  });

  it('合法秒数转毫秒', () => {
    expect(parseIntervalSeconds('5')).toBe(5000);
    expect(parseIntervalSeconds('30')).toBe(30000);
    expect(parseIntervalSeconds('0')).toBe(0);
  });

  it('非法/负数/非数字 → 0（关闭，不轮播不刷新）', () => {
    expect(parseIntervalSeconds('abc')).toBe(0);
    expect(parseIntervalSeconds('-5')).toBe(0);
    expect(parseIntervalSeconds('Infinity')).toBe(0);
    expect(parseIntervalSeconds('NaN')).toBe(0);
  });
});

describe('parsePlaybackConfig', () => {
  it('默认不轮播、不自动刷新', () => {
    const cfg = parsePlaybackConfig('');
    expect(cfg.carouselIntervalMs).toBe(0);
    expect(cfg.refreshIntervalMs).toBe(0);
  });

  it('解析 carousel 与 refresh', () => {
    const cfg = parsePlaybackConfig('?carousel=10&refresh=60');
    expect(cfg.carouselIntervalMs).toBe(10000);
    expect(cfg.refreshIntervalMs).toBe(60000);
  });

  it('只开一个不影响另一个', () => {
    const cfg = parsePlaybackConfig('?carousel=15');
    expect(cfg.carouselIntervalMs).toBe(15000);
    expect(cfg.refreshIntervalMs).toBe(0);
  });

  it('carousel=0 显式关闭', () => {
    const cfg = parsePlaybackConfig('?carousel=0&refresh=0');
    expect(cfg.carouselIntervalMs).toBe(0);
    expect(cfg.refreshIntervalMs).toBe(0);
  });
});
