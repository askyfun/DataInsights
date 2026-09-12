// store.updateDataset contract pin (Batch 3 Task 1): the store hands the
// DatasetFormData with ARRAY fields straight to datasetsApi.update without
// pre-serializing — serialization belongs to the api layer alone, so arrays
// can never be double-stringified. State update comes from the response.

import type { AxiosResponse } from 'axios';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { type Dataset, datasetsApi } from '../../api';
import type { ApiResponse } from '../../lib/api/client';
import { useStore } from '../../store';

vi.mock('../../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api')>();
  return {
    ...actual,
    datasetsApi: {
      ...actual.datasetsApi,
      update: vi.fn(),
    },
  };
});

const mockUpdate = vi.mocked(datasetsApi.update);

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

const saved: Dataset = {
  id: 4,
  name: 'upd',
  datasource_id: 2,
  table_name: 'orders',
  query_sql: null,
  query_type: 'table',
  mode: 'direct',
  accelerate_config: null,
  description: null,
  tags: '[]',
  refresh_strategy: null,
  preview_data: null,
  quality_rules: '[]',
  columns: '[]',
  shard_enabled: true,
  shard_keys: '["id"]',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

beforeEach(() => {
  vi.clearAllMocks();
  useStore.setState({ datasets: [{ ...saved, name: 'old' }] });
});

describe('store.updateDataset', () => {
  it('passes array fields through to datasetsApi.update untouched', async () => {
    mockUpdate.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'success', trace: '', data: saved })
    );

    const result = await useStore.getState().updateDataset(4, {
      name: 'upd',
      datasource_id: 2,
      query_type: 'table',
      table_name: 'orders',
      shard_enabled: true,
      shard_keys: ['id'],
    });

    expect(mockUpdate).toHaveBeenCalledWith(4, {
      name: 'upd',
      datasource_id: 2,
      query_type: 'table',
      table_name: 'orders',
      shard_enabled: true,
      shard_keys: ['id'],
    });
    expect(result).toEqual(saved);
    expect(useStore.getState().datasets).toEqual([saved]);
  });
});
