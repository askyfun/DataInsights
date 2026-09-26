// 创建向导「字段」步的虚拟字段回归（2026-09-26 审查修复）：
//   1) 创建流里新增虚拟字段必须真的进入待提交列表（历史 bug：handleSaveVirtualField
//      只在 editingDataset 分支落状态，创建向导里保存是 no-op）；
//   2) 编辑某个虚拟字段不得波及其他列（列 id 落地后向导期全为空串，
//      按 id 匹配会一次覆写整表——修复为对象引用锁定）。
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
    if (url === '/api/datasets') return Promise.resolve(envelope([]));
    if (url === '/api/datasources')
      return Promise.resolve(envelope([{ id: 1, name: 'PG-1', type: 'postgresql' }]));
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
  instance.post.mockImplementation((url: string) => {
    if (url === '/api/datasources/1/preview')
      return Promise.resolve(envelope({ columns: ['a'], rows: [] }));
    return Promise.reject(new Error(`unexpected POST ${url}`));
  });
});

/** rc-select 的 placeholder 文本渲染在 .ant-select 内；mouseDown 它即可展开下拉。 */
const openSelectByPlaceholder = (placeholder: string) => {
  const node = screen.getByText(placeholder).closest('.ant-select');
  expect(node).not.toBeNull();
  fireEvent.mouseDown(node!.querySelector('.ant-select-selector') ?? node!);
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

/** 打开创建向导（step 0），填 SQL 模式的必填项并进入「字段」步。 */
const openWizardAtFieldsStep = async () => {
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

  fireEvent.change(screen.getByPlaceholderText('请输入名称'), {
    target: { value: 'wizard-test' },
  });
  openSelectByPlaceholder('请选择数据源');
  await pickOption('PG-1');
  // 查询类型有初始值「表」，无 placeholder 可抓；弹窗内第二个 combobox 即它
  const modalComboboxes = document.querySelectorAll('.ant-modal [role="combobox"]');
  fireEvent.mouseDown(modalComboboxes[1]);
  await pickOption('SQL');
  fireEvent.change(screen.getByPlaceholderText('请输入 SQL 查询'), {
    target: { value: 'SELECT 1' },
  });
  // SQL 变更走 100ms setTimeout 触发 handleTableOrSqlChange（会清空 datasetColumns），
  // 必须等它落地再进「字段」步，否则它会晚于添加的虚拟字段执行、把列表抹掉
  await new Promise((resolve) => setTimeout(resolve, 200));
  fireEvent.click(screen.getByRole('button', { name: /下\s*一\s*步/ }));

  await screen.findByRole('button', { name: /添加虚拟字段/ });
};

/** 在虚拟字段弹窗里填一项并提交。 */
const submitVirtualField = async (name: string, expr: string, submitName: RegExp) => {
  fireEvent.change(screen.getByPlaceholderText('e.g., total_price'), {
    target: { value: name },
  });
  fireEvent.change(screen.getByPlaceholderText('请输入表达式'), { target: { value: expr } });
  fireEvent.click(screen.getByRole('button', { name: submitName }));
};

const fieldRows = () =>
  Array.from(document.querySelectorAll('tbody tr')).filter((tr) =>
    tr.querySelector('button.ant-btn-dangerous')
  );

describe('Dataset wizard virtual fields', () => {
  it('keeps added virtual fields in the wizard list', async () => {
    await openWizardAtFieldsStep();
    fireEvent.click(screen.getByRole('button', { name: /添加虚拟字段/ }));
    await submitVirtualField('a', '[x] + 1', /新\s*增/);

    await waitFor(() => expect(fieldRows().length).toBe(1));
    expect(fieldRows()[0].textContent).toContain('[x] + 1');
  });

  it('editing one virtual field does not overwrite the others', { timeout: 15_000 }, async () => {
    await openWizardAtFieldsStep();
    fireEvent.click(screen.getByRole('button', { name: /添加虚拟字段/ }));
    await submitVirtualField('a', '[x] + 1', /新\s*增/);
    await waitFor(() => expect(fieldRows().length).toBe(1));

    fireEvent.click(screen.getByRole('button', { name: /添加虚拟字段/ }));
    await submitVirtualField('b', '[y] * 2', /新\s*增/);
    await waitFor(() => expect(fieldRows().length).toBe(2));

    // 编辑第一行（name 'a'）：向导期两行 id 都是空串，按 id 匹配会把 b 一起覆写。
    // 行内按钮顺序：[Switch(角色), 编辑, 删除]，编辑取第一个 text 按钮（删除带 dangerous）
    const rowA = fieldRows().find((tr) => tr.textContent?.includes('a'));
    expect(rowA).toBeTruthy();
    fireEvent.click(rowA!.querySelector('button.ant-btn-text')!);
    await submitVirtualField('a', '[x] + 99', /更\s*新/);

    await waitFor(() => {
      const texts = fieldRows().map((tr) => tr.textContent);
      expect(texts.some((t) => t?.includes('[x] + 99'))).toBe(true);
    });
    const texts = fieldRows().map((tr) => tr.textContent);
    expect(texts.some((t) => t?.includes('[y] * 2') && t?.includes('b'))).toBe(true);
    expect(fieldRows().length).toBe(2);
  });
});
