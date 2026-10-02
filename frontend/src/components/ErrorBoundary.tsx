import * as Sentry from '@sentry/react';
import { Button, Result } from 'antd';
import React from 'react';
import { useIntl } from 'react-intl';

interface Props {
  children: React.ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * 根级错误边界：子树渲染期抛错时兜底为可重试的错误页，避免整站白屏。
 * 错误统一上报 Sentry（未初始化时 captureException 为 no-op）。
 */
class ErrorBoundaryImpl extends React.Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    Sentry.captureException(error, { extra: { componentStack: info.componentStack } });
  }

  private reset = () => {
    this.setState({ error: null });
  };

  render() {
    if (!this.state.error) return this.props.children;
    return <ErrorFallback onRetry={this.reset} />;
  }
}

const ErrorFallback: React.FC<{ onRetry: () => void }> = ({ onRetry }) => {
  const intl = useIntl();
  return (
    <Result
      status="error"
      title={intl.formatMessage({ id: 'errorBoundary.title' })}
      subTitle={intl.formatMessage({ id: 'errorBoundary.subtitle' })}
      extra={
        <Button type="primary" onClick={onRetry}>
          {intl.formatMessage({ id: 'errorBoundary.retry' })}
        </Button>
      }
    />
  );
};

/** 在 Provider（intl/Sentry 上下文可用）之内、路由之外使用 */
const ErrorBoundary: React.FC<Props> = ({ children }) => (
  <ErrorBoundaryImpl>{children}</ErrorBoundaryImpl>
);

export default ErrorBoundary;
