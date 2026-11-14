import { describe, expect, it } from 'vitest';
import type { ChartConfigDocumentInput } from '@/lib/querySpec';
import { buildQuerySpecDocument, withDynamicSelections } from '@/lib/querySpec';

const input: ChartConfigDocumentInput = {
  chartType: 'bar',
  title: 't',
  queryConfig: {
    dimensionGroups: [
      { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', fieldId: 'd-1', candidates: ['d-2'] }] },
    ],
    metricGroups: [{ id: 'metric-group-1', bindings: [{ bindingId: 'b-1', fieldId: 'm-1' }] }],
    filters: [],
    limit: 10,
  },
  // 与 queryConfig 字段组同源（除 filters/sort/limit 外），文档实际写这一份
  activeQueryConfig: {
    dimensionGroups: [
      { id: 'dim-group-1', bindings: [{ bindingId: 'b-0', fieldId: 'd-1', candidates: ['d-2'] }] },
    ],
    metricGroups: [{ id: 'metric-group-1', bindings: [{ bindingId: 'b-1', fieldId: 'm-1' }] }],
  },
  dimensionLabels: {},
  metricAggregations: {},
  metricAliases: {},
  metricUnits: {},
  metricFormats: {},
  chartStyle: {} as ChartConfigDocumentInput['chartStyle'],
  chartQueryOptions: {},
};

describe('withDynamicSelections：?q= 快照携带当时的动态槽位选择（issue #134）', () => {
  it('字段组取自 activeQueryConfig 也必须被物化（回归：只改 queryConfig 会写回默认列）', () => {
    const spec = buildQuerySpecDocument(withDynamicSelections(input, { 'b-0': 'd-2' }));
    expect(spec.document.query.dimensionGroups[0].bindings[0].fieldId).toBe('d-2');
    // 候选集原样保留，还原后仍是动态槽位
    expect(spec.document.query.dimensionGroups[0].bindings[0].candidates).toEqual(['d-1']);
    // 其余小节透传
    expect(spec.document.query.limit).toBe(10);
  });

  it('没有选择时返回原输入引用（快照与图表配置逐字节一致）', () => {
    expect(withDynamicSelections(input, {})).toBe(input);
  });

  it('不改写输入（会话选择不落库）', () => {
    withDynamicSelections(input, { 'b-0': 'd-2' });
    expect(input.activeQueryConfig.dimensionGroups[0].bindings[0].fieldId).toBe('d-1');
  });
});
