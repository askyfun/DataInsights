import { beforeEach, describe, expect, it } from 'vitest';
import { useStore } from '../../store';

/**
 * moveBinding：查询配置区「拖字段标签」的唯一落点处理。
 *
 * 覆盖三类真实拖拽路径：维度组之间（透视表行/列维度）、指标组之间（组合图主/次轴）、
 * 组内换序。另覆盖三类必须拒绝/无动作的输入：跨 kind、目标组已有同名列、目标组不存在。
 * 关键不变量：移动必须保留 bindingId——别名/聚合/单位/格式都挂在 bindingId 上，
 * 换号等于把用户配好的元数据丢在原地。
 */
describe('moveBinding', () => {
  beforeEach(() => {
    useStore.getState().resetChartBuilder();
    // v1 契约：fieldId 即稳定列名
    useStore.setState({
      chartBuilderFields: [
        { id: 'region', name: 'region', type: 'dimension', dataType: 'string' },
        { id: 'city', name: 'city', type: 'dimension', dataType: 'string' },
        { id: 'month', name: 'month', type: 'dimension', dataType: 'int' },
        { id: 'revenue', name: 'revenue', type: 'metric', dataType: 'int' },
        { id: 'profit', name: 'profit', type: 'metric', dataType: 'int' },
      ],
    });
  });

  const fields = () => useStore.getState().chartBuilderFields;
  const dimBindings = (groupIndex: number) =>
    useStore.getState().queryConfig.dimensionGroups[groupIndex]?.bindings ?? [];
  const metricBindings = (groupIndex: number) =>
    useStore.getState().queryConfig.metricGroups[groupIndex]?.bindings ?? [];

  it('把行维度拖到列维度：绑定从源组摘除并追加到目标组末尾', () => {
    const { addDimensionField, moveBinding } = useStore.getState();
    addDimensionField(fields()[0], 0); // region → b-0 @ group0（行）
    addDimensionField(fields()[1], 0); // city   → b-1 @ group0（行）
    addDimensionField(fields()[2], 1); // month  → b-2 @ group1（列）

    const result = moveBinding(
      { kind: 'dimension', groupIndex: 0, bindingId: 'b-0' },
      { kind: 'dimension', groupIndex: 1 }
    );

    expect(result).toBe('moved');
    expect(dimBindings(0)).toEqual([{ bindingId: 'b-1', fieldId: 'city' }]);
    expect(dimBindings(1)).toEqual([
      { bindingId: 'b-2', fieldId: 'month' },
      { bindingId: 'b-0', fieldId: 'region' },
    ]);
  });

  it('跨组移动到指定下标（插到目标组首位）', () => {
    const { addDimensionField, moveBinding } = useStore.getState();
    addDimensionField(fields()[0], 0); // region → b-0
    addDimensionField(fields()[2], 1); // month  → b-1

    moveBinding(
      { kind: 'dimension', groupIndex: 0, bindingId: 'b-0' },
      { kind: 'dimension', groupIndex: 1, index: 0 }
    );

    expect(dimBindings(1)).toEqual([
      { bindingId: 'b-0', fieldId: 'region' },
      { bindingId: 'b-1', fieldId: 'month' },
    ]);
  });

  it('组内换序：把第二个字段拖到第一个前面', () => {
    const { addDimensionField, moveBinding } = useStore.getState();
    addDimensionField(fields()[0], 0); // region → b-0
    addDimensionField(fields()[1], 0); // city   → b-1

    const result = moveBinding(
      { kind: 'dimension', groupIndex: 0, bindingId: 'b-1' },
      { kind: 'dimension', groupIndex: 0, index: 0 }
    );

    expect(result).toBe('moved');
    expect(dimBindings(0).map((b) => b.bindingId)).toEqual(['b-1', 'b-0']);
  });

  it('指标组之间移动（组合图主/次轴）', () => {
    const { addMetricField, moveBinding } = useStore.getState();
    addMetricField(fields()[3], 0); // revenue → b-0
    addMetricField(fields()[4], 1); // profit  → b-1

    const result = moveBinding(
      { kind: 'metric', groupIndex: 0, bindingId: 'b-0' },
      { kind: 'metric', groupIndex: 1 }
    );

    expect(result).toBe('moved');
    expect(metricBindings(0)).toEqual([]);
    expect(metricBindings(1)).toEqual([
      { bindingId: 'b-1', fieldId: 'profit' },
      { bindingId: 'b-0', fieldId: 'revenue' },
    ]);
  });

  it('移动保留 bindingId，挂在它上面的别名/聚合元数据跟着字段走', () => {
    const { addMetricField, addMetricGroup, moveBinding, setMetricAggregation, setMetricAlias } =
      useStore.getState();
    addMetricField(fields()[3], 0); // revenue → b-0 @ 主指标组
    addMetricGroup(); // 副指标组（组合图的次轴槽位，初始为空）
    setMetricAggregation('b-0', 'avg');
    setMetricAlias('b-0', '平均营收');

    moveBinding(
      { kind: 'metric', groupIndex: 0, bindingId: 'b-0' },
      { kind: 'metric', groupIndex: 1 }
    );

    expect(metricBindings(1)).toEqual([{ bindingId: 'b-0', fieldId: 'revenue' }]);
    expect(useStore.getState().metricAggregations['b-0']).toBe('avg');
    expect(useStore.getState().metricAliases['b-0']).toBe('平均营收');
  });

  it('落在原位时不做任何变更（noop）', () => {
    const { addDimensionField, moveBinding } = useStore.getState();
    addDimensionField(fields()[0], 0); // region → b-0
    addDimensionField(fields()[1], 0); // city   → b-1
    const before = useStore.getState().queryConfig;

    expect(
      moveBinding(
        { kind: 'dimension', groupIndex: 0, bindingId: 'b-1' },
        { kind: 'dimension', groupIndex: 0, index: 1 }
      )
    ).toBe('noop');
    // 落在本组空白处（无下标）同样不改顺序
    expect(
      moveBinding(
        { kind: 'dimension', groupIndex: 0, bindingId: 'b-1' },
        { kind: 'dimension', groupIndex: 0 }
      )
    ).toBe('noop');
    expect(useStore.getState().queryConfig).toBe(before);
  });

  it('跨 kind 拖动不变更状态（维度不能进指标组，noop）', () => {
    const { addDimensionField, addMetricField, moveBinding } = useStore.getState();
    addDimensionField(fields()[0], 0); // region → b-0 @ 维度组
    addMetricField(fields()[3], 0); // revenue → b-1 @ 指标组

    const result = moveBinding(
      { kind: 'dimension', groupIndex: 0, bindingId: 'b-0' },
      { kind: 'metric', groupIndex: 0 }
    );

    expect(result).toBe('noop');
    expect(dimBindings(0)).toEqual([{ bindingId: 'b-0', fieldId: 'region' }]);
    expect(metricBindings(0)).toEqual([{ bindingId: 'b-1', fieldId: 'revenue' }]);
  });

  it('目标组已有同名列时拒绝（单组内不允许重复列）', () => {
    const { addDimensionField, moveBinding } = useStore.getState();
    addDimensionField(fields()[0], 0); // region → b-0 @ group0
    addDimensionField(fields()[0], 1); // region → b-1 @ group1（跨组同名允许）

    const result = moveBinding(
      { kind: 'dimension', groupIndex: 0, bindingId: 'b-0' },
      { kind: 'dimension', groupIndex: 1 }
    );

    expect(result).toBe('rejected');
    expect(dimBindings(0)).toEqual([{ bindingId: 'b-0', fieldId: 'region' }]);
    expect(dimBindings(1)).toEqual([{ bindingId: 'b-1', fieldId: 'region' }]);
  });

  it('源绑定不存在或目标组不存在时不做任何变更（noop）', () => {
    const { addDimensionField, moveBinding } = useStore.getState();
    addDimensionField(fields()[0], 0);
    const before = useStore.getState().queryConfig;

    expect(
      moveBinding(
        { kind: 'dimension', groupIndex: 0, bindingId: 'b-99' },
        { kind: 'dimension', groupIndex: 0, index: 0 }
      )
    ).toBe('noop');
    expect(
      moveBinding(
        { kind: 'dimension', groupIndex: 0, bindingId: 'b-0' },
        { kind: 'dimension', groupIndex: 5 }
      )
    ).toBe('noop');
    expect(useStore.getState().queryConfig).toBe(before);
  });
});
