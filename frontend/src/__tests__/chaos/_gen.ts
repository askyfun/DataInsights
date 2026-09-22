/* 混沌测试共享工具（2026-09-20 转正为常驻）：带种子的 PRNG + 随机 JSON 生成器 + JSON 级最小化器。
 * 禁安装依赖，故手写 PRNG。 */

/** mulberry32：32 位种子 → [0,1) 均匀随机 */
export function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export class Rng {
  readonly seed: number;
  private next: () => number;

  constructor(seed: number) {
    this.seed = seed;
    this.next = mulberry32(seed);
  }

  float(): number {
    return this.next();
  }

  int(maxExclusive: number): number {
    if (maxExclusive <= 0) return 0;
    return Math.floor(this.next() * maxExclusive);
  }

  intRange(min: number, max: number): number {
    return min + this.int(max - min + 1);
  }

  bool(p = 0.5): boolean {
    return this.next() < p;
  }

  pick<T>(arr: readonly T[]): T {
    return arr[this.int(arr.length)];
  }

  /** 递归随机 JSON 值（保证 JSON.stringify 后仍是同样的值：无 undefined/NaN/Symbol） */
  json(depth = 3): unknown {
    const choices = depth <= 0 ? 8 : 12;
    switch (this.int(choices)) {
      case 0:
        return null;
      case 1:
        return this.bool();
      case 2:
        return this.int(1000);
      case 3:
        return this.pick([
          0,
          -0,
          1e308,
          -1e308,
          1e-308,
          0.1 + 0.2,
          Number.MAX_SAFE_INTEGER + 1,
          2 ** 53,
          1 / 3,
        ]);
      case 4:
        return this.pick([
          '',
          ' ',
          'x',
          'field-0',
          'field-99',
          'region',
          'gmv',
          '__proto__',
          'constructor',
          'prototype',
          'a'.repeat(200),
          '\u0000',
          '{"a":1}',
          'null',
          'NaN',
          'Infinity',
        ]);
      case 5:
        return this.pick([
          '__proto__',
          'constructor',
          'prototype',
          'toString',
          'valueOf',
          'hasOwnProperty',
          'length',
          '',
          'id',
          'fields',
          'bindings',
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
          'alias',
          'label',
          'aggregation',
          'unit',
          'format',
          'order',
          'name',
          'value',
        ]);
      default: {
        if (this.bool(0.45)) {
          const n = this.int(depth <= 1 ? 3 : 6);
          const arr: unknown[] = [];
          for (let i = 0; i < n; i++) arr.push(this.json(depth - 1));
          return arr;
        }
        const n = this.int(depth <= 1 ? 3 : 6);
        const obj: Record<string, unknown> = {};
        for (let i = 0; i < n; i++) {
          obj[this.pick(RNG_KEYS)] = this.json(depth - 1);
        }
        return obj;
      }
    }
  }
}

const RNG_KEYS: readonly string[] = [
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

/** 打印用：保持可粘贴的紧凑 JSON */
export function j(value: unknown): string {
  try {
    return JSON.stringify(value) ?? String(value);
  } catch {
    return '<unstringifiable>';
  }
}

/** 生成 candidates：把当前 JSON 字符串简化一档（删键/截数组/换小值），全部仍是合法 JSON */
function candidates(raw: string): string[] {
  let node: unknown;
  try {
    node = JSON.parse(raw);
  } catch {
    return [];
  }
  const out: string[] = [];
  const emit = (v: unknown) => {
    try {
      out.push(JSON.stringify(v));
    } catch {
      /* ignore */
    }
  };

  const walk = (cur: unknown, depth: number): void => {
    if (depth > 4) return;
    if (Array.isArray(cur)) {
      if (cur.length === 0) return;
      // 去掉单个元素
      for (let i = 0; i < cur.length && i < 8; i++) {
        const copy = cur.slice();
        copy.splice(i, 1);
        emit(copy);
      }
      // 折半
      if (cur.length > 1) {
        emit(cur.slice(0, Math.floor(cur.length / 2)));
        emit(cur.slice(Math.floor(cur.length / 2)));
        emit(cur.slice(0, 1));
      }
      // 递归简化元素
      for (let i = 0; i < cur.length && i < 4; i++) {
        const before = JSON.stringify(cur[i]);
        const simple = simplifyValue(cur[i]);
        if (simple !== undefined && JSON.stringify(simple) !== before) {
          const copy = cur.slice();
          copy[i] = simple;
          emit(copy);
        }
        walk(cur[i], depth + 1);
      }
      return;
    }
    if (cur && typeof cur === 'object') {
      const keys = Object.keys(cur as Record<string, unknown>);
      for (const k of keys) {
        const copy: Record<string, unknown> = {};
        for (const kk of keys) if (kk !== k) copy[kk] = (cur as Record<string, unknown>)[kk];
        emit(copy);
      }
      // 保序删前半/后半
      if (keys.length > 2) {
        const half = keys.slice(0, Math.floor(keys.length / 2));
        const rest = keys.slice(Math.floor(keys.length / 2));
        for (const subset of [half, rest]) {
          const copy: Record<string, unknown> = {};
          for (const k of subset) copy[k] = (cur as Record<string, unknown>)[k];
          emit(copy);
        }
      }
      for (const k of keys) {
        const v = (cur as Record<string, unknown>)[k];
        const simple = simplifyValue(v);
        if (simple !== undefined) {
          const copy: Record<string, unknown> = {};
          for (const kk of keys) copy[kk] = (cur as Record<string, unknown>)[kk];
          copy[k] = simple;
          emit(copy);
        }
        walk(v, depth + 1);
      }
    }
  };

  walk(node, 0);
  return out.filter((c) => c !== raw);
}

function simplifyValue(v: unknown): unknown {
  if (v === null) return undefined;
  if (Array.isArray(v)) return v.length === 0 ? undefined : [];
  if (typeof v === 'object') return Object.keys(v as object).length === 0 ? undefined : {};
  if (typeof v === 'string') return v === '' ? undefined : '';
  if (typeof v === 'number') return v === 0 ? undefined : 0;
  if (typeof v === 'boolean') return v === false ? undefined : false;
  return undefined;
}

/**
 * JSON 级贪心最小化：不断尝试更短且仍满足 fails 的输入。
 * 返回最小输入字符串（保证 fails 为真）。
 */
export function shrinkRaw(raw: string, fails: (raw: string) => boolean, maxRounds = 200): string {
  let best = raw;
  let improved = true;
  let rounds = 0;
  while (improved && rounds < maxRounds) {
    improved = false;
    rounds++;
    for (const c of candidates(best)) {
      if (c.length >= best.length) continue;
      if (fails(c)) {
        best = c;
        improved = true;
        break;
      }
    }
  }
  return best;
}
