// List-page inline edit submit path (Batch 3 review fix 1): the edit modal
// only manages name/datasource/query_type/table_name/query_sql, but the
// backend PUT handler overlays every scalar it receives — an omitted
// shard_enabled would JSON-default to false and silently disable sharding
// configured in DatasetEdit (mode likewise defaults to "direct"). So the
// caller must pass the stored row's shard_enabled/mode through untouched.
// The api module is NOT mocked here (only axios), so this pins the whole
// Dataset.tsx → store.updateDataset → datasetsApi.update wire body.
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { messages } from '../../i18n/useLocale';
import DatasetPage from '../../pages/Dataset';

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

// Stored row with sharding ON and a non-default mode — exactly the fields
// the inline modal does not manage and must not clobber.
const storedDataset = {
  id: 4,
  name: 'sales',
  datasource_id: 2,
  table_name: 'orders',
  query_sql: null,
  query_type: 'table',
  mode: 'accelerated',
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
    if (url === '/api/datasets') return Promise.resolve(envelope([storedDataset]));
    if (url === '/api/datasources') return Promise.resolve(envelope([{ id: 2, name: 'sr' }]));
    if (url === '/api/datasources/2/tables') return Promise.resolve(envelope([{ name: 'orders' }]));
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
  instance.put.mockResolvedValue(envelope(storedDataset));
});

const renderPage = () =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <MemoryRouter initialEntries={['/datasets']}>
        <Routes>
          <Route path="/datasets" element={<DatasetPage />} />
        </Routes>
      </MemoryRouter>
    </IntlProvider>
  );

describe('Dataset list inline edit submit', () => {
  it('carries the stored shard_enabled and mode into the PUT body when renaming', async () => {
    renderPage();

    // Open the edit modal for the row that has sharding configured.
    const editButton = await screen.findByLabelText('Edit dataset');
    fireEvent.click(editButton);

    const nameInput = await screen.findByPlaceholderText('请输入名称');
    fireEvent.change(nameInput, { target: { value: 'sales-renamed' } });

    // The modal's save button submits the Form; it is gated on the
    // datasource selection handleEdit populated from the row.
    const saveButton = await waitFor(() => {
      const button = screen.getByRole('button', { name: /保存|save/i });
      expect(button).toBeEnabled();
      return button;
    });
    fireEvent.click(saveButton);

    await waitFor(() => expect(instance.put).toHaveBeenCalledTimes(1));
    const [url, body] = instance.put.mock.calls[0];
    expect(url).toBe('/api/datasets/4');
    // Renamed fields from the form; shard_enabled/mode passed through from
    // the stored row (toEqual ignores the undefined table/query keys).
    expect(body).toEqual({
      name: 'sales-renamed',
      datasource_id: 2,
      query_type: 'table',
      table_name: 'orders',
      mode: 'accelerated',
      shard_enabled: true,
    });
  });
});
