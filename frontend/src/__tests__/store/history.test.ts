import { beforeEach, describe, expect, it } from 'vitest';
import type { ChartField } from '@/store';
import { useStore } from '@/store';

/**
 * 图表构建页撤销/重做时间线（store/history.ts 的 withHistory）。
 *
 * 覆盖 AC 的四类操作：增删维度/指标/筛选、图表类型切换、聚合方式变更；
 * 以及三条不变式：非配置态（分页/标题/数据加载）不入栈、新操作清空 redo 分支、
 * undo 后 redo 精确回到原位。
 */

const DIM: ChartField = { id: 'province', name: 'province', type: 'dimension', dataType: 'text' };
const METRIC: ChartField = { id: 'sales', name: 'sales', type: 'metric', dataType: 'double' };

const snap = () => useStore.getState();

beforeEach(() => {
  useStore.getState().resetChartBuilder();
  useStore.getState().resetHistory();
});

describe('undo/redo · 筛选', () => {
  it('addFilter 入栈；undo 撤销该条件；redo 精确恢复', () => {
    snap().addFilter({ fieldId: 'province' });
    expect(snap().past).toHaveLength(1);
    expect(snap().queryConfig.filters).toHaveLength(1);

    snap().undo();
    expect(snap().queryConfig.filters).toHaveLength(0);
    expect(snap().future).toHaveLength(1);

    snap().redo();
    expect(snap().queryConfig.filters).toHaveLength(1);
    expect(snap().queryConfig.filters[0].fieldId).toBe('province');
  });

  it('updateFilter 可撤销回旧值', () => {
    snap().addFilter({ fieldId: 'province', operator: 'eq', value: 'A' });
    const id = snap().queryConfig.filters[0].id;
    snap().updateFilter(id, { value: 'B' });
    expect(snap().queryConfig.filters[0].value).toBe('B');

    snap().undo();
    expect(snap().queryConfig.filters[0].value).toBe('A');
  });
});

describe('undo/redo · 维度/指标', () => {
  it('addDimensionField → undo → 维度组回到空', () => {
    snap().addDimensionField(DIM);
    expect(snap().queryConfig.dimensionGroups[0].bindings).toHaveLength(1);

    snap().undo();
    const bindings = snap().queryConfig.dimensionGroups[0]?.bindings ?? [];
    expect(bindings).toHaveLength(0);
  });

  it('addMetricField + setMetricAggregation 两步各自可撤销', () => {
    snap().addMetricField(METRIC);
    const bindingId = snap().queryConfig.metricGroups[0].bindings[0].bindingId;
    snap().setMetricAggregation(bindingId, 'max');
    expect(snap().metricAggregations[bindingId]).toBe('max');
    expect(snap().past).toHaveLength(2);

    // 撤销聚合变更
    snap().undo();
    expect(snap().metricAggregations[bindingId]).toBeUndefined();
    // 再撤销指标添加
    snap().undo();
    expect(snap().queryConfig.metricGroups[0]?.bindings ?? []).toHaveLength(0);

    // 重做两步回到原位
    snap().redo();
    snap().redo();
    expect(snap().metricAggregations[bindingId]).toBe('max');
  });
});

describe('undo/redo · 图表类型', () => {
  it('切换 chartType 可撤销，且 title 不受影响', () => {
    snap().setChartBuilderConfig({ title: '我的图' });
    // title-only 变更不入栈
    expect(snap().past).toHaveLength(0);

    snap().setChartBuilderConfig({ chartType: 'bar' });
    expect(snap().chartBuilderConfig.chartType).toBe('bar');
    expect(snap().past).toHaveLength(1);

    snap().undo();
    expect(snap().chartBuilderConfig.chartType).toBe('table');
    // title 是投影外字段，撤销图表类型不应回退标题
    expect(snap().chartBuilderConfig.title).toBe('我的图');
  });
});

describe('undo/redo · 不变式', () => {
  it('非可撤销状态（分页）变化不入栈', () => {
    snap().setTablePagination({ page: 2, pageSize: 100, total: 500 });
    expect(snap().past).toHaveLength(0);
  });

  it('新操作清空 redo 分支', () => {
    snap().addFilter({ fieldId: 'a' });
    snap().addFilter({ fieldId: 'b' });
    snap().undo();
    expect(snap().future).toHaveLength(1);

    snap().addFilter({ fieldId: 'c' });
    expect(snap().future).toHaveLength(0);
  });

  it('空栈时 undo/redo 是安全的 no-op', () => {
    snap().undo();
    snap().redo();
    expect(snap().past).toHaveLength(0);
    expect(snap().future).toHaveLength(0);
    expect(snap().queryConfig.filters).toHaveLength(0);
  });

  it('resetHistory 清空两条栈', () => {
    snap().addFilter({ fieldId: 'a' });
    snap().addFilter({ fieldId: 'b' });
    snap().undo();
    snap().resetHistory();
    expect(snap().past).toHaveLength(0);
    expect(snap().future).toHaveLength(0);
  });
});
