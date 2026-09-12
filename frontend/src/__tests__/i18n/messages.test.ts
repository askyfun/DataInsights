// Locale catalog parity: zh-CN and en-US message maps must expose identical
// key sets, so a page rendered under either locale never falls back to raw
// message ids (e.g. dataset.edit.title rendering as the key itself).
import { describe, expect, it } from 'vitest';
import { messages } from '../../i18n/useLocale';

describe('i18n message catalogs', () => {
  const locales = Object.keys(messages) as Array<keyof typeof messages>;

  it('every locale exposes the same set of message keys', () => {
    const [base, ...others] = locales;
    const baseKeys = Object.keys(messages[base]).sort();
    for (const locale of others) {
      expect(Object.keys(messages[locale]).sort(), `keys of ${locale}`).toEqual(baseKeys);
    }
  });

  it('interpolation placeholders match between zh-CN and en-US', () => {
    const placeholders = (text: string) => [...text.matchAll(/\{[^}]+\}/g)].map((m) => m[0]).sort();
    for (const id of Object.keys(messages['zh-CN'])) {
      expect(placeholders(messages['en-US'][id]), id).toEqual(placeholders(messages['zh-CN'][id]));
    }
  });
});
