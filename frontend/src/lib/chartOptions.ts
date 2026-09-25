/**
 * 共享 ECharts option 构造器（纯函数模块：不依赖 store、不依赖 React）。
 *
 * ChartBuilder 预览（ChartCanvas）与 ShareView 此前各自维护一份
 * getChartOption，对同一 chartType 生成的 option 细节不一致（D3）。
 * 本模块把两处逻辑合并为 buildChartOption 唯一出口，并以 ChartCanvas
 * 的更完整版本为准（axisLabel 旋转/UTC 后缀截断、connectNulls、smooth）。
 *
 * data 参数同时支持两条路径（D7 修复后 builder 与 share 消费同一形状）：
 * - 结构化联合类型（POST /charts/query、v1 配置的 GET /charts/:id/data）；
 * - legacy 裸行数组（旧结构/损坏/空配置分享的回退，按列名索引行）。
 *
 * 依赖说明：ChartStyleConfig 仅 type-only 引入（编译期擦除），
 * 运行时不加载 zustand store，可在无 React/store 环境下直接调用与测试。
 */
import type { EChartsOption } from 'echarts';
import type {
  AxisResponse,
  BoxplotResponse,
  ChartDataResponse,
  HistogramResponse,
  PieResponse,
  RadarResponse,
  ScatterResponse,
} from '../api';
import type { ChartStyleConfig } from '../store';
import type { ChartType } from './chartConfigSchema';

/** ChartStyleConfig 默认值（持久化 style 缺失/形状非法时的兜底） */
const DEFAULT_CHART_STYLE: ChartStyleConfig = {
  colors: [],
  smooth: false,
  tableRowSize: 'small',
};

/**
 * 两臂统一判空：v1 配置的聚合负载（判别对象形状，空结果时内层数组为空，
 * 与 builder 预览一致）vs legacy 的裸行数组回退。
 */
export function isEmptyPayload(data: ChartDataResponse): boolean {
  if (Array.isArray(data)) {
    return data.length === 0;
  }
  if ('x_axis' in data) {
    return data.x_axis.length === 0;
  }
  if ('data' in data) {
    return data.data.length === 0;
  }
  // histogram（R-57）：{bins} 形状不命中上面任何一臂，缺了这条会落到末尾 return true，
  // buildChartOption 直接返回 null → 直方图永远渲染空。
  if ('bins' in data) {
    return data.bins.length === 0;
  }
  // radar（R-62）：{indicators, series} 形状同款问题——不命中任何一臂就落末尾 return true
  // → 雷达图永远渲染空。indicators 空即视为无轴可画（等价于无数据）。
  if ('indicators' in data) {
    return data.indicators.length === 0;
  }
  // boxplot（R-52）：{whisker_low,q1,median,q3,whisker_high,outliers,...} 结构体恒存在，
  // 后端对空数据集也返回退化零值结构而非 null。0 是合法分位值、无法据五数概括判空，
  // 只要命中 boxplot 形状就按"有内容可画"处理（宁可渲染贴零的箱，也不整图空白）。
  if ('q1' in data) {
    return false;
  }
  return true;
}

/**
 * 把持久化文档里的 style 小节（schema 上是 unknown）安全窄化为 ChartStyleConfig。
 * 调用场景：ShareView 从 chartDoc.style 恢复样式；缺失/非法字段一律回落默认值。
 */
export function normalizeChartStyle(style: unknown): ChartStyleConfig {
  if (typeof style !== 'object' || style === null || Array.isArray(style)) {
    return { ...DEFAULT_CHART_STYLE };
  }
  const raw = style as Record<string, unknown>;
  const tableRowSize = raw.tableRowSize;
  const stack = raw.stack;
  const orientation = raw.orientation;
  return {
    colors: Array.isArray(raw.colors)
      ? raw.colors.filter((color): color is string => typeof color === 'string')
      : [],
    smooth: raw.smooth === true,
    tableRowSize: tableRowSize === 'middle' || tableRowSize === 'large' ? tableRowSize : 'small',
    // 非法/缺失的 stack 直接不带该键（与 DEFAULT_CHART_STYLE 的形状保持一致），等价于 'none'。
    ...(stack === 'none' || stack === 'normal' || stack === 'percent' ? { stack } : {}),
    // orientation/donut 同例：非法/缺失不带键，分别等价于 'vertical'/false。
    ...(orientation === 'vertical' || orientation === 'horizontal' ? { orientation } : {}),
    ...(raw.donut === true ? { donut: true } : {}),
  };
}

