import { describe, expect, it } from 'vitest';
import fixture from '../../lib/__fixtures__/filterOperators.json';
import { FILTER_OPERATORS, normalizeFilterOperator } from '../../lib/filterOperators';

/**
 * 过滤算子归一化的**双端一致性**用例表。
 *
 * 同一份用例表也被后端 `backend/internal/query/filter_operators_test.go` 读取：
 * 两侧必须逐例一致，任一侧漂移都会让对应用例失败。
 */

interface SharedCase {
  name: string;
  raw: string;
  want: string | null;
}

const table = fixture.cases as SharedCase[];

describe('过滤算子归一化：双端共用用例表', () => {
  it('用例表非空且每例都有名字', () => {
    expect(table.length).toBeGreaterThan(20);
    for (const testCase of table) {
      expect(testCase.name).toBeTruthy();
    }
  });

  it.each(table)('$name', ({ raw, want }) => {
    expect(normalizeFilterOperator(raw)).toBe(want);
  });

  it('规范算子列表与归一化幂等，且无重复', () => {
    expect(new Set(FILTER_OPERATORS).size).toBe(FILTER_OPERATORS.length);
    for (const op of FILTER_OPERATORS) {
      expect(normalizeFilterOperator(op)).toBe(op);
    }
  });
});
