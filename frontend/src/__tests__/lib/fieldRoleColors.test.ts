import { describe, expect, it } from 'vitest';
import { roleColor } from '../../lib/fieldRoleColors';

describe('roleColor（#265 角色语义色单一来源）', () => {
  it('metric → 指标绿', () => {
    expect(roleColor('metric')).toBe('var(--dr-metric)');
  });
  it('dimension / 其他 → 维度蓝', () => {
    expect(roleColor('dimension')).toBe('var(--dr-dim)');
    expect(roleColor(undefined)).toBe('var(--dr-dim)');
    expect(roleColor('')).toBe('var(--dr-dim)');
  });
});
