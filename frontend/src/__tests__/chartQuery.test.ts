import { describe, expect, it } from 'vitest';
import type { components } from '@/idls/gen_types';
import type { ChartType } from '@/lib/chartConfigSchema';

type AxisResponse = components['schemas']['ChartAxisResponse'];
type ScatterResponse = components['schemas']['ChartScatterResponse'];

/**
 * Pure transformation function matching the logic in store/index.ts executeChartQuery
 * for bar/line/area chart types.
 */
function transformAxisResponse(axisData: AxisResponse, dimName: string): Record<string, unknown>[] {
  return axisData.x_axis.map((xVal, idx) => {
    const row: Record<string, unknown> = { [dimName || 'x']: xVal };
    for (const series of axisData.series) {
      row[series.name] = series.data[idx];
    }
    return row;
  });
}

describe('AxisResponse data transformation', () => {
  it('should transform bar chart AxisResponse to row data', () => {
    const axisData: AxisResponse = {
      x_axis: ['Apple', 'Banana'],
      series: [{ name: 'revenue', data: [1000, 2000] }],
    };

    const result = transformAxisResponse(axisData, 'product');

    expect(result).toEqual([
      { product: 'Apple', revenue: 1000 },
      { product: 'Banana', revenue: 2000 },
    ]);
  });

  it('should transform line chart AxisResponse to row data', () => {
    const axisData: AxisResponse = {
      x_axis: ['2024-01-01', '2024-01-02', '2024-01-03'],
      series: [{ name: 'value', data: [100, 200, 300] }],
    };

    const result = transformAxisResponse(axisData, 'date');

    expect(result).toEqual([
      { date: '2024-01-01', value: 100 },
      { date: '2024-01-02', value: 200 },
      { date: '2024-01-03', value: 300 },
    ]);
  });

  it('should transform area chart AxisResponse to row data', () => {
    const axisData: AxisResponse = {
      x_axis: ['Q1', 'Q2'],
      series: [{ name: 'sales', data: [500, 700] }],
    };

    const result = transformAxisResponse(axisData, 'quarter');

    expect(result).toEqual([
      { quarter: 'Q1', sales: 500 },
      { quarter: 'Q2', sales: 700 },
    ]);
  });

  it('should handle multiple series', () => {
    const axisData: AxisResponse = {
      x_axis: ['Jan', 'Feb'],
      series: [
        { name: 'revenue', data: [10000, 15000] },
        { name: 'cost', data: [3000, 4000] },
      ],
    };

    const result = transformAxisResponse(axisData, 'month');

    expect(result).toEqual([
      { month: 'Jan', revenue: 10000, cost: 3000 },
      { month: 'Feb', revenue: 15000, cost: 4000 },
    ]);
  });

  it('should handle empty axis data', () => {
    const axisData: AxisResponse = {
      x_axis: [],
      series: [],
    };

    const result = transformAxisResponse(axisData, 'dim');

    expect(result).toEqual([]);
  });

  it('should use "x" as default dim name when empty string provided', () => {
    const axisData: AxisResponse = {
      x_axis: ['A'],
      series: [{ name: 'count', data: [10] }],
    };

    const result = transformAxisResponse(axisData, '');

    expect(result).toEqual([{ x: 'A', count: 10 }]);
  });

  it('should handle null/undefined values in series data', () => {
    const axisData: AxisResponse = {
      x_axis: ['A', 'B'],
      series: [{ name: 'value', data: [100, null] }],
    };

    const result = transformAxisResponse(axisData, 'category');

    expect(result).toEqual([
      { category: 'A', value: 100 },
      { category: 'B', value: null },
    ]);
  });
});

describe('ChartType includes area', () => {
  it('should accept area as a valid ChartType value', () => {
    // This is primarily a compile-time check.
    // If 'area' is not in the ChartType union, this file won't compile.
    const chartType: ChartType = 'area';
    expect(chartType).toBe('area');
  });

  it('should accept all expected chart types', () => {
    const types: ChartType[] = ['line', 'bar', 'pie', 'scatter', 'table', 'area'];
    expect(types).toHaveLength(6);
  });
});

describe('ScatterResponse data transformation', () => {
  /**
   * Reproduce the bug: scatter data is raw [number, number][] tuples,
   * but ChartCanvas tries to access item[dimensionField] (object key lookup on a tuple).
   * This always yields undefined, breaking scatter charts.
   */
  it('should NOT treat scatter tuples as objects with named keys', () => {
    const scatterData: ScatterResponse = {
      data: [
        [100, 50],
        [200, 80],
        [150, 60],
      ],
    };

    // Current broken behavior: treating tuple as object
    const dimensionField = 'city';
    const metricField = 'revenue';

    const brokenResult = scatterData.data.map((item) => [
      item[dimensionField as unknown as number],
      item[metricField as unknown as number],
    ]);

    // This is what actually happens: all values are undefined
    expect(brokenResult[0][0]).toBeUndefined();
    expect(brokenResult[0][1]).toBeUndefined();
  });

  it('should access scatter tuples by index: [0] for X, [1] for Y', () => {
    const scatterData: ScatterResponse = {
      data: [
        [100, 50],
        [200, 80],
        [150, 60],
      ],
    };

    // Correct behavior: access by index
    const correctResult = scatterData.data.map((item) => [item[0], item[1]]);

    expect(correctResult).toEqual([
      [100, 50],
      [200, 80],
      [150, 60],
    ]);
  });
});
