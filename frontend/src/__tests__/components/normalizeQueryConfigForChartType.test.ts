import { describe, expect, it } from 'vitest';
import { normalizeQueryConfigForChartType } from '@/components/ChartBuilder/chartDefinitions';
import type { FieldGroup, QueryConfig } from '@/store';

/**
 * normalizeQueryConfigForChartType：切换图表类型时对 queryConfig 的归一化。
 *
 * 两条历史行为必须同时保住：
 * ① 补齐最小字段组数量（旧行为）；
 * ② **被裁掉的字段要搬到同类型的保留组，而不是静默消失**（本轮修复）。
 * 触发场景举例：透视表（行/列两组维度）切成表格（只有一组维度）时，
 * 列维度上的字段此前会直接从请求里消失，用户配好的列维度白配了。
 */

const group = (id: string, fields: string[]): FieldGroup => ({
  id,
  bindings: fields.map((field, index) => ({ bindingId: `${id}-b-${index}`, field })),
});

const makeConfig = (
  dimensionGroups: FieldGroup[],
  metricGroups: FieldGroup[] = []
): QueryConfig => ({
  dimensionGroups,
  metricGroups,
  filters: [],
  limit: 1000,
});

const bindingFields = (group: FieldGroup | undefined) =>
  (group?.bindings ?? []).map((binding) => binding.field);

describe('normalizeQueryConfigForChartType', () => {
  it('透视表 → 表格：列维度字段搬进行维度，而不是被裁掉', () => {
    const next = normalizeQueryConfigForChartType(
      'table',
      makeConfig(
        [group('rows', ['region']), group('columns', ['month'])],
        [group('values', ['revenue'])]
      )
    );

    expect(next.dimensionGroups).toHaveLength(1);
    expect(bindingFields(next.dimensionGroups[0])).toEqual(['region', 'month']);
  });

  it('搬运保留 bindingId，别名/单位/格式等按 bindingId 索引的元数据不脱钩', () => {
    const next = normalizeQueryConfigForChartType(
      'table',
      makeConfig([group('rows', ['region']), group('columns', ['month'])])
    );

    expect(next.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'rows-b-0', field: 'region' },
      { bindingId: 'columns-b-0', field: 'month' },
    ]);
  });

  it('目标组为空时，溢出的字段落进第 0 组', () => {
    const next = normalizeQueryConfigForChartType(
      'table',
      makeConfig([group('rows', []), group('columns', ['month'])])
    );

    expect(bindingFields(next.dimensionGroups[0])).toEqual(['month']);
  });

  it('同名列已存在时按列名去重，不重复搬运', () => {
    const next = normalizeQueryConfigForChartType(
      'table',
      makeConfig([group('rows', ['region']), group('columns', ['region', 'month'])])
    );

    expect(bindingFields(next.dimensionGroups[0])).toEqual(['region', 'month']);
  });

  it('组合图 → 表格：第二组维度并入 X 轴、次轴指标并入主轴', () => {
    // 第二组沿用历史 id color_group（该槽位已于 2026-09-19 下线，但老文档里就是这么存的）
    const next = normalizeQueryConfigForChartType(
      'table',
      makeConfig(
        [group('x_axis', ['region']), group('color_group', ['city'])],
        [group('primary_values', ['revenue']), group('secondary_values', ['profit'])]
      )
    );

    expect(bindingFields(next.dimensionGroups[0])).toEqual(['region', 'city']);
    expect(bindingFields(next.metricGroups[0])).toEqual(['revenue', 'profit']);
  });

  it('历史 bar 图表（x_axis + color_group 两组）→ 归一化为单组 X 轴、字段不丢', () => {
    const next = normalizeQueryConfigForChartType(
      'bar',
      makeConfig([group('x_axis', ['region']), group('color_group', ['city'])])
    );

    expect(next.dimensionGroups).toHaveLength(1);
    expect(bindingFields(next.dimensionGroups[0])).toEqual(['region', 'city']);
    // bindingId 原样保留：别名/单位/格式等按 bindingId 索引的元数据不脱钩
    expect(next.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'x_axis-b-0', field: 'region' },
      { bindingId: 'color_group-b-0', field: 'city' },
    ]);
  });

  it('历史 radar 图表（indicators + series_group 两组）→ 归一化为单组指标维度、字段不丢', () => {
    const next = normalizeQueryConfigForChartType(
      'radar',
      makeConfig([group('indicators', ['attr']), group('series_group', ['team'])])
    );

    expect(next.dimensionGroups).toHaveLength(1);
    expect(bindingFields(next.dimensionGroups[0])).toEqual(['attr', 'team']);
  });

  it('新图型没有该类型槽位时原样保留（切回旧图型字段还在），不做破坏性裁剪', () => {
    const source = makeConfig([group('rows', ['region']), group('columns', ['month'])]);
    const next = normalizeQueryConfigForChartType('kpi', source);

    expect(next.dimensionGroups).toEqual(source.dimensionGroups);
  });

  it('保留旧的补齐行为：表格 → 透视表补出第二组维度', () => {
    const table = normalizeQueryConfigForChartType(
      'table',
      makeConfig([group('dimensions', ['region'])])
    );
    expect(table.dimensionGroups).toHaveLength(1);

    const pivot = normalizeQueryConfigForChartType('pivot', table);
    expect(pivot.dimensionGroups).toHaveLength(2);
    expect(bindingFields(pivot.dimensionGroups[0])).toEqual(['region']);
    expect(bindingFields(pivot.dimensionGroups[1])).toEqual([]);
  });

  it('组数未超限时不搬运也不改动原组', () => {
    const source = makeConfig([group('x_axis', ['region'])], [group('values', ['revenue'])]);
    const next = normalizeQueryConfigForChartType('bar', source);

    expect(next.dimensionGroups).toEqual(source.dimensionGroups);
    expect(next.metricGroups).toEqual(source.metricGroups);
  });

  it('溢出的组本来就是空组时只做裁剪，不产生搬运', () => {
    const source = makeConfig([group('rows', ['region']), group('columns', [])]);
    const next = normalizeQueryConfigForChartType('table', source);

    expect(next.dimensionGroups).toHaveLength(1);
    expect(bindingFields(next.dimensionGroups[0])).toEqual(['region']);
  });
});
