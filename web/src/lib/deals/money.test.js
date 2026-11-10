import { describe, expect, it } from 'vitest';
import { formatMoney, majorText, minorUnitDigits, parseMoney } from './money.js';

describe('minor unit digits', () => {
  it('follows the currency: two for most, none for yen, three for dinars', () => {
    expect(minorUnitDigits('EUR')).toBe(2);
    expect(minorUnitDigits('JPY')).toBe(0);
    expect(minorUnitDigits('KWD')).toBe(3);
  });

  it('assumes two for a code Intl does not know, which the API accepts on format alone', () => {
    expect(minorUnitDigits('ZZZ')).toBe(2);
    expect(minorUnitDigits('')).toBe(2);
  });
});

describe('formatting money', () => {
  it('shows minor units as the currency reads in the locale', () => {
    expect(formatMoney(123456, 'EUR', 'en-US')).toBe('€1,234.56');
    expect(formatMoney(5000, 'JPY', 'en-US')).toBe('¥5,000');
    expect(formatMoney(0, 'USD', 'en-US')).toBe('$0.00');
  });

  it('shows a code Intl has no symbol for as the code, and a malformed one the same way', () => {
    expect(formatMoney(123456, 'ZZZ', 'en-US').replace(' ', ' ')).toBe('ZZZ 1,234.56');
    expect(formatMoney(123456, 'E1', 'en-US')).toBe('E1 1,234.56');
  });

  it('can round to whole units for a compact column total', () => {
    expect(formatMoney(123456, 'EUR', 'en-US', { compact: true })).toBe('€1,235');
    expect(formatMoney(5000, 'JPY', 'en-US', { compact: true })).toBe('¥5,000');
  });
});

describe('parsing an amount typed in a form', () => {
  it('turns a decimal amount into minor units, with commas as grouping', () => {
    expect(parseMoney('1,234.56', 'EUR')).toEqual({ minor: 123456 });
    expect(parseMoney(' 1234 ', 'EUR')).toEqual({ minor: 123400 });
    expect(parseMoney('0.5', 'EUR')).toEqual({ minor: 50 });
    expect(parseMoney('5000', 'JPY')).toEqual({ minor: 5000 });
    expect(parseMoney('1.250', 'KWD')).toEqual({ minor: 1250 });
  });

  it('treats a blank amount as zero, since a deal may have no value yet', () => {
    expect(parseMoney('', 'EUR')).toEqual({ minor: 0 });
  });

  it('refuses more decimals than the currency has, letters, and negatives', () => {
    expect(parseMoney('1.234', 'EUR')).toEqual({ error: 'EUR amounts have at most 2 decimals.' });
    expect(parseMoney('12.5', 'JPY')).toEqual({ error: 'JPY amounts have no decimals.' });
    expect(parseMoney('12 euros', 'EUR')).toEqual({ error: 'Enter an amount such as 1250 or 1250.50.' });
    expect(parseMoney('-5', 'EUR')).toEqual({ error: 'Enter an amount such as 1250 or 1250.50.' });
  });

  it('stops at the API bound of 999999999999999 minor units', () => {
    expect(parseMoney('9999999999999.99', 'EUR')).toEqual({ minor: 999999999999999 });
    expect(parseMoney('10000000000000', 'EUR')).toEqual({ error: 'This amount is larger than the API accepts.' });
  });
});

describe('the amount a form starts from', () => {
  it('writes minor units back as the decimal the person would type', () => {
    expect(majorText(123456, 'EUR')).toBe('1234.56');
    expect(majorText(50, 'EUR')).toBe('0.50');
    expect(majorText(5000, 'JPY')).toBe('5000');
    expect(majorText(0, 'EUR')).toBe('');
  });
});
