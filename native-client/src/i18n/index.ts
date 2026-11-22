// The language of the screens. The English text is the key: `t('Sign in')` is English as it
// stands, and Spanish is looked up in `es.ts`, falling back to the English text for
// anything not translated. The language lives in a store; the root layout remounts the
// screens when it changes, so a plain `t()` call in a component is enough.

import { createStore, useStore } from '../lib/store';
import { deviceStorage } from '../lib/storage';
import { es, esDefaultNames } from './es';

export const LOCALES = ['en', 'es'] as const;
export type Locale = (typeof LOCALES)[number];

export const LOCALE_NAMES: Record<Locale, string> = { en: 'English', es: 'Español' };

const KEY = 'crm.locale';

export const localeStore = createStore<{ locale: Locale }>({ locale: 'en' });

const known = (code: string | null | undefined): Locale | undefined => LOCALES.find((locale) => locale === code?.toLowerCase().split(/[-_]/)[0]);

/** The saved choice if there is one, otherwise the first device language the app has, otherwise English. */
export function pickLocale(saved: string | null, languages: readonly string[]): Locale {
  return known(saved) ?? languages.map(known).find(Boolean) ?? 'en';
}

/** The device's languages, most preferred first. */
function deviceLanguages(): string[] {
  const found: string[] = [];
  const nav = typeof navigator === 'undefined' ? undefined : navigator;
  if (nav?.languages?.length) found.push(...nav.languages);
  else if (nav?.language) found.push(nav.language);
  try {
    found.push(Intl.DateTimeFormat().resolvedOptions().locale);
  } catch {
    // No Intl: English.
  }
  return found;
}

/** Reads the saved or the device's language. Called once when the app starts. */
export async function startLocale(): Promise<void> {
  let saved: string | null = null;
  try {
    saved = await deviceStorage.get(KEY);
  } catch {
    // No storage: the device's language decides.
  }
  localeStore.set({ locale: pickLocale(saved, deviceLanguages()) });
}

export async function setLocale(locale: Locale): Promise<void> {
  localeStore.set({ locale });
  try {
    await deviceStorage.set(KEY, locale);
  } catch {
    // The choice lasts until the app closes.
  }
}

/** Re-renders the component when the language changes. */
export const useLocale = (): Locale => useStore(localeStore).locale;

/** The text in the current language; `{name}` marks a value from `params`. */
export function t(text: string, params?: Record<string, string | number>): string {
  const translated = localeStore.get().locale === 'es' ? (es[text] ?? text) : text;
  if (!params) return translated;
  return translated.replace(/\{(\w+)\}/g, (whole, name: string) => (name in params ? String(params[name]) : whole));
}

/**
 * A pipeline or stage name as the person sees it: the ones a workspace starts with are
 * translated, and any name they typed is left alone.
 */
export const nameOf = (name: string): string => (localeStore.get().locale === 'es' ? (esDefaultNames[name] ?? name) : name);

/** The language dates and numbers are written in. */
export const dateLocale = (): string => (localeStore.get().locale === 'es' ? 'es' : 'en-US');

/** A member's role as a word. */
export const roleLabel = (role: string): string => ({ owner: t('Owner'), admin: t('Admin'), member: t('Member') })[role as 'owner'] ?? role;
