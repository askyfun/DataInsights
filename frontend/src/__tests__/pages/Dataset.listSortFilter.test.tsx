// Dataset 列表页的排序与筛选（2026-09-25 改造）：后端补 ORDER BY id DESC 之后，
// 前端默认视图必须与之同序，否则"列表顺序"在两个层面各说各话。本文件钉死三件事：
//   1) 默认按 id 倒序渲染（不依赖 dataSource 的到达顺序）；
//   2) 名称/时间列的表头点击能真正重排（历史事故：表头挂了非受控 sorter，
//      箭头会动、数据不动）；
//   3) 关键字与数据源筛选真的会收窄结果集，关键字覆盖表名而不只是名称。
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
});

// dataSource 故意按 1,3,2 的乱序喂进来：默认视图必须在客户端自己排成 id 倒序，
// 而不是听天由命地照抄服务端返回的顺序（这正是改造前"顺序随机"的症状）。
const datasets = [
  row(1, 'orders', 1, 'orders_raw', 'table'),
  row(3, 'alpha', 1, 'a_tbl', 'table'),
  row(2, 'zebra', 2, 'z_tbl', 'sql'),
];

beforeEach(() => {
  vi.clearAllMocks();
  instance.get.mockImplementation((url: string) => {
    if (url === '/api/datasets') return Promise.resolve(envelope(datasets));
    if (url === '/api/datasources')
      return Promise.resolve(
        envelope([
          { id: 1, name: 'sr-a', type: 'starrocks' },
          { id: 2, name: 'sr-b', type: 'starrocks' },
        ])
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

/**
 * 选中工具栏第 index 个下拉里的某一项。
 * ⚠️ 不能点 role="option"：rc-select 有一份 height/width=0 的隐藏 listbox 专供
 * 无障碍与测量，其选项的文本是 value 而非 label，点它不会触发 onChange。真正可点
 * 的是门户里 .ant-select-item-option（文本为 label）。
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

describe('Dataset list sorting and filtering', () => {
  it('renders id descending by default, regardless of the incoming array order', async () => {
    renderPage();

    await waitFor(() => expect(visibleIds()).toEqual([3, 2, 1]));
  });

  it('reorders rows when the name header is clicked', async () => {
    renderPage();
    await waitFor(() => expect(visibleIds()).toEqual([3, 2, 1]));

    const nameHeader = screen.getByText('名称').closest('th');
    expect(nameHeader).not.toBeNull();

    // 首次点击 = 升序：alpha(3) → orders(1) → zebra(2)
    fireEvent.click(nameHeader as HTMLElement);
    await waitFor(() => expect(visibleIds()).toEqual([3, 1, 2]));

    // 再次点击 = 降序
    fireEvent.click(nameHeader as HTMLElement);
    await waitFor(() => expect(visibleIds()).toEqual([2, 1, 3]));
  });

  it('matches the keyword against the table name, not just the dataset name', async () => {
    renderPage();
    await waitFor(() => expect(visibleIds()).toHaveLength(3));

    fireEvent.change(screen.getByPlaceholderText('搜索名称或表名'), {
      target: { value: 'a_tbl' },
    });

    await waitFor(() => expect(visibleIds()).toEqual([3]));
  });

  it('narrows rows by datasource', async () => {
    renderPage();
    await waitFor(() => expect(visibleIds()).toHaveLength(3));

    await pickSelectOption(0, 'sr-b');

    await waitFor(() => expect(visibleIds()).toEqual([2]));
  });

  it('narrows rows by query type', async () => {
    renderPage();
    await waitFor(() => expect(visibleIds()).toHaveLength(3));

    // 第二个下拉 = 查询类型。选项文案走 i18n（表 / SQL）。
    await pickSelectOption(1, 'SQL');

    await waitFor(() => expect(visibleIds()).toEqual([2]));
  });
});
