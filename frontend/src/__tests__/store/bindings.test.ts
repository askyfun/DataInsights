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

/**
 * removeDimensionGroup/removeMetricGroup 整组删除时，必须把该组所有 binding 的
 * bindingId 从五个元数据 Record 中清理掉。否则整组删除后 nextBindingId(max+1) 会从
 * b-0 重新分配，新列静默继承被删组里同号列的 aggregation/alias/unit/format/label
 * （与 removeDimensionField/removeMetricField 同一 bug 类）。
 */
describe('removeDimensionGroup/removeMetricGroup 清理整组 bindingId 元数据', () => {
  beforeEach(() => {
    useStore.getState().resetChartBuilder();
    useStore.setState({
      chartBuilderFields: [
        { id: 'revenue', name: 'revenue', type: 'metric', dataType: 'float' },
        { id: 'profit', name: 'profit', type: 'metric', dataType: 'float' },
        { id: 'cost', name: 'cost', type: 'metric', dataType: 'float' },
        { id: 'city', name: 'city', type: 'dimension', dataType: 'string' },
        { id: 'date', name: 'date', type: 'dimension', dataType: 'date' },
        { id: 'region', name: 'region', type: 'dimension', dataType: 'string' },
      ],
    });
  });

  it('删除整个指标组后组内所有 bindingId 元数据清理，复用号不继承旧列元数据', () => {
    const {
      addMetricField,
      removeMetricGroup,
      setMetricAggregation,
      setMetricAlias,
      setMetricUnit,
      setMetricFormat,
    } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addMetricField(fields[0], 0); // revenue → b-0（metric-group-1）
    addMetricField(fields[1], 0); // profit → b-1
    setMetricAggregation('b-0', 'max');
    setMetricAlias('b-0', '营业收入');
    setMetricUnit('b-0', '元');
    setMetricFormat('b-0', '0.00');
    setMetricAggregation('b-1', 'avg');

    const groupId = useStore.getState().queryConfig.metricGroups[0].id;
    removeMetricGroup(groupId);

    // 整组删除后无指标组，b-0/b-1 在五个 Record 中全部清理
    expect(useStore.getState().queryConfig.metricGroups).toHaveLength(0);
    for (const id of ['b-0', 'b-1']) {
      const s = useStore.getState();
      expect(s.metricAggregations[id]).toBeUndefined();
      expect(s.metricAliases[id]).toBeUndefined();
      expect(s.metricUnits[id]).toBeUndefined();
      expect(s.metricFormats[id]).toBeUndefined();
    }

    // 组已空 → nextBindingId 从 b-0 重新分配；加入不同列 cost 复用 b-0
    addMetricField(fields[2], 0);
    expect(useStore.getState().queryConfig.metricGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'cost' },
    ]);
    const after = useStore.getState();
    expect(after.metricAggregations['b-0']).toBeUndefined();
    expect(after.metricAliases['b-0']).toBeUndefined();
    expect(after.metricUnits['b-0']).toBeUndefined();
    expect(after.metricFormats['b-0']).toBeUndefined();
  });

  it('删除整个维度组后 dimensionLabels 清理，复用号不继承旧 label', () => {
    const { addDimensionField, removeDimensionGroup, setDimensionLabel } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionField(fields[3], 0); // city → b-0（dim-group-1）
    addDimensionField(fields[4], 0); // date → b-1
    setDimensionLabel('b-0', '城市');
    setDimensionLabel('b-1', '日期');

    const groupId = useStore.getState().queryConfig.dimensionGroups[0].id;
    removeDimensionGroup(groupId);

    expect(useStore.getState().queryConfig.dimensionGroups).toHaveLength(0);
    expect(useStore.getState().dimensionLabels['b-0']).toBeUndefined();
    expect(useStore.getState().dimensionLabels['b-1']).toBeUndefined();

    addDimensionField(fields[5], 0); // region → b-0（复用）
    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'region' },
    ]);
    expect(useStore.getState().dimensionLabels['b-0']).toBeUndefined();
  });

  it('只清理被删组的 bindingId，其它组的元数据保留', () => {
    const { addMetricField, removeMetricGroup, setMetricAggregation } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addMetricField(fields[0], 0); // revenue → b-0（metric-group-1）
    addMetricField(fields[1], 1); // profit → b-1（metric-group-2）
    setMetricAggregation('b-0', 'sum');
    setMetricAggregation('b-1', 'avg');

    const group1Id = useStore.getState().queryConfig.metricGroups[0].id;
    removeMetricGroup(group1Id);

    const s = useStore.getState();
    expect(s.metricAggregations['b-0']).toBeUndefined();
    expect(s.metricAggregations['b-1']).toBe('avg'); // 另一组保留
  });
});

