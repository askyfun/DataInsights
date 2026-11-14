import { message } from 'antd';
import type { MessageInstance } from 'antd/es/message/interface';
import type { IntlShape } from 'react-intl';

import { ApiClientError, type ErrorKind } from './api/client';

// R-100（/version，PR #242）合入后改为运行时获取构建元数据；当前保持 dev 占位。
const APP_VERSION = 'dev';

/** componentStack 在复制文本里的截断上限：足够定位渲染错误，又不至于刷屏。 */
const COMPONENT_STACK_LIMIT = 800;

export interface DescribedApiError {
  kind: ErrorKind;
  /** 给用户看的本地化文案（不含编号） */
  text: string;
  /** 后端错误编号；断网 / 取消 / 无信封时为 undefined（没有编号就不显示，不给假线索） */
  errorId?: string;
  /** 多行复制文本：编号 / 请求 id / 业务码 / 路由 / 版本 / 时间（渲染错误另附 componentStack） */
  reportText: string;
}

export interface ReportInput {
  errorId?: string;
  requestId?: string;
  code?: number;
  status?: number;
  /** 复制时刻的页面路由；由调用方传 window.location，纯函数便于测试 */
  route: string;
  time?: Date;
  componentStack?: string;
}

/**
 * 拼多行报障文本（issue #222 验收 2）。字段标签固定英文：报障文本的读者是
 * 开发者 / 维护者，且编号本身即 DI-… 英文形态，避免两套文案再对不齐。
 */
export function buildReportText(input: ReportInput): string {
  const lines: string[] = [];
  if (input.errorId) lines.push(`Error ID: ${input.errorId}`);
  if (input.requestId) lines.push(`Request ID: ${input.requestId}`);
  if (input.code !== undefined) lines.push(`Biz code: ${input.code}`);
  if (input.status !== undefined) lines.push(`HTTP status: ${input.status}`);
  lines.push(`Route: ${input.route}`);
  lines.push(`Version: ${APP_VERSION}`);
  lines.push(`Time: ${(input.time ?? new Date()).toISOString()}`);
  if (input.componentStack) {
    const stack =
      input.componentStack.length > COMPONENT_STACK_LIMIT
        ? `${input.componentStack.slice(0, COMPONENT_STACK_LIMIT)}…(truncated)`
        : input.componentStack;
    lines.push('--- component stack ---', stack);
  }
  return lines.join('\n');
}

const KIND_MESSAGE_KEY: Record<ErrorKind, string> = {
  offline: 'error.kind.offline',
  timeout: 'error.kind.timeout',
  network: 'error.kind.network',
  server: 'error.kind.server',
  business: 'error.kind.business',
  canceled: 'error.kind.canceled',
};

/**
 * 失败文案的唯一出口（issue #222 验收 5）：任意失败 → 分型 + 本地化文案 + 报障文本。
 * 非 ApiClientError 的未知错误按 server 档处理（无编号）。canceled 也会拿到描述，
 * 但展示侧（notifyApiError）对它静默。
 *
 * business 档例外：后端业务码的 msg 本来就是写给用户看的中文（如「文件夹下还有 N 个
 * 仪表盘，请先移动或删除它们」，有测试钉死），优先原样展示；没有 msg 才落回通用文案。
 * server 档则刻意**不用**后端原文 —— 那是 `dial tcp …` 一类的内部细节，只给编号。
 */
export function describeApiError(err: unknown, intl: IntlShape): DescribedApiError {
  const isClientErr = err instanceof ApiClientError;
  const kind: ErrorKind = isClientErr ? err.kind : 'server';
  const text =
    kind === 'business' && isClientErr && err.message
      ? err.message
      : intl.formatMessage({ id: KIND_MESSAGE_KEY[kind] });
  const errorId = isClientErr ? err.errorId : undefined;
  const reportText = buildReportText({
    errorId,
    requestId: isClientErr ? err.requestId : undefined,
    code: isClientErr ? err.code : undefined,
    status: isClientErr ? err.status : undefined,
    route: `${window.location.pathname}${window.location.search}`,
  });
  return { kind, text, errorId, reportText };
}

/** 写剪贴板：优先 async Clipboard API，非安全上下文（http + 非 localhost）退回 execCommand。 */
export async function copyReportText(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // 落入 execCommand 兜底
    }
  }
  try {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(textarea);
    return ok;
  } catch {
    return false;
  }
}

/**
 * 高频失败入口的统一 toast（issue #222）：canceled 静默；offline 只给文案不给编号
 * （没有请求就没有 id，不能给假线索）；server/business 带编号，点按 toast 复制完整报障文本。
 *
 * messageApi 传入页面从 `App.useApp()` 拿到的实例（推荐）：antd 静态 message 在
 * App 上下文之外拿不到主题/容器，部分环境（测试的 jsdom）甚至不渲染进 DOM；
 * 未传时回退静态出口，保证 lib 单测与非页面调用方可用。
 */
export function notifyApiError(
  err: unknown,
  intl: IntlShape,
  messageApi?: Pick<MessageInstance, 'open' | 'success' | 'warning'>
): void {
  const described = describeApiError(err, intl);
  if (described.kind === 'canceled') return;
  const content = described.errorId
    ? intl.formatMessage(
        { id: 'error.toast.withId' },
        { text: described.text, id: described.errorId }
      )
    : described.text;
  const api = messageApi ?? message;
  api.open({
    type: 'error',
    content,
    duration: 6,
    onClick: () => {
      void copyReportText(described.reportText).then((ok) => {
        if (ok) api.success(intl.formatMessage({ id: 'error.report.copied' }));
        else api.warning(intl.formatMessage({ id: 'error.report.copyFailed' }));
      });
    },
  });
}
