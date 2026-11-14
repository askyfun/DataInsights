import { describe, expect, it } from 'vitest';
import { expressionFieldRef, validateExpr } from '../../lib/expr';

/**
 * 字段存在性检查（前端独有；后端只校验语法）。
 * 语法面由双端共用用例表覆盖，见 expr.cases.test.ts。
 */
describe('validateExpr 字段检查', () => {
  const fields = ['price', 'quantity', 'order_date'];

  it('全部标识符都在字段表里时放行', () => {
    expect(validateExpr('price * quantity', fields)).toEqual({ ok: true });
  });

  it('函数名不当作字段', () => {
    expect(validateExpr('sum(price) / count(*)', fields)).toEqual({ ok: true });
  });

  it('未知字段报 unknown_field 并带上列名', () => {
    const result = validateExpr('price * discount', fields);
    expect(result).toEqual({ ok: false, code: 'unknown_field', detail: 'discount' });
  });

  it('字段匹配忽略大小写', () => {
    expect(validateExpr('PRICE + Quantity', fields)).toEqual({ ok: true });
  });

  it('限定名按最后一段匹配字段', () => {
    expect(validateExpr('`db.tbl.price` + 1', fields)).toEqual({ ok: true });
  });

  it('不提供字段表时只校验语法', () => {
    expect(validateExpr('anything * 2')).toEqual({ ok: true });
  });

  it('反引号内的未知字段同样报错', () => {
    const result = validateExpr('`missing_col`', fields);
    expect(result).toEqual({ ok: false, code: 'unknown_field', detail: 'missing_col' });
  });

  it('语法错误优先于字段检查', () => {
    const result = validateExpr('price --comment', fields);
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.code).toBe('comment_not_allowed');
    }
  });
});

describe('expressionFieldRef', () => {
  it('普通字段名裸写', () => {
    expect(expressionFieldRef('amount')).toBe('amount');
  });

  it('保留字加反引号，保证插入后仍能过白名单', () => {
    expect(expressionFieldRef('order')).toBe('`order`');
    expect(validateExpr(expressionFieldRef('order')).ok).toBe(true);
    expect(validateExpr('order').ok).toBe(false);
  });
});

describe('validateExpr 错误分类', () => {
  it.each([
    ['   ', 'empty'],
    ['price +', 'unexpected_token'],
    ['(price', 'unbalanced_parenthesis'],
    ['select', 'reserved_keyword'],
    ['pg_sleep(1)', 'function_not_allowed'],
    ['price /* x */', 'unexpected_token'],
    ['`price', 'quoted_identifier'],
    ["'abc", 'unterminated_string'],
  ] as const)('%s → %s', (expr, code) => {
    const result = validateExpr(expr);
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.code).toBe(code);
    }
  });
});
