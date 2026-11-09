// The fields of contacts and companies: checks that mirror the API's limits, and the
// request bodies. A create leaves out blank optional fields. A PATCH names only what
// changed, clears an optional field with an empty string (the API refuses null on
// text), and unlinks a contact's company with company_id: null.

import { EMAIL_SHAPE } from '../forms.js';

/**
 * @typedef {{ first_name?: string, last_name?: string, email?: string, phone?: string, title?: string, company_id?: string | null }} ContactDraft
 * @typedef {{ name?: string, domain?: string, phone?: string, industry?: string, address?: string }} CompanyDraft
 */

export const CONTACT_TEXT = /** @type {const} */ (['first_name', 'last_name', 'email', 'phone', 'title']);
export const COMPANY_TEXT = /** @type {const} */ (['name', 'domain', 'phone', 'industry', 'address']);

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
const bytes = (/** @type {string} */ text) => new TextEncoder().encode(text).length;

/** @param {unknown} value */
const text = (value) => (typeof value === 'string' ? value.trim() : '');
const chars = (/** @type {string} */ value) => [...value].length;

/** @param {string} value @param {number} max */
function tooLong(value, max) {
  return chars(value) > max ? `Use at most ${max} characters.` : '';
}

/** @param {string} value */
function checkOptionalEmail(value) {
  return value && !EMAIL_SHAPE.test(value) ? 'Enter an email address such as ada@example.com.' : '';
}

/** @param {string} value */
function checkOptionalPhone(value) {
  if (!value) return '';
  if (!DIGIT.test(value)) return 'A phone number needs at least one digit.';
  return tooLong(value, LIMITS.phone);
}

/** A host name such as acme.example, the way the API checks it. @param {string} domain */
export function isDomain(domain) {
  if (bytes(domain) > MAX_DOMAIN_BYTES) return false;
  const labels = domain.split('.');
  return (
    labels.length >= 2 &&
    labels.every((label) => label && bytes(label) <= MAX_LABEL_BYTES && !label.startsWith('-') && !label.endsWith('-') && LABEL.test(label))
  );
}

/** @param {Record<string, string>} errors */
const compact = (errors) => Object.fromEntries(Object.entries(errors).filter(([, message]) => message));

/** @param {ContactDraft} draft @returns {Record<string, string>} */
export function validateContact(draft) {
  const first = text(draft.first_name);
  return compact({
    first_name: first ? tooLong(first, LIMITS.first_name) : 'Enter a first name.',
    last_name: tooLong(text(draft.last_name), LIMITS.last_name),
    email: checkOptionalEmail(text(draft.email)),
    phone: checkOptionalPhone(text(draft.phone)),
    title: tooLong(text(draft.title), LIMITS.title)
  });
}

/** @param {CompanyDraft} draft @returns {Record<string, string>} */
export function validateCompany(draft) {
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
/** @param {string} field @param {unknown} value */
const stored = (field, value) => (field === 'domain' ? text(value).toLowerCase() : text(value));

/**
 * @param {Record<string, unknown>} draft
 * @param {readonly string[]} fields
 */
function createText(draft, fields) {
  /** @type {Record<string, string>} */
  const body = {};
  for (const field of fields) {
    const value = stored(field, draft[field]);
    if (value) body[field] = value;
  }
  return body;
}

/**
 * @param {Record<string, unknown>} original
 * @param {Record<string, unknown>} draft
 * @param {readonly string[]} fields
 */
function patchText(original, draft, fields) {
  /** @type {Record<string, string>} */
  const body = {};
  for (const field of fields) {
    const value = stored(field, draft[field]);
    if (value !== stored(field, original[field])) body[field] = value;
  }
  return body;
}

/** @param {ContactDraft} draft */
export function contactCreateBody(draft) {
  /** @type {Record<string, string>} */
  const body = createText(draft, CONTACT_TEXT);
  if (draft.company_id) body.company_id = draft.company_id;
  return body;
}

/**
 * @param {ContactDraft & Record<string, unknown>} original the contact as the API sent it
 * @param {ContactDraft} draft
 * @returns {Record<string, string | null>}
 */
export function contactPatchBody(original, draft) {
  /** @type {Record<string, string | null>} */
  const body = patchText(original, draft, CONTACT_TEXT);
  const before = original.company_id ?? null;
  const after = draft.company_id ?? null;
  if (after !== before) body.company_id = after;
  return body;
}

/** @param {CompanyDraft} draft */
export function companyCreateBody(draft) {
  return createText(draft, COMPANY_TEXT);
}

/** @param {CompanyDraft & Record<string, unknown>} original @param {CompanyDraft} draft */
export function companyPatchBody(original, draft) {
  return patchText(original, draft, COMPANY_TEXT);
}
