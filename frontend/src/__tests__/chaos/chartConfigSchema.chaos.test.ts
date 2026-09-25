/* chartConfigSchema.ts 属性/随机回归测试（混沌探针转正，2026-09-20） */
import { describe, expect, it } from 'vitest';
import { type ChartType, migrateChartConfig } from '@/lib/chartConfigSchema';
import { j, Rng, shrinkRaw } from './_gen';

const CHART_TYPES: ChartType[] = [
  'bar',
  'line',
  'pie',
  'area',
  'scatter',
  'table',
  'pivot',
  'combo',
  'kpi',
  'histogram',
  'funnel',
  'radar',
  'boxplot',
];

const FIELD_IDS = ['field-0', 'field-1', 'field-2'];
// 新契约：id 是列的稳定标识，legacyId 承担旧结构位置 id 的解析职责
const FIELDS = [
  { id: 'col-0', name: 'region', legacyId: 'field-0' },
  { id: 'col-1', name: 'gmv', legacyId: 'field-1' },
  { id: 'col-2', name: 'order_date', legacyId: 'field-2' },
];

const KEYS = [
  '__proto__',
  'constructor',
  'prototype',
  'toString',
  'valueOf',
  '',
  'id',
  'fields',
  'bindings',
  'alias',
  'field',
  'bindingId',
  'version',
  'chartType',
  'title',
  'query',
  'queryConfig',
  'dimensionGroups',
  'metricGroups',
  'filters',
  'sort',
  'limit',
  'fieldMeta',
  'style',
  'queryOptions',
  'chartStyle',
  'chartQueryOptions',
  'dimensionLabels',
  'metricAggregations',
  'metricAliases',
  'metricUnits',
  'metricFormats',
];

/** 定义自有键（绕开 __proto__ 的 setter，制造 JSON.parse 风格的 own key） */
function setOwn(obj: Record<string, unknown>, key: string, value: unknown): void {
  Object.defineProperty(obj, key, { value, enumerable: true, writable: true, configurable: true });
}

function randomStrRecord(rng: Rng): unknown {
  if (rng.bool(0.15)) return rng.json(2);
  const out: Record<string, unknown> = {};
  const n = rng.int(5);
  for (let i = 0; i < n; i++) {
    setOwn(
      out,
      rng.pick([...FIELD_IDS, 'region', 'gmv', 'field-99', '__proto__', 'b-0', '']),
      rng.pick(['sum', 'avg', '别名', '', 1, null, {}, []])
    );
  }
  return out;
}

function randomFieldMeta(rng: Rng): unknown {
  if (rng.bool(0.15)) return rng.json(2);
  const out: Record<string, unknown> = {};
  const n = rng.int(5);
  for (let i = 0; i < n; i++) {
    const meta: Record<string, unknown> = {};
    const m = rng.int(4);
    for (let k = 0; k < m; k++) {
      setOwn(meta, rng.pick(KEYS), rng.pick(['x', '', 1, null, {}, []]));
    }
    setOwn(out, rng.pick([...FIELD_IDS, 'region', 'gmv', '__proto__', 'b-0', 'b-1', '']), meta);
  }
  return out;
}

function randomGroupEntry(rng: Rng, index: number): unknown {
  const mode = rng.int(8);
  if (mode === 0) return rng.json(2);
  if (mode === 1) return null;
  if (mode === 2) return [];
  const g: Record<string, unknown> = {};
  if (rng.bool(0.9))
    setOwn(g, 'id', rng.pick(['', 'dim-group-1', 'x_axis', 'values', '__proto__', `g-${index}`]));
  if (rng.bool(0.6)) {
    setOwn(
      g,
      'fields',
      rng.bool(0.75)
        ? Array.from({ length: rng.int(4) }, () =>
            rng.pick([...FIELD_IDS, 'region', 'gmv', 'field-99', '', '__proto__'])
          )
        : rng.json(2)
    );
  }
  if (rng.bool(0.6)) {
    setOwn(
      g,
      'bindings',
      rng.bool(0.75)
        ? Array.from({ length: rng.int(4) }, () =>
            rng.bool(0.8)
              ? {
                  bindingId: rng.pick(['b-0', 'b-1', 'b-0', '', 'x']),
                  fieldId: rng.pick([...FIELD_IDS, 'region', 'gmv', '']),
                }
              : rng.json(2)
          )
        : rng.json(2)
    );
  }
  if (rng.bool(0.2)) setOwn(g, 'alias', rng.pick(['别名', '', 1, null]));
  return g;
}

