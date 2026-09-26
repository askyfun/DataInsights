import {
  AppstoreOutlined,
  AreaChartOutlined,
  BarChartOutlined,
  BoxPlotOutlined,
  DashboardOutlined,
  DotChartOutlined,
  FundOutlined,
  FunnelPlotOutlined,
  LineChartOutlined,
  PieChartOutlined,
  RadarChartOutlined,
  StockOutlined,
  TableOutlined,
} from '@ant-design/icons';
import type { ComponentType } from 'react';
import type { ChartConfig, ChartStyleConfig, QueryConfig } from '@/store';

/**
 * ⚠️ 改槽位（fieldGroups/styleKeys）必须按此清单同步，漏一步会静默丢功能：
 * 1. chartDefinitions 的 fieldGroups(+styleKeys)
 * 2. 后端 internal/query/processor.go、processor_stats.go 的槽位常量
 *    （SlotXAxis / SlotIndicators / SlotSeriesGroup / SlotColorGroup 等）与归槽分支
 * 3. 孤儿槽位（定义裁剪后不再被引用的字段组）
 * 4. chartOptions.ts 渲染分支
 * 5. 相关测试（chartDefinitions.test / builder / normalize 等共 4 处）
 * 6. 历史图表兼容（migrateChartConfig / normalizeQueryConfigForChartType 的搬运逻辑）
 * 7. 后端槽位常量默认不删（含已下线的 color_group/series_group，归槽分支刻意保留）
 */

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
        // 分组维度（2026-09-26）：可选槽位。留空 = 经典直方图（对单列数值分箱计数）；
        // 拖入维度 = 每个维度值一个系列（后端 GROUP BY 维度 + bin，前端堆叠渲染）。
        // 走 v1 平铺协议（dims 直传），无具名槽位语义。
        id: 'group',
        kind: 'dimension',
        label: '分组维度',
        emptyText: '可选：拖入维度按其值拆分分布，或点击+添加',
        minGroups: 1,
      },
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
    // FunnelPlotOutlined 是 @ant-design/icons 的漏斗形图标，语义与 funnel 图型精确对应。
    icon: FunnelPlotOutlined,
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
  radar: {
    type: 'radar',
    label: '雷达图',
    // radar（R-62）走后端 RadarProcessor：SQL 是 GROUP BY indicators + AGG(value)，
    // 处理器重塑为 RadarResponse{indicators:[{name,max}], series:[{name,values}]}。
    // 2026-09-19 起不再提供 series_group 槽位（单系列），但后端仍支持该槽位以兼容历史请求。
    resultShape: 'radar',
    // RadarChartOutlined 是 @ant-design/icons 的雷达图图标，语义精确。
    icon: RadarChartOutlined,
    // radar 消费 ECharts 调色板（每条系列一色）；不消费 smooth/stack/orientation/donut/tableRowSize。
    styleKeys: ['colors'],
    fieldGroups: [
      {
        // 指标维度：雷达轴（每个维度值一条轴），单字段。
        id: 'indicators',
        kind: 'dimension',
        label: '指标维度',
        emptyText: '拖拽指标维度到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
      },
      {
        // 数值指标：单值，恒产出单条系列（与后端 Metrics[0] 对齐）。
        id: 'values',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽数值指标到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
      },
    ],
  },
  boxplot: {
    type: 'boxplot',
    label: '箱线图',
    // boxplot（R-52）走后端 executeBoxplot 三查询 + BoxplotProcessor.Assemble：
    // 返回 BoxplotResponse{whisker_low,q1,median,q3,whisker_high,outliers,outlier_total,truncated}，
    // 前端 buildChartOption 用 ECharts boxplot（五数概括）+ scatter（离群点）渲染。
    resultShape: 'boxplot',
    // BoxPlotOutlined 是 @ant-design/icons 的箱线图图标，语义精确。
    icon: BoxPlotOutlined,
    // boxplot 消费 ECharts 调色板；不消费 smooth/stack/orientation/donut/tableRowSize。
    styleKeys: ['colors'],
    fieldGroups: [
      {
        // 单个 metric 槽位、maxFields:1，对齐后端取 Metrics[0].Field 算分位/离群点。
        id: 'value',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽要求分布的数值字段到此，或点击+添加',
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
 * 把超出目标组数的字段组里的绑定搬回保留下来的同类型组，避免切换图表类型时静默剪掉用户已配好的字段。
 * 调用场景：normalizeQueryConfigForChartType（图表类型切换、配置文档恢复）。
 * 主要逻辑：前 limit 个组原位保留；其后的组按顺序把绑定并入"最后一个非空保留组"
 * （保留组全空则并入第 0 组），按列名去重——与 addDimensionField/addMetricField 的单组去重口径一致。
 * limit 为 0 表示新图型没有这类槽位（kpi/histogram/boxplot/scatter 都没有维度槽位）：
 * 此时**不裁剪 state**，请求侧本就会把这类组 slice 掉，保留原样可让用户切回旧图型时字段还在。
 *
 * 边界：并入目标槽位定义的 maxFields（如 funnel.stages=1）可能被突破。maxFields 目前只是
 * 声明性字段、无运行时校验，此处刻意不以"超出即丢弃"的方式满足它——丢弃正是本函数要消除的行为。
 */
const relocateOverflowBindings = (
  groups: QueryConfig['dimensionGroups'],
  limit: number
): QueryConfig['dimensionGroups'] => {
  if (groups.length <= limit || limit <= 0) {
    return groups;
  }

  const overflow = groups.slice(limit).flatMap((group) => group.bindings ?? []);
  const kept = groups.slice(0, limit).map((group) => ({ ...group, bindings: [...group.bindings] }));

  if (overflow.length === 0) {
    return kept;
  }

  // 落到最后一个非空保留组：与"行维度留原样、列维度并进来"的直觉一致。
  let targetIndex = 0;
  for (let index = kept.length - 1; index >= 0; index -= 1) {
    if (kept[index].bindings.length > 0) {
      targetIndex = index;
      break;
    }
  }

  const target = kept[targetIndex];
  const seen = new Set(target.bindings.map((binding) => binding.fieldId));
  for (const binding of overflow) {
    if (seen.has(binding.fieldId)) {
      continue;
    }
    seen.add(binding.fieldId);
    target.bindings.push(binding);
  }

  return kept;
};

/**
 * 根据图表定义补齐最小字段组数量，并把被裁掉的字段搬回同类型保留组。
 * 调用场景：ChartBuilder 切换 chartType 时同步修正 queryConfig。
 * 主要逻辑：先按定义的组数把溢出的绑定搬进保留下来的同类型组（不再静默丢字段），
 * 再按定义补齐最小组数——仍不主动删除已有用户配置。
 */
export const normalizeQueryConfigForChartType = (
  chartType: BuilderChartType,
  queryConfig: QueryConfig
): QueryConfig => {
  const definition = chartDefinitions[chartType];
  const dimensionDefs = definition.fieldGroups.filter((group) => group.kind === 'dimension');
  const metricDefs = definition.fieldGroups.filter((group) => group.kind === 'metric');
  const requiredDimensionGroups = dimensionDefs.reduce(
    (maxCount, group) => Math.max(maxCount, group.minGroups),
    0
  );
  const requiredMetricGroups = metricDefs.reduce(
    (maxCount, group) => Math.max(maxCount, group.minGroups),
    0
  );

  return {
    ...queryConfig,
    dimensionGroups: ensureGroupCount(
      relocateOverflowBindings(queryConfig.dimensionGroups, dimensionDefs.length),
      requiredDimensionGroups,
      'dim-group'
    ),
    metricGroups: ensureGroupCount(
      relocateOverflowBindings(queryConfig.metricGroups, metricDefs.length),
      requiredMetricGroups,
      'metric-group'
    ),
  };
};
