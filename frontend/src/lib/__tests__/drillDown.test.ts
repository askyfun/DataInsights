/**
 * 维度层级下钻纯逻辑单测（issue #136）。
 *
 * 覆盖：drillNextId / drillParentId（层级查找）、applyDrillDown（下钻换列 + 追加
 * 父级过滤）、applyDrillUp（上钻换列 + 精确移除下钻过滤）、isDrilledDown（判定）。
 * 关键不变量：下钻/上钻不修改原对象（返回新副本）；同列重复下钻替换而非叠加条件；
 * 上钻只移除由下钻产生的 drill-<parentId> 条件，不误删用户自配过滤。
 */
import { describe, expect, it } from 'vitest';
import type { ChartField, QueryConfig } from '../../store';
import {
  applyDrillDown,
  applyDrillUp,
  DRILL_FILTER_PREFIX,
  drillNextId,
  drillParentId,
  isDrilledDown,
} from '../drillDown';

/** 构造一个字段；hierarchy 可省。 */
function field(
  id: string,
  name: string,
  hierarchy?: { parentId?: string; nextId?: string }
): ChartField {
  return { id, name, type: 'dimension', dataType: 'string', hierarchy };
}

const province = field('col-prov', '省份', { nextId: 'col-city' });
const city = field('col-city', '城市', { parentId: 'col-prov', nextId: 'col-district' });
const district = field('col-district', '区县', { parentId: 'col-city' });
const standalone = field('col-x', '无层级列');

/** 一份最小 QueryConfig：单个维度槽位 + 一条用户自配过滤。 */
function baseConfig(): QueryConfig {
  return {
    dimensionGroups: [{ id: 'g-0', bindings: [{ bindingId: 'b-0', fieldId: 'col-prov' }] }],
    metricGroups: [{ id: 'g-1', bindings: [{ bindingId: 'b-1', fieldId: 'col-metric' }] }],
    filters: [{ id: 'user-filter', fieldId: 'col-x', operator: 'eq', value: 'keep', logic: 'and' }],
  };
}

describe('drillNextId / drillParentId', () => {
  it('返回层级下一级/上一级列 id', () => {
    expect(drillNextId(province)).toBe('col-city');
    expect(drillParentId(district)).toBe('col-city');
  });

  it('无层级或叶子返回 undefined', () => {
    expect(drillNextId(standalone)).toBeUndefined();
    expect(drillParentId(province)).toBeUndefined(); // 根无上一级
    expect(drillNextId(district)).toBeUndefined(); // 叶子无下一级
    expect(drillNextId(undefined)).toBeUndefined();
    expect(drillParentId(undefined)).toBeUndefined();
  });
});

describe('applyDrillDown', () => {
  it('把维度换成层级下一级列 + 追加父级列过滤', () => {
    const cfg = baseConfig();
    const next = applyDrillDown(cfg, 'b-0', province, '广东', 'col-city');

    expect(next).not.toBe(cfg);
    expect(next.dimensionGroups[0].bindings[0].fieldId).toBe('col-city');
    // 下钻过滤：fieldId 是父级列 id，值=点击值，id 带 drill- 前缀
    const drill = next.filters.find((f) => f.id.startsWith(DRILL_FILTER_PREFIX));
    expect(drill).toBeDefined();
    expect(drill?.fieldId).toBe('col-prov');
    expect(drill?.operator).toBe('eq');
    expect(drill?.value).toBe('广东');
    // 用户自配过滤保留
    expect(next.filters.find((f) => f.id === 'user-filter')).toBeDefined();
    // 原对象未被改动
    expect(cfg.dimensionGroups[0].bindings[0].fieldId).toBe('col-prov');
    expect(cfg.filters).toHaveLength(1);
  });

  it('同父级列重复下钻替换条件，不叠加', () => {
    const cfg = baseConfig();
    const first = applyDrillDown(cfg, 'b-0', province, '广东', 'col-city');
    const second = applyDrillDown(first, 'b-0', province, '广西', 'col-city');

    const drillFilters = second.filters.filter((f) => f.id.startsWith(DRILL_FILTER_PREFIX));
    expect(drillFilters).toHaveLength(1);
    expect(drillFilters[0].value).toBe('广西');
  });
});

describe('applyDrillUp', () => {
  it('把维度换回上一级列 + 移除下钻过滤', () => {
    const cfg = applyDrillDown(baseConfig(), 'b-0', province, '广东', 'col-city');
    const up = applyDrillUp(cfg, 'b-0', 'col-prov');

    expect(up).not.toBe(cfg);
    expect(up.dimensionGroups[0].bindings[0].fieldId).toBe('col-prov');
    expect(up.filters.find((f) => f.id.startsWith(DRILL_FILTER_PREFIX))).toBeUndefined();
    // 用户自配过滤不受影响
    expect(up.filters.find((f) => f.id === 'user-filter')).toBeDefined();
  });

  it('多级下钻：drill-<父级id> 只移除对应层级条件', () => {
    let cfg = applyDrillDown(baseConfig(), 'b-0', province, '广东', 'col-city');
    // 再下钻一级（此时维度是 city）
    cfg = applyDrillDown(cfg, 'b-0', city, '深圳', 'col-district');
    expect(cfg.filters.filter((f) => f.id.startsWith(DRILL_FILTER_PREFIX))).toHaveLength(2);

    const up = applyDrillUp(cfg, 'b-0', 'col-city');
    const remaining = up.filters.filter((f) => f.id.startsWith(DRILL_FILTER_PREFIX));
    expect(remaining).toHaveLength(1);
    expect(remaining[0].fieldId).toBe('col-prov'); // 只清了 col-city 那条
    expect(up.dimensionGroups[0].bindings[0].fieldId).toBe('col-city');
  });

  it('未命中绑定时原样返回', () => {
    const cfg = applyDrillDown(baseConfig(), 'b-0', province, '广东', 'col-city');
    const up = applyDrillUp(cfg, 'nonexistent', 'col-prov');
    expect(up).toBe(cfg);
  });
});

describe('isDrilledDown', () => {
  const fields = [province, city, district, standalone];

  it('维度列带 parentId 时视为已下钻', () => {
    const cfg = baseConfig();
    const down = applyDrillDown(cfg, 'b-0', province, '广东', 'col-city');
    expect(isDrilledDown(down, fields)).toBe(true);
    expect(isDrilledDown(cfg, fields)).toBe(false);
  });

  it('无层级列不触发下钻判定', () => {
    const cfg: QueryConfig = {
      ...baseConfig(),
      dimensionGroups: [{ id: 'g-0', bindings: [{ bindingId: 'b-0', fieldId: 'col-x' }] }],
    };
    expect(isDrilledDown(cfg, fields)).toBe(false);
  });
});
