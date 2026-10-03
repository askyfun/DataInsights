import type { StateCreator } from 'zustand';
import type { ChartConfig, QueryConfig } from './types';

/**
 * 图表构建页的撤销/重做（undo/redo）中间件——零依赖手写时间线。
 *
 * 设计要点：
 * - 只快照「可撤销投影」（维度/指标/筛选/排序 + 图表类型 + 字段元数据），
 *   数据加载、loading、查询结果等非配置态变化不进历史，避免污染栈。
 * - 图表类型来自 chartBuilderConfig.chartType，但标题（title）逐字符编辑不入栈，
 *   否则打字会让 undo 变成「逐字回退」——故投影只取 chartType，恢复时把当前 title 带回去。
 * - 变更判定用引用比较：现有 action 一律以新对象替换对应切片，引用变化即视为一步操作。
 * - 栈深上限，超出丢最旧；任何一次新的可撤销操作清空 redo 分支（标准时间线语义）。
 */

/** 可撤销投影：undo/redo 只搬运这几项，其余状态原样保留。 */
export interface UndoableSnapshot {
  queryConfig: QueryConfig;
  chartType: ChartConfig['chartType'];
  dimensionLabels: Record<string, string>;
  metricAggregations: Record<string, string>;
  metricAliases: Record<string, string>;
  metricUnits: Record<string, string>;
  metricFormats: Record<string, string>;
}

/** 中间件对宿主状态的最小结构要求（AppState 天然满足）。 */
export interface UndoableState {
  queryConfig: QueryConfig;
  chartBuilderConfig: ChartConfig;
  dimensionLabels: Record<string, string>;
  metricAggregations: Record<string, string>;
  metricAliases: Record<string, string>;
  metricUnits: Record<string, string>;
  metricFormats: Record<string, string>;
}

export interface HistoryState {
  past: UndoableSnapshot[];
  future: UndoableSnapshot[];
}

export interface HistoryActions {
  undo: () => void;
  redo: () => void;
  resetHistory: () => void;
}

export type HistoryFields = HistoryState & HistoryActions;

export type WithHistory<S> = S & HistoryFields;

const HISTORY_LIMIT = 50;

function project(s: UndoableState): UndoableSnapshot {
  return {
    queryConfig: s.queryConfig,
    chartType: s.chartBuilderConfig.chartType,
    dimensionLabels: s.dimensionLabels,
    metricAggregations: s.metricAggregations,
    metricAliases: s.metricAliases,
    metricUnits: s.metricUnits,
    metricFormats: s.metricFormats,
  };
}

/** 把投影写回状态：chartType 合入 chartBuilderConfig，保留 title 等其余字段。 */
function applySnapshot(s: UndoableState, snap: UndoableSnapshot): Partial<UndoableState> {
  return {
    queryConfig: snap.queryConfig,
    dimensionLabels: snap.dimensionLabels,
    metricAggregations: snap.metricAggregations,
    metricAliases: snap.metricAliases,
    metricUnits: snap.metricUnits,
    metricFormats: snap.metricFormats,
    chartBuilderConfig: { ...s.chartBuilderConfig, chartType: snap.chartType },
  };
}

function sameSnapshot(a: UndoableSnapshot, b: UndoableSnapshot): boolean {
  return (
    a.queryConfig === b.queryConfig &&
    a.chartType === b.chartType &&
    a.dimensionLabels === b.dimensionLabels &&
    a.metricAggregations === b.metricAggregations &&
    a.metricAliases === b.metricAliases &&
    a.metricUnits === b.metricUnits &&
    a.metricFormats === b.metricFormats
  );
}

export function withHistory<S extends UndoableState & HistoryFields>(
  creator: StateCreator<S, [], [], Omit<S, keyof HistoryFields>>
): StateCreator<S> {
  return (set, get, api) => {
    const rawSet = api.setState.bind(api) as (partial: Partial<S>, replace?: boolean) => void;

    const recordingSet: typeof set = (partial, replace) => {
      const prev = get();
      const delta =
        typeof partial === 'function'
          ? (partial as (state: S) => Partial<S>)(prev)
          : (partial as Partial<S>);
      const merged = { ...prev, ...delta } as S;
      if (sameSnapshot(project(prev), project(merged))) {
        rawSet(delta, replace);
        return;
      }
      const past = [...prev.past, project(prev)];
      if (past.length > HISTORY_LIMIT) past.shift();
      rawSet({ ...delta, past, future: [] } as Partial<S>, replace);
    };

    // 内部 action 经 set 走记录；替换 api.setState 让外部 setState 行为一致。
    api.setState = recordingSet as typeof api.setState;
    const core = creator(recordingSet, get, api) as S;

    return {
      ...core,
      past: [],
      future: [],
      undo: () => {
        const s = get();
        if (s.past.length === 0) return;
        const previous = s.past[s.past.length - 1];
        rawSet({
          ...applySnapshot(s, previous),
          past: s.past.slice(0, -1),
          future: [...s.future, project(s)],
        } as Partial<S>);
      },
      redo: () => {
        const s = get();
        if (s.future.length === 0) return;
        const next = s.future[s.future.length - 1];
        rawSet({
          ...applySnapshot(s, next),
          past: [...s.past, project(s)],
          future: s.future.slice(0, -1),
        } as Partial<S>);
      },
      resetHistory: () => {
        rawSet({ past: [], future: [] } as unknown as Partial<S>);
      },
    };
  };
}
