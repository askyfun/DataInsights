import { beforeEach, describe, expect, it } from 'vitest';
import { useStore } from '../../store';

describe('reorderDimensionField', () => {
  beforeEach(() => {
    const store = useStore.getState();
    store.resetChartBuilder();
    // 模拟已有字段（v1 契约：fieldId 即稳定列名）
    useStore.setState({
      chartBuilderFields: [
        { id: 'name', name: 'name', type: 'dimension', dataType: 'string' },
        { id: 'city', name: 'city', type: 'dimension', dataType: 'string' },
        { id: 'date', name: 'date', type: 'dimension', dataType: 'date' },
      ],
    });
  });

  it('addDimensionField 按顺序追加，并分配递增 bindingId', () => {
    const { addDimensionField } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionField(fields[0]);
    addDimensionField(fields[1]);

    const dims = useStore.getState().queryConfig.dimensionGroups[0]?.bindings;
    expect(dims).toEqual([
      { bindingId: 'b-0', field: 'name' },
      { bindingId: 'b-1', field: 'city' },
    ]);
  });

  it('reorderDimensionField 交换两个字段（bindingId 随字段移动）', () => {
    const { addDimensionField, reorderDimensionField } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionField(fields[0]);
    addDimensionField(fields[1]);

    // 初始顺序: [name, city]
    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'name' },
      { bindingId: 'b-1', field: 'city' },
    ]);

    // 交换: [city, name]
    reorderDimensionField(0, 1);
    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'b-1', field: 'city' },
      { bindingId: 'b-0', field: 'name' },
    ]);
  });

  it('reorderDimensionField 移动到末尾', () => {
    const { addDimensionField, reorderDimensionField } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionField(fields[0]);
    addDimensionField(fields[1]);
    addDimensionField(fields[2]);

    // 初始: [name, city, date]
    reorderDimensionField(0, 2);
    // 期望: [city, date, name]
    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings.map((b) => b.field)).toEqual(
      ['city', 'date', 'name']
    );
  });

  it('reorderDimensionField 移动到开头', () => {
    const { addDimensionField, reorderDimensionField } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionField(fields[0]);
    addDimensionField(fields[1]);
    addDimensionField(fields[2]);

    // 初始: [name, city, date]
    reorderDimensionField(2, 0);
    // 期望: [date, name, city]
    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings.map((b) => b.field)).toEqual(
      ['date', 'name', 'city']
    );
  });

  it('addDimensionField 支持追加到指定维度组', () => {
    const { addDimensionGroup, addDimensionField } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addDimensionGroup({ id: 'dim-group-main', bindings: [] });
    addDimensionGroup({ id: 'dim-group-secondary', bindings: [] });

    addDimensionField(fields[0], 0);
    addDimensionField(fields[1], 1);

    expect(useStore.getState().queryConfig.dimensionGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'name' },
    ]);
    expect(useStore.getState().queryConfig.dimensionGroups[1].bindings).toEqual([
      { bindingId: 'b-1', field: 'city' },
    ]);
  });

  it('addMetricField 支持追加到指定指标组', () => {
    useStore.setState({
      chartBuilderFields: [
        { id: 'revenue', name: 'revenue', type: 'metric', dataType: 'float' },
        { id: 'profit', name: 'profit', type: 'metric', dataType: 'float' },
      ],
    });

    const { addMetricGroup, addMetricField } = useStore.getState();
    const fields = useStore.getState().chartBuilderFields;

    addMetricGroup({ id: 'metric-group-main', bindings: [] });
    addMetricGroup({ id: 'metric-group-secondary', bindings: [] });

    addMetricField(fields[0], 0);
    addMetricField(fields[1], 1);

    expect(useStore.getState().queryConfig.metricGroups[0].bindings).toEqual([
      { bindingId: 'b-0', field: 'revenue' },
    ]);
    expect(useStore.getState().queryConfig.metricGroups[1].bindings).toEqual([
      { bindingId: 'b-1', field: 'profit' },
    ]);
  });

  it('addMetricField 写入第 2 个指标组时会自动补齐前置空组，避免稀疏数组', () => {
    useStore.setState({
      chartBuilderFields: [{ id: 'revenue', name: 'revenue', type: 'metric', dataType: 'float' }],
    });

    const { addMetricField } = useStore.getState();
    const field = useStore.getState().chartBuilderFields[0];

    addMetricField(field, 1);

    expect(useStore.getState().queryConfig.metricGroups).toEqual([
      { id: 'metric-group-1', bindings: [] },
      { id: 'metric-group-2', bindings: [{ bindingId: 'b-0', field: 'revenue' }] },
    ]);
  });
});
