/**
 * 表达式白名单校验的 TS 镜像（issue #181 表达式编辑器）。
 *
 * 后端 `internal/expr` 是**唯一权威**：写入口（datasetService.UpdateColumns）与
 * 查询期（query.buildColumnIndex）都过那道闸。本文件只为了让编辑器能**即时**反馈
 * 语法错误与未知字段，不替代也不绕过后端。
 *
 * 两份实现必须逐字同义：语法用例表 `__fixtures__/exprCases.json` 由 Go 与 TS
 * 各跑一遍（expr_shared_test.go / expr.cases.test.ts），任一侧单独改动都会变红。
 * 字段存在性检查只存在于前端（后端只校验语法），因此不进共用用例表。
 */

/** 与后端 expr.maxDepth 同值；改一边必须改另一边。 */
export const EXPR_MAX_DEPTH = 32;

const FUNC_WHITELIST = new Set(['count', 'sum', 'avg', 'min', 'max']);

/** 与后端 expr.reserved 同一词表（跨方言的 DML/DDL/查询结构关键字）。 */
const RESERVED = new Set([
  'select',
  'insert',
  'update',
  'delete',
  'drop',
  'alter',
  'create',
  'truncate',
  'union',
  'all',
  'distinct',
  'from',
  'where',
  'group',
  'order',
  'by',
  'having',
  'limit',
  'offset',
  'join',
  'left',
  'right',
  'inner',
  'outer',
  'full',
  'cross',
  'on',
  'as',
  'and',
  'or',
  'not',
  'in',
  'is',
  'null',
  'case',
  'when',
  'then',
  'else',
  'end',
  'between',
  'like',
  'exists',
  'into',
  'values',
  'set',
  'grant',
  'revoke',
  'exec',
  'execute',
  'sleep',
  'benchmark',
  'load_file',
]);

export type ExprErrorCode =
  | 'empty'
  | 'unexpected_token'
  | 'unbalanced_parenthesis'
  | 'reserved_keyword'
  | 'function_not_allowed'
  | 'nesting_too_deep'
  | 'comment_not_allowed'
  | 'string_backslash'
  | 'unterminated_string'
  | 'quoted_identifier'
  /** 仅前端：标识符不在数据集字段里（后端不做此项校验）。 */
  | 'unknown_field';

export type ExprValidation = { ok: true } | { ok: false; code: ExprErrorCode; detail?: string };

const OK: ExprValidation = { ok: true };

/**
 * 字段名 → 可插入表达式的引用形态。
 * 与后端词表一致：保留字裸写会被拒，必须用反引号包起来（`` `order` ``）。
 */
export function expressionFieldRef(name: string): string {
  return RESERVED.has(name.toLowerCase()) ? `\`${name}\`` : name;
}

function fail(code: ExprErrorCode, detail?: string): ExprValidation {
  return { ok: false, code, detail };
}

/**
 * 校验单个标量表达式。
 *
 * `fields` 提供时额外做**字段存在性**检查（忽略大小写；带 `.` 的限定名取最后一段，
 * 以兼容 `db.tbl.col` 形态）。不传则只校验语法，与后端行为一致。
 */
export function validateExpr(expr: string, fields?: readonly string[]): ExprValidation {
  const parser = new ExprParser(expr.trim());
  const result = parser.run();
  if (!result.ok || !fields || fields.length === 0) {
    return result;
  }
  const known = new Set(fields.map((name) => name.toLowerCase()));
  for (const ident of parser.identifiers) {
    const lower = ident.toLowerCase();
    const last = lower.includes('.') ? lower.slice(lower.lastIndexOf('.') + 1) : lower;
    if (!known.has(lower) && !known.has(last)) {
      return fail('unknown_field', ident);
    }
  }
  return OK;
}

class ExprParser {
  /** 解析过程中收集的**非函数**标识符，供字段存在性检查使用。 */
  readonly identifiers: string[] = [];
  private pos = 0;

  constructor(private readonly src: string) {}

  run(): ExprValidation {
    if (this.src.length === 0) {
      return fail('empty');
    }
    const err = this.expr(0);
    if (err) {
      return err;
    }
    this.skipSpace();
    if (this.pos !== this.src.length) {
      return fail('unexpected_token', this.src[this.pos]);
    }
    return OK;
  }

  private skipSpace() {
    while (this.pos < this.src.length) {
      const r = this.src[this.pos];
      if (r === ' ' || r === '\t' || r === '\n' || r === '\r') {
        this.pos++;
      } else {
        return;
      }
    }
  }

  /**
   * 相邻两个减号即 SQL 行注释开头；二元位与一元位都必须走这里
   * （与后端 rejectDoubleDash 同一判据，建立在对原始文本的紧邻判定上）。
   */
  private rejectDoubleDash(at: number): ExprValidation | null {
    if (this.src[at] === '-' && this.src[at + 1] === '-') {
      return fail('comment_not_allowed');
    }
    return null;
  }

  private expr(depth: number): ExprValidation | null {
    if (depth > EXPR_MAX_DEPTH) {
      return fail('nesting_too_deep');
    }
    let err = this.term(depth);
    if (err) {
      return err;
    }
    for (;;) {
      this.skipSpace();
      if (this.pos >= this.src.length) {
        return null;
      }
      const r = this.src[this.pos];
      if (r === '+' || r === '-') {
        if (r === '-') {
          const comment = this.rejectDoubleDash(this.pos);
          if (comment) {
            return comment;
          }
        }
        this.pos++;
      } else if (r === '|' && this.src[this.pos + 1] === '|') {
        this.pos += 2;
      } else {
        return null;
      }
      err = this.term(depth);
      if (err) {
        return err;
      }
    }
  }