function randomGroups(rng: Rng): unknown {
  if (rng.bool(0.15)) return rng.json(2);
  const n = rng.int(5);
  return Array.from({ length: n }, (_, i) => randomGroupEntry(rng, i));
}

function randomQuery(rng: Rng): unknown {
  if (rng.bool(0.1)) return rng.json(2);
  const q: Record<string, unknown> = {};
  if (rng.bool(0.9)) setOwn(q, 'dimensionGroups', randomGroups(rng));
  if (rng.bool(0.9)) setOwn(q, 'metricGroups', randomGroups(rng));
  if (rng.bool(0.7)) {
    setOwn(
      q,
      'filters',
      rng.bool(0.8)
        ? Array.from({ length: rng.int(4) }, () =>
            rng.bool(0.7)
              ? {
                  fieldId: rng.pick([...FIELD_IDS, 'region', 'gmv', '__proto__']),
                  operator: 'eq',
                  value: rng.int(5),
                }
              : rng.json(2)
          )
        : rng.json(2)
    );
  }
  if (rng.bool(0.5)) {
    setOwn(
      q,
      'sort',
      rng.bool(0.8)
        ? {
            [rng.bool() ? 'bindingId' : 'field']: rng.pick(['b-0', 'b-1', 'region', 'gmv', '']),
            order: rng.pick(['asc', 'desc', '', 1, null]),
          }
        : rng.json(2)
    );
  }
  if (rng.bool(0.5))
    setOwn(q, 'limit', rng.pick([0, 100, -5, 1.5, Number.MAX_SAFE_INTEGER, 1e308]));
  return q;
}

function randomDoc(rng: Rng): Record<string, unknown> {
  const doc: Record<string, unknown> = {};
  if (rng.bool(0.9))
    setOwn(doc, 'version', rng.pick([0, 1, 2, 999, -1, 1.5, '1', '2', null, true, {}, []]));
  if (rng.bool(0.8))
    setOwn(doc, 'chartType', rng.pick([...CHART_TYPES, 'sunburst', '', null, 1, {}, []]));
  if (rng.bool(0.7)) setOwn(doc, 'title', rng.pick(['标题', '', 'a'.repeat(500), null, 42]));
  if (rng.bool(0.45)) setOwn(doc, 'query', randomQuery(rng));
  if (rng.bool(0.45)) setOwn(doc, 'queryConfig', randomQuery(rng));
  if (rng.bool(0.35)) setOwn(doc, 'fieldMeta', randomFieldMeta(rng));
  for (const rec of [
    'dimensionLabels',
    'metricAggregations',
    'metricAliases',
    'metricUnits',
    'metricFormats',
  ]) {
    if (rng.bool(0.3)) setOwn(doc, rec, randomStrRecord(rng));
  }
  if (rng.bool(0.2)) setOwn(doc, 'xAxisField', rng.pick([...FIELD_IDS, null, '', 1]));
  if (rng.bool(0.2)) setOwn(doc, 'yAxisFields', rng.bool(0.8) ? [...FIELD_IDS] : rng.json(2));
  if (rng.bool(0.35)) setOwn(doc, 'style', rng.json(2));
  if (rng.bool(0.35)) setOwn(doc, 'chartStyle', rng.json(2));
  if (rng.bool(0.3)) setOwn(doc, 'queryOptions', rng.json(2));
  if (rng.bool(0.3)) setOwn(doc, 'chartQueryOptions', rng.json(2));
  const extra = rng.int(4);
  for (let i = 0; i < extra; i++) setOwn(doc, rng.pick(KEYS), rng.json(2));
  return doc;
}

interface Violation {
  kind: string;
  detail: string;
}

const META_KEYS = ['label', 'aggregation', 'alias', 'unit', 'format'];

/** 迭代式检查：输出中每个普通对象的原型都必须是 Object.prototype（防 __proto__ setter 掉数据） */
function protoViolations(root: unknown): Violation[] {
  const out: Violation[] = [];
  const stack: Array<{ v: unknown; path: string }> = [{ v: root, path: '$' }];
  let steps = 0;
  while (stack.length > 0 && steps < 200000) {
    const { v, path } = stack.pop() as { v: unknown; path: string };
    steps++;
    if (Array.isArray(v)) {
      for (let i = 0; i < v.length; i++) stack.push({ v: v[i], path: `${path}[${i}]` });
      if (Object.getPrototypeOf(v) !== Array.prototype)
        out.push({ kind: 'proto', detail: `${path} 数组原型异常` });
      continue;
    }
    if (v !== null && typeof v === 'object') {
      if (Object.getPrototypeOf(v) !== Object.prototype) {
        out.push({ kind: 'proto', detail: `${path} 原型被替换为 ${j(Object.getPrototypeOf(v))}` });
      }
      for (const [k, val] of Object.entries(v)) stack.push({ v: val, path: `${path}.${k}` });
    }
  }
  return out;
}

