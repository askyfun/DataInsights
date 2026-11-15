import { RocketOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Form, Input, Space, Steps, Typography } from 'antd';
import { useState } from 'react';
import { useIntl } from 'react-intl';
import { useNavigate } from 'react-router-dom';
import { authApi, sampleDatasetApi } from '../../api';
import { setBearerToken } from '../../lib/api/client';
import { setSessionUser } from '../../lib/authSession';
import { markSetupSkipped } from '../../lib/firstRun';

const { Title, Paragraph } = Typography;

// 首启向导（R-101 / #226）：三步步进复用既有能力，不新造流程。
// 完成（出第一张图 / 载示例）后写会话级标记（见 lib/firstRun），本次访问不再顶替列表；
// 库里有了管理员之后 `needs_bootstrap` 自然为假，向导就不会再出现。
//
// allowSkip=false 用于强制鉴权（enforce=on）的部署：那时匿名"先逛逛"只会被守卫弹到登录页，
// 给一个点了就撞墙的按钮比不给更糟。
const FirstRunGuide: React.FC<{ onDone: () => void; allowSkip?: boolean }> = ({
  onDone,
  allowSkip = true,
}) => {
  const intl = useIntl();
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

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
      setStep(2);
    } catch (e) {
      setErr(e instanceof Error ? e.message : intl.formatMessage({ id: 'setup.loadFailed' }));
    } finally {
      setBusy(false);
    }
  };

  const goBuilder = () => {
    finish();
    navigate('/chart-builder');
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
                <Button onClick={() => navigate('/datasets')}>
                  {intl.formatMessage({ id: 'setup.upload' })}
                </Button>
                <Button onClick={() => navigate('/datasources')}>
                  {intl.formatMessage({ id: 'setup.connectDb' })}
                </Button>
              </Space>
            </>
          )}

          {step === 2 && (
            <>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                {intl.formatMessage({ id: 'setup.chartHint' })}
              </Paragraph>
              <Space>
                <Button type="primary" onClick={goBuilder}>
                  {intl.formatMessage({ id: 'setup.toBuilder' })}
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
