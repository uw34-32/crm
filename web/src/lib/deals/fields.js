// The fields of a deal: checks that mirror the API's limits, and the request bodies.
// A form holds the value as the text a person types; the body carries minor units.
// The stage and the pipeline change only through a move, never through a PATCH.

import { parseMoney } from './money.js';

/**
 * @typedef {{
 *   title?: string,
 *   value?: string,
 *   currency?: string,
 *   pipeline_id?: string,
 *   stage_id?: string,
 *   expected_close_date?: string,
 *   company_id?: string | null,
 *   contact_id?: string | null,
 *   owner_id?: string | null
 * }} DealDraft
 */

export const DEAL_LIMITS = { title: 200, lost_reason: 500 };

const CURRENCY = /^[A-Za-z]{3}$/;
const DATE = /^(\d{4})-(\d{2})-(\d{2})$/;
const MIN_YEAR = 1900;
const MAX_YEAR = 2200;

const LINKS = /** @type {const} */ (['company_id', 'contact_id', 'owner_id']);

/** @param {unknown} value */
const text = (value) => (typeof value === 'string' ? value.trim() : '');

/** @param {string} value @param {number} max */
const tooLong = (value, max) => ([...value].length > max ? `Use at most ${max} characters.` : '');

/** A YYYY-MM-DD that names a real day. @param {string} value */
export function checkDate(value) {
  if (!value) return '';
  const match = DATE.exec(value);
  if (!match) return 'Enter a date such as 2027-03-14.';
  const [, y, m, d] = match.map(Number);
  const date = new Date(Date.UTC(y, m - 1, d));
  if (date.getUTCFullYear() !== y || date.getUTCMonth() !== m - 1 || date.getUTCDate() !== d) return 'Enter a date such as 2027-03-14.';
  if (y < MIN_YEAR || y > MAX_YEAR) return 'Use a date from 1900 to 2200.';
  return '';
}

/** @param {Record<string, string>} errors */
const compact = (errors) => Object.fromEntries(Object.entries(errors).filter(([, message]) => message));

/** @param {DealDraft} draft @returns {Record<string, string>} */
export function validateDeal(draft) {
  const title = text(draft.title);
  const currency = text(draft.currency);
  const currencyOk = CURRENCY.test(currency);
  return compact({
    title: title ? tooLong(title, DEAL_LIMITS.title) : 'Enter a title.',
    currency: currencyOk ? '' : 'Enter a three-letter currency code such as EUR.',
    value: currencyOk ? (parseMoney(text(draft.value), currency).error ?? '') : '',
    pipeline_id: text(draft.pipeline_id) ? '' : 'Choose a pipeline.',
    expected_close_date: checkDate(text(draft.expected_close_date))
  });
}

/** @param {DealDraft} draft */
export function dealCreateBody(draft) {
  const currency = text(draft.currency).toUpperCase();
  /** @type {Record<string, string | number>} */
  const body = {
    title: text(draft.title),
    value_minor: parseMoney(text(draft.value), currency).minor ?? 0,
    currency,
    pipeline_id: text(draft.pipeline_id)
  };
  if (text(draft.stage_id)) body.stage_id = text(draft.stage_id);
  if (text(draft.expected_close_date)) body.expected_close_date = text(draft.expected_close_date);
  for (const link of LINKS) {
    const id = text(draft[link]);
    if (id) body[link] = id;
  }
  return body;
}

/**
 * @param {DealDraft & Record<string, unknown>} original the deal as the API sent it
 * @param {DealDraft} draft
 * @returns {Record<string, string | number | null>}
 */
export function dealPatchBody(original, draft) {
  /** @type {Record<string, string | number | null>} */
  const body = {};
  const title = text(draft.title);
  if (title !== text(original.title)) body.title = title;
  const currency = text(draft.currency).toUpperCase();
  if (currency !== text(original.currency)) body.currency = currency;
  const minor = parseMoney(text(draft.value), currency).minor;
  if (minor !== undefined && minor !== original.value_minor) body.value_minor = minor;
  const date = text(draft.expected_close_date) || null;
  if (date !== (original.expected_close_date ?? null)) body.expected_close_date = date;
  for (const link of LINKS) {
    const after = text(draft[link]) || null;
    if (after !== (original[link] ?? null)) body[link] = after;
  }
  return body;
}
