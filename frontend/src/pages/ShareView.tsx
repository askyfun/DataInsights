import {
  BarChartOutlined,
  LineChartOutlined,
  LockOutlined,
  PieChartOutlined,
  ShareAltOutlined,
} from '@ant-design/icons';
import { Button, Card, Input, Result, Space, Spin, Tag, Typography } from 'antd';
import ReactECharts from 'echarts-for-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Chart, type ChartDataResponse, chartsApi, isPivotV2Payload, sharesApi } from '../api';
import { chartDefinitions } from '../components/ChartBuilder/chartDefinitions';
import KpiCard from '../components/ChartBuilder/KpiCard';
import PivotTable from '../components/ChartBuilder/PivotTable';
import TableChart from '../components/ChartBuilder/TableChart';
import LoadingPlaceholder from '../components/LoadingPlaceholder';
import PageHeader from '../components/PageHeader';
import { type ChartType, migrateChartConfig } from '../lib/chartConfigSchema';
import { buildChartOption, isEmptyPayload, normalizeChartStyle } from '../lib/chartOptions';

const { Title, Text } = Typography;

interface ShareInfo {
  id: number;
  token: string;
  chart_id: number;
  password?: string;
  // The wire sends null (not absent) for an unset expires_at; created_at
  // mirrors the optional shape for locally built ShareInfo values.
  expires_at?: string | null;
  created_at?: string | null;
}

/** legacy 回退的裸数据行按列名索引 */
type RawRow = Record<string, unknown>;

