// The fields of contacts and companies: checks that mirror the API's limits, and the
// request bodies. A create leaves out blank optional fields. A PATCH names only what
// changed, clears an optional field with an empty string (the API refuses null on
// text), and unlinks a contact's company with company_id: null.

import { EMAIL_SHAPE } from '../forms';

export type ContactDraft = {
  first_name?: string;
  last_name?: string;
  email?: string;
  phone?: string;
  title?: string;
  company_id?: string | null;
};
export type CompanyDraft = { name?: string; domain?: string; phone?: string; industry?: string; address?: string };

export const CONTACT_TEXT = ['first_name', 'last_name', 'email', 'phone', 'title'] as const;
export const COMPANY_TEXT = ['name', 'domain', 'phone', 'industry', 'address'] as const;

export const LIMITS = {
  first_name: 100,
  last_name: 100,
  title: 200,
  company_name: 200,
  industry: 100,
  address: 500,
  phone: 50
};

const MAX_DOMAIN_BYTES = 253;
const MAX_LABEL_BYTES = 63;
const DIGIT = /\p{Nd}/u;
const LABEL = /^[\p{L}\p{Nd}-]+$/u;
const bytes = (value: string) => new TextEncoder().encode(value).length;

const text = (value: unknown) => (typeof value === 'string' ? value.trim() : '');
const chars = (value: string) => [...value].length;

function tooLong(value: string, max: number) {
  return chars(value) > max ? `Use at most ${max} characters.` : '';
}

function checkOptionalEmail(value: string) {
  return value && !EMAIL_SHAPE.test(value) ? 'Enter an email address such as ada@example.com.' : '';
}

function checkOptionalPhone(value: string) {
  if (!value) return '';
  if (!DIGIT.test(value)) return 'A phone number needs at least one digit.';
  return tooLong(value, LIMITS.phone);
}

/** A host name such as acme.example, the way the API checks it. */
export function isDomain(domain: string) {
  if (bytes(domain) > MAX_DOMAIN_BYTES) return false;
  const labels = domain.split('.');
  return (
    labels.length >= 2 &&
    labels.every((label: string) => label && bytes(label) <= MAX_LABEL_BYTES && !label.startsWith('-') && !label.endsWith('-') && LABEL.test(label))
  );
}

const compact = (errors: Record<string, string>) => Object.fromEntries(Object.entries(errors).filter(([, message]) => message));

export function validateContact(draft: ContactDraft): Record<string, string> {
  const first = text(draft.first_name);
  return compact({
    first_name: first ? tooLong(first, LIMITS.first_name) : 'Enter a first name.',
    last_name: tooLong(text(draft.last_name), LIMITS.last_name),
    email: checkOptionalEmail(text(draft.email)),
    phone: checkOptionalPhone(text(draft.phone)),
    title: tooLong(text(draft.title), LIMITS.title)
  });
}

export function validateCompany(draft: CompanyDraft): Record<string, string> {
  const name = text(draft.name);
  const domain = text(draft.domain).toLowerCase();
  return compact({
    name: name ? tooLong(name, LIMITS.company_name) : 'Enter a company name.',
    domain: domain && !isDomain(domain) ? 'Enter a domain such as acme.example, without https:// or a path.' : '',
    phone: checkOptionalPhone(text(draft.phone)),
    industry: tooLong(text(draft.industry), LIMITS.industry),
    address: tooLong(text(draft.address), LIMITS.address)
  });
}

/** How the API will store a field, so a comparison sees what would really change. */
const stored = (field: string, value: unknown) => (field === 'domain' ? text(value).toLowerCase() : text(value));

function createText(draft: Record<string, unknown>, fields: readonly string[]) {
  const body: Record<string, string> = {};
  for (const field of fields) {
    const value = stored(field, draft[field]);
    if (value) body[field] = value;
  }
  return body;
}

function patchText(original: Record<string, unknown>, draft: Record<string, unknown>, fields: readonly string[]) {
  const body: Record<string, string> = {};
  for (const field of fields) {
    const value = stored(field, draft[field]);
    if (value !== stored(field, original[field])) body[field] = value;
  }
  return body;
}

export function contactCreateBody(draft: ContactDraft) {
  const body = createText(draft, CONTACT_TEXT);
  if (draft.company_id) body.company_id = draft.company_id;
  return body;
}

/** original is the contact as the API sent it. */
export function contactPatchBody(original: Record<string, unknown> & { company_id?: string | null }, draft: ContactDraft) {
  const body: Record<string, string | null> = patchText(original, draft, CONTACT_TEXT);
  const before = original.company_id ?? null;
  const after = draft.company_id ?? null;
  if (after !== before) body.company_id = after;
  return body;
}

export function companyCreateBody(draft: CompanyDraft) {
  return createText(draft, COMPANY_TEXT);
}

export function companyPatchBody(original: Record<string, unknown>, draft: CompanyDraft) {
  return patchText(original, draft, COMPANY_TEXT);
}
