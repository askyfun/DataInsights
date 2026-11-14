import * as Sentry from '@sentry/react';
import { Button, Result, Typography } from 'antd';
import React from 'react';
import { useIntl } from 'react-intl';

import { ApiClientError } from '../lib/api/client';
import { buildReportText, copyReportText } from '../lib/describeApiError';

interface Props {
  children: React.ReactNode;
}

interface State {
  error: Error | null;
  componentStack?: string;
}

/**
 * 根级错误边界：子树渲染期抛错时兜底为可重试的错误页，避免整站白屏。
 * 错误统一上报 Sentry（未初始化时 captureException 为 no-op）。
 * R-97（issue #222）：错误页展示后端错误编号（若错误携带）并提供「复制报障信息」，
 * 复制文本含编号 / 请求 id / 业务码 / 路由 / 版本 / 时间与截断后的 componentStack。
 */
class ErrorBoundaryImpl extends React.Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    this.setState({ componentStack: info.componentStack ?? undefined });
    Sentry.captureException(error, { extra: { componentStack: info.componentStack } });
  }

  private reset = () => {
    this.setState({ error: null, componentStack: undefined });
  };

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <ErrorFallback
        onRetry={this.reset}
        error={this.state.error}
        componentStack={this.state.componentStack}
      />
    );
  }
}

const ErrorFallback: React.FC<{
  onRetry: () => void;
  error: Error | null;
  componentStack?: string;
}> = ({ onRetry, error, componentStack }) => {
  const intl = useIntl();
  const isClientErr = error instanceof ApiClientError;
  const errorId = isClientErr ? error.errorId : undefined;
  const reportText = buildReportText({
    errorId,
    requestId: isClientErr ? error.requestId : undefined,
    code: isClientErr ? error.code : undefined,
    status: isClientErr ? error.status : undefined,
    route: `${window.location.pathname}${window.location.search}`,
    componentStack,
  });
  const copyReport = () => {
    void copyReportText(reportText);
  };
  return (
    <Result
      status="error"
      title={intl.formatMessage({ id: 'errorBoundary.title' })}
      subTitle={
        <>
          {intl.formatMessage({ id: 'errorBoundary.subtitle' })}
          {errorId && (
            <Typography.Paragraph copyable={{ text: reportText }} style={{ marginTop: 8 }}>
              {intl.formatMessage({ id: 'errorBoundary.errorId' })}：{errorId}
            </Typography.Paragraph>
          )}
        </>
      }
      extra={
        <>
          <Button type="primary" onClick={onRetry}>
            {intl.formatMessage({ id: 'errorBoundary.retry' })}
          </Button>
          <Button onClick={copyReport}>
            {intl.formatMessage({ id: 'errorBoundary.copyReport' })}
          </Button>
        </>
      }
    />
  );
};

/** 在 Provider（intl/Sentry 上下文可用）之内、路由之外使用 */
const ErrorBoundary: React.FC<Props> = ({ children }) => (
  <ErrorBoundaryImpl>{children}</ErrorBoundaryImpl>
);

export default ErrorBoundary;
