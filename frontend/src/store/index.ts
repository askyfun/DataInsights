import { create } from 'zustand';
import { type ChartBuilderSlice, createChartBuilderSlice } from './chartBuilder';
import { type ChartsSlice, createChartsSlice } from './charts';
import { createDatasetsSlice, type DatasetsSlice } from './datasets';
import { createDatasourcesSlice, type DatasourcesSlice } from './datasources';
import { type HistoryFields, withHistory } from './history';

// 类型与纯工具仍从 store 入口对外暴露：所有消费方（页面/组件/测试）的
// `import { useStore, ChartField, ... } from '@/store'` 保持原样，拆分对它们是透明的。
export type {
  BindingDropTarget,
  BindingInstance,
  BindingLocation,
  BoundField,
  ChartConfig,
  ChartField,
  ChartQueryOptions,
  ChartStyleConfig,
  ComparisonConfig,
  FieldGroup,
  FieldType,
  FilterCondition,
  FilterOperator,
  MoveBindingResult,
  QueryConfig,
  ReferenceLine,
  TopNConfig,
} from './types';
export { nextBindingId, reconcileGroupBindings } from './types';

/**
 * 应用唯一状态树：四个领域切片（数据源 / 数据集 / 图表 / 图表构建器）+ 撤销重做时间线。
 *
 * 切片只是**代码组织**上的拆分：它们共用同一个 `create`（同一个 `set`），
 * 合成后仍是改动前的单一 store，因此 `useStore.getState()/setState()`、跨切片读写
 * （如 removeDimensionGroup 同时改 queryConfig 与元数据 Record）与撤销重做的
 * 快照投影语义都保持不变。
 */
export interface AppState
  extends DatasourcesSlice,
    DatasetsSlice,
    ChartsSlice,
    ChartBuilderSlice,
    HistoryFields {}

export const useStore = create<AppState>()(
  withHistory((...args) => ({
    ...createDatasourcesSlice(...args),
    ...createDatasetsSlice(...args),
    ...createChartsSlice(...args),
    ...createChartBuilderSlice(...args),
  }))
);

export default useStore;
