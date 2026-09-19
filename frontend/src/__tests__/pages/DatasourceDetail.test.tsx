import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import DatasourceDetailPage from '../../pages/DatasourceDetail';

// Mock the API module
vi.mock('../../api', () => ({
  datasourcesApi: {
    getTables: vi.fn(),
    getTableColumns: vi.fn(),
    getTableData: vi.fn(),
  },
}));

// Mock the store
vi.mock('../../store', () => ({
  useStore: vi.fn(),
}));

import { datasourcesApi } from '../../api';
import { useStore } from '../../store';

const mockUseStore = useStore as unknown as ReturnType<typeof vi.fn>;
const mockGetTables = datasourcesApi.getTables as ReturnType<typeof vi.fn>;
const mockGetTableColumns = datasourcesApi.getTableColumns as ReturnType<typeof vi.fn>;
const mockGetTableData = datasourcesApi.getTableData as ReturnType<typeof vi.fn>;

const mockDatasource = {
  id: 1,
  name: 'Test DB',
  type: 'postgresql',
  host: 'localhost',
  port: 5432,
  database_name: 'testdb',
  username: 'admin',
  password: '',
};

// Backend returns lowercase JSON fields
const mockTables = [
  { name: 'users', comment: 'User table' },
  { name: 'orders', comment: 'Order table' },
];

const mockColumns = [
  { name: 'id', data_type: 'int', comment: 'Primary key' },
  { name: 'email', data_type: 'varchar', comment: 'User email' },
];

// renderPage renders the datasource detail route with a concrete datasource id so tests can
// exercise route params, store lookup, and table preview interactions together.
function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/datasources/1']}>
      <IntlProvider locale="en" messages={{}}>
        <Routes>
          <Route path="/datasources/:id" element={<DatasourceDetailPage />} />
        </Routes>
      </IntlProvider>
    </MemoryRouter>
  );
}

describe('DatasourceDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseStore.mockReturnValue({
      datasources: [mockDatasource],
      fetchDatasources: vi.fn(),
    });
    mockGetTables.mockResolvedValue({ data: { data: mockTables } });
    mockGetTableColumns.mockResolvedValue({ data: { data: mockColumns } });
    mockGetTableData.mockResolvedValue({
      data: {
        data: {
          columns: ['id'],
          data: null,
          total: 0,
          primary_keys: [],
          page: 1,
          page_size: 20,
        },
      },
    });
  });

  it('should render table names from backend lowercase fields', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('users')).toBeInTheDocument();
      expect(screen.getByText('orders')).toBeInTheDocument();
    });
  });

  it('should render table comments from backend lowercase fields', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('User table')).toBeInTheDocument();
      expect(screen.getByText('Order table')).toBeInTheDocument();
    });
  });

  it('should render column details when table is expanded', async () => {
    renderPage();

    // Wait for tables to load
    await waitFor(() => {
      expect(screen.getByText('users')).toBeInTheDocument();
    });

    // Click expand button for first table
    const expandButtons = document.querySelectorAll('.ant-table-row-expand-icon');
    (expandButtons[0] as HTMLElement).click();

    await waitFor(() => {
      expect(mockGetTableColumns).toHaveBeenCalledWith(1, 'users');
    });

    await waitFor(() => {
      expect(screen.getByText('id')).toBeInTheDocument();
      expect(screen.getByText('int')).toBeInTheDocument();
      expect(screen.getByText('Primary key')).toBeInTheDocument();
      expect(screen.getByText('email')).toBeInTheDocument();
      expect(screen.getByText('varchar')).toBeInTheDocument();
    });
  });

  it('should render an empty preview when backend returns null row data', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('users')).toBeInTheDocument();
    });

    fireEvent.click(screen.getAllByRole('button', { name: /Preview/i })[0]);

    await waitFor(() => {
      expect(mockGetTableData).toHaveBeenCalledWith(1, 'users', 1, 20, '', 'asc');
    });

    await waitFor(() => {
      // antd Table 空数据时不渲染分页条，因此没有 "0 rows" 文本可断言
      expect(screen.getAllByText('No data').length).toBeGreaterThan(0);
    });
  });
});
