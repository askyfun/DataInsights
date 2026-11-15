import { BugOutlined, CopyOutlined, DownloadOutlined } from '@ant-design/icons';
import { App, Button, Card, Descriptions, Space, Tag, Typography } from 'antd';
import { useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import { diagnosticsApi, healthApi, systemApi, type VersionInfo } from '../api';
import { useSessionUser } from '../lib/authSession';
import { canSeeUsers } from '../lib/authz';

const { Title, Paragraph } = Typography;

// 关于 / 自检面板（R-104 / #229）：版本、健康状态、下载诊断包（仅管理员）、
// 最近一次前端错误编号 + 复制、报障入口。不收集任何本机信息。
const AboutPage: React.FC = () => {
  const intl = useIntl();
  const { message } = App.useApp();
  const user = useSessionUser();
  const [ver, setVer] = useState<VersionInfo | null>(null);
  const [ready, setReady] = useState<boolean | null>(null);
  const [lastError, setLastError] = useState<string>('');
  const frontendVersion = (import.meta.env.VITE_APP_VERSION as string | undefined) || 'dev';

  useEffect(() => {
    systemApi
      .getVersion()
      .then((r) => setVer(r.data.data))
      .catch(() => {});
    healthApi
      .ready()
      .then((r) => setReady(r.data.data?.status === 'ready'))
      .catch(() => setReady(false));
    try {
      setLastError(localStorage.getItem('last_error_id') || '');
    } catch {
      /* ignore */
    }
  }, []);

  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      message.success(intl.formatMessage({ id: 'about.copied' }));
    } catch {
      /* ignore */
    }
  };

  const download = async () => {
    try {
      const r = await diagnosticsApi.download();
      const url = URL.createObjectURL(r.data as Blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'diagnostics.tar.gz';
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      message.error(e instanceof Error ? e.message : 'download failed');
    }
  };

  const issueUrl = 'https://github.com/askyfun/TableWave/issues/new?template=bug_report.md';

  return (
    <div style={{ padding: 16, maxWidth: 900, margin: '0 auto' }}>
      <Card>
        <Title level={4}>{intl.formatMessage({ id: 'about.title' })}</Title>
        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label={intl.formatMessage({ id: 'about.frontendVersion' })}>
            {frontendVersion}
          </Descriptions.Item>
          <Descriptions.Item label={intl.formatMessage({ id: 'about.backendVersion' })}>
            {ver ? `${ver.version} (${ver.commit}, ${ver.build_time})` : '-'}
          </Descriptions.Item>
          <Descriptions.Item label={intl.formatMessage({ id: 'about.health' })}>
            {ready === null ? (
              '-'
            ) : ready ? (
              <Tag color="green">ready</Tag>
            ) : (
              <Tag color="red">not ready</Tag>
            )}
          </Descriptions.Item>
          <Descriptions.Item label={intl.formatMessage({ id: 'about.lastError' })}>
            {lastError ? (
              <Space>
                <code>{lastError}</code>
                <Button size="small" icon={<CopyOutlined />} onClick={() => copy(lastError)}>
                  {intl.formatMessage({ id: 'about.copy' })}
                </Button>
              </Space>
            ) : (
              intl.formatMessage({ id: 'about.none' })
            )}
          </Descriptions.Item>
        </Descriptions>
        <Space style={{ marginTop: 16 }} wrap>
          {canSeeUsers(user?.role) && (
            <Button type="primary" icon={<DownloadOutlined />} onClick={download}>
              {intl.formatMessage({ id: 'about.download' })}
            </Button>
          )}
          <Button icon={<BugOutlined />} href={issueUrl} target="_blank" rel="noreferrer">
            {intl.formatMessage({ id: 'about.reportBug' })}
          </Button>
        </Space>
        <Paragraph type="secondary" style={{ marginTop: 12, fontSize: 12 }}>
          {intl.formatMessage({ id: 'about.reportHint' })}
        </Paragraph>
      </Card>
    </div>
  );
};

export default AboutPage;