function deepEqOwn(a: unknown, b: unknown, path = '$'): string | null {
  if (a === b) return null;
  if (typeof a !== typeof b) return `${path}: 类型不同 ${typeof a} vs ${typeof b}`;
  if (a === null || b === null) return a === b ? null : `${path}: ${j(a)} vs ${j(b)}`;
  if (typeof a !== 'object') return `${path}: ${j(a)} vs ${j(b)}`;
  if (Array.isArray(a) !== Array.isArray(b)) return `${path}: 数组/对象不同`;
  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return `${path}: 长度 ${a.length} vs ${b.length}`;
    for (let i = 0; i < a.length; i++) {
      const r = deepEqOwn(a[i], b[i], `${path}[${i}]`);
      if (r) return r;
    }
    return null;
  }
  const ka = Object.keys(a as object);
  const kb = Object.keys(b as object);
  const sa = new Set(ka);
  for (const k of kb) if (!sa.has(k)) return `${path}: 期望缺键 ${k}`;
  const sb = new Set(kb);
  for (const k of ka) if (!sb.has(k)) return `${path}: 多余键 ${k}`;
  for (const k of ka) {
    const r = deepEqOwn(
      (a as Record<string, unknown>)[k],
      (b as Record<string, unknown>)[k],
      `${path}.${k}`
    );
    if (r) return r;
  }
  return null;
}

function checkRaw(
  raw: string,
  fallback: ChartType,
  fields?: { id: string; name: string; legacyId?: string }[]
): Violation[] {
  const v: Violation[] = [];
  let out: ReturnType<typeof migrateChartConfig>;
  try {
    out = migrateChartConfig(raw, fallback, fields);
  } catch (err) {
    return [{ kind: 'throw', detail: `迁移抛异常: ${String(err)}` }];
  }
  if (out.version !== 2) v.push({ kind: 'version', detail: `version=${j(out.version)}` });
  if (!CHART_TYPES.includes(out.chartType))
    v.push({ kind: 'chartType', detail: `chartType=${j(out.chartType)}` });
  if (typeof out.title !== 'string') v.push({ kind: 'title', detail: `title=${j(out.title)}` });
  for (const key of ['dimensionGroups', 'metricGroups', 'filters'] as const) {
    if (!Array.isArray(out.query[key]))
      v.push({ kind: 'shape', detail: `query.${key} 非数组: ${j(out.query[key])}` });
  }
  if (out.query.limit !== undefined && typeof out.query.limit !== 'number')
    v.push({ kind: 'limit', detail: `limit=${j(out.query.limit)}` });
  if (out.query.sort !== undefined) {
    const s = out.query.sort as { bindingId?: unknown; order?: unknown };
    if (typeof s.bindingId !== 'string' || s.bindingId === '' || typeof s.order !== 'string')
      v.push({ kind: 'sort', detail: `sort=${j(s)}` });
  }
  if (out.fieldMeta === null || typeof out.fieldMeta !== 'object' || Array.isArray(out.fieldMeta))
    v.push({ kind: 'fieldMeta', detail: `fieldMeta=${j(out.fieldMeta)}` });

  const allBindings: Array<{ bindingId: string; fieldId: string }> = [];
  for (const gkey of ['dimensionGroups', 'metricGroups'] as const) {
    for (const g of out.query[gkey]) {
      if (typeof g.id !== 'string' || g.id === '')
        v.push({ kind: 'groupId', detail: `组 id=${j(g.id)}` });
      for (const b of g.bindings) {
        if (
          !b ||
          typeof b.bindingId !== 'string' ||
          b.bindingId === '' ||
          typeof b.fieldId !== 'string' ||
          b.fieldId === ''
        ) {
          v.push({ kind: 'binding', detail: `binding=${j(b)}` });
          continue;
        }
        allBindings.push(b);
      }
    }
  }

  let inputVersion: unknown;
  try {
    inputVersion = (JSON.parse(raw) as { version?: unknown } | null)?.version;
  } catch {
    inputVersion = undefined;
  }

  if (fields && inputVersion !== 2) {
    // 只有"输入里存在可解析映射却没解析"才算违规；已知不可解析的位置 id 是文档化的有损路径
    for (const b of allBindings) {
      if (/^field-\d+$/.test(b.fieldId) && fields.some((f) => f.legacyId === b.fieldId)) {
        v.push({ kind: 'positional', detail: `字段仍是位置 id: ${j(b)}` });
      }
    }
  }

  // 幂等：再迁移一次必须逐键相等
  let second: ReturnType<typeof migrateChartConfig> | undefined;
  try {
    second = migrateChartConfig(JSON.stringify(out), fallback, fields);
  } catch (err) {
    v.push({ kind: 'throw2', detail: `二次迁移抛异常: ${String(err)}` });
  }
  if (second) {
    const diff = deepEqOwn(out, second);
    if (diff) v.push({ kind: 'idempotent', detail: diff });
  }

  // 原型污染 / 数据被 setter 吞掉
  v.push(...protoViolations(out));

  // 元数据值必须是字符串
  if (out.fieldMeta && typeof out.fieldMeta === 'object') {
    for (const [k, meta] of Object.entries(out.fieldMeta)) {
      if (meta === null || typeof meta !== 'object') {
        v.push({ kind: 'metaShape', detail: `fieldMeta[${k}]=${j(meta)}` });
        continue;
      }
      for (const [mk, mv] of Object.entries(meta)) {
        if (!META_KEYS.includes(mk) || typeof mv !== 'string')
          v.push({ kind: 'metaShape', detail: `fieldMeta[${k}].${mk}=${j(mv)}` });
      }
    }
  }
  return v;
}

