import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { act, renderHook } from '@testing-library/react-native';
import { es } from '../src/i18n/es';
import { dateLocale, nameOf, pickLocale, roleLabel, setLocale, t, useLocale } from '../src/i18n';

afterEach(async () => {
  await setLocale('en');
});

describe('picking the language', () => {
  it('prefers the saved choice, then the device languages in order, then English', () => {
    expect(pickLocale('es', ['en-US'])).toBe('es');
    expect(pickLocale(null, ['fr-FR', 'es-MX', 'en'])).toBe('es');
    expect(pickLocale(null, ['es_ES'])).toBe('es');
    expect(pickLocale(null, ['en-GB'])).toBe('en');
    expect(pickLocale(null, ['fr'])).toBe('en');
    expect(pickLocale('xx', [])).toBe('en');
  });
});

describe('translating', () => {
  it('is the English text itself in English, with the values filled in', () => {
    expect(t('Sign in')).toBe('Sign in');
    expect(t('Added {name}', { name: 'Acme' })).toBe('Added Acme');
    expect(nameOf('Qualified')).toBe('Qualified');
    expect(dateLocale()).toBe('en-US');
    expect(roleLabel('owner')).toBe('Owner');
  });

  it('looks the text up in Spanish and falls back to English for what is missing', async () => {
    await setLocale('es');
    expect(t('Sign in')).toBe('Iniciar sesión');
    expect(t('Added {name}', { name: 'Acme' })).toBe('Se añadió Acme');
    expect(t('A text nobody translated')).toBe('A text nobody translated');
    expect(roleLabel('owner')).toBe('Propietario');
    expect(dateLocale()).toBe('es');
  });

  it('translates the names a workspace starts with and leaves the ones people typed', async () => {
    await setLocale('es');
    expect(nameOf('Qualified')).toBe('Calificado');
    expect(nameOf('My own stage')).toBe('My own stage');
  });

  it('leaves a mark with no value as it is', () => {
    expect(t('Added {name}', {})).toBe('Added {name}');
  });
});

describe('following a change of language', () => {
  it('re-renders a component that uses the hook, without remounting it', async () => {
    let renders = 0;
    const { result } = await renderHook(() => {
      renders++;
      useLocale();
      return t('Sign in');
    });
    expect(result.current).toBe('Sign in');
    const before = renders;
    await act(async () => {
      await setLocale('es');
    });
    expect(result.current).toBe('Iniciar sesión');
    expect(renders).toBeGreaterThan(before);
  });
});

describe('the Spanish table', () => {
  it('keeps the same {marks} as the English text of every entry', () => {
    const marks = (text: string) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
    for (const [english, spanish] of Object.entries(es)) expect(marks(spanish)).toEqual(marks(english));
  });
});

describe('the texts in the code', () => {
  it('all have a Spanish translation', () => {
    const files = (dir: string): string[] =>
      readdirSync(dir).flatMap((name: string) => {
        const path = join(dir, name);
        return statSync(path).isDirectory() ? files(path) : /\.tsx?$/.test(name) && !path.includes(join('src', 'i18n', 'es.ts')) ? [path] : [];
      });
    const missing = new Set<string>();
    for (const path of files('src')) {
      for (const match of readFileSync(path, 'utf8').matchAll(/\bt\('((?:[^'\\]|\\.)*)'/g)) {
        const text = match[1].replaceAll("\\'", "'");
        if (!(text in es)) missing.add(text);
      }
    }
    expect([...missing]).toEqual([]);
  });
});
