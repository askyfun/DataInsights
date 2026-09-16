import {
  AppstoreOutlined,
  AreaChartOutlined,
  BarChartOutlined,
  DotChartOutlined,
  FundOutlined,
  LineChartOutlined,
  PieChartOutlined,
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