describe('chaos · chartConfigSchema 迁移属性', () => {
  it('seed 1..400 随机文档：不抛 / 幂等 / version=2 / 字段为稳定列名 / 无原型污染', () => {
    const failures: Array<{ seed: number; raw: string; violations: Violation[] }> = [];
    const idempCategories = new Map<string, { seed: number; raw: string; diff: string }>();
    const bindingSamples = new Map<string, { seed: number; raw: string; detail: string }>();
    let cases = 0;
    let positionalLeaks = 0;
    const kindCount: Record<string, number> = {};

    for (let seed = 1; seed <= 400; seed++) {
      const rng = new Rng(seed);
      const docs = [randomDoc(rng), randomDoc(rng), randomDoc(rng)];
      for (const doc of docs) {
        const raw = JSON.stringify(doc) ?? 'null';
        cases++;
        const violations = checkRaw(raw, rng.pick(CHART_TYPES), FIELDS);
        for (const viol of violations) kindCount[viol.kind] = (kindCount[viol.kind] ?? 0) + 1;
        for (const viol of violations) {
          if (viol.kind === 'idempotent') {
            const cat = viol.detail.replace(/\[\d+\]/g, '[]').replace(/\$\.[A-Za-z]+/g, '$');
            if (!idempCategories.has(cat) && idempCategories.size < 20) {
              idempCategories.set(cat, { seed, raw, diff: viol.detail });
            }
          }
          if (
            viol.kind === 'binding' &&
            !bindingSamples.has(viol.detail) &&
            bindingSamples.size < 10
          ) {
            bindingSamples.set(viol.detail, { seed, raw, detail: viol.detail });
          }
        }
        // 位置 id 泄漏统计（非违规，仅暴露面）
        const out = migrateChartConfig(raw, 'bar', FIELDS);
        for (const g of [...out.query.dimensionGroups, ...out.query.metricGroups]) {
          for (const b of g.bindings) if (/^field-\d+$/.test(b.fieldId)) positionalLeaks++;
        }
        if (violations.length > 0 && failures.length < 12) {
          failures.push({ seed, raw, violations });
        }
      }
    }

    // eslint-disable-next-line no-console
    console.log(
      '[T1] 随机文档用例数:',
      cases,
      '违规分类计数:',
      JSON.stringify(kindCount),
      '无法解析的位置 id 保留次数:',
      positionalLeaks
    );
    // eslint-disable-next-line no-console
    console.log(
      '[T1] 幂等差异类别:',
      JSON.stringify([...idempCategories].map(([cat, s]) => ({ cat, seed: s.seed, diff: s.diff })))
    );
    // eslint-disable-next-line no-console
    console.log(
      '[T1] binding 形状违规样例:',
      JSON.stringify([...bindingSamples.values()].map((s) => ({ seed: s.seed, detail: s.detail })))
    );

    if (failures.length > 0) {
      const first = failures[0];
      const target = first.violations[0].kind;
      const shrunk = shrinkRaw(first.raw, (r) =>
        checkRaw(r, 'table', FIELDS).some((x) => x.kind === target)
      );
      const shrunkViolations = checkRaw(shrunk, 'table', FIELDS);
      expect({
        seed: first.seed,
        violationKind: target,
        minimalInput: shrunk,
        minimalViolations: shrunkViolations,
        allSeeds: failures.map((f) => f.seed),
        classified: kindCount,
      }).toEqual('无违规');
    }
    expect(failures).toHaveLength(0);
  });

  it('最小复现 A（已修复）：v1 空字符串字段被丢弃，不再产出非法 binding，迁移幂等', () => {
    const raw = JSON.stringify({ version: 1, query: { metricGroups: [{ fields: [''] }] } });
    const first = migrateChartConfig(raw, 'bar');
    const second = migrateChartConfig(JSON.stringify(first), 'bar');
    // eslint-disable-next-line no-console
    console.log(
      '[T1-A] 一次迁移 bindings:',
      JSON.stringify(first.query.metricGroups[0].bindings),
      '二次迁移 bindings:',
      JSON.stringify(second.query.metricGroups[0].bindings)
    );
    // 修复前：空字段生成 {bindingId:'b-0',fieldId:''}，二次迁移被 isBindingInstance 丢弃 → 非幂等。
    // 修复后：空字段在建组阶段即被过滤，binding 为空且二次迁移相等。
    expect(first.query.metricGroups[0].bindings).toEqual([]);
    expect(second).toEqual(first);
  });

  it('最小复现 B：字段名命中 Object.prototype 成员时凭空多出 fieldMeta 空条目 → 非幂等', () => {
    for (const name of ['constructor', 'toString', 'valueOf', 'hasOwnProperty', '__proto__']) {
      const raw = JSON.stringify({ version: 1, query: { metricGroups: [{ fields: [name] }] } });
      const first = migrateChartConfig(raw, 'bar');
      const second = migrateChartConfig(JSON.stringify(first), 'bar');
      // eslint-disable-next-line no-console
      console.log(
        `[T1-B] field=${name} → fieldMeta:`,
        JSON.stringify(first.fieldMeta),
        '二次 fieldMeta:',
        JSON.stringify(second.fieldMeta),
        '幂等:',
        JSON.stringify(second) === JSON.stringify(first)
      );
      expect(Object.keys(first.fieldMeta)).toEqual([]);
    }
  });

  it('最小复现 C：v2 直通不解析位置 id——带 fields 也无法修复已腐蚀的 v2 文档', () => {
    const raw = JSON.stringify({
      version: 2,
      chartType: 'bar',
      query: {
        dimensionGroups: [{ id: 'x_axis', bindings: [{ bindingId: 'b-0', fieldId: 'field-0' }] }],
        metricGroups: [],
        filters: [],
      },
      fieldMeta: {},
      style: {},
      queryOptions: {},
    });
    const doc = migrateChartConfig(raw, 'bar', FIELDS);
    // eslint-disable-next-line no-console
    console.log(
      '[T1-C] v2 位置 id + fields 参数 → 输出字段:',
      JSON.stringify(doc.query.dimensionGroups[0].bindings)
    );
    expect(doc.query.dimensionGroups[0].bindings[0].fieldId).toBe('field-0');
  });

  it('无 fields 参数：旧位置 id 原样保留（有损路径）——统计暴露面', () => {
    const raw = JSON.stringify({
      queryConfig: {
        dimensionGroups: [{ fields: ['field-0'] }],
        metricGroups: [{ fields: ['field-1'] }],
      },
    });
    const doc = migrateChartConfig(raw, 'bar');
    const fields = [...doc.query.dimensionGroups, ...doc.query.metricGroups].flatMap((g) =>
      g.bindings.map((b) => b.fieldId)
    );
    // eslint-disable-next-line no-console
    console.log('[T1] 无 fields 时输出字段:', JSON.stringify(fields), 'version:', doc.version);
    expect(fields).toEqual(['field-0', 'field-1']);
    // 二次迁移（v2 直通）不会修复它
    const again = migrateChartConfig(JSON.stringify(doc), 'bar');
    expect(again.query.metricGroups[0].bindings[0].fieldId).toBe('field-1');
  });

  it('fieldMeta 键为 __proto__ 时元数据必须仍是自有键（不得被 setter 吞掉）', () => {
    const raw = JSON.stringify({
      version: 2,
      chartType: 'bar',
      query: { dimensionGroups: [], metricGroups: [], filters: [] },
      fieldMeta: JSON.parse('{"__proto__":{"label":"地区"}}'),
      style: {},
      queryOptions: {},
    });
    const doc = migrateChartConfig(raw, 'bar');
    // 判自有键不能用 Object.hasOwn（tsc lib=ES2020 报 TS2550），也不能写
    // hasOwnProperty.call（biome --write 会重写成前者）——与 chartConfigSchema.hasOwn 同源。
    const own = Object.getOwnPropertyDescriptor(doc.fieldMeta, '__proto__') !== undefined;
    const roundTripped = JSON.parse(JSON.stringify(doc.fieldMeta));
    // eslint-disable-next-line no-console
    console.log('[T1] __proto__ 自有键:', own, '序列化后:', JSON.stringify(roundTripped));
    // 修复后 __proto__ 必须作为自有键保留（此前 `fieldMeta[name]=` 会被 setter 吞成原型）。
    // 期望对象同样用 setOwn 制造自有 '__proto__' 键——代码字面量 { __proto__: x } 走的是原型 setter，
    // 编码的是"被吞"旧行为，故不能用它表达修复后的正确结果。
    const expected: Record<string, unknown> = {};
    setOwn(expected, '__proto__', { label: '地区' });
    expect(own).toBe(true);
    expect(roundTripped).toEqual(expected);
  });

  it('压力：1000 层嵌套 / 2 万元素组数组 / 1MB 标题字符串 不抛异常', () => {
    // 1000 层嵌套 style
    let nested = '{}';
    for (let i = 0; i < 1000; i++) nested = `{"style":${nested}}`;
    const deep = `{"version":2,"chartType":"bar","query":{"dimensionGroups":[],"metricGroups":[],"filters":[]},"fieldMeta":{},"style":${nested}}`;
    expect(() => migrateChartConfig(deep, 'bar')).not.toThrow();

    // 2 万元素组数组（v1 路径，逐个分配 bindingId）
    const groups = Array.from({ length: 20000 }, (_, i) => ({
      id: `g-${i}`,
      fields: [`field-${i % 300}`],
    }));
    const big = JSON.stringify({
      version: 1,
      chartType: 'table',
      query: { dimensionGroups: groups },
    });
    const t0 = Date.now();
    const bigDoc = migrateChartConfig(big, 'table');
    const ms = Date.now() - t0;
    const bindings = bigDoc.query.dimensionGroups.flatMap((g) => g.bindings);
    // eslint-disable-next-line no-console
    console.log('[T1] 2 万组迁移耗时(ms):', ms, '产出 bindings:', bindings.length);
    expect(bindings.length).toBe(20000);
    expect(new Set(bindings.map((b) => b.bindingId)).size).toBe(20000);

    // 1MB 标题
    const huge = JSON.stringify({ version: 2, chartType: 'bar', title: 'x'.repeat(1024 * 1024) });
    const hugeDoc = migrateChartConfig(huge, 'bar');
    expect(hugeDoc.title.length).toBe(1024 * 1024);
  });

  it('v2 直通遭遇重复 bindingId：迁移不修复（记录为已知脆弱点）', () => {
    const raw = JSON.stringify({
      version: 2,
      chartType: 'bar',
      query: {
        dimensionGroups: [{ id: 'x_axis', bindings: [{ bindingId: 'b-0', fieldId: 'region' }] }],
        metricGroups: [{ id: 'values', bindings: [{ bindingId: 'b-0', fieldId: 'gmv' }] }],
        filters: [],
      },
      fieldMeta: { 'b-0': { aggregation: 'sum' } },
      style: {},
      queryOptions: {},
    });
    const doc = migrateChartConfig(raw, 'bar');
    const ids = [...doc.query.dimensionGroups, ...doc.query.metricGroups].flatMap((g) =>
      g.bindings.map((b) => b.bindingId)
    );
    // eslint-disable-next-line no-console
    console.log(
      '[T1] v2 重复 bindingId 输入 → 输出 id:',
      JSON.stringify(ids),
      'fieldMeta:',
      JSON.stringify(doc.fieldMeta)
    );
    expect(new Set(ids).size).toBeLessThan(ids.length);
  });
});
