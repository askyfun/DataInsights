import { describe, expect, it } from 'vitest';
import { useStore } from '@/store';

describe('Store', () => {
  it('should have initial state', () => {
    const store = useStore.getState();
    expect(store.datasources).toEqual([]);
    expect(store.datasets).toEqual([]);
    expect(store.charts).toEqual([]);
    expect(store.datasourcesLoading).toBe(false);
    expect(store.datasetsLoading).toBe(false);
    expect(store.chartsLoading).toBe(false);
  });

  it('should have action functions', () => {
    const store = useStore.getState();
    expect(typeof store.fetchDatasources).toBe('function');
    expect(typeof store.fetchDatasets).toBe('function');
    expect(typeof store.fetchCharts).toBe('function');
  });

  // 表格/透视表默认一页 100 条（用户约定）：初始态与「resetChartBuilder 后」都必须是 100，
  // 否则首次 table 查询会按旧默认 page_size=10 拉取、翻页次数暴涨。
  it('tablePagination defaults to pageSize 100 (initial and after reset)', () => {
    expect(useStore.getState().tablePagination.pageSize).toBe(100);
    useStore.setState({ tablePagination: { page: 3, pageSize: 20, total: 999 } });
    useStore.getState().resetChartBuilder();
    expect(useStore.getState().tablePagination).toEqual({ page: 1, pageSize: 100, total: 0 });
  });
});
