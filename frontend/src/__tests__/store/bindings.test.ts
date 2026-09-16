import { beforeEach, describe, expect, it } from 'vitest';
import { type BindingInstance, nextBindingId, reconcileGroupBindings, useStore } from '@/store';

/**
 * bindingId 生成与 QueryPanel Select diff 的纯函数测试。
 *
 * nextBindingId：全局唯一、顺序递增，删除中间 binding 后不复用旧号。
 * reconcileGroupBindings：QueryPanel 多选 Select 变更时，保留仍选中列名的现有
 * bindingId（避免按 bindingId 存的 aggregation/alias 丢失），只为新增列名生成新号，
 * 移除取消选中的 binding，输出顺序跟随 Select 的 values。
 */

const b = (bindingId: string, field: string): BindingInstance => ({ bindingId, field });

describe('nextBindingId', () => {
  it('空视野返回 b-0', () => {
    expect(nextBindingId([])).toBe('b-0');
    expect(nextBindingId([[], []])).toBe('b-0');
  });

  it('返回组内最大序号 +1', () => {
    expect(nextBindingId([[b('b-0', 'a'), b('b-1', 'b')]])).toBe('b-2');
  });

  it('跨多个组扫描全局最大号', () => {
    expect(nextBindingId([[b('b-0', 'a')], [b('b-5', 'b')], [b('b-2', 'c')]])).toBe('b-6');
  });

  it('删除中间 binding 后新增不复用被删的号', () => {
    // 曾有 b-0/b-1/b-2，删掉 b-1 后剩 b-0/b-2 → 下一个是 b-3（不复用 b-1）
    expect(nextBindingId([[b('b-0', 'a'), b('b-2', 'c')]])).toBe('b-3');
  });

  it('忽略不符合 b-N 形式的 id（健壮性）', () => {
    expect(nextBindingId([[b('weird', 'a'), b('b-1', 'b')]])).toBe('b-2');
  });
});

describe('reconcileGroupBindings', () => {
  it('保留仍被选中列名的现有 bindingId', () => {
    const group = [b('b-0', 'a'), b('b-1', 'b')];
    expect(reconcileGroupBindings(group, ['a', 'b'], [group])).toEqual([
      b('b-0', 'a'),
      b('b-1', 'b'),
    ]);
  });

  it('只为新增列名生成新号，且不与其它组已占用的号冲突', () => {
    const group = [b('b-0', 'a')];
    const other = [b('b-1', 'x')]; // 另一组已占用 b-1
    expect(reconcileGroupBindings(group, ['a', 'c'], [group, other])).toEqual([
      b('b-0', 'a'),
      b('b-2', 'c'),
    ]);
  });

  it('移除被取消选中的列名对应 binding', () => {
    const group = [b('b-0', 'a'), b('b-1', 'b')];
    expect(reconcileGroupBindings(group, ['b'], [group])).toEqual([b('b-1', 'b')]);
  });

  it('输出顺序跟随 selectedFields（重排保留各自 bindingId）', () => {
    const group = [b('b-0', 'a'), b('b-1', 'b')];
    expect(reconcileGroupBindings(group, ['b', 'a'], [group])).toEqual([
      b('b-1', 'b'),
      b('b-0', 'a'),
    ]);
  });

  it('一次新增多个列名时号互不冲突', () => {
    const group: BindingInstance[] = [];
    expect(reconcileGroupBindings(group, ['a', 'b', 'c'], [group])).toEqual([
      b('b-0', 'a'),
      b('b-1', 'b'),
      b('b-2', 'c'),
    ]);
  });
});

/**
 * removeDimensionField/removeMetricField 必须在过滤 bindings 的同一次 set() 里
 * 清掉被删 bindingId 在五个元数据 Record（dimensionLabels/metricAggregations/
 * metricAliases/metricUnits/metricFormats）中的键。nextBindingId 取 max+1，
 * 删除最大号后新增列会复用该号——不清理会让新列静默继承旧列的元数据（跨列污染）。
 */
describe('removeDimensionField/removeMetricField 清理 bindingId 元数据', () => {
  beforeEach(() => {
    useStore.getState().resetChartBuilder();
    useStore.setState({
      chartBuilderFields: [
        { id: 'revenue', name: 'revenue', type: 'metric', dataType: 'float' },
        { id: 'profit', name: 'profit', type: 'metric', dataType: 'float' },
        { id: 'city', name: 'city', type: 'dimension', dataType: 'string' },
        { id: 'date', name: 'date', type: 'dimension', dataType: 'date' },
      ],
    });
  });

  it('删除唯一指标后，复用的 bindingId 不继承旧列的 aggregation/alias/unit/format', () => {
    const {
      addMetricField,
      removeMetricField,
      setMetricAggregation,
      setMetricAlias,
      setMetricUnit,
      setMetricFormat,
    } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addMetricField(fields[0]); // revenue → b-0
    setMetricAggregation('b-0', 'max');
    setMetricAlias('b-0', '营业收入');
    setMetricUnit('b-0', '元');
    setMetricFormat('b-0', '0.00');

    removeMetricField('b-0');

    // 删除后五个 Record 不得残留 b-0 的键
    const afterRemove = useStore.getState();
    expect(afterRemove.metricAggregations['b-0']).toBeUndefined();
    expect(afterRemove.metricAliases['b-0']).toBeUndefined();
    expect(afterRemove.metricUnits['b-0']).toBeUndefined();
    expect(afterRemove.metricFormats['b-0']).toBeUndefined();

    // nextBindingId 为 max+1：删掉最大号 b-0 后新增 profit 复用 b-0
    addMetricField(fields[1]);
    expect(useStore.getState().queryConfig.metricGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'profit' },
    ]);

    // 复用的 b-0 属于新列 profit，不得带上 revenue 的旧元数据
    const afterReadd = useStore.getState();
    expect(afterReadd.metricAggregations['b-0']).toBeUndefined();
    expect(afterReadd.metricAliases['b-0']).toBeUndefined();
    expect(afterReadd.metricUnits['b-0']).toBeUndefined();
    expect(afterReadd.metricFormats['b-0']).toBeUndefined();
  });

  it('删除维度后 dimensionLabels 同步清理，复用号不继承旧 label', () => {
    const { addDimensionField, removeDimensionField, setDimensionLabel } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionField(fields[2]); // city → b-0
    setDimensionLabel('b-0', '城市');

    removeDimensionField('b-0');
    expect(useStore.getState().dimensionLabels['b-0']).toBeUndefined();

    addDimensionField(fields[3]); // date → b-0（复用）
    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'date' },
    ]);
    expect(useStore.getState().dimensionLabels['b-0']).toBeUndefined();
  });

  it('只清理被删 bindingId 的元数据，同组其它 binding 的元数据保留', () => {
    const { addMetricField, removeMetricField, setMetricAggregation } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addMetricField(fields[0]); // revenue → b-0
    addMetricField(fields[1]); // profit → b-1
    setMetricAggregation('b-0', 'sum');
    setMetricAggregation('b-1', 'avg');

    removeMetricField('b-1');

    const state = useStore.getState();
    expect(state.metricAggregations['b-0']).toBe('sum');
    expect(state.metricAggregations['b-1']).toBeUndefined();
  });
});
