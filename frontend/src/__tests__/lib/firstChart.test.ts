import { describe, expect, it } from 'vitest';
import type { DatasetColumn } from '../../api';
import { buildFirstChartSpec } from '../../lib/firstChart';
import { parseQuerySpecDocument } from '../../lib/querySpec';

// 回归钉子（#226 AC6「端到端 < 5 分钟」现场实测）：向导第②步载入示例后只落了数据集，
// 第③步把用户送到 /chart-builder 的空画布，于是「出第一张图」全靠自己拖字段——
// 实测 9 分钟仍无图。修法是载入示例时直接配好一份查询记录（维度 + 求和指标），
// 第③步点开即有图。这里钉住「配得对」与「配不出来要返回 null（由调用方降级）」。

const col = (over: Partial<DatasetColumn> & Pick<DatasetColumn, 'id' | 'name'>): DatasetColumn =>
  ({
    expr: over.name,
    type: 'string',
    comment: '',
    role: 'dimension',
    ...over,
  }) as DatasetColumn;

const sampleColumns: DatasetColumn[] = [
  col({ id: 'c-order-id', name: 'order_id', type: 'integer' }),
  col({ id: 'c-date', name: 'order_date', type: 'date' }),
  col({ id: 'c-region', name: 'region', type: 'string' }),
  col({ id: 'c-amount', name: 'amount', type: 'float', role: 'metric' }),
  col({ id: 'c-qty', name: 'quantity', type: 'integer', role: 'metric' }),
];

describe('buildFirstChartSpec（首启示例图）', () => {
  it('日期维度 + 指标 → 折线图，落一份合法的 v2 查询文档', () => {
    const built = buildFirstChartSpec(sampleColumns, '示例：电商订单');
    expect(built).not.toBeNull();
    if (!built) return;

    expect(built.chartType).toBe('line');
    expect(built.dimension.name).toBe('order_date');
    expect(built.metric.name).toBe('amount');
    expect(built.spec.v).toBe(1);
    expect(built.spec.document.version).toBe(2);
  });

  it('文档能被 ChartBuilder 的还原入口解析（否则 ?q= 直链会退化成空画布）', () => {
    const built = buildFirstChartSpec(sampleColumns, '示例：电商订单');
    if (!built) throw new Error('expected a build');

    const parsed = parseQuerySpecDocument(built.spec);
    expect(parsed).not.toBeNull();
    if (!parsed) return;

    const dimBindings = parsed.query.dimensionGroups.flatMap((g) => g.bindings);
    const metricBindings = parsed.query.metricGroups.flatMap((g) => g.bindings);
    // 槽位 id 必须是图型定义里的那一套（x_axis / values），否则字段会被搬进保留槽位、
    // 甚至当溢出绑定丢掉。
    expect(parsed.query.dimensionGroups[0]?.id).toBe('x_axis');
    expect(parsed.query.metricGroups[0]?.id).toBe('values');
    // 引用的是列的稳定 id，不是列名（列名可改，id 不可变）。
    expect(dimBindings[0]?.fieldId).toBe('c-date');
    expect(metricBindings[0]?.fieldId).toBe('c-amount');
    // bindingId 全局唯一：两个组共用同一个 id 会让渲染层把维度当指标画。
    const ids = [...dimBindings, ...metricBindings].map((b) => b.bindingId);
    expect(new Set(ids).size).toBe(ids.length);
    // 指标必须有聚合口径，否则后端生成 SQL 时 group by 不成立。
    expect(parsed.fieldMeta[metricBindings[0]?.bindingId ?? '']?.aggregation).toBe('sum');
  });

  it('没有数值列 → null（调用方降级为普通空画布，不硬塞一个查不出数的图）', () => {
    const noMetric = sampleColumns.filter((c) => c.role !== 'metric' && c.type !== 'integer');
    expect(buildFirstChartSpec(noMetric, '无指标')).toBeNull();
  });

  it('没有日期列时退成柱状图，且优先挑维度角色的字符串列而非 id 类整数列', () => {
    const noDate = sampleColumns.filter((c) => c.id !== 'c-date');
    const built = buildFirstChartSpec(noDate, '示例：电商订单');
    expect(built?.chartType).toBe('bar');
    expect(built?.dimension.name).toBe('region');
  });

  it('空列集 → null', () => {
    expect(buildFirstChartSpec([], '空')).toBeNull();
  });
});