/** buildChartOption 的调用方上下文（纯数据，由调用方从各自状态计算后传入） */
export interface ChartOptionContext {
  /** 图表标题，写入 option.title.text */
  title: string;
  /** 维度列名，按配置顺序；首项作为类目轴/饼图分类字段 */
  dimensions: string[];
  /** 指标列名，按配置顺序；散点取前两项作 X/Y */
  metrics: string[];
  /**
   * 指标槽位分组（可选，仅 combo 双轴组合图填充）：每项给出槽位名与该槽位的指标列名。
   * combo 用它把每条 series 归到左/右 Y 轴（primary_values→yAxisIndex 0、
   * secondary_values→yAxisIndex 1）。其余图型留空 undefined，调用方无需改动（裁定）。
   */
  metricSlots?: Array<{ slot: string; metrics: string[] }>;
}

/** legacy 裸行按列名索引 */
type RawRow = Record<string, unknown>;

/**
 * 聚合负载（axis series）与 legacy 裸行的值是 unknown：
 * 收敛为 ECharts 可渲染的类目/数值，null/undefined 统一为 null（配合 connectNulls）。
 */
function toOptionValue(value: unknown): string | number | null {
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value === 'number' || typeof value === 'string') {
    return value;
  }
  return String(value);
}

function pieSliceName(value: unknown): string {
  return value === null || value === undefined ? '' : String(value);
}

/** 类目轴值不接受 null：legacy 行里的空值收敛为 ''（ECharts 类目数据约束） */
function toCategoryValue(value: unknown): string | number {
  const converted = toOptionValue(value);
  return converted === null ? '' : converted;
}

/** 饼图 value 需要数值：数字/数值字符串转 number，其余视为缺失（undefined） */
function toPieValue(value: unknown): number | undefined {
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : undefined;
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const numeric = Number(value);
    return Number.isFinite(numeric) ? numeric : undefined;
  }
  return undefined;
}

/** 截断 "+0000 UTC" 后缀，保留日期+时间（模块级常量：保证同输入产出引用相等的 option） */
const truncateUtcSuffix = (val: string) => {
  let out = String(val).replace(/\s*\+\d{4}\s*UTC$/, '');
  // ISO 午夜零点（2026-01-01T00:00:00Z）→ 仅显示日期部分
  out = out.replace(/T00:00:00(\.\d+)?Z?$/, '');
  // 其余 ISO 时间戳（2026-01-01T08:30:00Z）→ 空格分隔、去尾 Z
  return out.replace('T', ' ').replace(/Z$/, '');
};

/**
 * 判断 x 轴值是否为可排序的时间戳：全部命中 ISO 日期前缀（YYYY-MM-DD...）时
 * 返回按时间升序的排列索引，否则返回 null（非时间维度保持后端原始顺序）。
 */
function temporalSortOrder(values: readonly unknown[]): number[] | null {
  const isoDatePattern = /^\d{4}-\d{2}-\d{2}([T\s]\d{2}:\d{2}:\d{2})?/;
  const times: number[] = [];
  for (const value of values) {
    if (typeof value !== 'string' || !isoDatePattern.test(value)) {
      return null;
    }
    const time = Date.parse(value);
    if (Number.isNaN(time)) {
      return null;
    }
    times.push(time);
  }
  if (times.length === 0) {
    return null;
  }
  // 稳定排序：时间相同时保持原相对顺序
  return times
    .map((time, index) => ({ time, index }))
    .sort((a, b) => a.time - b.time || a.index - b.index)
    .map((item) => item.index);
}

/**
 * 时间维度的类目轴按时间升序重排（D3 修复）：后端 GROUP BY 不保证有序，
 * 折线/面积图直接消费会呈现锯齿假象。x_axis 与各 series.data 是平行数组，
 * 必须用同一份排列索引同步重排。非时间维度原样返回。
 */
