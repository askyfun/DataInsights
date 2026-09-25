import { beforeEach, describe, expect, it } from 'vitest';
import { useStore } from '../../store';

/**
 * swapDimensionGroups：透视表「行列切换」快捷按钮的唯一落点。
 *
 * 关键不变量：只对调两个组的 bindings，组自身的 id 不变——槽位语义（rows/columns）
 * 由组下标决定，组 id 保持稳定可让 React key 与持久化文档不抖动；bindingId 随绑定整体
 * 迁移，别名/单位/格式等按 bindingId 索引的元数据不会脱钩。
 */
describe('swapDimensionGroups', () => {
  beforeEach(() => {
    useStore.getState().resetChartBuilder();
    useStore.setState({
      chartBuilderFields: [
        { id: 'region', name: 'region', type: 'dimension', dataType: 'string' },
        { id: 'month', name: 'month', type: 'dimension', dataType: 'int' },
      ],
    });
  });

  const dimensionGroups = () => useStore.getState().queryConfig.dimensionGroups;

  const seedBothGroups = () => {
    const state = useStore.getState();
    state.addDimensionField(state.chartBuilderFields[0], 0); // region → b-0 @ group0（行）
    state.addDimensionField(state.chartBuilderFields[1], 1); // month  → b-1 @ group1（列）
  };

  it('对调行/列两组的绑定内容', () => {
    seedBothGroups();

    useStore.getState().swapDimensionGroups(0, 1);

    const groups = dimensionGroups();
    expect(groups[0].bindings).toEqual([{ bindingId: 'b-1', fieldId: 'month' }]);
    expect(groups[1].bindings).toEqual([{ bindingId: 'b-0', fieldId: 'region' }]);
  });

  it('组 id 保持不变，只有 bindings 换位', () => {
    seedBothGroups();
    const before = dimensionGroups().map((group) => group.id);

    useStore.getState().swapDimensionGroups(0, 1);

    expect(dimensionGroups().map((group) => group.id)).toEqual(before);
  });

  it('再次切换回到初始状态（幂等可逆）', () => {
    seedBothGroups();
    const before = dimensionGroups();

    useStore.getState().swapDimensionGroups(0, 1);
    useStore.getState().swapDimensionGroups(0, 1);

    expect(dimensionGroups()).toEqual(before);
  });

  it('一组为空时相当于把字段搬到另一组', () => {
    const state = useStore.getState();
    state.addDimensionField(state.chartBuilderFields[0], 0); // 只有行维度有字段
    state.addDimensionGroup(); // 显式建出列维度组，使其存在但为空

    useStore.getState().swapDimensionGroups(0, 1);

    expect(dimensionGroups()[0].bindings).toEqual([]);
    expect(dimensionGroups()[1].bindings).toEqual([{ bindingId: 'b-0', fieldId: 'region' }]);
  });

  it('下标相同或越界时不做任何变更', () => {
    seedBothGroups();
    const before = dimensionGroups();

    useStore.getState().swapDimensionGroups(0, 0);
    useStore.getState().swapDimensionGroups(0, 9);

    expect(dimensionGroups()).toEqual(before);
  });
});
