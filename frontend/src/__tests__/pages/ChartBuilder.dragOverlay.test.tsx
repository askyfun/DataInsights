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
        data: [
          { name: 'region', expr: 'region', type: 'string', comment: '', role: 'dimension' },
          { name: 'city', expr: 'city', type: 'string', comment: '', role: 'dimension' },
          { name: 'revenue', expr: 'revenue', type: 'integer', comment: '', role: 'metric' },
        ],
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

    expect(useStore.getState().queryConfig.dimensionGroups[0]?.bindings).toEqual([]);
    expect(useStore.getState().queryConfig.dimensionGroups[1]?.bindings).toEqual([
      { bindingId: 'b-0', field: 'region' },
    ]);
  });

  /**
   * 查询配置区内部的"搬字段"：拖起的是已有字段标签（binding-source），不是左侧字段。
   * 这三条用例锁死三条真实路径：透视表行维度↔列维度、组内换序、跨 kind 拒绝。
   */
  describe('配置区字段标签互相拖拽', () => {
    const bindingDrag = (bindingId: string, kind: 'dimension' | 'metric', groupIndex: number) => ({
      active: {
        data: {
          current: {
            type: 'binding-source',
            bindingId,
            kind,
            groupIndex,
            label: bindingId,
            color: 'blue',
          },
        },
      },
    });

    const seedPivotRows = () => {
      fireEvent.click(screen.getByRole('button', { name: /透视表/ }));
      act(() => {
        const state = useStore.getState();
        const fields = state.chartBuilderFields;
        state.addDimensionField(fields[0], 0); // region → b-0 @ 行维度
        state.addDimensionField(fields[1], 0); // city   → b-1 @ 行维度
        state.addMetricField(fields[2], 0); // revenue → b-2 @ 值指标
      });
    };

    it('把行维度拖到列维度：绑定换组而不是复制', async () => {
      renderChartBuilder();
      await waitFor(() => {
        expect(useStore.getState().chartBuilderFields).toHaveLength(3);
      });
      seedPivotRows();

      act(() => {
        dndCallbacks.onDragEnd?.({
          ...bindingDrag('b-0', 'dimension', 0),
          over: { data: { current: { type: 'dimension', groupIndex: 1 } } },
        });
      });

      const groups = useStore.getState().queryConfig.dimensionGroups;
      expect(groups[0].bindings).toEqual([{ bindingId: 'b-1', field: 'city' }]);
      expect(groups[1].bindings).toEqual([{ bindingId: 'b-0', field: 'region' }]);
    });

    it('落在同组另一个字段标签上时组内换序', async () => {
      renderChartBuilder();
      await waitFor(() => {
        expect(useStore.getState().chartBuilderFields).toHaveLength(3);
      });
      seedPivotRows();

      act(() => {
        dndCallbacks.onDragEnd?.({
          ...bindingDrag('b-1', 'dimension', 0),
          over: {
            data: {
              current: {
                type: 'binding-slot',
                kind: 'dimension',
                groupIndex: 0,
                index: 0,
                bindingId: 'b-0',
              },
            },
          },
        });
      });

      expect(
        useStore.getState().queryConfig.dimensionGroups[0].bindings.map((b) => b.bindingId)
      ).toEqual(['b-1', 'b-0']);
    });

    it('维度拖进指标组被拒绝，两边状态都不变', async () => {
      renderChartBuilder();
      await waitFor(() => {
        expect(useStore.getState().chartBuilderFields).toHaveLength(3);
      });
      seedPivotRows();

      act(() => {
        dndCallbacks.onDragEnd?.({
          ...bindingDrag('b-0', 'dimension', 0),
          over: { data: { current: { type: 'metric', groupIndex: 0 } } },
        });
      });

      const config = useStore.getState().queryConfig;
      expect(config.dimensionGroups[0].bindings.map((b) => b.bindingId)).toEqual(['b-0', 'b-1']);
      expect(config.metricGroups[0].bindings.map((b) => b.bindingId)).toEqual(['b-2']);
    });

    it('指标组之间移动（值指标组之间）', async () => {
      renderChartBuilder();
      await waitFor(() => {
        expect(useStore.getState().chartBuilderFields).toHaveLength(3);
      });
      seedPivotRows();
      act(() => {
        useStore.getState().addMetricGroup();
      });

      act(() => {
        dndCallbacks.onDragEnd?.({
          ...bindingDrag('b-2', 'metric', 0),
          over: { data: { current: { type: 'metric', groupIndex: 1 } } },
        });
      });

      const metricGroups = useStore.getState().queryConfig.metricGroups;
      expect(metricGroups[0].bindings).toEqual([]);
      expect(metricGroups[1].bindings).toEqual([{ bindingId: 'b-2', field: 'revenue' }]);
    });
  });

  /**
   * 过滤字段组：从左侧字段列表拖入即在过滤区追加一条条件。
   * 与维度/指标组共用同一套落点协议（data.type === 'filter'），这里锁死两件事：
   * 条件按列名记录、多条件恒为「且」（logic 恒 and），且同一字段可重复加入。
   */
  describe('过滤字段组拖入', () => {
    const fieldDrag = (name: string) => {
      const field = useStore.getState().chartBuilderFields.find((item) => item.name === name);
      if (!field) {
        throw new Error(`字段 ${name} 尚未加载`);
      }
      return {
        active: { data: { current: { type: 'field', field, fieldType: field.type } } },
      };
    };

    const dropIntoFilterZone = () => ({
      over: { data: { current: { type: 'filter', groupIndex: 0 } } },
    });

    it('拖入维度字段即新增一条「且」条件', async () => {
      renderChartBuilder();
      await waitFor(() => {
        expect(useStore.getState().chartBuilderFields).toHaveLength(3);
      });

      act(() => {
        dndCallbacks.onDragEnd?.({ ...fieldDrag('region'), ...dropIntoFilterZone() });
      });

      const filters = useStore.getState().queryConfig.filters;
      expect(filters).toHaveLength(1);
      expect(filters[0]).toMatchObject({ field: 'region', operator: 'eq', logic: 'and' });
    });

    it('拖入指标字段同样成立（维度与指标都可参与过滤）', async () => {
      renderChartBuilder();
      await waitFor(() => {
        expect(useStore.getState().chartBuilderFields).toHaveLength(3);
      });

      act(() => {
        dndCallbacks.onDragEnd?.({ ...fieldDrag('revenue'), ...dropIntoFilterZone() });
      });

      expect(useStore.getState().queryConfig.filters[0]).toMatchObject({
        field: 'revenue',
        logic: 'and',
      });
    });

    it('同一字段连续拖两次得到两条独立条件（区间筛选）', async () => {
      renderChartBuilder();
      await waitFor(() => {
        expect(useStore.getState().chartBuilderFields).toHaveLength(3);
      });

      act(() => {
        dndCallbacks.onDragEnd?.({ ...fieldDrag('revenue'), ...dropIntoFilterZone() });
        dndCallbacks.onDragEnd?.({ ...fieldDrag('revenue'), ...dropIntoFilterZone() });
      });

      const filters = useStore.getState().queryConfig.filters;
      expect(filters.map((filter) => filter.field)).toEqual(['revenue', 'revenue']);
      expect(filters[0].id).not.toBe(filters[1].id);
    });
  });
});
