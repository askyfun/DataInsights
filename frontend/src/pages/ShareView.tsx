import {
  BarChartOutlined,
  LineChartOutlined,
  LockOutlined,
  PieChartOutlined,
  ShareAltOutlined,
} from '@ant-design/icons';
import { Button, Card, Input, Result, Space, Spin, Tag, Typography } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Chart, type ChartDataResponse, chartsApi, datasetsApi, sharesApi } from '../api';
import ChartView from '../components/ChartView/ChartView';
import LoadingPlaceholder from '../components/LoadingPlaceholder';
import PageHeader from '../components/PageHeader';

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
  const [chartData, setChartData] = useState<ChartDataResponse>([]);
  // 列 ID → 列名：持久化配置引用列 ID，负载键与输出别名是列名，渲染前需翻译一次
  const [fieldNames, setFieldNames] = useState<Record<string, string>>({});
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

      const loadedChart = chartResponse.data.data;
      setChart(loadedChart);
      setChartData(dataResponse.data.data);

      // 字段列表用于把配置里的列 ID 换成列名。取不到（数据集被删/无权限）时保持空映射，
      // ChartView 会回落 field 本身，页面仍可渲染。
      try {
        const columnsResponse = await datasetsApi.getColumns(loadedChart.dataset_id);
        const names: Record<string, string> = {};
        for (const column of columnsResponse.data.data ?? []) {
          names[column.id] = column.name;
        }
        setFieldNames(names);
      } catch (columnError) {
        console.error('Failed to fetch dataset columns:', columnError);
        setFieldNames({});
      }
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
  // 迁移函数读取、按 chart_type 分派、四臂兜底全部落在 ChartView 内部，
  // 分享页不再持有第二份渲染逻辑，这里只负责加载态与「图表尚未就绪」。
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
        {chartDataLoading || !chart ? (
          <LoadingPlaceholder text="Loading chart data..." />
        ) : (
          <ChartView chart={chart} data={chartData} fieldNames={fieldNames} />
        )}
      </Card>
    </div>
  );
};

export default ShareView;