/**
 * QueryPanel 多选 Select 取消选中列名时，reconcileGroupFields 必须在改写 bindings 的
 * 同一次更新里，把被移除 bindingId 从五个元数据 Record 中清理掉。否则该号被 nextBindingId
 * 复用后，新列会静默继承被取消选中列的元数据（与整组删除同一 bug 类）。
 */
describe('reconcileGroupFields 清理取消选中的 bindingId 元数据', () => {
  beforeEach(() => {
    useStore.getState().resetChartBuilder();
    useStore.setState({
      chartBuilderFields: [
        { id: 'revenue', name: 'revenue', type: 'metric', dataType: 'float' },
        { id: 'profit', name: 'profit', type: 'metric', dataType: 'float' },
        { id: 'cost', name: 'cost', type: 'metric', dataType: 'float' },
        { id: 'city', name: 'city', type: 'dimension', dataType: 'string' },
        { id: 'date', name: 'date', type: 'dimension', dataType: 'date' },
      ],
    });
  });

  it('取消选中指标后被移除 bindingId 元数据清理，复用号不继承旧列元数据', () => {
    const {
      addMetricField,
      reconcileGroupFields,
      setMetricAggregation,
      setMetricAlias,
      setMetricUnit,
      setMetricFormat,
    } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addMetricField(fields[0], 0); // revenue → b-0
    addMetricField(fields[1], 0); // profit → b-1
    setMetricAggregation('b-1', 'max');
    setMetricAlias('b-1', '利润');
    setMetricUnit('b-1', '元');
    setMetricFormat('b-1', '0.00');

    const groupId = useStore.getState().queryConfig.metricGroups[0].id;

    // 用户在 Select 里取消选中 profit，只保留 revenue
    reconcileGroupFields('metric', groupId, ['revenue']);

    expect(useStore.getState().queryConfig.metricGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'revenue' },
    ]);
    const afterDeselect = useStore.getState();
    expect(afterDeselect.metricAggregations['b-1']).toBeUndefined();
    expect(afterDeselect.metricAliases['b-1']).toBeUndefined();
    expect(afterDeselect.metricUnits['b-1']).toBeUndefined();
    expect(afterDeselect.metricFormats['b-1']).toBeUndefined();

    // 当前最大号回退到 b-0 → 再选中不同列 cost 会复用 b-1
    reconcileGroupFields('metric', groupId, ['revenue', 'cost']);
    expect(useStore.getState().queryConfig.metricGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'revenue' },
      { bindingId: 'b-1', field: 'cost' },
    ]);
    const afterReuse = useStore.getState();
    expect(afterReuse.metricAggregations['b-1']).toBeUndefined();
    expect(afterReuse.metricAliases['b-1']).toBeUndefined();
    expect(afterReuse.metricUnits['b-1']).toBeUndefined();
    expect(afterReuse.metricFormats['b-1']).toBeUndefined();
  });

  it('取消选中维度后 dimensionLabels 清理', () => {
    const { addDimensionField, reconcileGroupFields, setDimensionLabel } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionField(fields[3], 0); // city → b-0
    addDimensionField(fields[4], 0); // date → b-1
    setDimensionLabel('b-1', '日期');

    const groupId = useStore.getState().queryConfig.dimensionGroups[0].id;
    reconcileGroupFields('dimension', groupId, ['city']); // 取消选中 date

    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'city' },
    ]);
    expect(useStore.getState().dimensionLabels['b-1']).toBeUndefined();
  });

  it('保留仍选中列名的元数据（只清被取消选中的 bindingId）', () => {
    const { addMetricField, reconcileGroupFields, setMetricAggregation } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addMetricField(fields[0], 0); // revenue → b-0
    addMetricField(fields[1], 0); // profit → b-1
    setMetricAggregation('b-0', 'sum');
    setMetricAggregation('b-1', 'avg');

    const groupId = useStore.getState().queryConfig.metricGroups[0].id;
    reconcileGroupFields('metric', groupId, ['revenue']);

    const s = useStore.getState();
    expect(s.metricAggregations['b-0']).toBe('sum'); // revenue 保留
    expect(s.metricAggregations['b-1']).toBeUndefined(); // profit 清理
  });
});
