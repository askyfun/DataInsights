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
import { Chart, chartsApi, sharesApi } from '../api';
import TableChart from '../components/ChartBuilder/TableChart';
import { type ChartType, migrateChartConfig } from '../lib/chartConfigSchema';

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

const ShareView: React.FC = () => {
  const { token } = useParams<{ token: string }>();
  const [loading, setLoading] = useState(true);
  const [authLoading, setAuthLoading] = useState(false);
  const [shareInfo, setShareInfo] = useState<ShareInfo | null>(null);
  const [chart, setChart] = useState<Chart | null>(null);
  const [chartData, setChartData] = useState<any[]>([]);
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
      for (const name of group.fields) {
        const meta = chartDoc.fieldMeta[name];
        labels[name] = meta?.label || meta?.alias || name;
      }
    }
    return labels;
  }, [chartDoc]);

  // Generate chart option
  const getChartOption = useCallback(() => {
    if (!chart || !chartDoc || chartData.length === 0) {
      return null;
    }

    // 表格类走 TableChart 渲染，不使用 ECharts option
    if (chartDoc.chartType === 'table' || chartDoc.chartType === 'pivot') {
      return null;
    }

    const dimensionNames = chartDoc.query.dimensionGroups.flatMap((g) => g.fields);
    const metricNames = chartDoc.query.metricGroups.flatMap((g) => g.fields);
    const labelOf = (name: string) => displayLabels[name] || name;

    const commonOptions = {
      title: {
        text: chartDoc.title || chart.name,
        left: 'center',
      },
      tooltip: {
        trigger: 'axis',
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '3%',
        containLabel: true,
      },
    };

    // 散点图只需两个指标，维度可选
    if (chartDoc.chartType === 'scatter') {
      const [xField, yField] = metricNames;
      if (!xField || !yField) {
        return null;
      }
      return {
        ...commonOptions,
        xAxis: {
          type: 'value',
          name: labelOf(xField),
        },
        yAxis: {
          type: 'value',
          name: labelOf(yField),
        },
        series: [
          {
            type: 'scatter',
            data: chartData.map((item) => [item[xField], item[yField]]),
          },
        ],
      };
    }

    const xAxisField = dimensionNames[0];
    if (!xAxisField || metricNames.length === 0) {
      return null;
    }

    const xAxisData = chartData.map((item) => item[xAxisField]);

    switch (chartDoc.chartType) {
      case 'line':
      case 'area':
        return {
          ...commonOptions,
          xAxis: {
            type: 'category',
            data: xAxisData,
          },
          yAxis: {
            type: 'value',
          },
          series: metricNames.map((yField) => ({
            name: labelOf(yField),
            type: 'line',
            ...(chartDoc.chartType === 'area' ? { areaStyle: {} } : {}),
            data: chartData.map((item) => item[yField]),
          })),
        };

      case 'bar':
        return {
          ...commonOptions,
          xAxis: {
            type: 'category',
            data: xAxisData,
          },
          yAxis: {
            type: 'value',
          },
          series: metricNames.map((yField) => ({
            name: labelOf(yField),
            type: 'bar',
            data: chartData.map((item) => item[yField]),
          })),
        };

      case 'pie': {
        const valueField = metricNames[0];
        return {
          ...commonOptions,
          series: [
            {
              name: labelOf(valueField) || 'Value',
              type: 'pie',
              radius: '50%',
              data: chartData.map((item) => ({
                name: item[xAxisField],
                value: item[valueField],
              })),
              emphasis: {
                itemStyle: {
                  shadowBlur: 10,
                  shadowOffsetX: 0,
                  shadowColor: 'rgba(0, 0, 0, 0.5)',
                },
              },
            },
          ],
        };
      }

      default:
        return null;
    }
  }, [chart, chartData, chartDoc, displayLabels]);

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
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '100vh',
          background: '#f0f2f5',
        }}
      >
        <Spin size="large" />
      </div>
    );
  }

  // Expired share
  if (shareInfo?.expires_at && new Date(shareInfo.expires_at) < new Date()) {
    return (
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '100vh',
          background: '#f0f2f5',
          padding: 24,
        }}
      >
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
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '100vh',
          background: '#f0f2f5',
          padding: 24,
        }}
      >
        <Card style={{ width: 400, textAlign: 'center' }}>
          <div style={{ marginBottom: 24 }}>
            <LockOutlined style={{ fontSize: 48, color: '#faad14' }} />
          </div>
          <Title level={4}>Password Required</Title>
          <Text type="secondary" style={{ display: 'block', marginBottom: 24 }}>
            This chart is password protected. Please enter the password to view.
          </Text>
          <Space direction="vertical" style={{ width: '100%' }}>
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
  const chartOption = getChartOption();
  const isTableLike = chartDoc?.chartType === 'table' || chartDoc?.chartType === 'pivot';

  return (
    <div style={{ minHeight: '100vh', background: '#f0f2f5', padding: 24 }}>
      <Card>
        {/* Header */}
        <div
          style={{
            marginBottom: 24,
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
          }}
        >
          <Space>
            {chart && getChartTypeIcon(chart.chart_type)}
            <Title level={4} style={{ margin: 0 }}>
              {chart?.name || 'Shared Chart'}
            </Title>
          </Space>
          <Space>
            <Tag icon={<ShareAltOutlined />} color="blue">
              Shared View
            </Tag>
          </Space>
        </div>

        {/* Chart */}
        {chartDataLoading ? (
          <div style={{ textAlign: 'center', padding: '100px 0' }}>
            <Spin size="large" />
            <div style={{ marginTop: 16 }}>
              <Text type="secondary">Loading chart data...</Text>
            </div>
          </div>
        ) : isTableLike && chartData.length > 0 ? (
          <TableChart data={chartData} loading={false} columnLabels={displayLabels} />
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
