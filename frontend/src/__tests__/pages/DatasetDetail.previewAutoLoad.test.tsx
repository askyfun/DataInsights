import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import DatasetDetailPage from '../../pages/DatasetDetail';

// 数据预览 tab 自动加载（此前必须手点刷新，空态误导为无数据）：
// 切到 preview tab 时应自动发一次 GET /api/datasets/:id/preview，且仅一次（已加载过不重复发）。

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

const mockDataset = {
  id: 48,
  name: '温度数据集',
  description: '',
  datasource_id: 2,
  query_type: 'table',
  table_name: 'raw_city_weather_2026',
  created_at: '2026-09-25T14:56:21Z',
};

const mockColumns = [
  { id: 'c1', name: 'date', type: 'string', role: 'dimension', comment: '' },
  { id: 'c2', name: 'temp_max', type: 'float', role: 'metric', comment: '' },
];

const mockPreview = {
  columns: ['date', 'temp_max'],
  data: [{ date: '2026-01-01T00:00:00Z', temp_max: 6 }],
};

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

describe('DatasetDetailPage 数据预览自动加载', () => {
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
    mockGetColumns.mockResolvedValue({ data: { data: mockColumns } });
    mockGetPreview.mockResolvedValue({ data: { data: mockPreview } });
  });

  it('切到数据预览 tab 自动发一次 preview 请求', async () => {
    renderPage();
    // 等初始 dataset/columns 加载完成（页面先渲染 Spin）
    await screen.findByRole('tab', { name: /dataset.detail.tabFields/ });
    // 初始不在 preview tab，不应提前加载
    expect(mockGetPreview).not.toHaveBeenCalled();

    // antd Tabs 的 tab 标签（i18n key 直显）
    fireEvent.click(screen.getByRole('tab', { name: /dataset.detail.tabPreview/ }));

    await waitFor(() => {
      expect(mockGetPreview).toHaveBeenCalledTimes(1);
    });
  });

  it('预览已加载后再次切 tab 不重复请求', async () => {
    renderPage();
    await screen.findByRole('tab', { name: /dataset.detail.tabFields/ });
    fireEvent.click(screen.getByRole('tab', { name: /dataset.detail.tabPreview/ }));
    await waitFor(() => {
      expect(mockGetPreview).toHaveBeenCalledTimes(1);
    });

    // 切走再切回
    fireEvent.click(screen.getByRole('tab', { name: /dataset.detail.tabFields/ }));
    fireEvent.click(screen.getByRole('tab', { name: /dataset.detail.tabPreview/ }));

    await waitFor(() => {
      // 稳定后仍只调过 1 次（无新请求）
      expect(mockGetPreview).toHaveBeenCalledTimes(1);
    });
  });
});
