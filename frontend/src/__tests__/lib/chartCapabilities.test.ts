import { describe, expect, it } from 'vitest';
import type { DatasourceCapabilities } from '@/api';
import { chartUnavailableReason, dialectLabel } from '@/lib/chartCapabilities';
import type { ChartType } from '@/lib/chartConfigSchema';

const ALL_CHART_TYPES: ChartType[] = [
  'bar',
  'line',
  'pie',
  'area',
  'scatter',
  'table',
  'pivot',
  'combo',
  'kpi',
  'histogram',
  'funnel',
  'radar',
  'boxplot',
];

function caps(
  dialect: string,
  percentileStrategy: string,
  supportsPercentileCont = percentileStrategy !== 'unsupported'
): DatasourceCapabilities {
  return {
    dialect,
    supports_grouping_sets: false,
    supports_percentile_cont: supportsPercentileCont,
    supports_window_functions: false,
    percentile_strategy: percentileStrategy,
  };
}

// 方言 × 图型矩阵：只有「箱线图 + percentile 策略 unsupported」这一格是门控的。
const MATRIX: Array<{
  name: string;
  capabilities: DatasourceCapabilities;
  gated: ChartType[];
}> = [
  {
    name: 'PostgreSQL (percentile_cont)',
    capabilities: caps('postgresql', 'percentile_cont'),
    gated: [],
  },
  {
    name: 'StarRocks (percentile_cont_args_first)',
    capabilities: caps('starrocks', 'percentile_cont_args_first'),
    gated: [],
  },
  {
    name: 'ClickHouse (quantilesExactInclusive)',
    capabilities: caps('clickhouse', 'quantilesExactInclusive'),
    gated: [],
  },
  {
    name: 'MySQL (unsupported)',
    capabilities: caps('mysql', 'unsupported', false),
    gated: ['boxplot'],
  },
  {
    name: 'ClickHouse 保守门控 (unsupported)',
    capabilities: caps('clickhouse', 'unsupported', false),
    gated: ['boxplot'],
  },
];

describe('chartUnavailableReason（R-78 方言 × 图型矩阵）', () => {
  for (const row of MATRIX) {
    it(row.name, () => {
      for (const chartType of ALL_CHART_TYPES) {
        const reason = chartUnavailableReason(chartType, row.capabilities);
        if (row.gated.includes(chartType)) {
          expect(reason, `${chartType} 应被门控`).toContain('暂不支持箱线图');
          expect(reason).toContain(dialectLabel(row.capabilities.dialect));
        } else {
          expect(reason, `${chartType} 不应被门控`).toBeUndefined();
        }
      }
    });
  }

  it('能力未知（未选数据集/请求失败）时按现状放开', () => {
    for (const chartType of ALL_CHART_TYPES) {
      expect(chartUnavailableReason(chartType, null)).toBeUndefined();
      expect(chartUnavailableReason(chartType, undefined)).toBeUndefined();
    }
  });

  it('方言展示名：已知方言映射，未知原样回显', () => {
    expect(dialectLabel('mysql')).toBe('MySQL');
    expect(dialectLabel('postgresql')).toBe('PostgreSQL');
    expect(dialectLabel('clickhouse')).toBe('ClickHouse');
    expect(dialectLabel('starrocks')).toBe('StarRocks');
    expect(dialectLabel('duckdb')).toBe('duckdb');
  });
});
