// Deal values travel as whole minor units with a currency code, the way the API
// stores them. These turn them into text for the screen and back from a form.

/** The API's upper bound on value_minor. */
export const MAX_MINOR = 999_999_999_999_999;

const AMOUNT_SHAPE = /^\d+(\.\d+)?$/;

/** @param {string} currency @param {string} [locale] @param {Intl.NumberFormatOptions} [options] */
function currencyFormat(currency, locale, options = {}) {
  try {
    return new Intl.NumberFormat(locale, { style: 'currency', currency, ...options });
  } catch {
    return null;
  }
}

/** How many decimals a currency has: 2 for most, 0 for yen, 3 for some dinars. @param {string} currency */
export function minorUnitDigits(currency) {
  const format = currencyFormat(currency);
  return format?.resolvedOptions().maximumFractionDigits ?? 2;
}

/**
 * Minor units as the currency reads in the locale, such as €1,234.56; compact rounds
 * to whole units for a column total. A code with no symbol reads as the code and a
 * number, and so does a malformed one, which Intl refuses.
 * @param {number} minor
 * @param {string} currency
 * @param {string} [locale]
 * @param {{ compact?: boolean }} [options]
 */
export function formatMoney(minor, currency, locale, { compact = false } = {}) {
  const digits = minorUnitDigits(currency);
  const amount = minor / 10 ** digits;
  const fraction = compact ? { minimumFractionDigits: 0, maximumFractionDigits: 0 } : {};
  const format = currencyFormat(currency, locale, fraction);
  if (format) return format.format(amount);
  const plain = new Intl.NumberFormat(locale, { minimumFractionDigits: compact ? 0 : digits, maximumFractionDigits: compact ? 0 : digits });
  return `${currency} ${plain.format(amount)}`.trim();
}

/**
 * The amount a person typed, as minor units. Commas are grouping and a dot the
 * decimal point, as in 1,250.50; blank means no value yet.
 * @param {string} text
 * @param {string} currency
 * @returns {{ minor: number, error?: undefined } | { error: string, minor?: undefined }}
 */
export function parseMoney(text, currency) {
  const cleaned = text.replaceAll(',', '').replaceAll(/\s/g, '');
  if (!cleaned) return { minor: 0 };
  if (!AMOUNT_SHAPE.test(cleaned)) return { error: 'Enter an amount such as 1250 or 1250.50.' };
  const digits = minorUnitDigits(currency);
  const [whole, fraction = ''] = cleaned.split('.');
  if (fraction.length > digits) {
    const code = currency.toUpperCase();
    return { error: digits === 0 ? `${code} amounts have no decimals.` : `${code} amounts have at most ${digits} decimals.` };
  }
  // Digit by digit, so a large amount never rounds through a double.
  const minor = Number(whole + fraction.padEnd(digits, '0'));
  if (minor > MAX_MINOR) return { error: 'This amount is larger than the API accepts.' };
  return { minor };
}

/** Minor units as the decimal a person would type, such as 1234.56; blank for zero. @param {number} minor @param {string} currency */
export function majorText(minor, currency) {
  if (!minor) return '';
  const digits = minorUnitDigits(currency);
  const text = String(minor).padStart(digits + 1, '0');
  if (digits === 0) return text;
  return `${text.slice(0, -digits)}.${text.slice(-digits)}`;
}
