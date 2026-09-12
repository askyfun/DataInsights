// datasetsApi.update wire-shape pins (Batch 3 Task 1): the ghost-route fix
// made PUT /api/datasets/:id real; the body must match the create contract
// (datasetCreateIn / DatasetUpdateRequest): tags/shard_keys travel as
// JSON-array STRINGS, never array bodies. Stringification lives in the api
// layer — the single choke point both live callers (store.updateDataset and
// DatasetEdit submit) go through — so it can never double-encode.
import { beforeEach, describe, expect, it, vi } from 'vitest';

// One fake axios instance shared by the api module's axios.create() call.
const { instance } = vi.hoisted(() => {
  const inst = {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
  };
  return { instance: inst };
});

vi.mock('axios', () => ({
  default: { create: () => instance },
}));

import { datasetsApi } from '../../api';

const okResponse = { data: { code: 20000, msg: 'success', trace: '', data: {} } };

beforeEach(() => {
  vi.clearAllMocks();
  instance.put.mockResolvedValue(okResponse);
  instance.post.mockResolvedValue(okResponse);
});

describe('datasetsApi.update wire payload', () => {
  it('serializes tags/shard_keys arrays to JSON strings (create convention)', async () => {
    await datasetsApi.update(4, {
      name: 'upd',
      datasource_id: 2,
      query_type: 'table',
      table_name: 'orders',
      description: 'd',
      shard_enabled: true,
      shard_keys: ['id'],
      tags: ['a', 'b'],
    });

    expect(instance.put).toHaveBeenCalledTimes(1);
    const [url, body] = instance.put.mock.calls[0];
    expect(url).toBe('/api/datasets/4');
    expect(body).toEqual({
      name: 'upd',
      datasource_id: 2,
      query_type: 'table',
      table_name: 'orders',
      description: 'd',
      shard_enabled: true,
      shard_keys: '["id"]',
      tags: '["a","b"]',
    });
  });

  it('omits tags/shard_keys keys entirely when absent (backend defaults)', async () => {
    await datasetsApi.update(4, {
      name: 'x',
      datasource_id: 1,
      query_type: 'sql',
      query_sql: 'SELECT 1',
    });
    const [, body] = instance.put.mock.calls[0];
    expect('tags' in body).toBe(false);
    expect('shard_keys' in body).toBe(false);
    expect(body.query_sql).toBe('SELECT 1');
  });

  it('serializes an empty shard_keys array to "[]" (DatasetEdit disabled-shard path)', async () => {
    await datasetsApi.update(4, {
      name: 'x',
      datasource_id: 1,
      query_type: 'table',
      table_name: 'orders',
      shard_enabled: false,
      shard_keys: [],
    });
    const [, body] = instance.put.mock.calls[0];
    expect(body.shard_keys).toBe('[]');
  });

  it('create passes the payload through unchanged (no arrays sent by its callers today)', async () => {
    const data = { name: 'n', datasource_id: 1, query_type: 'table', table_name: 'orders' };
    await datasetsApi.create(data);
    expect(instance.post).toHaveBeenCalledWith('/api/datasets', data);
  });
});
