import * as Sentry from '@sentry/react';
import { App as AntdApp, ConfigProvider, theme } from 'antd';
import enUS from 'antd/locale/en_US';
import zhCN from 'antd/locale/zh_CN';
import React from 'react';
import ReactDOM from 'react-dom/client';
import { createIntl, IntlProvider } from 'react-intl';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import { cache, messages, useLocale } from './i18n/useLocale';
import './styles/index.css';

// Sentry DSN 走构建期环境变量（VITE_SENTRY_DSN），与后端 SENTRY_DSN 同一套口径：
// DSN 不该硬编码在源码里，谁拿到仓库谁就能往你的配额里灌事件。
// 未设置则完全不初始化 —— 本地开发与未配置监控的部署不产生任何 Sentry 副作用
// （client.ts 里的 captureMessage 在未初始化时是 no-op，不需要额外保护）。
const sentryDsn = import.meta.env.VITE_SENTRY_DSN;

if (sentryDsn) {
  Sentry.init({
    dsn: sentryDsn,
    integrations: [Sentry.browserTracingIntegration(), Sentry.replayIntegration()],
    tracesSampleRate: 1.0,
    replaysSessionSampleRate: 0.1,
    replaysOnErrorSampleRate: 1.0,
    enableLogs: true,
    tracePropagationTargets: ['localhost', /^\/api\//],
  });
}

const LocaleProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { locale } = useLocale();
  const intl = createIntl(
    {
      locale,
      messages: messages[locale],
    },
    cache
  );

  const antdLocale = locale === 'zh-CN' ? zhCN : enUS;

  return (
    <IntlProvider {...intl}>
      <ConfigProvider
        locale={antdLocale}
        theme={{
          algorithm: theme.defaultAlgorithm,
          token: {
            colorPrimary: '#1677ff',
          },
          components: {
            Table: {
              cellPaddingBlockSM: 4,
              cellPaddingInlineSM: 8,
              cellPaddingBlockMD: 6,
              cellPaddingInlineMD: 10,
              headerBg: '#fafafa',
              headerColor: '#333',
              borderColor: '#e8e8e8',
              rowHoverBg: '#f0f7ff',
            },
            // 全局卡片内边距收紧（默认 bodyPaddingSM/headerPaddingSM 均为 12，非 small 卡片
            // bodyPadding/headerPadding 为 24）。屏幕利用率优先：8/16 仍留有节奏，但一张卡片
            // 四周少掉 8~16px。需要更紧的页面用 styles={{ body: {...} }} 逐处覆盖。
            Card: {
              bodyPaddingSM: 8,
              headerPaddingSM: 8,
              bodyPadding: 16,
              headerPadding: 16,
              // 卡头是 min-height（非固定高），改小不会裁掉 extra 里的按钮；
              // 默认 = fontSize*lineHeight + paddingXS*2 ≈ 38，对一行标题明显偏高。
              headerHeightSM: 32,
            },
            // 输入类控件横向内边距收紧。默认 11px 来自 paddingSM(12) - lineWidth(1)，
            // 对 12~13px 的小字号输入框而言过宽（文字离边框太远，配置面板尤其松散）。
            //   Input / InputNumber：有公开的 paddingInline token，直接改。
            //   Select：没有公开 token —— inputPaddingHorizontalBase 不在 ComponentToken 里
            //     （实测覆盖被忽略），它硬编码自组件的 paddingSM。但组件 token 同时接受
            //     AliasToken，所以在 Select 作用域内就地重定义 paddingSM/paddingXS 即可：
            //     antd 会把它输出成 `--ant-padding-sm` 挂在 .ant-select-css-var 上，只影响
            //     Select 自身（不会污染表格/表单等 20 多个消费者的全局 paddingSM）。
            // 目标：默认尺寸三者文字左侧内缩统一 6px，小尺寸统一 5px。
            Input: {
              paddingInline: 6,
              paddingInlineSM: 5,
              paddingInlineLG: 8,
            },
            InputNumber: {
              paddingInline: 6,
              paddingInlineSM: 5,
              paddingInlineLG: 8,
            },
            Select: {
              paddingSM: 7, // - lineWidth(1) = 6px
              paddingXS: 6, // - lineWidth(1) = 5px（size="small" 时）
            },
          },
        }}
      >
        {/* antd 的 <App> 提供 message/notification/modal 的上下文实例，让它们能读到上面这份
            ConfigProvider 主题。静态 message.* 读不到上下文，antd 6 会打
            "Static function can not consume context like dynamic theme"。
            component={false} → 渲染 Fragment，不新增 DOM 层（避免影响全高布局）。 */}
        <AntdApp component={false}>
          <BrowserRouter>{children}</BrowserRouter>
        </AntdApp>
      </ConfigProvider>
    </IntlProvider>
  );
};

const rootEl = document.getElementById('root');
if (!rootEl) throw new Error('Root element not found');
ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <LocaleProvider>
      <App />
    </LocaleProvider>
  </React.StrictMode>
);