const ShareView: React.FC = () => {
  const { token } = useParams<{ token: string }>();
  const [loading, setLoading] = useState(true);
  const [authLoading, setAuthLoading] = useState(false);
  const [shareInfo, setShareInfo] = useState<ShareInfo | null>(null);
  const [chart, setChart] = useState<Chart | null>(null);
  const [chartData, setChartData] = useState<ChartDataResponse>([]);
  const [chartDataLoading, setChartDataLoading] = useState(false);
  const [needsPassword, setNeedsPassword] = useState(false);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [password, setPassword] = useState('');

  // Fetch chart data
  const fetchChart = useCallback(async (chartId: number) => {
    setChartDataLoading(true);
    try {
      const [chartResponse, dataResponse] = await Promise.all([
        chartsApi.getById(chartId),
        chartsApi.getChartData(chartId),
      ]);

      setChart(chartResponse.data.data);
      setChartData(dataResponse.data.data);
    } catch (error: any) {
      console.error('Failed to fetch chart:', error);
    } finally {
      setChartDataLoading(false);
    }
  }, []);

  // Fetch share info
  const fetchShareInfo = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const response = await sharesApi.getByToken(token);
      const share = response.data.data;

      // Check if share has expired
      if (share.expires_at && new Date(share.expires_at) < new Date()) {
        setShareInfo(share);
        setNeedsPassword(false);
        return;
      }

      setShareInfo(share);

      // Check if password is required
      if (share.has_password) {
        setNeedsPassword(true);
        setLoading(false);
        return;
      }

      // No password required, fetch chart directly
      await fetchChart(share.chart_id);
    } catch (error: any) {
      // 业务错误经拦截器 reject 为裸 Error（后端全部 200 信封），不存在
      // 401/403 状态分支；密码门由上方成功信封的 has_password 决定。
      console.error('Failed to fetch share:', error);
    } finally {
      setLoading(false);
    }
  }, [token, fetchChart]);

  // Fetch share info on mount
  useEffect(() => {
    if (token) {
      fetchShareInfo();
    }
  }, [token, fetchShareInfo]);

  // Verify password and fetch chart
  const handleVerifyPassword = async () => {
    if (!password) {
      setPasswordError('Please enter the password');
      return;
    }

    setAuthLoading(true);
    setPasswordError(null);

    try {
      if (!token) return;
      const response = await sharesApi.verifyPassword(token, password);
      setNeedsPassword(false);
      await fetchChart(response.data.data.chart_id);
    } catch (error: any) {
      // 密码错误同样以 200 信封返回并被拦截器 reject 为裸 Error，
      // error.message 即后端 msg；不存在 401/403 分支。
      setPasswordError(error.message || 'Verification failed');
    } finally {
      setAuthLoading(false);
    }
  };

  // 统一经迁移函数读取 v1 文档；ShareView 没有字段列表，
  // v1 配置的组字段本身就是稳定列名，可直接用于按列名索引的数据行。
  const chartDoc = useMemo(
    () => (chart ? migrateChartConfig(chart.config, chart.chart_type as ChartType) : null),
    [chart]
  );

  // 展示名：优先 fieldMeta 的 label/alias，其次列名
  const displayLabels = useMemo(() => {
    const labels: Record<string, string> = {};
    if (!chartDoc) return labels;
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
  const chartStyle = useMemo(() => normalizeChartStyle(chartDoc?.style), [chartDoc]);

  // option 构造与 builder 预览共用 buildChartOption 唯一出口（结构化聚合
  // 负载与 legacy 裸行回退两臂都在函数内部处理）。
  const chartOption = useMemo(() => {
    if (!chart || !chartDoc) {
      return null;
    }
    // combo 双轴：按图型定义的 metric 槽位（primary_values/secondary_values）与持久化文档的
    // metricGroups 按 index 对齐派生 metricSlots。ShareView 无运行时字段列表，持久化的
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
    return buildChartOption(chartDoc.chartType, chartData, chartStyle, displayLabels, {
      title: chartDoc.title || chart.name,
      dimensions: chartDoc.query.dimensionGroups.flatMap((g) => g.bindings.map((b) => b.field)),
      metrics: chartDoc.query.metricGroups.flatMap((g) => g.bindings.map((b) => b.field)),
      metricSlots,
    });
  }, [chart, chartDoc, chartData, chartStyle, displayLabels]);

  // Get chart type icon
  const getChartTypeIcon = (type: string) => {
    switch (type) {
      case 'line':
        return <LineChartOutlined />;
      case 'pie':
        return <PieChartOutlined />;
      default:
        return <BarChartOutlined />;
    }
  };

  // Loading state
  if (loading) {
    return (
      <div className="dr-page dr-page--center">
        <Spin size="large" />
      </div>
    );
  }

  // Expired share
  if (shareInfo?.expires_at && new Date(shareInfo.expires_at) < new Date()) {
    return (
      <div className="dr-page dr-page--center">
        <Result
          status="warning"
          title="Share Expired"
          subTitle="This share link has expired and is no longer accessible."
        />
      </div>
    );
  }

  // Password input
  if (needsPassword) {
    return (
      <div className="dr-page dr-page--center">
        <Card style={{ width: 400, textAlign: 'center' }}>
          <div style={{ marginBottom: 24 }}>
            <LockOutlined style={{ fontSize: 48, color: '#faad14' }} />
          </div>
          <Title level={4}>Password Required</Title>
          <Text type="secondary" style={{ display: 'block', marginBottom: 24 }}>
            This chart is password protected. Please enter the password to view.
          </Text>
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Input.Password
              placeholder="Enter password"
              value={password}
              onChange={(e) => {
                setPassword(e.target.value);
                setPasswordError(null);
              }}
              onPressEnter={handleVerifyPassword}
              status={passwordError ? 'error' : undefined}
            />
            {passwordError && (
              <Text type="danger" style={{ fontSize: 12 }}>
                {passwordError}
              </Text>
            )}
            <Button type="primary" loading={authLoading} onClick={handleVerifyPassword} block>
              View Chart
            </Button>
          </Space>
        </Card>
      </div>
    );
  }

  // Chart display
  const isTableLike = chartDoc?.chartType === 'table' || chartDoc?.chartType === 'pivot';
  // 聚合负载的 table/pivot 臂：TableResponse 带 pagination、PivotResponse 不带，
  // 两者都有 columns + data（行由后端按维度在前/指标别名在后组装，
  // pagination 仅回显服务端窗口，分享页是静态视图不接翻页）。
  const tablePayload =
    isTableLike && !Array.isArray(chartData) && 'columns' in chartData ? chartData : null;

  // pivot v2 臂（R-53）：交叉表负载（cells+col_headers+row_headers）按响应形状判别——
  // v1 平铺 pivot（{columns,data}）时 pivotPayload 为 null，仍走下方 TableChart 分支。
  const isPivot = chartDoc?.chartType === 'pivot';
  const pivotPayload = isPivot && isPivotV2Payload(chartData) ? chartData : null;

  // kpi 臂（R-51）：标量 {value, label}（ChartKpiResponse），不走 ECharts。
  // unit/format 从持久化 v2 文档的 fieldMeta 按 kpi 唯一 metric 槽位
  // （metricGroups[0] 的首个 binding）的 bindingId 取——后端 KpiResponse.Unit/Format
  // 恒为空（wire 协议限制，见 KpiProcessor 注释），展示信息只来自配置侧。
  const isKpi = chartDoc?.chartType === 'kpi';
  const kpiPayload =
    isKpi && !Array.isArray(chartData) && 'value' in chartData && 'label' in chartData
      ? chartData
      : null;
  const kpiBindingId = isKpi ? chartDoc?.query.metricGroups[0]?.bindings[0]?.bindingId : undefined;
  const kpiMeta = kpiBindingId ? chartDoc?.fieldMeta[kpiBindingId] : undefined;

  return (
    <div className="dr-page">
      <PageHeader
        icon={chart ? getChartTypeIcon(chart.chart_type) : undefined}
        title={chart?.name || 'Shared Chart'}
        extra={
          <Tag icon={<ShareAltOutlined />} color="blue">
            Shared View
          </Tag>
        }
      />

      <Card>
        {/* Chart */}
        {chartDataLoading ? (
          <LoadingPlaceholder text="Loading chart data..." />
        ) : isKpi ? (
          <KpiCard
            value={kpiPayload?.value ?? 0}
            label={kpiPayload?.label ?? ''}
            unit={kpiMeta?.unit}
            format={kpiMeta?.format}
            loading={false}
          />
        ) : pivotPayload ? (
          <PivotTable data={pivotPayload} columnLabels={displayLabels} />
        ) : isTableLike && !isEmptyPayload(chartData) ? (
          <TableChart
            data={tablePayload ? tablePayload.data : (chartData as RawRow[])}
            columns={tablePayload ? tablePayload.columns : undefined}
            loading={false}
            columnLabels={displayLabels}
          />
        ) : chartOption ? (
          <div style={{ height: 'calc(100vh - 250px)', minHeight: 400 }}>
            <ReactECharts
              option={chartOption}
              style={{ height: '100%', width: '100%' }}
              opts={{ renderer: 'canvas' }}
            />
          </div>
        ) : (
          <Result
            status="warning"
            title="Unable to display chart"
            subTitle="The chart configuration may be invalid or no data is available."
          />
        )}
      </Card>
    </div>
  );
};

export default ShareView;
