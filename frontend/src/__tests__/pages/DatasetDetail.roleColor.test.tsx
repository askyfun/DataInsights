// #265 页面渲染断言：数据集详情页字段行的角色开关颜色必须与 roleColor 一致
// （维度蓝 / 指标绿），不再出现「维度落灰」「颜色反转」。
import { render, waitFor } from '@testing-library/react';
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
vi.mock('../../store', () => ({ useStore: vi.fn() }));

import { datasetsApi } from '../../api';
import { useStore } from '../../store';

const mockUseStore = useStore as unknown as ReturnType<typeof vi.fn>;
const mockGetById = datasetsApi.getById as ReturnType<typeof vi.fn>;
const mockGetColumns = datasetsApi.getColumns as ReturnType<typeof vi.fn>;
const mockGetPreview = datasetsApi.getPreview as ReturnType<typeof vi.fn>;

const mockDataset = {
  id: 48,
  name: 'ds',
  datasource_id: 2,
  query_type: 'table',
  table_name: 't',
  created_at: '2026-09-25T14:56:21Z',
};
const columns = [
  { id: 'c1', name: 'region', type: 'string', role: 'dimension', comment: '', expr: 'region' },
  { id: 'c2', name: 'revenue', type: 'float', role: 'metric', comment: '', expr: 'revenue' },
];

describe('DatasetDetail 角色配色（#265）', () => {
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
    mockGetColumns.mockResolvedValue({ data: { data: columns } });
    mockGetPreview.mockResolvedValue({ data: { data: { columns: [], data: [] } } });
  });

  it('维度开关=蓝、指标开关=绿，与 roleColor 一致', async () => {
    render(
      <MemoryRouter initialEntries={['/datasets/48']}>
        <IntlProvider locale="en" messages={{}}>
          <Routes>
            <Route path="/datasets/:id" element={<DatasetDetailPage />} />
          </Routes>
        </IntlProvider>
      </MemoryRouter>
    );
    await waitFor(() => expect(mockGetColumns).toHaveBeenCalledTimes(1));

    const switches = Array.from(document.querySelectorAll('.ant-switch')) as HTMLElement[];
    expect(switches.length).toBeGreaterThanOrEqual(2);
    const bg = switches.map((s) => s.style.backgroundColor);
    expect(bg).toContain('var(--dr-dim)');
    expect(bg).toContain('var(--dr-metric)');
  });
});
