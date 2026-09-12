import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { AxiosResponse } from 'axios';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chartsApi, datasetsApi } from '../../api';
import type { ApiResponse } from '../../lib/api/client';
import ChartBuilder from '../../pages/ChartBuilder';
import { useStore } from '../../store';

interface MockDragStartEvent {
  active: {
    data: {
      current: unknown;
    };
  };
}

interface MockDndContextProps {
  children: React.ReactNode;
  onDragStart?: (event: MockDragStartEvent) => void;
  onDragEnd?: (event: {
    active: { data: { current: unknown } };
    over: { data: { current: unknown } } | null;
  }) => void;
  onDragCancel?: () => void;
}

const dndCallbacks: MockDndContextProps = {
  children: null,
};

function mockAxiosResponse<T>(data: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  return { data, status: 200, statusText: 'OK', headers: {}, config: {} as never };
}

/**
 * 模拟 dnd-kit 上下文，方便测试直接触发拖拽开始/取消事件。
 *
 * 调用场景：ChartBuilder 拖拽预览回归测试。
 * 主要逻辑：保留最近一次渲染传入的回调，并把 DragOverlay 渲染到测试 DOM 中。
 */
vi.mock('@dnd-kit/core', async () => {
  await import('react');

  return {
    DndContext: ({ children, onDragStart, onDragEnd, onDragCancel }: MockDndContextProps) => {
      dndCallbacks.onDragStart = onDragStart;
      dndCallbacks.onDragEnd = onDragEnd;
      dndCallbacks.onDragCancel = onDragCancel;
      return <div data-testid="dnd-context">{children}</div>;
    },
    DragOverlay: ({ children }: { children?: React.ReactNode }) => (
      <div data-testid="drag-overlay-root">{children}</div>
    ),
    PointerSensor: class PointerSensor {},
    useSensor: vi.fn(() => ({})),
    useSensors: vi.fn((...args: unknown[]) => args),
    useDraggable: vi.fn(() => ({
      attributes: {},
      listeners: {},
      setNodeRef: vi.fn(),
      isDragging: false,
    })),
    useDroppable: vi.fn(() => ({
      setNodeRef: vi.fn(),
      isOver: false,
    })),
  };
});

vi.mock('echarts-for-react', () => ({
  default: () => <div data-testid="echarts" />,
}));

vi.mock('../../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api')>();
  return {
    ...actual,
    datasetsApi: {
      ...actual.datasetsApi,
      getAll: vi.fn(),
      getColumns: vi.fn(),
    },
    chartsApi: {
      ...actual.chartsApi,
      getById: vi.fn(),
      executeChartQuery: vi.fn(),
    },
  };
});

const mockGetDatasets = vi.mocked(datasetsApi.getAll);
const mockGetColumns = vi.mocked(datasetsApi.getColumns);
const mockGetChartById = vi.mocked(chartsApi.getById);
const mockExecuteChartQuery = vi.mocked(chartsApi.executeChartQuery);

/**
 * 重置图表构建器测试状态，确保每个用例都从空白拖拽上下文开始。
 */
const resetChartBuilderState = (): void => {
  useStore.getState().resetChartBuilder();
};

/**
 * 渲染 ChartBuilder 编辑页，用于复现字段拖拽预览行为。
 */
const renderChartBuilder = () => {
  return render(
    <MemoryRouter initialEntries={['/chart-builder?edit=1&datasetId=1']}>
      <Routes>
        <Route path="/chart-builder" element={<ChartBuilder />} />
      </Routes>
    </MemoryRouter>
  );
};

describe('ChartBuilder drag overlay', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetChartBuilderState();

    mockGetDatasets.mockResolvedValue(
      mockAxiosResponse({ code: 20000, msg: 'ok', trace: '', data: [] })
    );
    mockGetColumns.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: [{ name: 'region', expr: 'region', type: 'string', comment: '', role: 'dimension' }],
      })
    );
    mockGetChartById.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: {
          id: 1,
          name: 'Sales',
          dataset_id: 1,
          chart_type: 'table',
          config: '{}',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      })
    );
    mockExecuteChartQuery.mockResolvedValue(
      mockAxiosResponse({
        code: 20000,
        msg: 'ok',
        trace: '',
        data: { data: [], select_sql: '', count_sql: '' },
      })
    );
  });

  it('shows a drag overlay preview while dragging a field', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(useStore.getState().chartBuilderFields[0]?.name).toBe('region');
    });

    act(() => {
      dndCallbacks.onDragStart?.({
        active: {
          data: {
            current: {
              type: 'field',
              field: useStore.getState().chartBuilderFields[0],
              fieldType: 'dimension',
            },
          },
        },
      });
    });

    expect(screen.getByTestId('drag-overlay-field')).toHaveTextContent('region');

    act(() => {
      dndCallbacks.onDragCancel?.();
    });

    expect(screen.queryByTestId('drag-overlay-field')).not.toBeInTheDocument();
  });

  it('drops a dimension field into the correct pivot dimension group', async () => {
    renderChartBuilder();

    await waitFor(() => {
      expect(useStore.getState().chartBuilderFields[0]?.name).toBe('region');
    });

    fireEvent.click(screen.getByRole('button', { name: /透视表/ }));

    act(() => {
      dndCallbacks.onDragEnd?.({
        active: {
          data: {
            current: {
              type: 'field',
              field: useStore.getState().chartBuilderFields[0],
              fieldType: 'dimension',
            },
          },
        },
        over: {
          data: {
            current: {
              type: 'dimension',
              groupIndex: 1,
            },
          },
        },
      });
    });

    expect(useStore.getState().queryConfig.dimensionGroups[0]?.fields).toEqual([]);
    expect(useStore.getState().queryConfig.dimensionGroups[1]?.fields).toEqual(['region']);
  });
});
