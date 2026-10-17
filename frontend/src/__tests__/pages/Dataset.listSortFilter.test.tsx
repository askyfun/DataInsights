// Dataset 列表页的服务端分页/排序/筛选（issue #205）。
// 旧实现是「一次拉全量 + 客户端 filter/sorter」；本文件钉死新契约：
//   1) 首屏请求带 limit/offset 与默认 sort=id&order=desc；
//   2) 表头点击把 sort/order 写进下一次请求；
//   3) 关键字与两个工具栏筛选分别进 keyword / datasource_id / query_type；
//   4) 页面渲染的是服务端返回的当页 items，total 用服务端 COUNT。
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

const row = (
  id: number,
  name: string,
  datasourceId: number,
  tableName: string,
  queryType: 'table' | 'sql'
) => ({
  id,
  name,
  datasource_id: datasourceId,
  table_name: tableName,
  query_sql: queryType === 'sql' ? 'select 1' : null,
  query_type: queryType,
  mode: 'direct',
  description: null,
  tags: '[]',
  quality_rules: '[]',
  columns: '[]',
  shard_enabled: false,
  shard_keys: '[]',
  created_at: '2026-09-18T16:36:56Z',
  updated_at: '2026-09-19T06:54:42Z',
  accelerate_config: null,
  refresh_strategy: null,
  preview_data: null,
});

const datasets = [
  row(1, 'orders', 1, 'orders_raw', 'table'),
  row(3, 'alpha', 1, 'a_tbl', 'table'),
  row(2, 'zebra', 2, 'z_tbl', 'sql'),
];

type Params = Record<string, unknown>;

/** 模拟后端的过滤/排序/分页，让页面断言仍然基于「服务端返回的当页 items」。 */
const serveDatasets = (params: Params, source: typeof datasets = datasets) => {
  let rows = [...source];
  const kw = String(params.keyword ?? '').toLowerCase();
  if (kw) {
    rows = rows.filter(
      (d) => d.name.toLowerCase().includes(kw) || (d.table_name ?? '').toLowerCase().includes(kw)
    );
  }
  if (params.datasource_id !== undefined) {
    rows = rows.filter((d) => d.datasource_id === params.datasource_id);
  }
  if (params.query_type !== undefined) {
    rows = rows.filter((d) => d.query_type === params.query_type);
  }
  const sort = String(params.sort ?? 'id');
  const dir = params.order === 'asc' ? 1 : -1;
  rows.sort((a, b) => {
    if (sort === 'name') return a.name.localeCompare(b.name) * dir;
    if (sort === 'created_at') return a.created_at.localeCompare(b.created_at) * dir || b.id - a.id;
    return (a.id - b.id) * dir;
  });
  const total = rows.length;
  const offset = Number(params.offset ?? 0);
  const limit = Number(params.limit ?? 10);
  return envelope({ items: rows.slice(offset, offset + limit), total });
};

beforeEach(() => {
  vi.clearAllMocks();
  instance.get.mockImplementation((url: string, config?: { params?: Params }) => {
    if (url === '/api/datasets') return Promise.resolve(serveDatasets(config?.params ?? {}));
    if (url === '/api/datasources')
      return Promise.resolve(
        envelope({
          items: [
            { id: 1, name: 'sr-a', type: 'starrocks' },
            { id: 2, name: 'sr-b', type: 'starrocks' },
          ],
          total: 2,
        })
      );
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
});

const renderPage = () => {
  const utils = render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <MemoryRouter initialEntries={['/datasets']}>
        <Routes>
          <Route path="/datasets" element={<DatasetPage />} />
        </Routes>
      </MemoryRouter>
    </IntlProvider>
  );
  return utils;
};

/** 当前表体每行指向的数据集 id，按渲染顺序返回。 */
const visibleIds = () =>
  Array.from(document.querySelectorAll('tbody tr')).map((tr) =>
    Number(
      tr.querySelector('a[href^="/datasets/"]')?.getAttribute('href')?.replace('/datasets/', '')
    )
  );

const datasetParams = (): Params[] =>
  instance.get.mock.calls
    .filter(([url]) => url === '/api/datasets')
    .map(([, config]) => (config as { params?: Params } | undefined)?.params ?? {});

const lastParams = (): Params => {
  const all = datasetParams();
  return all[all.length - 1];
};

/**
 * 选中工具栏第 index 个下拉里的某一项。
 * ⚠️ 不能点 role="option"：rc-select 有一份 height/width=0 的隐藏 listbox。
 * 真正可点的是门户里 .ant-select-item-option（文本为 label）。
 */
const pickSelectOption = async (index: number, label: string) => {
  const comboboxes = screen.getAllByRole('combobox');
  fireEvent.mouseDown(comboboxes[index]);

  const option = await waitFor(() => {
    const found = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (node) => node.textContent === label
    );
    expect(found).toBeTruthy();
    return found as HTMLElement;
  });
  fireEvent.click(option);
};

