import { describe, expect, it } from 'vitest';
import { type ChartConfigDocument, migrateChartConfig } from '@/lib/chartConfigSchema';

/**
 * chartConfigSchema v1 迁移测试。
 *
 * 背景：bi_chart.config 旧结构以 `field-${index}` 位置 id 存字段引用，
 * 列名只存在于运行时字段列表。migrateChartConfig 必须接受可选 fields
 * 参数把旧 id 解析为稳定列名；无 fields 时按原样透传（有损路径）。
 */

const FIELDS = [
  { id: 'field-0', name: 'region' },
  { id: 'field-1', name: 'gmv' },
  { id: 'field-2', name: 'order_date' },
];

/** 一份"今天 ChartBuilder.handleSave 真实会写出的"旧结构 JSON */
function legacyConfig(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    chartType: 'bar',
    xAxisField: null,
    yAxisFields: [],
    title: '各地区 GMV',
    queryConfig: {
      dimensionGroups: [{ id: 'dim-0', fields: ['field-0'] }],
      metricGroups: [{ id: 'metric-0', fields: ['field-1'], alias: '销售额' }],
      filters: [{ id: 'f-0', field: 'field-2', operator: 'gt', value: '2024-01-01', logic: 'and' }],
      sort: { field: 'field-1', order: 'desc' },
      limit: 1000,
    },
    dimensionLabels: { 'field-0': '地区' },
    metricAggregations: { 'field-1': 'sum' },
    metricAliases: { 'field-1': 'GMV' },
    metricUnits: { 'field-1': '元' },
    metricFormats: { 'field-1': 'thousands' },
    chartStyle: { colors: ['#1f77b4'], smooth: false, tableRowSize: 'middle' },
    chartQueryOptions: { pieMergeOtherBelowRatio: 0.03 },
    ...overrides,
  });
}

describe('migrateChartConfig：旧结构 → v1', () => {
  it('完整旧结构迁移为 v1 文档', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table', FIELDS);

    expect(doc.version).toBe(1);
    expect(doc.chartType).toBe('bar');
    expect(doc.title).toBe('各地区 GMV');

    // queryConfig → query
    expect(doc.query.dimensionGroups).toEqual([{ id: 'dim-0', fields: ['region'] }]);
    expect(doc.query.metricGroups).toEqual([{ id: 'metric-0', fields: ['gmv'], alias: '销售额' }]);
    expect(doc.query.limit).toBe(1000);

    // 5 个平铺 Record 收敛为 fieldMeta，键为列名
    expect(doc.fieldMeta).toEqual({
      region: { label: '地区' },
      gmv: { aggregation: 'sum', alias: 'GMV', unit: '元', format: 'thousands' },
    });

    // 轴字段被丢弃
    expect(doc).not.toHaveProperty('xAxisField');
    expect(doc).not.toHaveProperty('yAxisFields');
    expect(doc).not.toHaveProperty('queryConfig');
    expect(doc).not.toHaveProperty('dimensionLabels');
  });

  it('fieldId → 列名解析：组字段与 fieldMeta 同步解析', () => {
    const raw = JSON.stringify({
      chartType: 'line',
      title: '',
      queryConfig: {
        dimensionGroups: [{ id: 'dim-0', fields: ['field-0'] }],
        metricGroups: [{ id: 'metric-0', fields: ['field-1'] }],
        filters: [],
      },
      metricAggregations: { 'field-1': 'sum' },
    });
    const doc = migrateChartConfig(raw, 'table', FIELDS);

    expect(doc.query.dimensionGroups[0].fields).toEqual(['region']);
    expect(doc.query.metricGroups[0].fields).toEqual(['gmv']);
    expect(doc.fieldMeta.gmv).toEqual({ aggregation: 'sum' });
    expect(doc.fieldMeta['field-1']).toBeUndefined();
  });

  it('无 fields 参数时旧位置 id 原样透传（有损路径）', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table');

    expect(doc.query.dimensionGroups[0].fields).toEqual(['field-0']);
    expect(doc.query.metricGroups[0].fields).toEqual(['field-1']);
    expect(doc.fieldMeta['field-1']).toEqual({
      aggregation: 'sum',
      alias: 'GMV',
      unit: '元',
      format: 'thousands',
    });
  });

  it('fields 提供但解析不到的 id：组字段保留原值，fieldMeta 条目丢弃', () => {
    const raw = JSON.stringify({
      queryConfig: {
        dimensionGroups: [{ id: 'dim-0', fields: ['field-99'] }],
        metricGroups: [],
        filters: [],
      },
      metricAggregations: { 'field-99': 'avg' },
    });
    const doc = migrateChartConfig(raw, 'table', FIELDS);

    expect(doc.query.dimensionGroups[0].fields).toEqual(['field-99']);
    expect(doc.fieldMeta).toEqual({});
  });

  it('filters 混合 field 键（位置 id 与已是列名）均可解析为列名', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table', FIELDS);
    const filters = doc.query.filters as { field: string }[];

    expect(filters[0].field).toBe('order_date');
  });

  it('filters 中已是列名的字段原样保留', () => {
    const raw = JSON.stringify({
      queryConfig: {
        dimensionGroups: [],
        metricGroups: [],
        filters: [
          { id: 'f-0', field: 'field-0', operator: 'eq', value: '华东', logic: 'and' },
          { id: 'f-1', field: 'region', operator: 'neq', value: '华南', logic: 'and' },
        ],
      },
    });
    const doc = migrateChartConfig(raw, 'bar', FIELDS);
    const fields = (doc.query.filters as { field: string }[]).map((f) => f.field);

    expect(fields).toEqual(['region', 'region']);
  });

  it('sort 的 field 解析为列名，limit 保留', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table', FIELDS);

    expect(doc.query.sort).toEqual({ field: 'gmv', order: 'desc' });
    expect(doc.query.limit).toBe(1000);
  });

  it('chartStyle → style、chartQueryOptions → queryOptions 透传', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table', FIELDS);

    expect(doc.style).toEqual({
      colors: ['#1f77b4'],
      smooth: false,
      tableRowSize: 'middle',
    });
    expect(doc.queryOptions).toEqual({ pieMergeOtherBelowRatio: 0.03 });
  });

  it('缺失/非法 chartType 回退 fallbackType，缺失 title 回退空串', () => {
    const raw = JSON.stringify({ chartType: 'funnel', queryConfig: {} });
    const doc = migrateChartConfig(raw, 'pivot', FIELDS);

    expect(doc.chartType).toBe('pivot');
    expect(doc.title).toBe('');
  });
});

