// 虚拟字段保存后的状态回填（2026-09-26 审查修复）：
// updateColumns 成功后必须从服务端重取列，而不是把本地副本（新增列 id 仍是空串）
// 直接落进状态——否则下一次编辑/删除按 id 匹配会命中所有未回填的行。
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

const serverColumns = [
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

describe('DatasetDetailPage 虚拟字段保存后重取列', () => {
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
    mockGetColumns.mockResolvedValue({ data: { data: serverColumns } });
    mockGetPreview.mockResolvedValue({ data: { data: { columns: [], data: [] } } });
    mockUpdateColumns.mockResolvedValue({ data: { data: serverColumns } });
  });

  it('保存新增虚拟字段后重新 GET 列（本地不留空串 id 的副本）', async () => {
    renderPage();
    await screen.findByRole('tab', { name: /dataset.detail.tabFields/ });
    await waitFor(() => expect(mockGetColumns).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole('button', { name: /virtualField.add/ }));
    fireEvent.change(screen.getByPlaceholderText('e.g., total_price'), {
      target: { value: 'delta' },
    });
    fireEvent.change(screen.getByPlaceholderText('e.g., price * quantity'), {
      target: { value: '[temp_max] - [date]' },
    });
    // 页面上还有别的 common.save（如分片配置），限定在虚拟字段弹窗内点保存
    const modal = screen
      .getByPlaceholderText('e.g., total_price')
      .closest('.ant-modal') as HTMLElement;
    const saveButton = Array.from(modal.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('common.save')
    );
    fireEvent.click(saveButton!);

    await waitFor(() => expect(mockUpdateColumns).toHaveBeenCalledTimes(1));
    // 修复前：setColumns(本地数组) 后不再发 GET；修复后必须重取
    await waitFor(() => expect(mockGetColumns).toHaveBeenCalledTimes(2));

    const sentColumns = mockUpdateColumns.mock.calls[0][1];
    expect(sentColumns.some((col: { name: string }) => col.name === 'delta')).toBe(true);
  });
});
