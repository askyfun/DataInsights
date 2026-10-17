/**
 * 过滤算子归一化（前端镜像）。
 *
 * 与后端 `backend/internal/query/filter_operators.go` 的 `filterOperatorAliases`
 * 是同一张表，由 `frontend/src/lib/__fixtures__/filterOperators.json` 双端共读钉住。
 * 无法识别的算子返回 `null`——调用方必须拒绝，绝不允许回退等值比较（issue #204）。
 */

/** 规范算子（与 api/openapi.yaml 的 FilterConfig.op 枚举一致）。 */
export const FILTER_OPERATORS = [
  'eq',
  'neq',
  'gt',
  'gte',
  'lt',
  'lte',
  'like',
  'in',
  'notIn',
  'between',
  'isNull',
  'isNotNull',
  'startsWith',
  'endsWith',
  'isEmptyString',
  'isNotEmptyString',
] as const;

export type FilterOperator = (typeof FILTER_OPERATORS)[number];

const ALIASES: Record<string, FilterOperator> = {
  eq: 'eq',
  '=': 'eq',
  '==': 'eq',
  neq: 'neq',
  ne: 'neq',
  '!=': 'neq',
  '<>': 'neq',
  notequal: 'neq',
  not_equal: 'neq',
  gt: 'gt',
  '>': 'gt',
  gte: 'gte',
  ge: 'gte',
  '>=': 'gte',
  lt: 'lt',
  '<': 'lt',
  lte: 'lte',
  le: 'lte',
  '<=': 'lte',
  like: 'like',
  in: 'in',
  notin: 'notIn',
  not_in: 'notIn',
  between: 'between',
  isnull: 'isNull',
  is_null: 'isNull',
  isnotnull: 'isNotNull',
  is_not_null: 'isNotNull',
  startswith: 'startsWith',
  starts_with: 'startsWith',
  endswith: 'endsWith',
  ends_with: 'endsWith',
  isemptystring: 'isEmptyString',
  is_empty_string: 'isEmptyString',
  isnotemptystring: 'isNotEmptyString',
  is_not_empty_string: 'isNotEmptyString',
};

/** 别名/大小写/内部空白归一化；无法识别返回 null。 */
export function normalizeFilterOperator(raw: string): FilterOperator | null {
  const key = raw.trim().toLowerCase().replace(/\s+/g, '');
  if (!key) return null;
  return ALIASES[key] ?? null;
}