describe('migrateChartConfig：损坏输入', () => {
  const emptyDoc = (fallback: ChartConfigDocument['chartType']): ChartConfigDocument => ({
    version: 1,
    chartType: fallback,
    title: '',
    query: {
      dimensionGroups: [],
      metricGroups: [],
      filters: [],
      sort: undefined,
      limit: undefined,
    },
    fieldMeta: {},
    style: {},
    queryOptions: {},
  });

  it.each([
    ['空串', ''],
    ['损坏 JSON', '{"chartType": "bar"'],
    ['JSON 数组', '[1,2,3]'],
    ['JSON 字符串', '"hello"'],
    ['JSON 数字', '42'],
    ['null', 'null'],
  ])('%s → 返回空 v1 文档且不抛异常', (_label, raw) => {
    const doc = migrateChartConfig(raw, 'line');
    expect(doc).toEqual(emptyDoc('line'));
  });
});

describe('migrateChartConfig：v1 直通', () => {
  it('version===1 返回等价文档', () => {
    const migrated = migrateChartConfig(legacyConfig(), 'table', FIELDS);
    const passthrough = migrateChartConfig(JSON.stringify(migrated), 'pie', FIELDS);

    expect(passthrough).toEqual(migrated);
    expect(passthrough.version).toBe(1);
  });

  it('不改动输入：修改返回文档不影响后续解析结果', () => {
    const raw = JSON.stringify(migrateChartConfig(legacyConfig(), 'table', FIELDS));
    const first = migrateChartConfig(raw, 'table');
    const snapshot = JSON.parse(raw) as ChartConfigDocument;

    first.query.dimensionGroups.push({ id: 'evil', fields: ['x'] });
    first.fieldMeta.evil = { label: 'evil' };

    const second = migrateChartConfig(raw, 'table');
    expect(second).toEqual(snapshot);
  });

  it('v1 缺省小节时补默认值', () => {
    const doc = migrateChartConfig('{"version":1,"chartType":"area"}', 'bar');

    expect(doc).toEqual({
      version: 1,
      chartType: 'area',
      title: '',
      query: {
        dimensionGroups: [],
        metricGroups: [],
        filters: [],
        sort: undefined,
        limit: undefined,
      },
      fieldMeta: {},
      style: {},
      queryOptions: {},
    });
  });
});

describe('migrateChartConfig：ShareView 消费契约', () => {
  it('迁移产物的 query 组字段是列名，ShareView 无需字段列表即可取轴', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table', FIELDS);

    const dimensionNames = doc.query.dimensionGroups.flatMap((g) => g.fields);
    const metricNames = doc.query.metricGroups.flatMap((g) => g.fields);
    const knownNames = FIELDS.map((f) => f.name);

    expect(dimensionNames).toEqual(['region']);
    expect(metricNames).toEqual(['gmv']);
    for (const name of [...dimensionNames, ...metricNames]) {
      expect(knownNames).toContain(name);
      expect(name).not.toMatch(/^field-\d+$/);
    }
    // 轴配置读取 fieldMeta 也按列名命中
    expect(doc.fieldMeta.gmv.aggregation).toBe('sum');
  });
});
