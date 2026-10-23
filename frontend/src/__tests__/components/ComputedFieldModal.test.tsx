import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ComputedFieldModal from '../../components/ChartBuilder/ComputedFieldModal';
import { messages } from '../../i18n/useLocale';

vi.mock('../../api', () => ({
  datasetsApi: {
    getColumns: vi.fn(),
    updateColumns: vi.fn(),
  },
}));

import { datasetsApi } from '../../api';

const mockGetColumns = datasetsApi.getColumns as ReturnType<typeof vi.fn>;
const mockUpdateColumns = datasetsApi.updateColumns as ReturnType<typeof vi.fn>;

const serverColumns = [
  { id: 'c1', name: 'price', type: 'float', role: 'metric', comment: '', expr: 'price' },
  { id: 'c2', name: 'quantity', type: 'int', role: 'metric', comment: '', expr: 'quantity' },
];

function renderModal(onSaved = vi.fn()) {
  render(
    <IntlProvider locale="en-US" messages={messages['en-US']}>
      <ComputedFieldModal open datasetId={7} onClose={vi.fn()} onSaved={onSaved} />
    </IntlProvider>
  );
  return { onSaved };
}

describe('ComputedFieldModal（画布内新建计算字段）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetColumns.mockResolvedValue({ data: { data: serverColumns } });
    mockUpdateColumns.mockResolvedValue({ data: { data: serverColumns } });
  });

  it('保存时把新列追加到最新列数组并重拉字段', async () => {
    const { onSaved } = renderModal();
    await waitFor(() => expect(mockGetColumns).toHaveBeenCalledWith(7));

    fireEvent.change(await screen.findByTestId('computed-field-name'), {
      target: { value: 'revenue' },
    });
    fireEvent.change(screen.getByTestId('expression-input'), {
      target: { value: 'price * quantity' },
    });
    fireEvent.click(screen.getByTestId('computed-field-save'));

    await waitFor(() => expect(mockUpdateColumns).toHaveBeenCalledTimes(1));
    const [datasetId, columns] = mockUpdateColumns.mock.calls[0];
    expect(datasetId).toBe(7);
    expect(columns).toHaveLength(3);
    expect(columns[2]).toMatchObject({
      id: '',
      name: 'revenue',
      type: 'float',
      role: 'metric',
      expr: 'price * quantity',
    });
    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
  });

  it('表达式含未知字段时不发请求', async () => {
    renderModal();
    await waitFor(() => expect(mockGetColumns).toHaveBeenCalled());

    fireEvent.change(await screen.findByTestId('computed-field-name'), {
      target: { value: 'bad' },
    });
    fireEvent.change(screen.getByTestId('expression-input'), {
      target: { value: 'price * missing' },
    });
    fireEvent.click(screen.getByTestId('computed-field-save'));

    // 前端校验拦截；即便绕过，后端 UpdateColumns 的 expr 闸仍是最终防线。
    await waitFor(() =>
      expect(screen.getByTestId('expression-status').textContent).toContain('missing')
    );
    expect(mockUpdateColumns).not.toHaveBeenCalled();
  });
});
