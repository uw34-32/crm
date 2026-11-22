// Checks run before a form is sent. They mirror the API's own limits, so most mistakes
// get a message next to the field instead of a round trip; the API still decides.

import { t } from '../i18n';

const MAX_NAME_CHARS = 200;
const MIN_PASSWORD_CHARS = 8;
const MAX_PASSWORD_BYTES = 72;

// Something, an @, and a domain with a dot. The API is the judge of the rest.
export const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function checkEmail(value: string) {
  const email = value.trim();
  if (!email) return t('Enter your email address.');
  if (!EMAIL_SHAPE.test(email)) return t('Enter an email address such as ada@example.com.');
  return '';
}

/**
 * Only a new password is held to the length limits: an existing one is whatever it
 * is, and a wrong one gets the API's answer.
 */
export function checkPassword(value: string, { creating }: { creating: boolean }) {
  if (!value) return t('Enter your password.');
  if (!creating) return '';
  if ([...value].length < MIN_PASSWORD_CHARS) return t('Use at least {n} characters.', { n: MIN_PASSWORD_CHARS });
  // bcrypt reads 72 bytes, and accented letters and symbols take more than one.
  if (new TextEncoder().encode(value).length > MAX_PASSWORD_BYTES) {
    return t('This password is too long. Use at most 72 plain letters and digits, or fewer accented letters and symbols.');
  }
  return '';
}

export function checkName(value: string, missing: string) {
  const name = value.trim();
  if (!name) return missing;
  if ([...name].length > MAX_NAME_CHARS) return t('Use at most {n} characters.', { n: MAX_NAME_CHARS });
  return '';
}

/**
 * The API writes its messages as lowercase phrases; on screen they read as sentences.
 */
export function toSentence(message: string) {
  const text = message.trim();
  if (!text) return '';
  const capitalized = text[0].toUpperCase() + text.slice(1);
  return /[.!?]$/.test(capitalized) ? capitalized : `${capitalized}.`;
}
