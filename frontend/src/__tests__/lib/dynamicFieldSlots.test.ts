import { describe, expect, it } from 'vitest';
import {
  applyFieldOverrides,
  dynamicSlotsOf,
  effectiveFieldId,
  fieldOverridesOf,
  pruneSelections,
} from '@/lib/dynamicFieldSlots';
import type { BindingInstance, FieldGroup } from '@/store';

const fields = [
  { id: 'd-1', name: 'region' },
  { id: 'd-2', name: 'city' },
  { id: 'd-3', name: 'channel' },
];

const group = (bindings: BindingInstance[]): FieldGroup => ({ id: 'g', bindings });

describe('effectiveFieldId：会话选择只在候选内生效', () => {
  const binding: BindingInstance = { bindingId: 'b-0', fieldId: 'd-1', candidates: ['d-2'] };

  it('没有选择 / 选择默认列 → 默认列', () => {
    expect(effectiveFieldId(binding, undefined)).toBe('d-1');
    expect(effectiveFieldId(binding, {})).toBe('d-1');
    expect(effectiveFieldId(binding, { 'b-0': 'd-1' })).toBe('d-1');
  });

  it('选择候选列 → 候选列', () => {
    expect(effectiveFieldId(binding, { 'b-0': 'd-2' })).toBe('d-2');
  });

  it('选择非候选（过期/篡改）→ 回落默认列', () => {
    expect(effectiveFieldId(binding, { 'b-0': 'd-3' })).toBe('d-1');
    expect(effectiveFieldId({ bindingId: 'b-1', fieldId: 'd-1' }, { 'b-1': 'd-2' })).toBe('d-1');
  });
});

describe('dynamicSlotsOf：候选投影', () => {
  it('没有候选集 / 候选列已不在数据集 → 不产生槽位', () => {
    expect(dynamicSlotsOf([group([{ bindingId: 'b-0', fieldId: 'd-1' }])], fields)).toEqual([]);
    expect(
      dynamicSlotsOf([group([{ bindingId: 'b-0', fieldId: 'd-1', candidates: ['gone'] }])], fields)
    ).toEqual([]);
  });

  it('选项 = 默认列 + 有效候选（按配置顺序），标签可自定义', () => {
    const slots = dynamicSlotsOf(
      [group([{ bindingId: 'b-0', fieldId: 'd-1', candidates: ['d-3', 'gone', 'd-2'] }])],
      fields,
      () => '地区'
    );
    expect(slots).toEqual([
      {
        bindingId: 'b-0',
        label: '地区',
        defaultFieldId: 'd-1',
        options: [
          { id: 'd-1', name: 'region' },
          { id: 'd-3', name: 'channel' },
          { id: 'd-2', name: 'city' },
        ],
      },
    ]);
  });

  it('标签缺省时回落默认列名', () => {
    const slots = dynamicSlotsOf(
      [group([{ bindingId: 'b-0', fieldId: 'd-1', candidates: ['d-2'] }])],
      fields
    );
    expect(slots[0].label).toBe('region');
  });
});

describe('fieldOverridesOf / pruneSelections', () => {
  const slots = dynamicSlotsOf(
    [group([{ bindingId: 'b-0', fieldId: 'd-1', candidates: ['d-2'] }])],
    fields
  );

  it('只收录真正切换过的槽位', () => {
    expect(fieldOverridesOf(slots, {})).toEqual({});
    expect(fieldOverridesOf(slots, { 'b-0': 'd-1' })).toEqual({});
    expect(fieldOverridesOf(slots, { 'b-0': 'd-2' })).toEqual({ 'b-0': 'd-2' });
  });

  it('越界选择不产出覆盖（防止把过期列发进查询）', () => {
    expect(fieldOverridesOf(slots, { 'b-0': 'd-3' })).toEqual({});
  });

  it('pruneSelections：全有效时返回原引用，失效键被剔除', () => {
    const valid = { 'b-0': 'd-2' };
    expect(pruneSelections(valid, slots)).toBe(valid);
    expect(pruneSelections({ 'b-0': 'd-2', 'b-9': 'd-2' }, slots)).toEqual({ 'b-0': 'd-2' });
    expect(pruneSelections({ 'b-0': 'gone' }, slots)).toEqual({});
  });
});

describe('applyFieldOverrides：查询记录/分享快照物化', () => {
  it('空覆盖返回原引用（请求形状逐字节不变）', () => {
    const config = { dimensionGroups: [group([])], metricGroups: [group([])], limit: 5 };
    expect(applyFieldOverrides(config, {})).toBe(config);
  });

  it('替换 bindings 的 fieldId，并把原默认列补进候选集（还原后仍可来回切）', () => {
    const config = {
      dimensionGroups: [group([{ bindingId: 'b-0', fieldId: 'd-1', candidates: ['d-2'] }])],
      metricGroups: [],
      limit: 10,
    };
    const next = applyFieldOverrides(config, { 'b-0': 'd-2' });
    expect(next.dimensionGroups[0].bindings[0].fieldId).toBe('d-2');
    expect(next.dimensionGroups[0].bindings[0].candidates).toEqual(['d-1']);
    expect(next.limit).toBe(10);
    // 原对象不被改写（会话选择不落库）
    expect(config.dimensionGroups[0].bindings[0].fieldId).toBe('d-1');
  });
});
