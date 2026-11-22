// The language of the screens. The English text is the key: `t('Sign in')` is English as
// it stands, and Spanish is looked up in `es.js`, falling back to the English text for
// anything not translated. `i18n.locale` is reactive, so a screen that calls `t` in its
// markup follows a change of language without a reload.

import { es, esDefaultNames } from './es.js';
import { browserStorage } from './storage.js';

export const LOCALES = /** @type {const} */ (['en', 'es']);
/** @typedef {(typeof LOCALES)[number]} Locale */

const KEY = 'crm.locale';

export const i18n = $state({ locale: /** @type {Locale} */ ('en') });

/**
 * The saved choice if there is one, otherwise the first of the browser's languages that
 * the app has, otherwise English.
 * @param {string | null} saved
 * @param {readonly string[]} languages
 * @returns {Locale}
 */
export function pickLocale(saved, languages) {
  const known = (/** @type {string | null | undefined} */ code) => LOCALES.find((locale) => locale === code?.toLowerCase().split('-')[0]);
  return known(saved) ?? languages.map(known).find(Boolean) ?? 'en';
}

/** Reads the saved or the browser's language. Called once when the app starts. */
export function startLocale() {
  /** @type {string | null} */
  let saved = null;
  try {
    saved = browserStorage().getItem(KEY);
  } catch {
    // No storage: the browser's language decides.
  }
  const languages = typeof navigator === 'undefined' ? [] : navigator.languages?.length ? navigator.languages : [navigator.language];
  setLocale(pickLocale(saved, languages), false);
}

/** @param {Locale} locale @param {boolean} [save] */
export function setLocale(locale, save = true) {
  i18n.locale = locale;
  if (typeof document !== 'undefined') document.documentElement.lang = locale;
  if (!save) return;
  try {
    browserStorage().setItem(KEY, locale);
  } catch {
    // The choice lasts until the page closes.
  }
}

/**
 * The text in the current language; `{name}` marks a value from `params`.
 * @param {string} text the English text
 * @param {Record<string, string | number>} [params]
 */
export function t(text, params) {
  const translated = i18n.locale === 'es' ? (es[text] ?? text) : text;
  if (!params) return translated;
  return translated.replace(/\{(\w+)\}/g, (whole, name) => (name in params ? String(params[name]) : whole));
}

/**
 * A pipeline or stage name as the person sees it: the ones a workspace starts with are
 * translated, and any name they typed is left alone.
 * @param {string} name
 */
export const nameOf = (name) => (i18n.locale === 'es' ? (esDefaultNames[name] ?? name) : name);

/** Whether amounts are typed with a comma for the decimals, as in 1250,50. */
export const decimalComma = () => i18n.locale === 'es';

/** The language the dates and numbers are written in. */
export const dateLocale = () => (i18n.locale === 'es' ? 'es' : 'en-US');
