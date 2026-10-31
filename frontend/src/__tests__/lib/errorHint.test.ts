import { describe, expect, it } from 'vitest';
import { messages } from '../../i18n/useLocale';
import errorCodesFixture from '../../lib/__fixtures__/errorCodes.json';
import { ApiClientError } from '../../lib/api/client';
import { resolveApiErrorMessage } from '../../lib/api/errorHint';

// R-99（issue #224）：稳定错误码渲染侧的一致性与降级行为。
// fixture 与后端 response 包共读（errorCodes.json），任一端漂移即红。

type ErrCodeCase = { bizCode: number; stable: string; hintKey: string };

const cases = (errorCodesFixture as { codes: ErrCodeCase[] }).codes;

describe('errorCodes fixture ↔ i18n 词典', () => {
  it.each(cases.map((c) => [c.stable, c.hintKey] as const))(
    '%s 的 hint_key 在 zh-CN 与 en-US 词典中都有词条',
    (_stable, hintKey) => {
      expect(messages['zh-CN'][hintKey], `zh-CN 缺 ${hintKey}`).toBeTruthy();
      expect(messages['en-US'][hintKey], `en-US 缺 ${hintKey}`).toBeTruthy();
    }
  );
});

describe('resolveApiErrorMessage', () => {
  it.each(cases.map((c) => [c.stable, c.hintKey] as const))(
    '已识别 hint_key（%s）优先渲染词典文案而非 msg',
    (stable, hintKey) => {
      for (const locale of ['zh-CN', 'en-US'] as const) {
        const err = new ApiClientError('raw fallback message', {
          stableCode: stable,
          hintKey,
        });
        expect(resolveApiErrorMessage(err, messages[locale])).toBe(messages[locale][hintKey]);
        expect(resolveApiErrorMessage(err, messages[locale])).not.toBe('raw fallback message');
      }
    }
  );

  it('未识别 hint_key 回落到 msg，绝不显示 key 原文', () => {
    const err = new ApiClientError('raw fallback message', {
      stableCode: 'DATASOURCE_UNREACHABLE',
      hintKey: 'errors.brand.new.key',
    });
    expect(resolveApiErrorMessage(err, messages['zh-CN'])).toBe('raw fallback message');
    expect(resolveApiErrorMessage(err, messages['en-US'])).toBe('raw fallback message');
    expect(resolveApiErrorMessage(err, messages['zh-CN'])).not.toContain('errors.');
  });

  it('无 hint_key（未分类错误 / 普通错误）回落 message', () => {
    const err = new ApiClientError('plain message', { code: 20300 });
    expect(resolveApiErrorMessage(err, messages['zh-CN'])).toBe('plain message');
    const notApi = new Error('native');
    expect(resolveApiErrorMessage(notApi, messages['zh-CN'])).toBe('native');
  });
});
