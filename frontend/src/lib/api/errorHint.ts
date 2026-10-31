import { ApiClientError } from './client';

// R-99（issue #224）：稳定错误码的渲染侧。后端错误信封带 hint_key（i18n
// 词典键）时优先翻译；词典没有该 key（未知/前端未跟上）回落到 msg——
// 绝不把 key 原文（errors.datasource.unreachable）显示给用户。

/**
 * 把任意错误转成给用户看的文案。
 *
 * 调用场景：页面/store 的 catch 里展示 API 错误时统一走这里，替代
 * `err instanceof Error ? err.message : ...` 的散写。messages 传当前
 * locale 的词典（useLocale 的 messages 导出）。
 *
 * 逻辑：ApiClientError 且 hint_key 能在词典命中 → 译文；否则回落原始
 * message（业务错误 msg 本身是人类可读的确定性文案）。
 */
export function resolveApiErrorMessage(err: unknown, messages: Record<string, string>): string {
  if (err instanceof ApiClientError && err.hintKey) {
    const translated = messages[err.hintKey];
    if (translated) {
      return translated;
    }
  }
  return err instanceof Error ? err.message : String(err);
}
