import { beforeEach, describe, expect, it } from 'vitest';
import { useStore } from '@/store';

/**
 * 过滤条件 store 契约。
 *
 * 改造后过滤只支持「且」：addFilter 恒写入 logic: 'and'，UI 不再提供 AND/OR 切换；
 * 同一字段允许重复加入（区间筛选 = 同字段各一条 >= 与 <=），因此 addFilter 不做去重，
 * 但每条条件的 id 必须唯一——否则按 id 更新/删除会误伤另一条。
 */

describe('addFilter', () => {
  beforeEach(() => {
    useStore.getState().resetChartBuilder();
  });

  it('只传 field 时补齐默认值，logic 恒为 and', () => {
    useStore.getState().addFilter({ fieldId: 'sale_count' });

    const [filter] = useStore.getState().queryConfig.filters;
    expect(filter).toMatchObject({
      fieldId: 'sale_count',
      operator: 'eq',
      value: '',
      logic: 'and',
    });
    expect(filter.id).toMatch(/^filter-/);
  });

  it('连续追加同一字段的多条条件：不去重且 id 互不相同', () => {
    const { addFilter } = useStore.getState();
    addFilter({ fieldId: 'sale_count' });
    addFilter({ fieldId: 'sale_count' });

    const filters = useStore.getState().queryConfig.filters;
    expect(filters).toHaveLength(2);
    expect(filters[0].fieldId).toBe('sale_count');
    expect(filters[1].fieldId).toBe('sale_count');
    expect(filters[0].id).not.toBe(filters[1].id);
  });

  it('不传参数时追加一条待填写的空条件', () => {
    useStore.getState().addFilter();

    const [filter] = useStore.getState().queryConfig.filters;
    expect(filter).toMatchObject({ fieldId: '', operator: 'eq', logic: 'and' });
  });

  it('显式传入的字段覆盖默认值（兼容既有调用方）', () => {
    useStore.getState().addFilter({
      id: 'filter-gt-1',
      fieldId: 'revenue',
      operator: 'gt',
      value: 100,
      logic: 'and',
    });

    expect(useStore.getState().queryConfig.filters[0]).toEqual({
      id: 'filter-gt-1',
      fieldId: 'revenue',
      operator: 'gt',
      value: 100,
      logic: 'and',
    });
  });

  it('removeFilter 只删指定 id 的条件', () => {
    const { addFilter } = useStore.getState();
    addFilter({ fieldId: 'brand_name' });
    addFilter({ fieldId: 'sale_count' });

    const [first] = useStore.getState().queryConfig.filters;
    useStore.getState().removeFilter(first.id);

    const filters = useStore.getState().queryConfig.filters;
    expect(filters).toHaveLength(1);
    expect(filters[0].fieldId).toBe('sale_count');
  });
});