function sortByTemporalAxis(axis: AxisResponse): AxisResponse {
  const order = temporalSortOrder(axis.x_axis);
  if (!order) {
    return axis;
  }
  return {
    ...axis,
    x_axis: order.map((i) => axis.x_axis[i]),
    series: axis.series.map((s) => ({ ...s, data: order.map((i) => s.data[i]) })),
  };
}

/**
 * bar/line/area 的堆叠渲染：stack='normal' 时每条 series 打 stack:'total'；
 * stack='percent' 时在此基础上把每个 x 位置上所有 series 的数值归一化为百分比
 * （该位置数值之和为 100，非数值/null 原样保留，全 0 位置输出 0 而不是 NaN）。
 * 归一化在前端计算（buildChartOption 内部），后端 processor 仍返回原始值。
 * stack='none'/undefined 时原样返回，不加 stack 字段。
 */
type StackableSeries = { data: (string | number | null)[] };

function applyStack<T extends StackableSeries>(
  series: T[],
  stack: ChartStyleConfig['stack']
): (T & { stack?: string })[] {
  if (stack !== 'normal' && stack !== 'percent') {
    return series;
  }
  if (stack === 'normal') {
    return series.map((s) => ({ ...s, stack: 'total' }));
  }

  // percent：先按 x 位置累加所有 series 的数值和，再逐项归一化为百分比
  let positionCount = 0;
  for (const s of series) {
    positionCount = Math.max(positionCount, s.data.length);
  }
  const sums = new Array<number>(positionCount).fill(0);
  for (const s of series) {
    for (let i = 0; i < s.data.length; i++) {
      const v = s.data[i];
      if (typeof v === 'number' && Number.isFinite(v)) {
        sums[i] += v;
      }
    }
  }
  return series.map((s) => ({
    ...s,
    stack: 'total',
    data: s.data.map((v, i) => {
      if (typeof v !== 'number' || !Number.isFinite(v)) {
        return v;
      }
      const total = sums[i];
      return total > 0 ? (v / total) * 100 : 0;
    }),
  }));
}

/**
 * 唯一的 ECharts option 构造出口。
 * 空数据 / 维度指标不足 / table・pivot（走 TableChart）时返回 null，
 * 调用方据此渲染空状态。
 *
 * @param chartType 7 种图型联合（table/pivot 恒返回 null）
 * @param data 结构化联合响应或 legacy 裸行数组（Array.isArray 判别）
 * @param style 样式配置（colors/smooth）
 * @param labels 列名 → 显示名（builder 由 dimensionLabels/metricAliases/metricUnits
 *               组合计算；share 直接用 displayLabels），缺失时回落列名
 * @param context 标题与维度/指标列名（顺序即配置顺序）
 */
