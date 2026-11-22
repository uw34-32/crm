import { afterEach, describe, expect, it } from 'vitest';
import { es } from './es.js';
import { i18n, pickLocale, setLocale, t } from './i18n.svelte.js';

afterEach(() => setLocale('en', false));

describe('picking the language', () => {
  it('prefers the saved choice, then the browser languages in order, then English', () => {
    expect(pickLocale('es', ['en-US'])).toBe('es');
    expect(pickLocale(null, ['fr-FR', 'es-MX', 'en'])).toBe('es');
    expect(pickLocale(null, ['en-GB'])).toBe('en');
    expect(pickLocale(null, ['fr'])).toBe('en');
    expect(pickLocale('xx', [])).toBe('en');
  });
});

describe('translating', () => {
  it('is the English text itself in English, with the values filled in', () => {
    expect(t('Sign in')).toBe('Sign in');
    expect(t('Added {name}', { name: 'Acme' })).toBe('Added Acme');
  });

  it('looks the text up in Spanish and falls back to English for what is missing', () => {
    setLocale('es', false);
    expect(i18n.locale).toBe('es');
    expect(t('Sign in')).toBe('Iniciar sesión');
    expect(t('Added {name}', { name: 'Acme' })).toBe('Se añadió Acme');
    expect(t('A text nobody translated')).toBe('A text nobody translated');
  });

  it('leaves a mark with no value as it is', () => {
    expect(t('Added {name}', {})).toBe('Added {name}');
  });
});

describe('the Spanish table', () => {
  it('keeps the same {marks} as the English text of every entry', () => {
    const marks = (/** @type {string} */ text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
    for (const [english, spanish] of Object.entries(es)) expect(marks(spanish), english).toEqual(marks(english));
  });
});

describe('the texts in the code', () => {
  it('all have a Spanish translation', () => {
    const sources = /** @type {Record<string, string>} */ (import.meta.glob('/src/**/*.{svelte,js}', { query: '?raw', import: 'default', eager: true }));
    const missing = new Set();
    for (const [path, source] of Object.entries(sources)) {
      if (path.endsWith('.test.js') || path.endsWith('/es.js')) continue;
      for (const match of source.matchAll(/\bt\('((?:[^'\\]|\\.)*)'/g)) {
        const text = match[1].replaceAll("\\'", "'");
        if (!(text in es)) missing.add(text);
      }
    }
    expect([...missing]).toEqual([]);
  });
});
