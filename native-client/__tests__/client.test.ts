import { ApiError, createClient, REFRESH_TIMEOUT_MS, type Tokens } from '../src/api/client';
import { BASE, reply } from '../test-support/helpers';

function setup(fetch: jest.Mock, refresh: string | null = 'refresh-1') {
  let access = 'access-1';
  let stored = refresh;
  const tokens: Tokens = {
    getAccess: () => access,
    getRefresh: () => stored,
    save: (s) => {
      access = s.token;
      stored = s.refresh_token;
    },
    clear: () => {
      access = '';
      stored = null;
    }
  };
  const onSessionEnd = jest.fn();
  return { client: createClient({ baseUrl: BASE, fetch, tokens, onSessionEnd }), tokens, onSessionEnd };
}

describe('request', () => {
  it('sends the access token and answers the data', async () => {
    const fetch = jest.fn(async () => reply(200, { data: { id: 1 } }));
    const { client } = setup(fetch);
    expect(await client.request('/things')).toEqual({ id: 1 });
    expect((fetch.mock.calls[0] as unknown as [string, RequestInit])[1].headers).toMatchObject({ Authorization: 'Bearer access-1' });
  });

  it('answers null for 204', async () => {
    const { client } = setup(jest.fn(async () => reply(204)));
    expect(await client.request('/x', { method: 'DELETE' })).toBeNull();
  });

  it('passes the API message of a refused request through', async () => {
    const { client } = setup(jest.fn(async () => reply(409, { error: 'email taken' })));
    await expect(client.request('/x', { auth: false })).rejects.toMatchObject({ status: 409, message: 'email taken' });
  });

  it('keeps server details out of the message', async () => {
    const { client } = setup(jest.fn(async () => reply(500, { error: 'pq: boom' })));
    const err = (await client.request('/x').catch((e: unknown) => e)) as Error;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.message).not.toContain('boom');
  });

  it('reports an unreachable server as status 0', async () => {
    const { client } = setup(jest.fn(async () => Promise.reject(new TypeError('network'))));
    await expect(client.request('/x')).rejects.toMatchObject({ status: 0 });
  });

  it('names the wait of a rate limit', async () => {
    const { client } = setup(jest.fn(async () => reply(429, {}, { 'Retry-After': '7' })));
    await expect(client.request('/x')).rejects.toMatchObject({ status: 429, retryAfter: 7, message: expect.stringContaining('7 seconds') });
  });
});

describe('refresh', () => {
  it('retries a 401 once with a fresh token, and shares one refresh between callers', async () => {
    const fetch = jest.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) return reply(200, { data: { token: 'access-2', refresh_token: 'refresh-2' } });
      const bearer = (init.headers as Record<string, string>).Authorization;
      return bearer === 'Bearer access-2' ? reply(200, { data: 'ok' }) : reply(401, { error: 'expired' });
    });
    const { client, tokens } = setup(fetch);

    expect(await Promise.all([client.request('/a'), client.request('/b')])).toEqual(['ok', 'ok']);

    expect((fetch.mock.calls as unknown as [string][]).filter(([url]) => String(url).endsWith('/auth/refresh'))).toHaveLength(1);
    expect(tokens.getRefresh()).toBe('refresh-2');
  });

  it('ends the session when the refresh token is refused', async () => {
    const fetch = jest.fn(async (url: string) => (url.endsWith('/auth/refresh') ? reply(401, { error: 'invalid' }) : reply(401, {})));
    const { client, tokens, onSessionEnd } = setup(fetch);
    await expect(client.request('/a')).rejects.toMatchObject({ status: 401 });
    expect(onSessionEnd).toHaveBeenCalledTimes(1);
    expect(tokens.getRefresh()).toBeNull();
  });

  it('keeps the token when the refresh fails for a reason that says nothing about it', async () => {
    const fetch = jest.fn(async (url: string) => (url.endsWith('/auth/refresh') ? reply(429, {}) : reply(401, {})));
    const { client, tokens, onSessionEnd } = setup(fetch);
    await expect(client.request('/a')).rejects.toMatchObject({ status: 429 });
    expect(onSessionEnd).not.toHaveBeenCalled();
    expect(tokens.getRefresh()).toBe('refresh-1');
  });

  it('ends the session when there is no refresh token', async () => {
    const { client, onSessionEnd } = setup(jest.fn(async () => reply(401, {})), null);
    await expect(client.request('/a')).rejects.toMatchObject({ status: 401 });
    expect(onSessionEnd).toHaveBeenCalledTimes(1);
  });

  it('gives up a hung refresh and keeps the token', async () => {
    jest.useFakeTimers();
    try {
      const fetch = jest.fn(
        (_url: string, init: RequestInit) =>
          new Promise((_resolve, reject) => {
            init.signal?.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })));
          })
      );
      const { client, tokens, onSessionEnd } = setup(fetch as unknown as jest.Mock);
      const result = client.refresh().catch((e) => e);
      await jest.advanceTimersByTimeAsync(REFRESH_TIMEOUT_MS);
      expect(await result).toMatchObject({ status: 0 });
      expect(onSessionEnd).not.toHaveBeenCalled();
      expect(tokens.getRefresh()).toBe('refresh-1');
    } finally {
      jest.useRealTimers();
    }
  });
});
