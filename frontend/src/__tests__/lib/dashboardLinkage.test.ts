import { describe, expect, it } from 'vitest';
import type { DashboardWidget } from '@/lib/dashboardLayoutSchema';
import {
  type ActiveLinkageMap,
  incomingLinkages,
  linkageCandidates,
  linkageDefaultColumn,
  linkageKeyColumn,
  linkageQueryPayload,
} from '@/lib/dashboardLinkage';

/**
 * 图表联动的纯逻辑（issue #143）。
 *
 * 契约的另一半在后端 `service/dashboard/query.go` 的 `buildLinkageOverrides`（有同名对照
 * 用例）：载荷只带 `{sourceWidgetId, value[]}`，单值 → eq、多值 → in；落点列只从布局读。
 */

const chartWidget = (
  widgetId: string,
  chartId: number,
  targets?: Array<{ widgetId: string; column: string }>
): DashboardWidget => ({
  widgetId,
  type: 'chart',
  chartId,
  x: 0,
  y: 0,
  w: 6,
  h: 4,
  ...(targets ? { linkage: { targets } } : {}),
});

/** 一份 v2 图表文档：dimensionFields 决定维度个数（联动键列只在恰好一个维度时成立）。 */
const chartConfig = (dimensionFields: string[]): string =>
  JSON.stringify({
    version: 2,
    chartType: 'bar',
    title: '各地区 GMV',
    query: {
      dimensionGroups: [
        {
          id: 'dim-0',
          bindings: dimensionFields.map((fieldId, index) => ({
            bindingId: `b-${index}`,
            fieldId,
          })),
        },
      ],
      metricGroups: [{ id: 'metric-0', bindings: [{ bindingId: 'b-m', fieldId: 'gmv' }] }],
    },
    fieldMeta: {},
  });

describe('linkageQueryPayload：已激活的联动 → 请求载荷', () => {
  it('未取值 / 空串的来源不下发（空数组 = 未激活）', () => {
    const active: ActiveLinkageMap = {
      'w-1': { column: 'region', value: undefined },
      'w-2': { column: 'region', value: null },
      'w-3': { column: 'region', value: '' },
    };
    expect(linkageQueryPayload(active)).toEqual([]);
  });

  it('单值收成单元素数组（后端据此走 eq），0 / false 算真值', () => {
    expect(linkageQueryPayload({ 'w-1': { column: 'region', value: '华东' } })).toEqual([
      { sourceWidgetId: 'w-1', value: ['华东'] },
    ]);
    expect(linkageQueryPayload({ 'w-1': { column: 'amount', value: 0 } })).toEqual([
      { sourceWidgetId: 'w-1', value: [0] },
    ]);
  });
});

describe('incomingLinkages：某块图当前被谁筛着', () => {
  const widgets = [
    chartWidget('w-1', 42, [{ widgetId: 'w-2', column: 'region' }]),
    chartWidget('w-2', 43),
    chartWidget('w-3', 44, [{ widgetId: 'w-2', column: 'city' }]),
  ];

  it('只认确实勾选了本块为目标、且已激活的来源', () => {
    const active: ActiveLinkageMap = {
      'w-1': { column: 'region', value: '华东' },
      'w-3': { column: 'city', value: '' },
    };
    expect(incomingLinkages(widgets, active, 'w-2')).toEqual([
      { sourceWidgetId: 'w-1', column: 'region', value: '华东' },
    ]);
  });

  it('来源自己不算被自己联动，非目标块也不受影响', () => {
    const active: ActiveLinkageMap = { 'w-1': { column: 'region', value: '华东' } };
    expect(incomingLinkages(widgets, active, 'w-1')).toEqual([]);
    expect(incomingLinkages(widgets, active, 'w-3')).toEqual([]);
  });
});

describe('linkageKeyColumn：只有恰好一个维度才可作联动来源', () => {
  it('单维度取该列的稳定 id', () => {
    expect(linkageKeyColumn(chartConfig(['region']), 'bar')).toBe('region');
  });

  it('零维度 / 多维度都返回 null（点击分不出是哪个维度）', () => {
    expect(linkageKeyColumn(chartConfig([]), 'bar')).toBeNull();
    expect(linkageKeyColumn(chartConfig(['region', 'city']), 'bar')).toBeNull();
  });

  it('空 / 损坏配置按迁移函数的兜底文档处理，不抛异常', () => {
    expect(linkageKeyColumn(undefined, 'bar')).toBeNull();
    expect(linkageKeyColumn('{bad', 'bar')).toBeNull();
  });
});

describe('linkageDefaultColumn：勾选目标时的默认落点', () => {
  it('同数据集沿用来源的联动键列', () => {
    expect(linkageDefaultColumn('region', 7, 7)).toBe('region');
  });

  it('跨数据集无从推断 → 空串，要求用户显式选列', () => {
    expect(linkageDefaultColumn('region', 7, 9)).toBe('');
  });

  it('来源没有联动键列 / 数据集未知时也给空串', () => {
    expect(linkageDefaultColumn(null, 7, 7)).toBe('');
    expect(linkageDefaultColumn('region', undefined, 7)).toBe('');
  });
});

describe('linkageCandidates：可作为目标的其他图表块', () => {
  it('排除来源自己与非图表块', () => {
    const widgets: DashboardWidget[] = [
      chartWidget('w-1', 42),
      chartWidget('w-2', 43),
      { widgetId: 'w-t', type: 'text', markdown: 'x', x: 0, y: 0, w: 6, h: 4 },
      {
        widgetId: 'w-f',
        type: 'filter',
        binding: { datasetId: 7, column: 'region' },
        label: '区域',
        dataType: 'string',
        operator: 'in',
        multi: true,
        x: 0,
        y: 0,
        w: 3,
        h: 3,
      },
    ];
    expect(linkageCandidates(widgets, 'w-1').map((w) => w.widgetId)).toEqual(['w-2']);
  });
});
