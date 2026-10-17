import type { StateCreator } from 'zustand';
import { Chart, ChartFormData, chartsApi } from '../api';
import type { AppState } from './index';

export interface ChartsSlice {
  charts: Chart[];
  chartsLoading: boolean;
  chartsError: string | null;

  fetchCharts: () => Promise<void>;
  addChart: (data: ChartFormData) => Promise<Chart>;
  updateChart: (id: number, data: Partial<ChartFormData>) => Promise<Chart>;
  deleteChart: (id: number) => Promise<void>;
}

export const createChartsSlice: StateCreator<AppState, [], [], ChartsSlice> = (set) => ({
  charts: [],
  chartsLoading: false,
  chartsError: null,

  fetchCharts: async () => {
    set({ chartsLoading: true, chartsError: null });
    try {
      // 选择器 / 引用名解析需要完整集合：按上限页一次性取回（服务端分页上限 1000）。
      const response = await chartsApi.getAll({ limit: 1000 });
      set({ charts: response.data.data.items, chartsLoading: false });
    } catch (error: any) {
      set({
        chartsError: error.message || 'Failed to fetch',
        chartsLoading: false,
      });
    }
  },

  addChart: async (data: ChartFormData) => {
    const response = await chartsApi.create(data);
    const item = response.data.data;
    set((state) => ({
      charts: [...state.charts, item],
    }));
    return item;
  },

  updateChart: async (id: number, data: Partial<ChartFormData>) => {
    const response = await chartsApi.update(id, data);
    const item = response.data.data;
    set((state) => ({
      charts: state.charts.map((c) => (c.id === id ? item : c)),
    }));
    return item;
  },

  deleteChart: async (id: number) => {
    await chartsApi.delete(id);
    set((state) => ({
      charts: state.charts.filter((c) => c.id !== id),
    }));
  },
});
