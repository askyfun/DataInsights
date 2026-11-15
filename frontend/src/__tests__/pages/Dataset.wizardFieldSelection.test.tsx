// #267 P0 回归：创建向导「字段」步取消勾选某字段后，保存必须让它从数据集里消失。
// 修复前把「取消勾选」实现成给列打 visible:false 标记，仍把全部字段发给 updateColumns
// （后端无 visible 字段、被静默丢弃）→ 字段删不掉。修复后按勾选状态过滤后再提交，
// 且不再把 visible 带进契约。
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

class FakeResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', FakeResizeObserver);

const envelope = (data: unknown) => ({ data: { code: 20000, msg: 'success', trace: '', data } });

beforeEach(() => {
  vi.clearAllMocks();
  instance.get.mockImplementation((url: string) => {
    if (url === '/api/datasets') return Promise.resolve(envelope({ items: [], total: 0 }));
    if (url === '/api/datasources')
      return Promise.resolve(
        envelope({ items: [{ id: 1, name: 'PG-1', type: 'postgresql' }], total: 1 })
      );
    if (url === '/api/datasources/1/tables') return Promise.resolve(envelope([{ name: 'orders' }]));
    if (url === '/api/datasources/1/tables/orders/columns')
      return Promise.resolve(
        envelope([
          { name: 'id', data_type: 'int4', comment: '' },
          { name: 'amount', data_type: 'numeric', comment: '' },
          { name: 'note', data_type: 'varchar', comment: '' },
        ])
      );
    if (url.startsWith('/api/datasources/1/tables/orders/data'))
      return Promise.resolve(envelope({ columns: ['id', 'amount', 'note'], rows: [] }));
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
  instance.post.mockImplementation((url: string) => {
    if (url === '/api/datasets')
      return Promise.resolve(
        envelope({
          id: 99,
          name: 'wizard-test',
          datasource_id: 1,
          query_type: 'table',
          table_name: 'orders',
          created_at: '2026-11-15T00:00:00Z',
        })
      );
    if (url === '/api/datasets/99/columns') return Promise.resolve(envelope({ id: 99 }));
    return Promise.reject(new Error(`unexpected POST ${url}`));
  });
});

const openSelectByPlaceholder = (placeholder: string) => {
  const node = screen.getByText(placeholder).closest('.ant-select');
  if (!node) throw new Error(`select for ${placeholder} not found`);
  fireEvent.mouseDown(node.querySelector('.ant-select-selector') ?? node);
};

const pickOption = async (label: string) => {
  const option = await waitFor(() => {
    const found = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (node) => node.textContent === label && !node.closest('.ant-select-dropdown-hidden')
    );
    expect(found).toBeTruthy();
    return found as HTMLElement;
  });
  fireEvent.click(option);
};

/** 打开创建向导（表模式），选数据源 + 表，进入「字段」步。 */
const openWizardAtFieldsStepTableMode = async () => {
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <MemoryRouter initialEntries={['/datasets']}>
        <Routes>
          <Route path="/datasets" element={<DatasetPage />} />
        </Routes>
      </MemoryRouter>
    </IntlProvider>
  );

  fireEvent.click(await screen.findByRole('button', { name: /创建数据集/ }));
  fireEvent.change(screen.getByPlaceholderText('请输入名称'), { target: { value: 'wizard-test' } });
  openSelectByPlaceholder('请选择数据源');
  await pickOption('PG-1');

  // 表选择（默认查询类型就是「表」）
  await waitFor(() => expect(instance.get).toHaveBeenCalledWith('/api/datasources/1/tables'));
  openSelectByPlaceholder('请选择表');
  await pickOption('orders');

  // 表/预览是 setTimeout(100) 触发的，等它落地再进下一步
  await new Promise((resolve) => setTimeout(resolve, 250));
  fireEvent.click(screen.getByRole('button', { name: /下\s*一\s*步/ }));
  await screen.findByRole('button', { name: /保存并提交/ });
};

describe('Dataset 创建向导字段勾选（#267）', () => {
  it('取消勾选的字段保存时真正被剔除，且不携带 visible', async () => {
    await openWizardAtFieldsStepTableMode();

    // 表里应出现 3 个字段行
    const rowFor = (name: string) =>
      Array.from(document.querySelectorAll('tbody tr')).find((tr) =>
        tr.textContent?.includes(name)
      ) as HTMLElement | undefined;
    await waitFor(() => expect(rowFor('note')).toBeTruthy());

    // 取消勾选 note
    const noteRow = rowFor('note') as HTMLElement;
    const checkbox = noteRow.querySelector('input[type="checkbox"]') as HTMLInputElement;
    expect(checkbox).toBeTruthy();
    fireEvent.click(checkbox);

    fireEvent.click(screen.getByRole('button', { name: /保存并提交/ }));

    await waitFor(() => {
      const call = instance.post.mock.calls.find((c) => c[0] === '/api/datasets/99/columns');
      expect(call).toBeTruthy();
    });
    const sent = instance.post.mock.calls.find(
      (c) => c[0] === '/api/datasets/99/columns'
    )?.[1] as Array<{ name: string; visible?: boolean }>;

    // 修复前：sent 包含 note（只标 visible:false）；修复后：note 应被剔除
    expect(sent.map((c) => c.name)).toEqual(['id', 'amount']);
    expect(sent.some((c) => c.visible !== undefined)).toBe(false);
  });
});
