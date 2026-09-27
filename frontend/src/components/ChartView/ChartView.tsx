import { Result } from 'antd';
import ReactECharts from 'echarts-for-react';
import { type CSSProperties, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { type Chart, type ChartDataResponse, isPivotV2Payload } from '../../api';
import { type ChartType, migrateChartConfig } from '../../lib/chartConfigSchema';
import { buildChartOption, isEmptyPayload, normalizeChartStyle } from '../../lib/chartOptions';
import { useResolvedTheme } from '../../lib/theme';
import { chartDefinitions } from '../ChartBuilder/chartDefinitions';
import KpiCard from '../ChartBuilder/KpiCard';
import PivotTable from '../ChartBuilder/PivotTable';
import TableChart from '../ChartBuilder/TableChart';

/** legacy 回退的裸数据行按列名索引 */
type RawRow = Record<string, unknown>;

interface ChartViewProps {
  /** 图表资产本体（只读）：chart_type 决定渲染臂，config 经迁移函数读取。 */
  chart: Chart;
  /**
   * 图表数据负载，与 `GET /api/charts/{id}/data` 的 Envelope data 同形
   * （即 ChartDataResponse：聚合负载或 legacy 裸行数组）。
   */
  data: ChartDataResponse;
  /**
   * ECharts 容器尺寸。默认值是分享页的口径（按视口铺满一屏）；
   * 仪表盘块内应传 `{ height: '100%', minHeight: 0 }`，让图表随块高伸缩。
   */
  echartsStyle?: CSSProperties;
  /**
   * 列 ID → 列名映射（数据集列列表派生）。持久化文档的 `binding.field` 是**列的
   * 稳定 id**，而 SQL 输出别名与响应负载键是列名，因此本组件渲染前必须做一次
   * 翻译。缺省/未命中时回落 field 本身——历史文档里 field 本就是列名，行为不变。
   */
  fieldNames?: Record<string, string>;
}

/** 分享页的 ECharts 容器尺寸（改造前的原值，勿改：ShareView 行为按此冻结）。 */
const SHARE_ECHARTS_STYLE: CSSProperties = { height: 'calc(100vh - 250px)', minHeight: 400 };

/** 离屏主题探针样式：不影响布局，仅供 getComputedStyle 读取当前主题的 --dr-*。 */
const PROBE_STYLE: CSSProperties = {
  position: 'absolute',
  width: 0,
  height: 0,
  overflow: 'hidden',
  pointerEvents: 'none',
};

/**
 * 「图表 + 数据 → 渲染」的唯一出口。
 *
 * 调用场景：分享页（ShareView）与仪表盘画布的图表块（DashboardEditor）。
 * 两者共用同一套按 chart_type 分派的渲染臂，避免同一张图在两个页面长得不一样：
 *   - kpi   → KpiCard（AntD Statistic，不走 ECharts）
 *   - pivot → PivotTable（仅 v2 交叉表负载；v1 平铺 pivot 仍走 TableChart）
 *   - table → TableChart
 *   - 其余  → ReactECharts（option 由 buildChartOption 构造）
 * 任一臂都取不到可渲染内容时回落 Result 兜底，**不抛异常**。
 *
 * 数据来源说明：本组件不发起请求也不做加载态（调用方自己有更贴切的加载语汇）。
 * 字段标识来自持久化文档（列 id），而负载键与 SQL 输出别名是列名，翻译由调用方
 * 通过 `fieldNames` 注入（见该 prop 的说明）。
 */
const ChartView: React.FC<ChartViewProps> = ({ chart, data, echartsStyle, fieldNames }) => {
  const chartDoc = useMemo(
    () => migrateChartConfig(chart.config, chart.chart_type as ChartType),
    [chart]
  );

  /** 列 id → 列名；映射缺失（历史文档 field 本就是列名）时原样返回。
   * 只认自有属性：fieldId 恰为 constructor/toString 等继承成员时不得穿透原型链 */
  const nameOf = useCallback(
    (field: string): string =>
      (fieldNames && Object.getOwnPropertyDescriptor(fieldNames, field)?.value) ?? field,
    [fieldNames]
  );

  // 展示名：优先 fieldMeta 的 label/alias，其次列名
  const displayLabels = useMemo(() => {
    // 无原型容器：列名为 __proto__ 时 `labels[name] = ...` 才不会落进 setter 被吞
    const labels: Record<string, string> = Object.create(null);
    for (const group of [...chartDoc.query.dimensionGroups, ...chartDoc.query.metricGroups]) {
      for (const binding of group.bindings) {
        // fieldMeta 按 bindingId 取（v2 键），而 labels 的键必须是**列名**——
        // buildChartOption 用列名匹配结构化响应的 series name / x_axis。
        // 已知歧义：同一列有多个 binding（多个不同 label）时，共享列名键上
        // 后写入者覆盖先写入者。
        const meta = chartDoc.fieldMeta[binding.bindingId];
        const name = nameOf(binding.fieldId);
        labels[name] = meta?.label || meta?.alias || name;
      }
    }
    return labels;
  }, [chartDoc, nameOf]);

  // 样式来自持久化 v1 文档的 style 小节（schema 上是 unknown）：
  // 缺失/形状非法时经 normalizeChartStyle 回落 ChartStyleConfig 默认值。
  const chartStyle = useMemo(() => normalizeChartStyle(chartDoc.style), [chartDoc]);

  // option 构造与 builder 预览共用 buildChartOption 唯一出口（结构化聚合
  // 负载与 legacy 裸行回退两臂都在函数内部处理）。
  //
  // 主题宿主探针：ECharts 在 canvas 上渲染取不到 CSS 变量，须在构造 option 时把颜色
  // 算成字面值（见 chartPalette）。用一个离屏探针元素承载当前主题的 --dr-* 值：
  //  - 挂载前探针不存在 → 取浅色兜底；
  //  - 主题变化会连带重算 resolvedTheme → useMemo 重跑，此时 <html data-theme> 已由
  //    store 先行更新，探针（若已挂载）即反映新主题 → 实时重绘。
  // resolvedTheme 入依赖是重算的扳机（其值本身不参与 option 构造，只驱动重算时机）。
  const resolvedTheme = useResolvedTheme();
  const hostRef = useRef<HTMLDivElement>(null);
  const [hostEl, setHostEl] = useState<HTMLDivElement | null>(null);
  useEffect(() => {
    setHostEl(hostRef.current);
  }, []);

  const chartOption = useMemo(() => {
    // combo 双轴：按图型定义的 metric 槽位（primary_values/secondary_values）与持久化文档的
    // metricGroups 按 index 对齐派生 metricSlots。槽位名取自定义的 fieldGroup id（非组的位置 id）。
    // metrics 用 alias 优先（列名兜底），与 displayLabels 读 fieldMeta.alias 的方式一致——
    // 后端 series 名按 ResolveAlias()（alias 优先，列名兜底）生成，若这里只填列名，
    // 带别名的 series 会反查不到槽位而被静默分配到主轴。
    const metricSlots =
      chartDoc.chartType === 'combo'
        ? chartDefinitions[chartDoc.chartType].fieldGroups
            .filter((group) => group.kind === 'metric')
            .map((def, index) => ({
              slot: def.id,
              metrics: (chartDoc.query.metricGroups[index]?.bindings ?? []).map(
                (b) => chartDoc.fieldMeta[b.bindingId]?.alias || nameOf(b.fieldId)
              ),
            }))
        : undefined;
    return buildChartOption(
      chartDoc.chartType,
      data,
      chartStyle,
      displayLabels,
      {
        title: chartDoc.title || chart.name,
        dimensions: chartDoc.query.dimensionGroups.flatMap((g) =>
          g.bindings.map((b) => nameOf(b.fieldId))
        ),
        metrics: chartDoc.query.metricGroups.flatMap((g) =>
          g.bindings.map((b) => nameOf(b.fieldId))
        ),
        metricSlots,
      },
      resolvedTheme,
      hostEl
    );
  }, [chart, chartDoc, data, chartStyle, displayLabels, nameOf, hostEl, resolvedTheme]);

  const isTableLike = chartDoc.chartType === 'table' || chartDoc.chartType === 'pivot';
  // 聚合负载的 table/pivot 臂：TableResponse 带 pagination、PivotResponse 不带，
  // 两者都有 columns + data（行由后端按维度在前/指标别名在后组装，
  // pagination 仅回显服务端窗口，本组件是静态视图不接翻页）。
  const tablePayload = isTableLike && !Array.isArray(data) && 'columns' in data ? data : null;

  // pivot v2 臂（R-53）：交叉表负载（cells+col_headers+row_headers）按响应形状判别——
  // v1 平铺 pivot（{columns,data}）时 pivotPayload 为 null，仍走下方 TableChart 分支。
  const isPivot = chartDoc.chartType === 'pivot';
  const pivotPayload = isPivot && isPivotV2Payload(data) ? data : null;

  // kpi 臂（R-51）：标量 {value, label}（ChartKpiResponse），不走 ECharts。
  // unit/format 从持久化 v2 文档的 fieldMeta 按 kpi 唯一 metric 槽位
  // （metricGroups[0] 的首个 binding）的 bindingId 取——后端 KpiResponse.Unit/Format
  // 恒为空（wire 协议限制，见 KpiProcessor 注释），展示信息只来自配置侧。
  const isKpi = chartDoc.chartType === 'kpi';
  const kpiPayload =
    isKpi && !Array.isArray(data) && 'value' in data && 'label' in data ? data : null;
  const kpiBindingId = isKpi ? chartDoc.query.metricGroups[0]?.bindings[0]?.bindingId : undefined;
  const kpiMeta = kpiBindingId ? chartDoc.fieldMeta[kpiBindingId] : undefined;

  if (isKpi) {
    return (
      <KpiCard
        value={kpiPayload?.value ?? 0}
        label={kpiPayload?.label ?? ''}
        unit={kpiMeta?.unit}
        format={kpiMeta?.format}
        loading={false}
      />
    );
  }

  if (pivotPayload) {
    return <PivotTable data={pivotPayload} columnLabels={displayLabels} />;
  }

  if (isTableLike && !isEmptyPayload(data)) {
    return (
      <TableChart
        data={tablePayload ? tablePayload.data : (data as RawRow[])}
        columns={tablePayload ? tablePayload.columns : undefined}
        loading={false}
        columnLabels={displayLabels}
      />
    );
  }

  if (chartOption) {
    return (
      <>
        {/* 离屏主题探针：承载当前主题的 --dr-* 供 chartPalette 取字面色值（见上方说明）。
            aria-hidden + 离屏定位，不参与布局。 */}
        <div ref={hostRef} className="dr-chart-host" aria-hidden style={PROBE_STYLE} />
        <div style={echartsStyle ?? SHARE_ECHARTS_STYLE}>
          <ReactECharts
            option={chartOption}
            style={{ height: '100%', width: '100%' }}
            opts={{ renderer: 'canvas' }}
          />
        </div>
      </>
    );
  }

  return (
    <Result
      status="warning"
      title="Unable to display chart"
      subTitle="The chart configuration may be invalid or no data is available."
    />
  );
};

export default ChartView;
