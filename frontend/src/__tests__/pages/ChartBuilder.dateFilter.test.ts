import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DateFilterValue } from '@/lib/dateFilter';
import { composeChartQueryRequest } from '@/pages/ChartBuilder';
import type { FilterCondition, QueryConfig } from '@/store';

/**
 * 日期筛选条件在下发请求时的展开。
 *
 * 这里钉的是「动态日期是真的动态」：同一个筛选条件（`最近 7 天`）在不同时刻解析出不同区间，
 * 因为解析发生在构造请求这一刻，而不是用户点确定那一刻。
 */

const FIELDS = [
  { id: '0000i529', name: 'date', type: 'dimension' as const, dataType: 'date' },
  { id: '0000i52a', name: 'brand', type: 'dimension' as const, dataType: 'string' },
];

const dateFilter = (date: FilterCondition['date'], patch: Partial<FilterCondition> = {}) =>
  ({
    id: 'filter-1',
    fieldId: '0000i529',
    operator: 'between',
    value: '2026-09-18',
    valueEnd: '2026-09-24',
    logic: 'and',
    date,
    ...patch,
  }) as FilterCondition;

const baseQueryConfig = (filters: FilterCondition[]): QueryConfig => ({
  dimensionGroups: [{ id: 'dim-group-1', bindings: [{ bindingId: 'b-1', fieldId: '0000i52a' }] }],
  metricGroups: [],
  filters,
});

const compose = (filters: FilterCondition[]) =>
  composeChartQueryRequest({
    datasetId: 1,
    chartType: 'table',
    queryConfig: baseQueryConfig(filters),
    fields: FIELDS,
    metricAggregations: {},
    metricAliases: {},
    tablePagination: { page: 1, pageSize: 20 },
    queryOptions: {},
    includeSort: true,
  });

const intent = (value: DateFilterValue) => ({
  value,
  granularity: 'day' as const,
  weekStart: 1 as const,
  asFilter: false,
  label: '交易日期',
});

describe('composeChartQueryRequest：日期筛选意图的展开', () => {
  beforeEach(() => {
    // dayjs() 读 Date.now()，因此假时钟能把「最近 7 天」钉死。
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 8, 25, 14, 30, 45));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('动态日期在构造请求时解析成具体 between，且引用列的稳定 id', () => {
    const request = compose([dateFilter(intent({ kind: 'dynamic', preset: 'last7d' }))]);

    expect(request?.filters).toEqual([
      {
        field: '0000i529',
        operator: 'between',
        value: '2026-09-18',
        value_end: '2026-09-24',
        logic: 'and',
      },
    ]);
  });

  it('同一条件在不同时刻解析出不同区间（真的动态，不是保存时的快照）', () => {
    const filters = [dateFilter(intent({ kind: 'dynamic', preset: 'last7d' }))];

    expect(compose(filters)?.filters[0]).toMatchObject({
      value: '2026-09-18',
      value_end: '2026-09-24',
    });

    vi.setSystemTime(new Date(2026, 9, 2, 9, 0, 0));
    expect(compose(filters)?.filters[0]).toMatchObject({
      value: '2026-09-25',
      value_end: '2026-10-01',
    });
  });

  it('「包含空日期」展开成两条：区间 + IS NULL，第二条按 OR 连接', () => {
    const request = compose([
      dateFilter({
        ...intent({ kind: 'fixed', start: '2026-08-01', end: '2026-08-31', includeEmpty: true }),
      }),
    ]);

    expect(request?.filters).toEqual([
      {
        field: '0000i529',
        operator: 'between',
        value: '2026-08-01',
        value_end: '2026-08-31',
        logic: 'and',
      },
      { field: '0000i529', operator: 'isNull', value: '', logic: 'or' },
    ]);
  });

  it('高级模式的单侧无限制只下发一个方向的条件', () => {
    const request = compose([
      dateFilter({
        ...intent({
          kind: 'advanced',
          start: { type: 'fixed', value: '2023-06-14' },
          end: { type: 'unlimited' },
        }),
      }),
    ]);

    expect(request?.filters).toEqual([
      { field: '0000i529', operator: 'gte', value: '2023-06-14', logic: 'and' },
    ]);
  });

  it('特殊值：空日期 / 所有日期', () => {
    expect(compose([dateFilter(intent({ kind: 'special', value: 'empty' }))])?.filters).toEqual([
      { field: '0000i529', operator: 'isNull', value: '', logic: 'and' },
    ]);
    expect(compose([dateFilter(intent({ kind: 'special', value: 'all' }))])?.filters).toEqual([]);
  });

  it('没有日期意图的条件照旧直传（operator/value/value_end 不被动）', () => {
    const request = compose([
      {
        id: 'filter-2',
        fieldId: '0000i52a',
        operator: 'eq',
        value: '北京',
        logic: 'and',
      },
    ]);

    expect(request?.filters).toEqual([
      { field: '0000i52a', operator: 'eq', value: '北京', value_end: undefined, logic: 'and' },
    ]);
  });

  it('日期条件与普通条件可以共存，顺序保持', () => {
    const request = compose([
      { id: 'filter-2', fieldId: '0000i52a', operator: 'eq', value: '北京', logic: 'and' },
      dateFilter(intent({ kind: 'dynamic', preset: 'last1d' })),
    ]);

    expect(request?.filters.map((item) => item.operator)).toEqual(['eq', 'between']);
  });
});
