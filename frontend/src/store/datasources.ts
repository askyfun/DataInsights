import type { StateCreator } from 'zustand';
import { Datasource, DatasourceFormData, datasourcesApi } from '../api';
import type { AppState } from './index';

export interface DatasourcesSlice {
  datasources: Datasource[];
  datasourcesLoading: boolean;
  datasourcesError: string | null;
  selectedDatasourceId: number | null;

  fetchDatasources: () => Promise<void>;
  addDatasource: (data: DatasourceFormData) => Promise<Datasource>;
  updateDatasource: (id: number, data: DatasourceFormData) => Promise<Datasource>;
  deleteDatasource: (id: number) => Promise<void>;
  setSelectedDatasource: (id: number | null) => void;
}

export const createDatasourcesSlice: StateCreator<AppState, [], [], DatasourcesSlice> = (set) => ({
  datasources: [],
  datasourcesLoading: false,
  datasourcesError: null,
  selectedDatasourceId: null,

  fetchDatasources: async () => {
    set({ datasourcesLoading: true, datasourcesError: null });
    try {
      const response = await datasourcesApi.getAll();
      set({ datasources: response.data.data, datasourcesLoading: false });
    } catch (error: any) {
      set({
        datasourcesError: error.message || 'Failed to fetch',
        datasourcesLoading: false,
      });
    }
  },

  addDatasource: async (data: DatasourceFormData) => {
    const response = await datasourcesApi.create(data);
    const item = response.data.data;
    set((state) => ({
      datasources: [...state.datasources, item],
    }));
    return item;
  },

  updateDatasource: async (id: number, data: DatasourceFormData) => {
    const response = await datasourcesApi.update(id, data);
    const item = response.data.data;
    set((state) => ({
      datasources: state.datasources.map((ds) => (ds.id === id ? item : ds)),
    }));
    return item;
  },

  deleteDatasource: async (id: number) => {
    await datasourcesApi.delete(id);
    set((state) => ({
      datasources: state.datasources.filter((ds) => ds.id !== id),
      selectedDatasourceId: state.selectedDatasourceId === id ? null : state.selectedDatasourceId,
    }));
  },

  setSelectedDatasource: (id: number | null) => {
    set({ selectedDatasourceId: id });
  },
});
