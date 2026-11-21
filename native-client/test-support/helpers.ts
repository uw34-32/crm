import type { AsyncStorage } from '../src/api/tokens';

export const BASE = 'http://api.test';

export function reply(status: number, body: unknown = null, headers: Record<string, string> = {}) {
  return new Response(status === 204 ? null : JSON.stringify(body), { status, headers });
}

export function memoryStorage(initial: Record<string, string> = {}): AsyncStorage & { data: Map<string, string> } {
  const data = new Map(Object.entries(initial));
  return {
    data,
    get: async (key) => data.get(key) ?? null,
    set: async (key, value) => void data.set(key, value),
    remove: async (key) => void data.delete(key)
  };
}

export const failingStorage: AsyncStorage = {
  get: async () => {
    throw new Error('locked');
  },
  set: async () => {
    throw new Error('locked');
  },
  remove: async () => {
    throw new Error('locked');
  }
};
