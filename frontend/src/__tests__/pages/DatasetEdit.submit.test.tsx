// DatasetEdit submit path (Batch 3 Task 1 ghost-route fix): the page passes
// its DatasetFormData — shard_keys as a real string[] — into datasetsApi.update,
// and the wire body that leaves the api layer must carry the create
// convention's JSON-array-string form. The api module is NOT mocked here
// (only axios), so this pins the whole store/caller→api→wire serialization
// with exactly one encode.
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { messages } from '../../i18n/useLocale';
import DatasetEditPage from '../../pages/DatasetEdit';

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

// antd internals reach for ResizeObserver in jsdom.
class FakeResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', FakeResizeObserver);

const envelope = (data: unknown) => ({ data: { code: 20000, msg: 'success', trace: '', data } });

const dataset = {
  id: 4,
  name: 'sales',
  datasource_id: 2,
  table_name: 'orders',
  query_sql: null,
  query_type: 'table',
  mode: 'direct',
  description: null,
  tags: '[]',
  quality_rules: '[]',
  columns: '[]',
  shard_enabled: true,
  shard_keys: '["id"]',
  created_at: '',
  updated_at: '',
};

beforeEach(() => {
  vi.clearAllMocks();
  instance.get.mockImplementation((url: string) => {
    if (url === '/api/datasources') return Promise.resolve(envelope([{ id: 2, name: 'pg' }]));
    if (url === '/api/datasets/4') return Promise.resolve(envelope(dataset));
    if (url === '/api/datasets/4/columns') {
      return Promise.resolve(
        envelope([
          {
            name: 'id',
            expr: 'id',
            type: 'int',
            type_config: { precision: 0, scale: 0 },
            comment: '',
            role: 'metric',
          },
        ])
      );
    }
    if (url === '/api/datasources/2/tables') return Promise.resolve(envelope([{ name: 'orders' }]));
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
  instance.put.mockResolvedValue(envelope(dataset));
});

const renderPage = () =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <MemoryRouter initialEntries={['/datasets/4/edit']}>
        <Routes>
          <Route path="/datasets/:id/edit" element={<DatasetEditPage />} />
        </Routes>
      </MemoryRouter>
    </IntlProvider>
  );

describe('DatasetEdit submit', () => {
  it('PUTs the edited form with shard_keys serialized to a JSON string', async () => {
    renderPage();

    // The save button stays disabled until the dataset fetch set the
    // datasource selection.
    const saveButton = await waitFor(() => {
      const button = screen.getByRole('button', { name: /保存|save/i });
      expect(button).toBeEnabled();
      return button;
    });
    fireEvent.click(saveButton);

    await waitFor(() => expect(instance.put).toHaveBeenCalledTimes(1));
    const [url, body] = instance.put.mock.calls[0];
    expect(url).toBe('/api/datasets/4');
    // Array in, JSON string out — exactly one encode. description is the
    // empty form value, passed through as-is (the handler preserves the
    // stored value for empty optional metadata; cleared pickers send "[]").
    expect(body).toEqual({
      name: 'sales',
      datasource_id: 2,
      query_type: 'table',
      table_name: 'orders',
      description: '',
      shard_enabled: true,
      shard_keys: '["id"]',
    });
  });
});
