import {
  AppstoreOutlined,
  AreaChartOutlined,
  BarChartOutlined,
  DashboardOutlined,
  DotChartOutlined,
  FilterOutlined,
  FundOutlined,
  LineChartOutlined,
  PieChartOutlined,
  StockOutlined,
  TableOutlined,
} from '@ant-design/icons';
import type { ComponentType } from 'react';
import type { ChartConfig, ChartStyleConfig, QueryConfig } from '@/store';

export type BuilderChartType = ChartConfig['chartType'];
export type FieldGroupKind = 'dimension' | 'metric';

/**
 * 图表结果形状：后端 processor 返回结构 + 前端渲染器的组合语义。
 * 联合里同时包含已实现与尚未实现的形状，后续任务加新图型时无需再改这个类型定义。
 */
export type ResultShape =
  | 'axis'
  | 'pie'
  | 'table'
  | 'pivot'
  | 'scatter'
  | 'histogram'
  | 'boxplot'
  | 'radar'
  | 'kpi';

export interface ChartFieldGroupDefinition {
  id: string;
  kind: FieldGroupKind;
  label: string;
  emptyText: string;
  minGroups: number;
  /** 该槽位最多接受几个字段（undefined = 无限）。 */
  maxFields?: number;
  /** 是否可选槽位（minGroups=0 时自动为 true）。 */
  optional?: boolean;
}

export interface ChartDefinition {
  type: BuilderChartType;
  label: string;
  /** 结果形状：决定后端 processor 与前端渲染器的选择。 */
  resultShape: ResultShape;
  /** 图表类型图标。存组件引用（非 JSX 元素），由调用方实例化。 */
  icon: ComponentType;
  /** 该图型支持的样式开关集合，供 ConfigPanel 条件渲染（未填 = 暂无专属样式）。 */
  styleKeys?: Array<keyof ChartStyleConfig>;
  fieldGroups: ChartFieldGroupDefinition[];
}

