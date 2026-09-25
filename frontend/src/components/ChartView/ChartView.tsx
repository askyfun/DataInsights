import { Result } from 'antd';
import ReactECharts from 'echarts-for-react';
import { type CSSProperties, useMemo } from 'react';
import { type Chart, type ChartDataResponse, isPivotV2Payload } from '../../api';
import { type ChartType, migrateChartConfig } from '../../lib/chartConfigSchema';
import { buildChartOption, isEmptyPayload, normalizeChartStyle } from '../../lib/chartOptions';
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
}

/** 分享页的 ECharts 容器尺寸（改造前的原值，勿改：ShareView 行为按此冻结）。 */
const SHARE_ECHARTS_STYLE: CSSProperties = { height: 'calc(100vh - 250px)', minHeight: 400 };

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
 * v1 持久化文档的组字段本身就是稳定列名，可直接用于按列名索引的响应负载，
 * 故无需运行时字段列表——这也是本组件能被仪表盘块直接复用的原因。
 */
const ChartView: React.FC<ChartViewProps> = ({ chart, data, echartsStyle }) => {
  // 统一经迁移函数读取 v1 文档；ShareView 没有字段列表，
  // v1 配置的组字段本身就是稳定列名，可直接用于按列名索引的数据行。
  const chartDoc = useMemo(
    () => migrateChartConfig(chart.config, chart.chart_type as ChartType),
    [chart]
  );

  // 展示名：优先 fieldMeta 的 label/alias，其次列名
  const displayLabels = useMemo(() => {
    const labels: Record<string, string> = {};
    for (const group of [...chartDoc.query.dimensionGroups, ...chartDoc.query.metricGroups]) {
      for (const binding of group.bindings) {
        // fieldMeta 按 bindingId 取（v2 键），但 labels 的键保持列名——buildChartOption
        // 用列名匹配结构化响应的 series name / x_axis（后端目前仍按列名/别名返回）。
        // 已知歧义（Task 0-6+0-8 解决）：同一列名有多个 binding（多个不同 label）时，
        // 共享列名键上后写入者覆盖先写入者。
        const meta = chartDoc.fieldMeta[binding.bindingId];
        labels[binding.field] = meta?.label || meta?.alias || binding.field;
      }
    }
    return labels;
  }, [chartDoc]);

  // 样式来自持久化 v1 文档的 style 小节（schema 上是 unknown）：
  // 缺失/形状非法时经 normalizeChartStyle 回落 ChartStyleConfig 默认值。
  const chartStyle = useMemo(() => normalizeChartStyle(chartDoc.style), [chartDoc]);

  // option 构造与 builder 预览共用 buildChartOption 唯一出口（结构化聚合
  // 负载与 legacy 裸行回退两臂都在函数内部处理）。
  const chartOption = useMemo(() => {
    // combo 双轴：按图型定义的 metric 槽位（primary_values/secondary_values）与持久化文档的
    // metricGroups 按 index 对齐派生 metricSlots。本组件无运行时字段列表，持久化的
    // binding.field 本身即稳定列名，可直接使用。槽位名取自定义的 fieldGroup id（非组的位置 id）。
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
                (b) => chartDoc.fieldMeta[b.bindingId]?.alias || b.field
              ),
            }))
        : undefined;
    return buildChartOption(chartDoc.chartType, data, chartStyle, displayLabels, {
      title: chartDoc.title || chart.name,
      dimensions: chartDoc.query.dimensionGroups.flatMap((g) => g.bindings.map((b) => b.field)),
      metrics: chartDoc.query.metricGroups.flatMap((g) => g.bindings.map((b) => b.field)),
      metricSlots,
    });
  }, [chart, chartDoc, data, chartStyle, displayLabels]);

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
      <div style={echartsStyle ?? SHARE_ECHARTS_STYLE}>
        <ReactECharts
          option={chartOption}
          style={{ height: '100%', width: '100%' }}
          opts={{ renderer: 'canvas' }}
        />
      </div>
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
