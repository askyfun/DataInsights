import { describe, expect, it } from 'vitest';
import fixture from '../../lib/__fixtures__/exprCases.json';
import { validateExpr } from '../../lib/expr';

/**
 * 表达式校验的**双端一致性**用例表。
 *
 * 同一个用例表也被后端 `backend/internal/expr/expr_shared_test.go` 读取：
 * 两侧必须逐例一致，任一侧漂移都会让对应用例失败。
 * 字段存在性检查只在前端（见 expr.test.ts），不进这张表。
 */

interface SharedCase {
  name: string;
  expr: string;
  valid: boolean;
}

const table = fixture.cases as unknown as SharedCase[];

describe('表达式校验：双端共用用例表', () => {
  it('用例表非空且每例都有名字', () => {
    expect(table.length).toBeGreaterThan(40);
    for (const testCase of table) {
      expect(testCase.name.length).toBeGreaterThan(0);
    }
  });

  for (const testCase of table) {
    it(testCase.name, () => {
      const result = validateExpr(testCase.expr);
      expect(result.ok).toBe(testCase.valid);
    });
  }
});