export const chartDefinitions: Record<BuilderChartType, ChartDefinition> = {
  table: {
    type: 'table',
    label: '表格',
    resultShape: 'table',
    icon: TableOutlined,
    styleKeys: ['tableRowSize'],
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
    resultShape: 'axis',
    icon: BarChartOutlined,
    // bar 分支不消费 smooth（buildChartOption 里 bar 恒为 {}），故不列入 styleKeys，
    // 避免渲染一个不起作用的开关（裁定见 task-1-1-brief.md D 部分）。
    styleKeys: ['colors', 'stack', 'orientation'],
    fieldGroups: [
      {
        id: 'x_axis',
        kind: 'dimension',
        label: 'X 轴维度',
        emptyText: '拖拽 X 轴维度到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'color_group',
        kind: 'dimension',
        label: '颜色分组',
        emptyText: '拖拽颜色分组维度到此（可选）',
        minGroups: 0,
        maxFields: 1,
        optional: true,
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
    resultShape: 'axis',
    icon: LineChartOutlined,
    styleKeys: ['colors', 'smooth', 'stack'],
    fieldGroups: [
      {
        id: 'x_axis',
        kind: 'dimension',
        label: 'X 轴维度',
        emptyText: '拖拽 X 轴维度到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'color_group',
        kind: 'dimension',
        label: '颜色分组',
        emptyText: '拖拽颜色分组维度到此（可选）',
        minGroups: 0,
        maxFields: 1,
        optional: true,
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
    resultShape: 'pie',
    icon: PieChartOutlined,
    styleKeys: ['colors', 'donut'],
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
    resultShape: 'axis',
    icon: AreaChartOutlined,
    styleKeys: ['colors', 'smooth', 'stack'],
    fieldGroups: [
      {
        id: 'x_axis',
        kind: 'dimension',
        label: 'X 轴维度',
        emptyText: '拖拽 X 轴维度到此，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'color_group',
        kind: 'dimension',
        label: '颜色分组',
        emptyText: '拖拽颜色分组维度到此（可选）',
        minGroups: 0,
        maxFields: 1,
        optional: true,
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
    resultShape: 'scatter',
    icon: DotChartOutlined,
    // buildChartOption 的 scatter 分支当前不消费 colors（未写入 color 调色板），
    // 故 styleKeys 显式留空数组，避免渲染一个不起作用的色板控件。
    styleKeys: [],
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
    resultShape: 'pivot',
    icon: AppstoreOutlined,
    styleKeys: ['tableRowSize'],
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
  combo: {
    type: 'combo',
    label: '组合图',
    // 复用 AxisResponse（x_axis + series）：两个指标槽位的 metrics 合并进同一 metrics[]，
    // 后端 AxisProcessor 逐指标产出 series，前端按槽位名分配左右 Y 轴（buildChartOption）。
    resultShape: 'axis',
    icon: FundOutlined,
    // combo 本任务只支持主色，不支持 stack/orientation/smooth（plan 未要求，避免过度实现）。
    styleKeys: ['colors'],
    fieldGroups: [
      {
        id: 'x_axis',
        kind: 'dimension',
        label: 'X 轴维度',
        emptyText: '拖拽 X 轴维度到此',
        minGroups: 1,
        maxFields: 1,
      },
      {
        id: 'color_group',
        kind: 'dimension',
        label: '颜色分组',
        emptyText: '拖拽颜色分组维度到此（可选）',
        minGroups: 0,
        maxFields: 1,
        optional: true,
      },
      {
        id: 'primary_values',
        kind: 'metric',
        label: '主轴指标',
        emptyText: '拖拽主轴指标到此',
        minGroups: 1,
      },
      {
        id: 'secondary_values',
        kind: 'metric',
        label: '次轴指标',
        emptyText: '拖拽次轴指标到此',
        minGroups: 1,
      },
    ],
  },
  kpi: {
    type: 'kpi',
    label: 'KPI 卡',
    // kpi（R-51）不走 ECharts：后端 KpiProcessor 返回标量 {value, label}，
    // 前端由 KpiCard（AntD Statistic）渲染，buildChartOption 对 kpi 恒返回 null。
    resultShape: 'kpi',
    icon: DashboardOutlined,
    // KPI 卡不消费 colors/smooth/stack 等 ECharts 样式控件。
    styleKeys: [],
    fieldGroups: [
      {
        id: 'value',
        kind: 'metric',
        label: '指标',
        emptyText: '拖拽指标到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
      },
    ],
  },
  histogram: {
    type: 'histogram',
    label: '直方图',
    // histogram（R-57）：后端 HistogramProcessor 返回 { bins: [{bin_start,bin_end,count}] }，
    // 前端 buildChartOption 用 ECharts bar 渲染分箱分布。
    resultShape: 'histogram',
    icon: StockOutlined,
    // 用 bar 渲染，消费 colors 调色板；不消费 smooth/stack/orientation/donut/tableRowSize。
    styleKeys: ['colors'],
    fieldGroups: [
      {
        // 单个 metric 槽位、maxFields:1，对齐后端取 Metrics[0].Field 分箱、每箱 COUNT(*)。
        id: 'value',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽要分箱的数值字段到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
      },
    ],
  },
  funnel: {
    type: 'funnel',
    label: '漏斗图',
    // funnel（R-59）复用 pie 形状：后端 GetProcessor(funnel) → PieProcessor，
    // 返回 PieResponse{data:[{name,value}]}；前端 buildChartOption 渲染为 ECharts funnel。
    resultShape: 'pie',
    // FunnelPlotOutlined 在 @ant-design/icons v6.3.4 不存在，用漏斗形滤镜图标（裁定 E）。
    icon: FilterOutlined,
    // funnel 消费 ECharts 调色板；不消费 smooth/stack/orientation/donut/tableRowSize。
    styleKeys: ['colors'],
    fieldGroups: [
      {
        // 阶段维度：有序、单字段（漏斗每层一个阶段）。
        id: 'stages',
        kind: 'dimension',
        label: '阶段',
        emptyText: '拖拽阶段维度到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
      },
      {
        // 数值指标：单值；查询期强制按该值降序（composeChartQueryRequest，裁定 F）。
        id: 'value',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽数值指标到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
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
