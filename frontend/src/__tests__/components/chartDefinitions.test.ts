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
  funnel: FilterOutlined,
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

describe('chartDefinitions histogram（R-57）', () => {
  it('单个 value 指标槽位、maxFields 1、无维度槽位（对齐后端 Metrics[0].Field）', () => {
    const definition = chartDefinitions.histogram;

    expect(definition.type).toBe('histogram');
    expect(definition.label).toBe('直方图');
    expect(definition.styleKeys).toEqual(['colors']);
    expect(definition.fieldGroups).toEqual([
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
