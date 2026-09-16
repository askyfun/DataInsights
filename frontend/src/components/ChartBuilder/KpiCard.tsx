import { Card, Statistic } from 'antd';

interface KpiCardProps {
  value: number;
  label: string;
  /** 单位后缀（AntD Statistic suffix），由调用方从前端配置侧传入（见组件 doc）。 */
  unit?: string;
  /**
   * 数字格式化标识（如千分位/百分比/小数位数）。已知限制（Task 1-5 范围内接受）：
   * 本组件当前只做最简支持——unit 作 Statistic suffix 展示；复杂 format 字符串的
   * 解析超出本任务范围，暂不消费该字段（props 保留接口，后续任务需要时再接线）。
   */
  format?: string;
  loading?: boolean;
}

/**
 * KPI 单值卡（R-51）：用 AntD Statistic 渲染标量聚合结果 {value, label}，不走 ECharts。
 * 调用场景：ChartBuilder renderPreview 与 ShareView 的 chartType==='kpi' 分支。
 *
 * unit/format 数据来源：后端 KpiResponse.Unit/Format 因 wire 协议限制恒为空
 * （MetricConfig 无 Unit/Format 字段，见 backend/internal/query/processor.go 的
 * KpiProcessor 注释），因此由调用方从前端配置侧传入——builder 用 store 的
 * metricUnits[bindingId]/metricFormats[bindingId]，share 用 chartDoc.fieldMeta[bindingId]。
 */
const KpiCard: React.FC<KpiCardProps> = ({ value, label, unit, loading }) => {
  return (
    <Card loading={loading} style={{ height: '100%' }}>
      <Statistic title={label} value={value} suffix={unit} />
    </Card>
  );
};

export default KpiCard;