export function buildChartOption(
  chartType: ChartType,
  data: ChartDataResponse,
  style: ChartStyleConfig,
  labels: Record<string, string>,
  context: ChartOptionContext
): EChartsOption | null {
  // 表格类走 TableChart 渲染，不产出 ECharts option
  if (chartType === 'table' || chartType === 'pivot') {
    return null;
  }
  if (isEmptyPayload(data)) {
    return null;
  }

  // 只认自有键：列名恰为 __proto__/toString 等继承成员时不得穿透原型链
  const labelOf = (name: string) => Object.getOwnPropertyDescriptor(labels, name)?.value || name;
  // echarts 6 起，setOption 显式传入 `color: undefined` 会在 option 合并时覆盖掉
  // 默认调色板（实测 model.get('color') 变 undefined），导致所有系列无填充色——
  // 图例/坐标轴正常但扇区/柱体完全透明。因此空 colors 时必须完全不带 color 键，
  // 由 colorOf() 按 palette 有无返回片段对象（不能用 `color: undefined`）。
  const palette = style.colors.length > 0 ? style.colors : undefined;
  const colorOf = (): { color?: string[] } => (palette ? { color: palette } : {});

  const commonOptions = {
    title: {
      text: context.title,
      left: 'center' as const,
    },
    tooltip: {
      trigger: 'axis' as const,
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      containLabel: true,
    },
  };

  const pieTooltip = {
    trigger: 'item' as const,
    formatter: '{b}: {c} ({d}%)',
  };

  const pieEmphasis = {
    itemStyle: {
      shadowBlur: 10,
      shadowOffsetX: 0,
      shadowColor: 'rgba(0, 0, 0, 0.5)',
    },
  };

  // 类目超过 10 个时旋转标签
  const categoryAxisLabel = (categoryCount: number) => ({
    rotate: categoryCount > 10 ? 30 : 0,
    formatter: truncateUtcSuffix,
  });

  // 环形图（R-52）：donut=true 时用内外双半径，否则保持实心饼图
  const pieRadius: string | string[] = style.donut === true ? ['40%', '70%'] : '50%';

  // 横向条形图：仅 bar 消费 orientation，'horizontal' 时交换类目轴/值轴
  const horizontalBar = chartType === 'bar' && style.orientation === 'horizontal';

  if (!Array.isArray(data)) {
    // ---- 正常路径：结构化聚合响应 ----
    switch (chartType) {
      case 'scatter': {
        const [xField, yField] = context.metrics;
        if (!xField || !yField || !('data' in data)) {
          return null;
        }
        // ScatterResponse.data 是 [x, y] 二元组数组，原样喂给 ECharts
        const scatter = data as ScatterResponse;
        return {
          ...commonOptions,
          xAxis: { type: 'value', name: labelOf(xField) },
          yAxis: { type: 'value', name: labelOf(yField) },
          series: [{ type: 'scatter' as const, data: scatter.data }],
        };
      }

      case 'pie': {
        const valueField = context.metrics[0];
        if (!valueField || context.dimensions.length === 0 || !('data' in data)) {
          return null;
        }
        const pie = data as PieResponse;
        return {
          ...commonOptions,
          tooltip: pieTooltip,
          legend: { orient: 'vertical' as const, left: 'left' },
          series: [
            {
              name: labelOf(valueField),
              type: 'pie' as const,
              radius: pieRadius,
              data: pie.data.map((item) => ({ name: item.name, value: item.value })),
              emphasis: pieEmphasis,
              label: { formatter: '{b}: {d}%' },
            },
          ],
          ...colorOf(),
        };
      }

      case 'funnel': {
        // funnel（漏斗图，R-59）复用 PieResponse（后端 GetProcessor(funnel) → PieProcessor），
        // 渲染为 ECharts funnel。查询已强制按 value 降序（composeChartQueryRequest），
        // series.sort:'descending' 是防御性兜底：即便数据未排好也按值降序展示。
        const valueField = context.metrics[0];
        if (!valueField || context.dimensions.length === 0 || !('data' in data)) {
          return null;
        }
        const pie = data as PieResponse;
        return {
          ...commonOptions,
          tooltip: { trigger: 'item' as const, formatter: '{b}: {c}' },
          legend: { orient: 'vertical' as const, left: 'left' },
          series: [
            {
              name: labelOf(valueField),
              type: 'funnel' as const,
              sort: 'descending' as const,
              gap: 2,
              label: { position: 'inside' as const, formatter: '{b}: {c}' },
              data: pie.data.map((item) => ({ name: item.name, value: item.value })),
            },
          ],
          ...colorOf(),
        };
      }

      case 'line':
      case 'bar':
      case 'area': {
        if (
          context.dimensions.length === 0 ||
          context.metrics.length === 0 ||
          !('x_axis' in data)
        ) {
          return null;
        }
        // 'x_axis' 判别已将该臂收窄为 ChartAxisResponse。
        const axis: AxisResponse = sortByTemporalAxis(data);
        const categoryAxis = {
          type: 'category' as const,
          data: axis.x_axis,
          name: labelOf(context.dimensions[0]),
          axisLabel: categoryAxisLabel(axis.x_axis.length),
        };
        const valueAxis = { type: 'value' as const };
        return {
          ...commonOptions,
          xAxis: horizontalBar ? valueAxis : categoryAxis,
          yAxis: horizontalBar ? categoryAxis : valueAxis,
          series: applyStack(
            axis.series.map((s) => ({
              name: labelOf(s.name),
              type: chartType === 'bar' ? ('bar' as const) : ('line' as const),
              ...(chartType === 'area' ? { areaStyle: {} } : {}),
              ...(chartType === 'bar' ? {} : { smooth: style.smooth, connectNulls: true }),
              data: s.data.map(toOptionValue),
            })),
            style.stack
          ),
          ...colorOf(),
        };
      }

      case 'histogram': {
        // 'bins' 判别已将该臂收窄为 HistogramResponse（空 bins 已被 isEmptyPayload 拦下）。
        if (!('bins' in data)) {
          return null;
        }
        const histogram: HistogramResponse = data;
        // 直方图用纵向条（类目轴在 y、自上而下排列），与柱状图（类目沿 x 横向展开、
        // 易联想到时间轴）在视觉语义上区分：柱状图看趋势，直方图看分布。
        return {
          ...commonOptions,
          xAxis: { type: 'value' as const },
          yAxis: {
            type: 'category' as const,
            data: histogram.bins.map((bin) => `${bin.bin_start} ~ ${bin.bin_end}`),
            inverse: true,
          },
          series: [
            {
              // value 槽位列名在 context.metrics[0]；缺失（防御）回退 'count'
              name: labelOf(context.metrics[0] ?? 'count'),
              type: 'bar' as const,
              data: histogram.bins.map((bin) => bin.count),
            },
          ],
          ...colorOf(),
        };
      }

      case 'radar': {
        // 'indicators' 判别已将该臂收窄为 RadarResponse（空 indicators 已被 isEmptyPayload 拦下）。
        if (!('indicators' in data) || !('series' in data)) {
          return null;
        }
        const radar: RadarResponse = data;
        if (radar.indicators.length === 0) {
          return null;
        }
        return {
          ...commonOptions,
          tooltip: { trigger: 'item' as const },
          legend: {
            data: radar.series.map((s) => s.name),
            orient: 'vertical' as const,
            left: 'left',
          },
          // 后端 RadarResponse 的 indicator.name / series.name 已是最终展示值
          // （indicator=维度值、series=value 别名；历史请求带 series_group 时=分组值），
          // 不再二次 labelOf——context.dimensions/metrics 是列名，语义与轴/系列名不同。
          radar: {
            indicator: radar.indicators.map((i) => ({ name: i.name, max: i.max })),
          },
          series: [
            {
              type: 'radar' as const,
              data: radar.series.map((s) => ({ name: s.name, value: s.values })),
            },
          ],
          ...colorOf(),
        };
      }

      case 'boxplot': {
        // 'q1' 判别已将该臂收窄为 BoxplotResponse（空/退化已由 isEmptyPayload 放行渲染贴零箱）。
        if (!('q1' in data) || !('outliers' in data)) {
          return null;
        }
        const box: BoxplotResponse = data;
        const boxName = labelOf(context.metrics[0] ?? 'value');
        return {
          ...commonOptions,
          tooltip: { trigger: 'item' as const },
          // 单箱：水平 value 轴承载统计值，类目轴只一条（箱名）。
          xAxis: { type: 'value' as const, scale: true },
          yAxis: { type: 'category' as const, data: [boxName] },
          series: [
            {
              // ECharts boxplot 数据项即五数概括 [min, Q1, median, Q3, max]。
              name: boxName,
              type: 'boxplot' as const,
              data: [[box.whisker_low, box.q1, box.median, box.q3, box.whisker_high]],
            },
            {
              // 离群点按 [值, 类目索引0] 落在同一箱的水平线上（唯一类目在索引 0）。
              name: '离群点',
              type: 'scatter' as const,
              data: box.outliers.map((o) => [o, 0]),
              symbolSize: 8,
            },
          ],
          ...colorOf(),
        };
      }

      case 'combo': {
        if (
          context.dimensions.length === 0 ||
          context.metrics.length === 0 ||
          !('x_axis' in data)
        ) {
          return null;
        }
        // 'x_axis' 判别已将该臂收窄为 ChartAxisResponse。
        const axis: AxisResponse = sortByTemporalAxis(data);
        // series→slot 映射：后端 AxisProcessor 逐指标产出一条 series，
        // name = metric.ResolveAlias()（默认等于列名）。据 metricSlots 建立「指标列名 →
        // yAxisIndex」映射（primary_values→0、secondary_values→1），再按 series.name 反查。
        // metricSlots 缺失（防御）或 name 反查不到（用户设了别名；或历史图表残留第二个维度组，
        // 后端按 dims[1:] 拆系列产出"别名 - 维度值"复合名——combo 渲染分支不精确支持该形态）时，
        // 一律回落 yAxisIndex 0（主轴），保证不抛异常、图仍渲染（退化限制见 task 报告）。
        const slotAxisIndex = new Map<string, number>();
        for (const metricSlot of context.metricSlots ?? []) {
          const axisIndex = metricSlot.slot === 'secondary_values' ? 1 : 0;
          for (const metricName of metricSlot.metrics) {
            slotAxisIndex.set(metricName, axisIndex);
          }
        }
        const categoryAxis = {
          type: 'category' as const,
          data: axis.x_axis,
          name: labelOf(context.dimensions[0]),
          axisLabel: categoryAxisLabel(axis.x_axis.length),
        };
        return {
          ...commonOptions,
          xAxis: categoryAxis,
          // 双 Y 轴：index 0 主轴（左）、index 1 次轴（右）——这是 combo 与单轴
          // bar/line/area 在 option 结构上的关键差异（yAxis 是数组而非单个对象）。
          yAxis: [
            { type: 'value' as const },
            { type: 'value' as const, position: 'right' as const },
          ],
          series: axis.series.map((s) => {
            const yAxisIndex = slotAxisIndex.get(s.name) ?? 0;
            return {
              name: labelOf(s.name),
              // 主轴指标渲染为柱、次轴指标渲染为线（双轴「组合」图的组合语义）。
              type: yAxisIndex === 1 ? ('line' as const) : ('bar' as const),
              yAxisIndex,
              ...(yAxisIndex === 1 ? { connectNulls: true } : {}),
              data: s.data.map(toOptionValue),
            };
          }),
          ...colorOf(),
        };
      }

      default:
        return null;
    }
  }

  // ---- legacy 裸行回退（旧结构/损坏/空配置的分享仍可达）：按列名索引行 ----
  const rows = data as RawRow[];

  // 散点图只需两个指标，维度可选
  if (chartType === 'scatter') {
    const [xField, yField] = context.metrics;
    if (!xField || !yField) {
      return null;
    }
    return {
      ...commonOptions,
      xAxis: { type: 'value', name: labelOf(xField) },
      yAxis: { type: 'value', name: labelOf(yField) },
      series: [
        {
          type: 'scatter' as const,
          data: rows.map((item) => [toOptionValue(item[xField]), toOptionValue(item[yField])]),
        },
      ],
    };
  }

  const xAxisField = context.dimensions[0];
  if (!xAxisField || context.metrics.length === 0) {
    return null;
  }
  const xAxisData = rows.map((item) => toCategoryValue(item[xAxisField]));
  const categoryAxis = {
    type: 'category' as const,
    data: xAxisData,
    name: labelOf(xAxisField),
    axisLabel: categoryAxisLabel(xAxisData.length),
  };
  const valueAxis = { type: 'value' as const };

  switch (chartType) {
    case 'line':
    case 'area':
    case 'bar':
      return {
        ...commonOptions,
        xAxis: horizontalBar ? valueAxis : categoryAxis,
        yAxis: horizontalBar ? categoryAxis : valueAxis,
        series: applyStack(
          context.metrics.map((yField) => ({
            name: labelOf(yField),
            type: chartType === 'bar' ? ('bar' as const) : ('line' as const),
            ...(chartType === 'area' ? { areaStyle: {} } : {}),
            ...(chartType === 'bar' ? {} : { smooth: style.smooth, connectNulls: true }),
            data: rows.map((item) => toOptionValue(item[yField])),
          })),
          style.stack
        ),
        ...colorOf(),
      };

    case 'pie': {
      const valueField = context.metrics[0];
      return {
        ...commonOptions,
        tooltip: pieTooltip,
        legend: { orient: 'vertical' as const, left: 'left' },
        series: [
          {
            name: labelOf(valueField),
            type: 'pie' as const,
            radius: pieRadius,
            data: rows.map((item) => ({
              name: pieSliceName(item[xAxisField] ?? item.name),
              value: toPieValue(item[valueField] ?? item.value),
            })),
            emphasis: pieEmphasis,
            label: { formatter: '{b}: {d}%' },
          },
        ],
        ...colorOf(),
      };
    }

    default:
      return null;
  }
}
