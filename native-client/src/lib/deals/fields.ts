// The fields of a deal: checks that mirror the API's limits, and the request bodies.
// A form holds the value as the text a person types; the body carries minor units.
// The stage and the pipeline change only through a move, never through a PATCH.

import { parseMoney } from './money';
import { t } from '../../i18n';

export type DealDraft = {
  title?: string;
  value?: string;
  currency?: string;
  pipeline_id?: string;
  status?: string;
  expected_close_date?: string;
  company_id?: string | null;
  contact_id?: string | null;
  owner_id?: string | null;
};

export const DEAL_LIMITS = { title: 200, lost_reason: 500 };

const CURRENCY = /^[A-Za-z]{3}$/;
const DATE = /^(\d{4})-(\d{2})-(\d{2})$/;
const MIN_YEAR = 1900;
const MAX_YEAR = 2200;

const LINKS = ['company_id', 'contact_id', 'owner_id'] as const;

const text = (value: unknown) => (typeof value === 'string' ? value.trim() : '');

const tooLong = (value: string, max: number) => ([...value].length > max ? t('Use at most {n} characters.', { n: max }) : '');

/** A YYYY-MM-DD that names a real day. */
export function checkDate(value: string) {
  if (!value) return '';
  const match = DATE.exec(value);
  if (!match) return t('Enter a date such as 2027-03-14.');
  const [, y, m, d] = match.map(Number);
  const date = new Date(Date.UTC(y, m - 1, d));
  if (date.getUTCFullYear() !== y || date.getUTCMonth() !== m - 1 || date.getUTCDate() !== d) return t('Enter a date such as 2027-03-14.');
  if (y < MIN_YEAR || y > MAX_YEAR) return t('Use a date from 1900 to 2200.');
  return '';
}

const compact = (errors: Record<string, string>) => Object.fromEntries(Object.entries(errors).filter(([, message]) => message));

export function validateDeal(draft: DealDraft): Record<string, string> {
  const title = text(draft.title);
  const currency = text(draft.currency);
  const currencyOk = CURRENCY.test(currency);
  return compact({
    title: title ? tooLong(title, DEAL_LIMITS.title) : t('Enter a title.'),
    currency: currencyOk ? '' : t('Enter a three-letter currency code such as EUR.'),
    value: currencyOk ? (parseMoney(text(draft.value), currency).error ?? '') : '',
    pipeline_id: text(draft.pipeline_id) ? '' : t('Choose a pipeline.'),
    expected_close_date: checkDate(text(draft.expected_close_date))
  });
}

export function dealCreateBody(draft: DealDraft) {
  const currency = text(draft.currency).toUpperCase();
  const body: Record<string, string | number> = {
    title: text(draft.title),
    value_minor: parseMoney(text(draft.value), currency).minor ?? 0,
    currency,
    pipeline_id: text(draft.pipeline_id)
  };
  if (text(draft.expected_close_date)) body.expected_close_date = text(draft.expected_close_date);
  for (const link of LINKS) {
    const id = text(draft[link]);
    if (id) body[link] = id;
  }
  return body;
}

/** original is the deal as the API sent it. */
export function dealPatchBody(original: Record<string, unknown>, draft: DealDraft) {
  const body: Record<string, string | number | null> = {};
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