describe('Dataset list server-side paging/sorting/filtering', () => {
  it('asks for id DESC on first load and renders the returned page', async () => {
    renderPage();

    await waitFor(() => expect(visibleIds()).toEqual([3, 2, 1]));
    expect(datasetParams()[0]).toMatchObject({ limit: 10, offset: 0, sort: 'id', order: 'desc' });
    expect(screen.getByText('Total 3 items')).toBeInTheDocument();
  });

  it('sends sort=name with asc/desc when the name header is clicked', async () => {
    renderPage();
    await waitFor(() => expect(visibleIds()).toEqual([3, 2, 1]));

    const nameHeader = screen.getByText('名称').closest('th');
    expect(nameHeader).not.toBeNull();

    fireEvent.click(nameHeader as HTMLElement);
    await waitFor(() => expect(visibleIds()).toEqual([3, 1, 2]));
    expect(lastParams()).toMatchObject({
      sort: 'name',
      order: 'asc',
      offset: 0,
    });

    fireEvent.click(nameHeader as HTMLElement);
    await waitFor(() => expect(visibleIds()).toEqual([2, 1, 3]));
    expect(lastParams()).toMatchObject({
      sort: 'name',
      order: 'desc',
      offset: 0,
    });
  });

  it('sends the keyword after the debounce, and resets offset', async () => {
    renderPage();
    await waitFor(() => expect(visibleIds()).toHaveLength(3));

    fireEvent.change(screen.getByPlaceholderText('搜索名称或表名'), {
      target: { value: 'a_tbl' },
    });

    // 关键字覆盖表名而不只是名称——服务端 ILIKE 的同一口径。
    await waitFor(() => expect(visibleIds()).toEqual([3]), { timeout: 2000 });
    expect(lastParams()).toMatchObject({
      keyword: 'a_tbl',
      offset: 0,
    });
  });

  it('sends datasource_id when a datasource is picked', async () => {
    renderPage();
    await waitFor(() => expect(visibleIds()).toHaveLength(3));

    await pickSelectOption(0, 'sr-b');

    await waitFor(() => expect(visibleIds()).toEqual([2]));
    expect(lastParams()).toMatchObject({
      datasource_id: 2,
      offset: 0,
    });
  });

  it('sends query_type when a query type is picked', async () => {
    renderPage();
    await waitFor(() => expect(visibleIds()).toHaveLength(3));

    // 第二个下拉 = 查询类型。选项文案走 i18n（表 / SQL）。
    await pickSelectOption(1, 'SQL');

    await waitFor(() => expect(visibleIds()).toEqual([2]));
    expect(lastParams()).toMatchObject({
      query_type: 'sql',
      offset: 0,
    });
  });

  it('requests the next page with offset and keeps the active sort', async () => {
    // 12 条 → 两页；用独立的数据源避免影响其它用例。
    const many = Array.from({ length: 12 }, (_, i) =>
      row(i + 1, `ds-${String(i + 1).padStart(2, '0')}`, 1, 't', 'table')
    );
    instance.get.mockImplementation((url: string, config?: { params?: Params }) => {
      if (url === '/api/datasets')
        return Promise.resolve(serveDatasets(config?.params ?? {}, many));
      if (url === '/api/datasources')
        return Promise.resolve(
          envelope({ items: [{ id: 1, name: 'sr-a', type: 'starrocks' }], total: 1 })
        );
      return Promise.reject(new Error(`unexpected GET ${url}`));
    });

    renderPage();
    await waitFor(() => expect(visibleIds()).toHaveLength(10));

    const nameHeader = screen.getByText('名称').closest('th') as HTMLElement;
    fireEvent.click(nameHeader);
    await waitFor(() => expect(lastParams()).toMatchObject({ sort: 'name', order: 'asc' }));

    fireEvent.click(document.querySelector('.ant-pagination-item-2') as HTMLElement);

    // 翻页请求带 offset=10，且排序参数不被翻页重置（受控 sortOrder）。
    await waitFor(() => expect(visibleIds()).toEqual([11, 12]));
    expect(lastParams()).toMatchObject({ limit: 10, offset: 10, sort: 'name', order: 'asc' });
    expect(document.querySelector('.ant-table-column-sort')?.textContent).toContain('名称');
  });
});
