// #267 P0 回归：数据集详情页原先只有「虚拟字段」有编辑/删除入口，物理字段的「操作」列
// 直接 return null，用户无法删除物理字段。修复后：每个字段行都有删除按钮，删除走
// filter + updateColumns 全量提交（与虚拟字段删除同范式）；且不允许删到 0 个字段。
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import DatasetDetailPage from '../../pages/DatasetDetail';

vi.mock('../../api', () => ({
  datasetsApi: {
    getById: vi.fn(),
    getColumns: vi.fn(),
    getPreview: vi.fn(),
    updateColumns: vi.fn(),
  },
}));

vi.mock('../../store', () => ({
  useStore: vi.fn(),
}));

import { datasetsApi } from '../../api';
import { useStore } from '../../store';

const mockUseStore = useStore as unknown as ReturnType<typeof vi.fn>;
const mockGetById = datasetsApi.getById as ReturnType<typeof vi.fn>;
const mockGetColumns = datasetsApi.getColumns as ReturnType<typeof vi.fn>;
const mockGetPreview = datasetsApi.getPreview as ReturnType<typeof vi.fn>;
const mockUpdateColumns = datasetsApi.updateColumns as ReturnType<typeof vi.fn>;

const mockDataset = {
  id: 48,
  name: '温度数据集',
  description: '',
  datasource_id: 2,
  query_type: 'table',
  table_name: 'raw_city_weather_2026',
  created_at: '2026-09-25T14:56:21Z',
};

// 两个物理列（expr 恰等于列名本身）
const physicalColumns = [
  { id: 'c1', name: 'date', type: 'date', role: 'dimension', comment: '', expr: 'date' },
  { id: 'c2', name: 'temp_max', type: 'float', role: 'metric', comment: '', expr: 'temp_max' },
];

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/datasets/48']}>
      <IntlProvider locale="en" messages={{}}>
        <Routes>
          <Route path="/datasets/:id" element={<DatasetDetailPage />} />
        </Routes>
      </IntlProvider>
    </MemoryRouter>
  );
}

const confirmPopconfirm = async () => {
  const ok = await waitFor(() => {
    // antd Popconfirm 的确认按钮文案来自 okText（common.yes，空 messages 下渲染为键名）
    const btns = Array.from(document.querySelectorAll('.ant-popconfirm-buttons button'));
    const found = btns.find((b) => b.textContent?.includes('common.yes'));
    expect(found).toBeTruthy();
    return found as HTMLElement;
  });
  fireEvent.click(ok);
};

describe('DatasetDetail 物理字段删除（#267）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseStore.mockReturnValue({
      datasets: [mockDataset],
      fetchDatasets: vi.fn(),
      datasources: [],
      fetchDatasources: vi.fn(),
      deleteDataset: vi.fn(),
    });
    mockGetById.mockResolvedValue({ data: { data: mockDataset } });
    mockGetColumns.mockResolvedValue({ data: { data: physicalColumns } });
    mockGetPreview.mockResolvedValue({ data: { data: { columns: [], data: [] } } });
    mockUpdateColumns.mockResolvedValue({ data: { data: physicalColumns } });
  });

  it('物理字段行也有删除入口', async () => {
    renderPage();
    await waitFor(() => expect(mockGetColumns).toHaveBeenCalledTimes(1));
    // 修复前：物理字段无任何删除按钮（render 返回 null）
    expect(screen.getByTestId('field-delete-c1')).toBeTruthy();
    expect(screen.getByTestId('field-delete-c2')).toBeTruthy();
  });

  it('删除物理字段走 filter + updateColumns 全量提交', async () => {
    renderPage();
    await waitFor(() => expect(mockGetColumns).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByTestId('field-delete-c1'));
    await confirmPopconfirm();

    await waitFor(() => expect(mockUpdateColumns).toHaveBeenCalledTimes(1));
    const sent = mockUpdateColumns.mock.calls[0][1] as Array<{ id: string; name: string }>;
    expect(sent.map((c) => c.id)).toEqual(['c2']);
    // 不应把 visible 之类字段带进契约
    expect(sent.some((c) => Object.prototype.hasOwnProperty.call(c, 'visible'))).toBe(false);
  });

  it('只剩一个字段时不允许删除（给出提示、不发请求）', async () => {
    mockGetColumns.mockResolvedValue({ data: { data: [physicalColumns[0]] } });
    renderPage();
    await waitFor(() => expect(mockGetColumns).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByTestId('field-delete-c1'));
    await confirmPopconfirm();

    // 给异步一次机会；不应发出 updateColumns
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(mockUpdateColumns).not.toHaveBeenCalled();
  });
});
