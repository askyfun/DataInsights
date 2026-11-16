import { RocketOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Form, Input, Space, Steps, Typography } from 'antd';
import { useState } from 'react';
import { useIntl } from 'react-intl';
import { useNavigate } from 'react-router-dom';
import { authApi, datasetsApi, queriesApi, sampleDatasetApi } from '../../api';
import { setBearerToken } from '../../lib/api/client';
import { setAuthConfig } from '../../lib/authConfig';
import { setSessionUser } from '../../lib/authSession';
import { buildFirstChartSpec, pickSampleDataset } from '../../lib/firstChart';
import { markSetupInProgress, markSetupSkipped, setupInProgress } from '../../lib/firstRun';

const { Title, Paragraph } = Typography;

/**
 * 用刚载入的示例数据配一张能直接看的图（#226 AC6）。
 *
 * 走的是既有「地址栏即分享」通道：把配置落成一条查询记录，向导拿到短码后
 * 用 `/chart-builder?q=<短码>` 打开——那条路径已经有单测与还原逻辑，不新造入口。
 * 任何一步失败都返回 null，调用方降级成「打开空画布」：**首启流程不能因为
 * 配示例图失败而卡住**，那正是这条链最不该出问题的地方。
 */
const prepareFirstChart = async (): Promise<{
  shortId: string;
  dimension: string;
  metric: string;
} | null> => {
  try {
    const list = await datasetsApi.getAll({ limit: 50 });
    const dataset = pickSampleDataset(list.data.data.items);
    if (!dataset) {
      return null;
    }
    const columns = await datasetsApi.getColumns(dataset.id);
    const built = buildFirstChartSpec(columns.data.data, dataset.name);
    if (!built) {
      return null;
    }
    const saved = await queriesApi.save({
      dataset_id: dataset.id,
      spec: built.spec,
      source_type: 'build',
    });
    return {
      shortId: saved.data.data.query_id,
      dimension: built.dimension.name,
      metric: built.metric.name,
    };
  } catch {
    return null;
  }
};

