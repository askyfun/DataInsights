import type { StateCreator } from 'zustand';
import { Dataset, DatasetFormData, datasetsApi } from '../api';
import type { AppState } from './index';

export interface DatasetsSlice {
  datasets: Dataset[];
  datasetsLoading: boolean;
  datasetsError: string | null;
  selectedDatasetId: number | null;

  fetchDatasets: () => Promise<void>;
  addDataset: (data: DatasetFormData) => Promise<Dataset>;
  updateDataset: (id: number, data: DatasetFormData) => Promise<Dataset>;
  deleteDataset: (id: number) => Promise<void>;
  setSelectedDataset: (id: number | null) => void;
}

export const createDatasetsSlice: StateCreator<AppState, [], [], DatasetsSlice> = (set) => ({
  datasets: [],
  datasetsLoading: false,
  datasetsError: null,
  selectedDatasetId: null,

  fetchDatasets: async () => {
    set({ datasetsLoading: true, datasetsError: null });
    try {
      const response = await datasetsApi.getAll();
      set({ datasets: response.data.data, datasetsLoading: false });
    } catch (error: any) {
      set({
        datasetsError: error.message || 'Failed to fetch',
        datasetsLoading: false,
      });
    }
  },

  addDataset: async (data: DatasetFormData) => {
    const response = await datasetsApi.create(data);
    const item = response.data.data;
    set((state) => ({
      datasets: [...state.datasets, item],
    }));
    return item;
  },

  updateDataset: async (id: number, data: DatasetFormData) => {
    const response = await datasetsApi.update(id, data);
    const item = response.data.data;
    set((state) => ({
      datasets: state.datasets.map((ds) => (ds.id === id ? item : ds)),
    }));
    return item;
  },

  deleteDataset: async (id: number) => {
    await datasetsApi.delete(id);
    set((state) => ({
      datasets: state.datasets.filter((ds) => ds.id !== id),
      selectedDatasetId: state.selectedDatasetId === id ? null : state.selectedDatasetId,
    }));
  },

  setSelectedDataset: (id: number | null) => {
    set({ selectedDatasetId: id });
  },
});
