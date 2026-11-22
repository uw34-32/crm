// Deal values travel as whole minor units with a currency code, the way the API
// stores them. These turn them into text for the screen and back from a form.

import { decimalComma, t } from '../../i18n';

/** The API's upper bound on value_minor. */
export const MAX_MINOR = 999_999_999_999_999;

const AMOUNT_SHAPE = /^\d+(\.\d+)?$/;

function currencyFormat(currency: string, locale?: string, options: Intl.NumberFormatOptions = {}) {
  try {
    return new Intl.NumberFormat(locale, { style: 'currency', currency, ...options });
  } catch {
    return null;
  }
}

/** How many decimals a currency has: 2 for most, 0 for yen, 3 for some dinars. */
export function minorUnitDigits(currency: string) {
  const format = currencyFormat(currency);
  return format?.resolvedOptions().maximumFractionDigits ?? 2;
}

/**
 * Minor units as the currency reads in the locale, such as €1,234.56; compact rounds
 * to whole units for a column total. A code with no symbol reads as the code and a
 * number, and so does a malformed one, which Intl refuses.
 */
export function formatMoney(minor: number, currency: string, locale?: string, { compact = false }: { compact?: boolean } = {}) {
  const digits = minorUnitDigits(currency);
  const amount = minor / 10 ** digits;
  const fraction = compact ? { minimumFractionDigits: 0, maximumFractionDigits: 0 } : {};
  const format = currencyFormat(currency, locale, fraction);
  if (format) return format.format(amount);
  const plain = new Intl.NumberFormat(locale, { minimumFractionDigits: compact ? 0 : digits, maximumFractionDigits: compact ? 0 : digits });
  return `${currency} ${plain.format(amount)}`.trim();
}

/** The amount text with only digits and one dot for the decimals, or as typed when it has no such shape. */
function normalizeAmount(text: string): string {
  const spaced = text.replaceAll(/\s/g, '');
  if (!decimalComma()) return spaced.replaceAll(',', '');
  if (spaced.includes(',')) return spaced.replaceAll('.', '').replace(',', '.');
  return /^\d{1,3}(\.\d{3})+$/.test(spaced) ? spaced.replaceAll('.', '') : spaced;
}

/**
 * The amount a person typed, as minor units; blank means no value yet. In English commas
 * are grouping and a dot the decimal point, as in 1,250.50. In Spanish a comma is the
 * decimal point and dots group, as in 1.250,50, and a dot that is not followed by exactly
 * three digits is still read as the decimal point, so 1250.50 means what it says.
 */
export function parseMoney(text: string, currency: string): { minor: number; error?: undefined } | { error: string; minor?: undefined } {
  const cleaned = normalizeAmount(text);
  if (!cleaned) return { minor: 0 };
  if (!AMOUNT_SHAPE.test(cleaned)) return { error: t('Enter an amount such as 1250 or 1250.50.') };
  const digits = minorUnitDigits(currency);
  const [whole, fraction = ''] = cleaned.split('.');
  if (fraction.length > digits) {
    const code = currency.toUpperCase();
    return { error: digits === 0 ? t('{code} amounts have no decimals.', { code }) : t('{code} amounts have at most {n} decimals.', { code, n: digits }) };
  }
  // Digit by digit, so a large amount never rounds through a double.
  const minor = Number(whole + fraction.padEnd(digits, '0'));
  if (minor > MAX_MINOR) return { error: t('This amount is larger than the API accepts.') };
  return { minor };
}

/** Minor units as the decimal a person would type, such as 1234.56; blank for zero. */
export function majorText(minor: number, currency: string) {
  if (!minor) return '';
  const digits = minorUnitDigits(currency);
  const text = String(minor).padStart(digits + 1, '0');
  if (digits === 0) return text;
  return `${text.slice(0, -digits)}${decimalComma() ? ',' : '.'}${text.slice(-digits)}`;
}