// 首启向导（R-101 / #226）：三步步进复用既有能力，不新造流程。
// 完成（出第一张图 / 载示例）后写会话级标记（见 lib/firstRun），本次访问不再顶替列表；
// 库里有了管理员之后 `needs_bootstrap` 自然为假，向导就不会再出现。
//
// allowSkip=false 用于强制鉴权（enforce=on）的部署：那时匿名"先逛逛"只会被守卫弹到登录页，
// 给一个点了就撞墙的按钮比不给更糟。
//
// onAdminCreated 是「刚建好管理员」的回执：守卫据此把向导保留在页面上（见 lib/firstRun
// 的 inProgress），否则建号成功本身就会把这个组件卸载掉。
const FirstRunGuide: React.FC<{
  onDone: () => void;
  onAdminCreated?: () => void;
  allowSkip?: boolean;
}> = ({ onDone, onAdminCreated, allowSkip = true }) => {
  const intl = useIntl();
  const navigate = useNavigate();
  // 建号成功后本会话会重新挂载（Guard 换了判据但组件仍可能被路由变化重挂），
  // 因此起始步从会话标记读：已经过了第①步就别让用户再建一次管理员。
  const [step, setStep] = useState(() => (setupInProgress() ? 1 : 0));
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  // 载入示例时顺手配好的示例图短码；null = 没配成，第③步降级成空画布。
  const [firstChart, setFirstChart] = useState<{
    shortId: string;
    dimension: string;
    metric: string;
  } | null>(null);

  const finish = () => {
    markSetupSkipped();
    onDone();
  };

  const createAdmin = async (v: {
    username: string;
    password: string;
    bootstrap_token?: string;
  }) => {
    setBusy(true);
    setErr(null);
    try {
      const r = await authApi.register({
        username: v.username,
        password: v.password,
        bootstrap_token: v.bootstrap_token ?? '',
      });
      setBearerToken(r.data.data.token);
      setSessionUser(r.data.data.user);
      // 「库里已有管理员」从此为真：把探测结果刷新掉，否则右上角入口在本次访问里
      // 还会继续说「初始化管理员」。失败不影响流程（只是文案慢一拍）。
      authApi
        .config()
        .then((cfg) => setAuthConfig(cfg.data.data))
        .catch(() => undefined);
      // ②③ 步要靠这个标记活下来：hasUser 已为真，仅凭后端事实向导会被自己卸掉。
      markSetupInProgress();
      onAdminCreated?.();
      setStep(1);
    } catch (e) {
      setErr(e instanceof Error ? e.message : intl.formatMessage({ id: 'login.failed' }));
    } finally {
      setBusy(false);
    }
  };

  const loadSample = async () => {
    setBusy(true);
    setErr(null);
    try {
      await sampleDatasetApi.load();
      const chart = await prepareFirstChart();
      setFirstChart(chart);
      setStep(2);
    } catch (e) {
      setErr(e instanceof Error ? e.message : intl.formatMessage({ id: 'setup.loadFailed' }));
    } finally {
      setBusy(false);
    }
  };

  // 向导现在会跨步骤常驻（见 lib/firstRun 的 inProgress），所以**任何**离开向导的跳转都必须
  // 先 finish()：只 navigate 的话，守卫仍按「向导进行中」把向导画回来，用户会觉得自己
  // 点了按钮却原地没动（上传/连库这两条正是走去别的页面的路径）。
  const leaveTo = (path: string) => {
    finish();
    navigate(path);
  };

  const goFirstChart = () => {
    if (!firstChart) {
      leaveTo('/chart-builder');
      return;
    }
    leaveTo(`/chart-builder?q=${firstChart.shortId}`);
  };

  return (
    <div style={{ display: 'flex', justifyContent: 'center', padding: '32px 16px' }}>
      <Card style={{ width: 620, maxWidth: '100%' }}>
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <Title level={4} style={{ margin: 0 }}>
            <RocketOutlined /> {intl.formatMessage({ id: 'setup.title' })}
          </Title>
          <Steps
            size="small"
            current={step}
            items={[
              { title: intl.formatMessage({ id: 'setup.stepAdmin' }) },
              { title: intl.formatMessage({ id: 'setup.stepData' }) },
              { title: intl.formatMessage({ id: 'setup.stepChart' }) },
            ]}
          />
          {err && <Alert type="error" showIcon message={err} />}

          {step === 0 && (
            <>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                {intl.formatMessage({ id: 'setup.adminHint' })}
              </Paragraph>
              <Paragraph type="secondary" style={{ marginBottom: 0, fontSize: 12 }}>
                {intl.formatMessage({ id: 'setup.tokenHint' })}
              </Paragraph>
              <Form layout="vertical" onFinish={createAdmin} requiredMark={false}>
                <Form.Item
                  name="username"
                  label={intl.formatMessage({ id: 'login.username' })}
                  rules={[{ required: true }]}
                >
                  <Input autoComplete="username" />
                </Form.Item>
                <Form.Item
                  name="password"
                  label={intl.formatMessage({ id: 'login.password' })}
                  rules={[{ required: true }]}
                >
                  <Input.Password autoComplete="new-password" />
                </Form.Item>
                <Form.Item
                  name="bootstrap_token"
                  label={intl.formatMessage({ id: 'login.bootstrapToken' })}
                  rules={[{ required: true }]}
                >
                  <Input autoComplete="off" />
                </Form.Item>
                <Space>
                  <Button type="primary" htmlType="submit" loading={busy}>
                    {intl.formatMessage({ id: 'setup.createAdmin' })}
                  </Button>
                  {/* 跳过只在本次会话生效，下次访问会再提；强制鉴权时不提供（点了也是撞登录页）。 */}
                  {allowSkip && (
                    <Button type="link" onClick={finish} style={{ paddingInline: 0 }}>
                      {intl.formatMessage({ id: 'setup.skipAdmin' })}
                    </Button>
                  )}
                </Space>
              </Form>
            </>
          )}

          {step === 1 && (
            <>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                {intl.formatMessage({ id: 'setup.dataHint' })}
              </Paragraph>
              <Space wrap>
                <Button type="primary" loading={busy} onClick={loadSample}>
                  {intl.formatMessage({ id: 'setup.loadSample' })}
                </Button>
                <Button onClick={() => leaveTo('/datasets')}>
                  {intl.formatMessage({ id: 'setup.upload' })}
                </Button>
                <Button onClick={() => leaveTo('/datasources')}>
                  {intl.formatMessage({ id: 'setup.connectDb' })}
                </Button>
                {/* 向导现在会跨步骤常驻（见 lib/firstRun 的 inProgress），这一句是它的出口：
                    没有出口，走错路的人就只能刷新页面。 */}
                <Button type="link" onClick={finish} style={{ paddingInline: 0 }}>
                  {intl.formatMessage({ id: 'setup.skipToApp' })}
                </Button>
              </Space>
            </>
          )}

          {step === 2 && (
            <>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                {intl.formatMessage({
                  id: firstChart ? 'setup.chartReady' : 'setup.chartHint',
                })}
              </Paragraph>
              {firstChart && (
                <Paragraph type="secondary" style={{ marginBottom: 0, fontSize: 12 }}>
                  {intl.formatMessage(
                    { id: 'setup.chartReadyDetail' },
                    { dimension: firstChart.dimension, metric: firstChart.metric }
                  )}
                </Paragraph>
              )}
              <Space>
                <Button
                  type="primary"
                  onClick={firstChart ? goFirstChart : () => leaveTo('/chart-builder')}
                >
                  {intl.formatMessage({
                    id: firstChart ? 'setup.openSampleChart' : 'setup.toBuilder',
                  })}
                </Button>
                <Button onClick={finish}>{intl.formatMessage({ id: 'setup.skipToApp' })}</Button>
              </Space>
            </>
          )}
        </Space>
      </Card>
    </div>
  );
};

export default FirstRunGuide;
