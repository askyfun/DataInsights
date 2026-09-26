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
import { describe, expect, it } from 'vitest';
import {
  type BuilderChartType,
  type ChartDefinition,
  chartDefinitions,
  type ResultShape,
} from '@/components/ChartBuilder/chartDefinitions';

/**
 * chartDefinitions 的 resultShape / icon 契约测试（Task 0-4）。
 *
 * 背景：图表类型图标此前硬编码在 ChartBuilder.tsx 的局部 chartTypeOptions 数组里，
 * 与声明式定义分离——新增图型必须同时改两处，漏改就丢图标。Task 0-4 把 icon 移进
 * ChartDefinition，chartTypeOptions 改为从 chartDefinitions 派生。
 *
 * icon 刻意存组件引用（React.ComponentType）而非 JSX 元素，因此本文件无需 render
 * 即可断言「哪个图型对应哪个图标组件」，chartDefinitions.ts 也得以保持纯 .ts。
 *
 * 表驱动 + Object.keys 派生：后续任务新增图型时，「非空 resultShape/icon」这条自动
 * 覆盖新图型；两条 EXPECTED_* 表由 Record<BuilderChartType, ...> 强制补全。
 */

/** 期望的 chartType → resultShape 映射（Task 0-4 brief §A.4 钉死）。 */
const EXPECTED_RESULT_SHAPE: Record<BuilderChartType, ResultShape> = {
  table: 'table',
  bar: 'axis',
  line: 'axis',
  pie: 'pie',
  area: 'axis',
  scatter: 'scatter',
  pivot: 'pivot',
  combo: 'axis',
  kpi: 'kpi',
  histogram: 'histogram',
  funnel: 'pie',
  radar: 'radar',
  boxplot: 'boxplot',
};

/** 期望的 chartType → icon 组件映射，须与迁移前 ChartBuilder.tsx 的硬编码逐项一致。 */
const EXPECTED_ICON: Record<BuilderChartType, ChartDefinition['icon']> = {
  table: TableOutlined,
  bar: BarChartOutlined,
  line: LineChartOutlined,
  pie: PieChartOutlined,
  area: AreaChartOutlined,
  scatter: DotChartOutlined,
  pivot: AppstoreOutlined,
  combo: FundOutlined,
  kpi: DashboardOutlined,
  histogram: StockOutlined,
  funnel: FunnelPlotOutlined,
  radar: RadarChartOutlined,
  boxplot: BoxPlotOutlined,
};

const CHART_TYPES = Object.keys(chartDefinitions) as BuilderChartType[];

describe('chartDefinitions resultShape/icon', () => {
  it.each(CHART_TYPES)('%s 声明了非空 resultShape 与 icon', (chartType) => {
    const definition = chartDefinitions[chartType];

    expect(definition.resultShape).toBeTruthy();
    expect(definition.icon).toBeDefined();
  });

  it.each(CHART_TYPES)('%s 的 resultShape 与 icon 符合预期映射', (chartType) => {
    const definition = chartDefinitions[chartType];

    expect(definition.resultShape).toBe(EXPECTED_RESULT_SHAPE[chartType]);
    expect(definition.icon).toBe(EXPECTED_ICON[chartType]);
  });
});

/**
 * 分组维度槽位下线（2026-09-19）。
 *
 * 「颜色分组」（bar/line/area/combo）与「系列分组」（radar）是同一概念——按某维度的
 * 每个值把数据拆成多条系列（等价于 Excel 透视图的「图例字段」）。命名混乱导致理解成本高，
 * 产品裁定整体下线：这五个图型各自只保留一个维度槽位。
 *
 * 后端仍支持 v2 协议里的 color_group / series_group 槽位名，且后端 AxisProcessor 的
 * dims[1:] 位置拆系列逻辑不变——历史图表由 normalizeQueryConfigForChartType 合并进
 * 保留槽位，字段不丢（见 normalizeQueryConfigForChartType.test.ts）。
 */
describe('chartDefinitions 分组维度槽位下线（2026-09-19）', () => {
  const GROUPED_CHART_TYPES = ['bar', 'line', 'area', 'combo', 'radar'] as const;

  it.each(GROUPED_CHART_TYPES)('%s 不再声明 color_group / series_group 槽位', (chartType) => {
    const ids = chartDefinitions[chartType].fieldGroups.map((group) => group.id);

    expect(ids).not.toContain('color_group');
    expect(ids).not.toContain('series_group');
  });

  it.each(['bar', 'line', 'area', 'combo'] as const)('%s 只剩 X 轴一个维度槽位', (chartType) => {
    const dimensionIds = chartDefinitions[chartType].fieldGroups
      .filter((group) => group.kind === 'dimension')
      .map((group) => group.id);

    expect(dimensionIds).toEqual(['x_axis']);
  });

  it('radar 只剩指标维度一个维度槽位', () => {
    const dimensionIds = chartDefinitions.radar.fieldGroups
      .filter((group) => group.kind === 'dimension')
      .map((group) => group.id);

    expect(dimensionIds).toEqual(['indicators']);
  });
});

describe('chartDefinitions histogram（R-57）', () => {
  it('分组维度(可选) + 单个 value 指标槽位 maxFields 1（2026-09-26 起支持维度组，对齐后端 Metrics[0].Field）', () => {
    const definition = chartDefinitions.histogram;

    expect(definition.type).toBe('histogram');
    expect(definition.label).toBe('直方图');
    expect(definition.styleKeys).toEqual(['colors']);
    expect(definition.fieldGroups).toEqual([
      {
        id: 'group',
        kind: 'dimension',
        label: '分组维度',
        emptyText: '可选：拖入维度按其值拆分分布，或点击+添加',
        minGroups: 1,
      },
      {
        id: 'value',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽要分箱的数值字段到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
      },
    ]);
  });
});

describe('chartDefinitions funnel（R-59）', () => {
  it('复用 pie 形状，stages(维度,1) + value(指标,1) 两槽位', () => {
    const definition = chartDefinitions.funnel;

    expect(definition.type).toBe('funnel');
    expect(definition.label).toBe('漏斗图');
    expect(definition.resultShape).toBe('pie');
    expect(definition.styleKeys).toEqual(['colors']);
    expect(definition.fieldGroups).toEqual([
      {
        id: 'stages',
        kind: 'dimension',
        label: '阶段',
        emptyText: '拖拽阶段维度到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
      },
      {
        id: 'value',
        kind: 'metric',
        label: '数值',
        emptyText: '拖拽数值指标到此，或点击+添加',
        minGroups: 1,
        maxFields: 1,
      },
    ]);
  });
});
