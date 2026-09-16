import type { ChartConfig, QueryConfig } from '@/store';

export type BuilderChartType = ChartConfig['chartType'];
export type FieldGroupKind = 'dimension' | 'metric';

export interface ChartFieldGroupDefinition {
  id: string;
  kind: FieldGroupKind;
  label: string;
  emptyText: string;
  minGroups: number;
}

export interface ChartDefinition {
  type: BuilderChartType;
  label: string;
  fieldGroups: ChartFieldGroupDefinition[];
}

export const chartDefinitions: Record<BuilderChartType, ChartDefinition> = {
  table: {
    type: 'table',
    label: '表格',
    fieldGroups: [
      {
        id: 'dimensions',
        kind: 'dimension',
        label: '维度',
        emptyText: '拖拽维度字段到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'metrics',
        kind: 'metric',
        label: '指标',
        emptyText: '拖拽指标字段到此，或点击+添加',
        minGroups: 1,
      },
    ],
  },
  bar: {
    type: 'bar',
    label: '柱状图',
    fieldGroups: [
      {
        id: 'x_axis',
        kind: 'dimension',
        label: 'X 轴维度',
        emptyText: '拖拽 X 轴维度到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'values',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽数值指标到此，或点击+添加',
        minGroups: 1,
      },
    ],
  },
  line: {
    type: 'line',
    label: '折线图',
    fieldGroups: [
      {
        id: 'x_axis',
        kind: 'dimension',
        label: 'X 轴维度',
        emptyText: '拖拽 X 轴维度到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'values',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽数值指标到此，或点击+添加',
        minGroups: 1,
      },
    ],
  },
  pie: {
    type: 'pie',
    label: '饼图',
    fieldGroups: [
      {
        id: 'category',
        kind: 'dimension',
        label: '分类',
        emptyText: '拖拽分类维度到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'value',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽数值指标到此，或点击+添加',
        minGroups: 1,
      },
    ],
  },
  area: {
    type: 'area',
    label: '面积图',
    fieldGroups: [
      {
        id: 'x_axis',
        kind: 'dimension',
        label: 'X 轴维度',
        emptyText: '拖拽 X 轴维度到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'values',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽数值指标到此，或点击+添加',
        minGroups: 1,
      },
    ],
  },
  scatter: {
    type: 'scatter',
    label: '散点图',
    fieldGroups: [
      {
        id: 'x_metric',
        kind: 'metric',
        label: 'X 轴指标',
        emptyText: '拖拽 X 轴指标到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'y_metric',
        kind: 'metric',
        label: 'Y 轴指标',
        emptyText: '拖拽 Y 轴指标到此，或点击+添加',
        minGroups: 2,
      },
    ],
  },
  pivot: {
    type: 'pivot',
    label: '透视表',
    fieldGroups: [
      {
        id: 'rows',
        kind: 'dimension',
        label: '行维度',
        emptyText: '拖拽行维度到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'columns',
        kind: 'dimension',
        label: '列维度',
        emptyText: '拖拽列维度到此，或点击+添加',
        minGroups: 2,
      },
      {
        id: 'values',
        kind: 'metric',
        label: '值指标',
        emptyText: '拖拽值指标到此，或点击+添加',
        minGroups: 1,
      },
    ],
  },
};

const normalizeGroups = <T extends QueryConfig['dimensionGroups'] | QueryConfig['metricGroups']>(
  groups: T,
  prefix: 'dim-group' | 'metric-group'
): T => {
  return groups.map((group, index) => {
    if (group && Array.isArray(group.bindings)) {
      return group;
    }

    return {
      id: `${prefix}-${index + 1}`,
      bindings: [],
    };
  }) as T;
};

const ensureGroupCount = (
  groups: QueryConfig['dimensionGroups'] | QueryConfig['metricGroups'],
  requiredCount: number,
  prefix: 'dim-group' | 'metric-group'
) => {
  const nextGroups = [...normalizeGroups(groups, prefix)];
  while (nextGroups.length < requiredCount) {
    nextGroups.push({
      id: `${prefix}-${nextGroups.length + 1}`,
      bindings: [],
    });
  }
  return nextGroups;
};

/**
 * 根据图表定义补齐最小字段组数量，避免切换图表类型后缺少必要槽位。
 * 调用场景：ChartBuilder 切换 chartType 时同步修正 queryConfig。
 * 主要逻辑：按定义统计维度组/指标组所需最小组数，只做补齐，不主动删除已有用户配置。
 */
export const normalizeQueryConfigForChartType = (
  chartType: BuilderChartType,
  queryConfig: QueryConfig
): QueryConfig => {
  const definition = chartDefinitions[chartType];
  const requiredDimensionGroups = definition.fieldGroups
    .filter((group) => group.kind === 'dimension')
    .reduce((maxCount, group) => Math.max(maxCount, group.minGroups), 0);
  const requiredMetricGroups = definition.fieldGroups
    .filter((group) => group.kind === 'metric')
    .reduce((maxCount, group) => Math.max(maxCount, group.minGroups), 0);

  return {
    ...queryConfig,
    dimensionGroups: ensureGroupCount(
      queryConfig.dimensionGroups,
      requiredDimensionGroups,
      'dim-group'
    ),
    metricGroups: ensureGroupCount(queryConfig.metricGroups, requiredMetricGroups, 'metric-group'),
  };
};
