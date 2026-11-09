// Checks run before a form is sent. They mirror the API's own limits, so most mistakes
// get a message next to the field instead of a round trip; the API still decides.

const MAX_NAME_CHARS = 200;
const MIN_PASSWORD_CHARS = 8;
const MAX_PASSWORD_BYTES = 72;

// Something, an @, and a domain with a dot. The API is the judge of the rest.
const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** @param {string} value */
export function checkEmail(value) {
  const email = value.trim();
  if (!email) return 'Enter your email address.';
  if (!EMAIL_SHAPE.test(email)) return 'Enter an email address such as ada@example.com.';
  return '';
}

/**
 * Only a new password is held to the length limits: an existing one is whatever it
 * is, and a wrong one gets the API's answer.
 * @param {string} value
 * @param {{ creating: boolean }} options
 */
export function checkPassword(value, { creating }) {
  if (!value) return 'Enter your password.';
  if (!creating) return '';
  if ([...value].length < MIN_PASSWORD_CHARS) return `Use at least ${MIN_PASSWORD_CHARS} characters.`;
  // bcrypt reads 72 bytes, and accented letters and symbols take more than one.
  if (new TextEncoder().encode(value).length > MAX_PASSWORD_BYTES) {
    return 'This password is too long. Use at most 72 plain letters and digits, or fewer accented letters and symbols.';
  }
  return '';
}

/** @param {string} value @param {string} missing the message for a blank name */
export function checkName(value, missing) {
  const name = value.trim();
  if (!name) return missing;
  if ([...name].length > MAX_NAME_CHARS) return `Use at most ${MAX_NAME_CHARS} characters.`;
  return '';
}

/**
 * Where to go after signing in: a path inside the app, never another site and never
 * back to a sign-in page.
 * @param {string | null} next
 */
export function safeNext(next) {
  const fallback = '/dashboard';
  if (!next || !next.startsWith('/') || next.startsWith('//') || next.startsWith('/\\')) return fallback;
  const path = next.split(/[?#]/)[0];
  if (path === '/login' || path === '/register') return fallback;
  return next;
}