  private term(depth: number): ExprValidation | null {
    let err = this.factor(depth);
    if (err) {
      return err;
    }
    for (;;) {
      this.skipSpace();
      if (this.pos < this.src.length) {
        const r = this.src[this.pos];
        if (r === '*' || r === '/' || r === '%') {
          this.pos++;
          err = this.factor(depth);
          if (err) {
            return err;
          }
          continue;
        }
      }
      return null;
    }
  }

  private factor(depth: number): ExprValidation | null {
    this.skipSpace();
    if (this.src[this.pos] === '-') {
      const comment = this.rejectDoubleDash(this.pos);
      if (comment) {
        return comment;
      }
      this.pos++;
      if (depth + 1 > EXPR_MAX_DEPTH) {
        return fail('nesting_too_deep');
      }
      return this.factor(depth + 1);
    }
    return this.primary(depth);
  }

  private primary(depth: number): ExprValidation | null {
    this.skipSpace();
    if (this.pos >= this.src.length) {
      return fail('unexpected_token', 'end');
    }
    const r = this.src[this.pos];
    if (r === '(') {
      this.pos++;
      if (depth + 1 > EXPR_MAX_DEPTH) {
        return fail('nesting_too_deep');
      }
      const err = this.expr(depth + 1);
      if (err) {
        return err;
      }
      this.skipSpace();
      if (this.src[this.pos] !== ')') {
        return fail('unbalanced_parenthesis');
      }
      this.pos++;
      return null;
    }
    if (r >= '0' && r <= '9') {
      return this.number();
    }
    if (r === "'") {
      return this.stringLit();
    }
    if (r === '`' || r === '"') {
      return this.quotedIdent();
    }
    return this.identOrCall(depth);
  }

  private number(): ExprValidation | null {
    const start = this.pos;
    while (this.pos < this.src.length) {
      const r = this.src[this.pos];
      if (isDigit(r) || r === '.') {
        this.pos++;
      } else {
        break;
      }
    }
    return this.pos === start ? fail('unexpected_token') : null;
  }

  private stringLit(): ExprValidation | null {
    this.pos++;
    for (;;) {
      if (this.pos >= this.src.length) {
        return fail('unterminated_string');
      }
      const r = this.src[this.pos];
      if (r === '\\') {
        // 与后端一致：只认 '' 转义，反斜杠在各方言里边界含义不同，一律拒绝。
        return fail('string_backslash');
      }
      if (r === "'") {
        if (this.src[this.pos + 1] === "'") {
          this.pos += 2;
          continue;
        }
        this.pos++;
        return null;
      }
      this.pos++;
    }
  }

  private quotedIdent(): ExprValidation | null {
    const q = this.src[this.pos];
    this.pos++;
    const start = this.pos;
    while (this.pos < this.src.length && this.src[this.pos] !== q) {
      this.pos++;
    }
    if (this.pos >= this.src.length) {
      return fail('quoted_identifier');
    }
    const inner = this.src.slice(start, this.pos);
    this.pos++;
    if (inner.length === 0 || !validQuotedInner(inner)) {
      return fail('quoted_identifier');
    }
    this.identifiers.push(inner);
    return null;
  }

  private identOrCall(depth: number): ExprValidation | null {
    const start = this.pos;
    while (this.pos < this.src.length && isIdentChar(this.src[this.pos])) {
      this.pos++;
    }
    if (this.pos === start) {
      return fail('unexpected_token', this.src[this.pos]);
    }
    const ident = this.src.slice(start, this.pos);
    const name = ident.toLowerCase();
    if (RESERVED.has(name)) {
      return fail('reserved_keyword', name);
    }
    this.skipSpace();
    if (this.src[this.pos] !== '(') {
      this.identifiers.push(ident);
      return null;
    }
    if (!FUNC_WHITELIST.has(name)) {
      return fail('function_not_allowed', name);
    }
    this.pos++;
    this.skipSpace();
    if (this.src[this.pos] === '*') {
      if (name !== 'count') {
        return fail('function_not_allowed', `${name}(*)`);
      }
      this.pos++;
      this.skipSpace();
      if (this.src[this.pos] !== ')') {
        return fail('unbalanced_parenthesis');
      }
      this.pos++;
      return null;
    }
    if (this.src[this.pos] === ')') {
      this.pos++;
      return null;
    }
    for (;;) {
      if (depth + 1 > EXPR_MAX_DEPTH) {
        return fail('nesting_too_deep');
      }
      const err = this.expr(depth + 1);
      if (err) {
        return err;
      }
      this.skipSpace();
      if (this.src[this.pos] === ',') {
        this.pos++;
        continue;
      }
      break;
    }
    if (this.src[this.pos] !== ')') {
      return fail('unbalanced_parenthesis');
    }
    this.pos++;
    return null;
  }
}

function isDigit(r: string | undefined): boolean {
  return r !== undefined && r >= '0' && r <= '9';
}

/** 与后端 expr.isIdentChar 及下游标识符白名单逐字符一致：只认 [A-Za-z0-9_.]。 */
function isIdentChar(r: string): boolean {
  return (
    (r >= 'a' && r <= 'z') ||
    (r >= 'A' && r <= 'Z') ||
    (r >= '0' && r <= '9') ||
    r === '_' ||
    r === '.'
  );
}

function validQuotedInner(s: string): boolean {
  for (const r of s) {
    if (!isIdentChar(r)) {
      return false;
    }
  }
  return true;
}
