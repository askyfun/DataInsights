import { describe, expect, it } from 'vitest';
import { type ChartConfigDocument, migrateChartConfig } from '@/lib/chartConfigSchema';

/**
 * chartConfigSchema v2 迁移测试。
 *
 * 背景：bi_chart.config 旧结构以 `field-${index}` 位置 id 存字段引用，v1 统一为列名，
 * v2 进一步把字段组升级为 bindings（每个字段实例带全局唯一 bindingId），fieldMeta 的键
 * 由列名改为 bindingId（修复 D2：同列多组共享元数据）。migrateChartConfig 必须接受可选
 * fields 参数把旧 id 解析为列名；无 fields 时按原样透传（有损路径）。
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

describe('migrateChartConfig：旧结构 → v2', () => {
  it('完整旧结构迁移为 v2 文档', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table', FIELDS);

    expect(doc.version).toBe(2);
    expect(doc.chartType).toBe('bar');
    expect(doc.title).toBe('各地区 GMV');

    // queryConfig → query，fields → bindings（region=b-0，gmv=b-1）
    expect(doc.query.dimensionGroups).toEqual([
      { id: 'dim-0', bindings: [{ bindingId: 'b-0', field: 'region' }] },
    ]);
    expect(doc.query.metricGroups).toEqual([
      { id: 'metric-0', bindings: [{ bindingId: 'b-1', field: 'gmv' }], alias: '销售额' },
    ]);
    expect(doc.query.limit).toBe(1000);

    // 5 个平铺 Record 收敛为 fieldMeta，键由列名改为 bindingId
    expect(doc.fieldMeta).toEqual({
      'b-0': { label: '地区' },
      'b-1': { aggregation: 'sum', alias: 'GMV', unit: '元', format: 'thousands' },
    });

    // 轴字段被丢弃
    expect(doc).not.toHaveProperty('xAxisField');
    expect(doc).not.toHaveProperty('yAxisFields');
    expect(doc).not.toHaveProperty('queryConfig');
    expect(doc).not.toHaveProperty('dimensionLabels');
  });

  it('fieldId → 列名解析：组 bindings 与 fieldMeta 同步解析为 bindingId 键', () => {
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

    expect(doc.query.dimensionGroups[0].bindings).toEqual([{ bindingId: 'b-0', field: 'region' }]);
    expect(doc.query.metricGroups[0].bindings).toEqual([{ bindingId: 'b-1', field: 'gmv' }]);
    expect(doc.fieldMeta['b-1']).toEqual({ aggregation: 'sum' });
    // 列名键与旧位置 id 键都不再保留
    expect(doc.fieldMeta['field-1']).toBeUndefined();
    expect(doc.fieldMeta.gmv).toBeUndefined();
  });

  it('无 fields 参数时旧位置 id 原样透传（有损路径）', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table');

    expect(doc.query.dimensionGroups[0].bindings).toEqual([{ bindingId: 'b-0', field: 'field-0' }]);
    expect(doc.query.metricGroups[0].bindings).toEqual([{ bindingId: 'b-1', field: 'field-1' }]);
    expect(doc.fieldMeta['b-1']).toEqual({
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

    expect(doc.query.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'field-99' },
    ]);
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

  it('sort 的列名翻译为对应 binding 的 bindingId，limit 保留', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table', FIELDS);

    // legacy sort.field='field-1' → 列名 'gmv' → metric 组 binding b-1
    expect(doc.query.sort).toEqual({ bindingId: 'b-1', order: 'desc' });
    expect(doc.query.limit).toBe(1000);
  });

  it('sort 引用的列不在任何组里：丢弃 sort（undefined）', () => {
    const raw = JSON.stringify({
      chartType: 'bar',
      queryConfig: {
        dimensionGroups: [{ id: 'dim-0', fields: ['field-0'] }],
        metricGroups: [],
        sort: { field: 'field-1', order: 'desc' },
      },
    });
    // field-1 解析为列名 gmv，但 gmv 不在任何组里 → sort 丢弃
    expect(migrateChartConfig(raw, 'bar', FIELDS).query.sort).toBeUndefined();
  });

  it('sort 列名同时出现在维度组与指标组：取 bindingId 分配顺序的第一个（维度在前）', () => {
    const raw = JSON.stringify({
      chartType: 'pivot',
      queryConfig: {
        dimensionGroups: [{ id: 'rows', fields: ['field-1'] }],
        metricGroups: [{ id: 'v0', fields: ['field-1'] }],
        sort: { field: 'field-1', order: 'asc' },
      },
    });
    const doc = migrateChartConfig(raw, 'pivot', FIELDS);

    // gmv 在维度组是 b-0、指标组是 b-1：与 bindingId 分配顺序一致取 b-0
    expect(doc.query.dimensionGroups[0].bindings).toEqual([{ bindingId: 'b-0', field: 'gmv' }]);
    expect(doc.query.metricGroups[0].bindings).toEqual([{ bindingId: 'b-1', field: 'gmv' }]);
    expect(doc.query.sort).toEqual({ bindingId: 'b-0', order: 'asc' });
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
    // 注意：'funnel' 曾是本用例的非法示例，R-59 起已入 CHART_TYPES 白名单，
    // 改用仍未实现的 'sunburst' 作为非法值。
    const raw = JSON.stringify({ chartType: 'sunburst', queryConfig: {} });
    const doc = migrateChartConfig(raw, 'pivot', FIELDS);

    expect(doc.chartType).toBe('pivot');
    expect(doc.title).toBe('');
  });

  it('chartType histogram 合法（R-57）：迁移不回退，queryOptions.binCount 透传', () => {
    const v2Raw = JSON.stringify({
      version: 2,
      chartType: 'histogram',
      title: '金额分布',
      query: {
        dimensionGroups: [],
        metricGroups: [{ id: 'metric-group-1', bindings: [{ bindingId: 'b-0', field: 'gmv' }] }],
        filters: [],
      },
      fieldMeta: {},
      style: {},
      queryOptions: { binCount: 15 },
    });
    const v2Doc = migrateChartConfig(v2Raw, 'bar', FIELDS);

    expect(v2Doc.chartType).toBe('histogram');
    expect(v2Doc.queryOptions).toEqual({ binCount: 15 });

    // v1 文档同样接受 histogram（CHART_TYPES 白名单命中，不回落 fallbackType）
    const v1Raw = JSON.stringify({ version: 1, chartType: 'histogram' });
    expect(migrateChartConfig(v1Raw, 'bar', FIELDS).chartType).toBe('histogram');
  });

  it('chartType funnel 合法（R-59）：迁移不回退', () => {
    const v2Raw = JSON.stringify({
      version: 2,
      chartType: 'funnel',
      title: '转化漏斗',
      query: {
        dimensionGroups: [{ id: 'stages', bindings: [{ bindingId: 'b-0', field: 'stage' }] }],
        metricGroups: [{ id: 'value', bindings: [{ bindingId: 'b-1', field: 'cnt' }] }],
        filters: [],
      },
      fieldMeta: {},
      style: {},
      queryOptions: {},
    });
    const v2Doc = migrateChartConfig(v2Raw, 'bar', FIELDS);

    expect(v2Doc.chartType).toBe('funnel');
    expect(v2Doc.query.metricGroups[0].bindings).toEqual([{ bindingId: 'b-1', field: 'cnt' }]);

    // v1 文档同样接受 funnel（CHART_TYPES 白名单命中，不回落 fallbackType）
    const v1Raw = JSON.stringify({ version: 1, chartType: 'funnel' });
    expect(migrateChartConfig(v1Raw, 'bar', FIELDS).chartType).toBe('funnel');
  });
});

describe('migrateChartConfig：损坏输入', () => {
  const emptyDoc = (fallback: ChartConfigDocument['chartType']): ChartConfigDocument => ({
    version: 2,
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
  ])('%s → 返回空 v2 文档且不抛异常', (_label, raw) => {
    const doc = migrateChartConfig(raw, 'line');
    expect(doc).toEqual(emptyDoc('line'));
  });
});

describe('migrateChartConfig：v1 → v2 迁移（bindingId 生成）', () => {
  it('v1 文档转 v2：bindingId 按 dimension→metric、组内顺序连续生成', () => {
    const v1 = JSON.stringify({
      version: 1,
      chartType: 'bar',
      title: 'T',
      query: {
        dimensionGroups: [{ id: 'd0', fields: ['region', 'city'] }],
        metricGroups: [{ id: 'm0', fields: ['gmv'] }],
        filters: [],
        limit: 10,
      },
      fieldMeta: { region: { label: '地区' }, gmv: { aggregation: 'sum' } },
      style: {},
      queryOptions: {},
    });
    const doc = migrateChartConfig(v1, 'table');

    expect(doc.version).toBe(2);
    expect(doc.query.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'region' },
      { bindingId: 'b-1', field: 'city' },
    ]);
    expect(doc.query.metricGroups[0].bindings).toEqual([{ bindingId: 'b-2', field: 'gmv' }]);
    // fieldMeta 从列名键复制到 bindingId 键
    expect(doc.fieldMeta['b-0']).toEqual({ label: '地区' });
    expect(doc.fieldMeta['b-2']).toEqual({ aggregation: 'sum' });
    // 原列名键丢弃
    expect(doc.fieldMeta.region).toBeUndefined();
    expect(doc.fieldMeta.gmv).toBeUndefined();
  });

  it('同一列名出现在两个组：生成两个不同 bindingId，各得一份独立 fieldMeta 拷贝', () => {
    const v1 = JSON.stringify({
      version: 1,
      chartType: 'pivot',
      title: '',
      query: {
        dimensionGroups: [{ id: 'rows', fields: ['region'] }],
        metricGroups: [
          { id: 'v0', fields: ['amount'] },
          { id: 'v1', fields: ['amount'] },
        ],
        filters: [],
      },
      fieldMeta: { amount: { aggregation: 'sum', alias: '金额' } },
    });
    const doc = migrateChartConfig(v1, 'pivot');

    // region→b-0，amount(v0)→b-1，amount(v1)→b-2
    expect(doc.query.metricGroups[0].bindings).toEqual([{ bindingId: 'b-1', field: 'amount' }]);
    expect(doc.query.metricGroups[1].bindings).toEqual([{ bindingId: 'b-2', field: 'amount' }]);
    expect(doc.fieldMeta['b-1']).toEqual({ aggregation: 'sum', alias: '金额' });
    expect(doc.fieldMeta['b-2']).toEqual({ aggregation: 'sum', alias: '金额' });

    // 两份拷贝相互独立（D2 修复目标）：修改其一不影响其二
    expect(doc.fieldMeta['b-1']).not.toBe(doc.fieldMeta['b-2']);
    doc.fieldMeta['b-1'].alias = '主轴金额';
    expect(doc.fieldMeta['b-1'].alias).toBe('主轴金额');
    expect(doc.fieldMeta['b-2'].alias).toBe('金额');
  });

  it('v1 缺省小节时补默认值（输出仍是 v2）', () => {
    const doc = migrateChartConfig('{"version":1,"chartType":"area"}', 'bar');

    expect(doc).toEqual({
      version: 2,
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

describe('migrateChartConfig：v2 直通', () => {
  it('version===2 返回等价文档', () => {
    const migrated = migrateChartConfig(legacyConfig(), 'table', FIELDS);
    const passthrough = migrateChartConfig(JSON.stringify(migrated), 'pie', FIELDS);

    expect(passthrough).toEqual(migrated);
    expect(passthrough.version).toBe(2);
  });

  it('不改动输入：修改返回文档不影响后续解析结果', () => {
    const raw = JSON.stringify(migrateChartConfig(legacyConfig(), 'table', FIELDS));
    const first = migrateChartConfig(raw, 'table');
    const snapshot = JSON.parse(raw) as ChartConfigDocument;

    first.query.dimensionGroups.push({ id: 'evil', bindings: [{ bindingId: 'b-99', field: 'x' }] });
    first.fieldMeta.evil = { label: 'evil' };

    const second = migrateChartConfig(raw, 'table');
    expect(second).toEqual(snapshot);
  });

  it('v2 直通校验 bindings：剔除缺 bindingId/field 的非法条目', () => {
    const raw = JSON.stringify({
      version: 2,
      chartType: 'bar',
      query: {
        dimensionGroups: [
          {
            id: 'd0',
            bindings: [
              { bindingId: 'b-0', field: 'region' },
              { bindingId: '', field: 'bad' },
              { field: 'no-id' },
              'not-an-object',
            ],
          },
        ],
        metricGroups: [],
        filters: [],
      },
      fieldMeta: { 'b-0': { label: '地区' } },
    });
    const doc = migrateChartConfig(raw, 'bar');

    expect(doc.query.dimensionGroups[0].bindings).toEqual([{ bindingId: 'b-0', field: 'region' }]);
    expect(doc.fieldMeta).toEqual({ 'b-0': { label: '地区' } });
  });

  it('v2 直通：bindingId 形态的 sort 原样透传', () => {
    const raw = JSON.stringify({
      version: 2,
      chartType: 'table',
      query: {
        dimensionGroups: [{ id: 'd0', bindings: [{ bindingId: 'b-0', field: 'region' }] }],
        metricGroups: [{ id: 'm0', bindings: [{ bindingId: 'b-1', field: 'gmv' }] }],
        filters: [],
        sort: { bindingId: 'b-1', order: 'desc' },
      },
      fieldMeta: {},
    });
    const doc = migrateChartConfig(raw, 'table');

    expect(doc.query.sort).toEqual({ bindingId: 'b-1', order: 'desc' });
  });

  it('v2 直通兼容窗口期 field 键 sort：按列名翻译为 bindingId，翻译不到则丢弃', () => {
    const base = {
      version: 2,
      chartType: 'table',
      query: {
        dimensionGroups: [{ id: 'd0', bindings: [{ bindingId: 'b-0', field: 'region' }] }],
        metricGroups: [{ id: 'm0', bindings: [{ bindingId: 'b-1', field: 'gmv' }] }],
        filters: [],
      },
      fieldMeta: {},
    };

    // Task 0-3~1-7 窗口期保存的文档：sort.field 是列名
    const withField = migrateChartConfig(
      JSON.stringify({ ...base, query: { ...base.query, sort: { field: 'gmv', order: 'desc' } } }),
      'table'
    );
    expect(withField.query.sort).toEqual({ bindingId: 'b-1', order: 'desc' });

    const dangling = migrateChartConfig(
      JSON.stringify({
        ...base,
        query: { ...base.query, sort: { field: 'nonexistent', order: 'asc' } },
      }),
      'table'
    );
    expect(dangling.query.sort).toBeUndefined();
  });
});

describe('migrateChartConfig：ShareView 消费契约', () => {
  it('迁移产物的 query 组 bindings 携带列名，ShareView 无需字段列表即可取轴', () => {
    const doc = migrateChartConfig(legacyConfig(), 'table', FIELDS);

    const dimensionNames = doc.query.dimensionGroups.flatMap((g) => g.bindings.map((b) => b.field));
    const metricNames = doc.query.metricGroups.flatMap((g) => g.bindings.map((b) => b.field));
    const knownNames = FIELDS.map((f) => f.name);

    expect(dimensionNames).toEqual(['region']);
    expect(metricNames).toEqual(['gmv']);
    for (const name of [...dimensionNames, ...metricNames]) {
      expect(knownNames).toContain(name);
      expect(name).not.toMatch(/^field-\d+$/);
    }
    // 轴配置读取 fieldMeta 按 bindingId 命中（gmv 是 b-1）
    expect(doc.fieldMeta['b-1'].aggregation).toBe('sum');
  });
});
