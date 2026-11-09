import { describe, expect, it, vi } from 'vitest';
import { ApiError, createClient } from './client.js';
import { createTokenStore, REFRESH_TOKEN_KEY } from './tokens.js';

const BASE = 'http://api.test';

/** @param {number} status @param {unknown} [body] @param {Record<string, string>} [headers] */
function reply(status, body, headers = {}) {
  const text = body === undefined ? null : typeof body === 'string' ? body : JSON.stringify(body);
  return new Response(text, { status, headers });
}

/** A token holder like the session's, kept in plain variables. */
function memoryTokens(access = 'access-1', refresh = /** @type {string | null} */ ('refresh-1')) {
  const tokens = {
    access,
    refresh,
    saved: /** @type {unknown[]} */ ([]),
    cleared: 0,
    getAccess: () => tokens.access,
    getRefresh: () => tokens.refresh,
    /** @param {{ token: string, refresh_token: string }} session */
    save(session) {
      tokens.saved.push(session);
      tokens.access = session.token;
      tokens.refresh = session.refresh_token;
    },
    clear() {
      tokens.cleared++;
      tokens.access = '';
      tokens.refresh = null;
    }
  };
  return tokens;
}

/** A fake API: answers by route, and records what it was asked. */
function fakeApi() {
  /** @type {{ method: string, path: string, auth: string | null, body: unknown }[]} */
  const calls = [];
  /** @type {Record<string, (call: { method: string, path: string, auth: string | null, body: unknown }) => Response | Promise<Response>>} */
  const routes = {};
  const fetch = vi.fn(async (/** @type {RequestInfo | URL} */ url, /** @type {RequestInit} */ init = {}) => {
    const headers = new Headers(init.headers);
    const call = {
      method: init.method ?? 'GET',
      path: String(url).slice(BASE.length),
      auth: headers.get('Authorization'),
      body: init.body ? JSON.parse(String(init.body)) : undefined
    };
    calls.push(call);
    const route = routes[`${call.method} ${call.path}`];
    if (!route) throw new Error(`no route for ${call.method} ${call.path}`);
    return route(call);
  });
  // The access token the server accepts: the last one it issued.
  return { calls, routes, fetch, valid: 'access-1' };
}

/** @param {ReturnType<typeof fakeApi>} api @param {ReturnType<typeof memoryTokens>} tokens */
function clientFor(api, tokens, onSessionEnd = vi.fn()) {
  return { client: createClient({ baseUrl: BASE, fetch: api.fetch, tokens, onSessionEnd }), onSessionEnd };
}

/** Answers 200 to the access token the server last issued and 401 to anything else. */
function guarded(/** @type {ReturnType<typeof fakeApi>} */ api, data = /** @type {unknown} */ ({ ok: true })) {
  return (/** @type {{ auth: string | null }} */ call) =>
    call.auth === `Bearer ${api.valid}` ? reply(200, { data }) : reply(401, { error: 'invalid or expired token' });
}

function rotatingRefresh(/** @type {ReturnType<typeof fakeApi>} */ api) {
  let n = 1;
  return async () => {
    // Let parallel requests pile up behind one refresh before it answers.
    await new Promise((resolve) => setTimeout(resolve, 5));
    n++;
    api.valid = `access-${n}`;
    return reply(200, { data: { token: `access-${n}`, refresh_token: `refresh-${n}`, user: { id: 'u1' } } });
  };
}

describe('requests', () => {
  it('sends the access token and JSON, and unwraps data', async () => {
    const api = fakeApi();
    const tokens = memoryTokens();
    api.routes['POST /workspaces'] = (call) => reply(201, { data: { id: 'w1', name: call.body && /** @type {any} */ (call.body).name } });
    const { client } = clientFor(api, tokens);

    const got = await client.request('/workspaces', { method: 'POST', body: { name: 'Acme' } });

    expect(got).toEqual({ id: 'w1', name: 'Acme' });
    expect(api.calls[0].auth).toBe('Bearer access-1');
  });

  it('answers null for 204', async () => {
    const api = fakeApi();
    api.routes['POST /workspaces/w1/leave'] = () => reply(204);
    const { client } = clientFor(api, memoryTokens());

    expect(await client.request('/workspaces/w1/leave', { method: 'POST' })).toBeNull();
  });

  it('sends no token on a public route', async () => {
    const api = fakeApi();
    api.routes['POST /auth/login'] = () => reply(200, { data: { token: 't' } });
    const { client } = clientFor(api, memoryTokens());

    await client.request('/auth/login', { method: 'POST', body: {}, auth: false });

    expect(api.calls[0].auth).toBeNull();
  });
});

describe('refresh on 401', () => {
  it('refreshes once and retries the request with the new token', async () => {
    const api = fakeApi();
    const tokens = memoryTokens();
    tokens.access = 'expired';
    api.routes['GET /auth/me'] = guarded(api, { id: 'u1' });
    api.routes['POST /auth/refresh'] = rotatingRefresh(api);
    const { client } = clientFor(api, tokens);

    const got = await client.request('/auth/me');

    expect(got).toEqual({ id: 'u1' });
    const refreshes = api.calls.filter((c) => c.path === '/auth/refresh');
    expect(refreshes).toHaveLength(1);
    expect(refreshes[0].body).toEqual({ refresh_token: 'refresh-1' });
    expect(tokens.refresh).toBe('refresh-2');
    expect(api.calls.at(-1)?.auth).toBe('Bearer access-2');
  });

  it('shares one refresh among parallel requests, so the token rotates once', async () => {
    const api = fakeApi();
    const tokens = memoryTokens();
    tokens.access = 'expired';
    api.routes['GET /workspaces'] = guarded(api, []);
    api.routes['GET /auth/me'] = guarded(api, { id: 'u1' });
    api.routes['POST /auth/refresh'] = rotatingRefresh(api);
    const { client } = clientFor(api, tokens);

    const results = await Promise.all([client.request('/workspaces'), client.request('/auth/me'), client.request('/workspaces')]);

    expect(results).toEqual([[], { id: 'u1' }, []]);
    expect(api.calls.filter((c) => c.path === '/auth/refresh')).toHaveLength(1);
    expect(tokens.saved).toHaveLength(1);
  });

  it('retries with the current token, without refreshing, when another request already did', async () => {
    const api = fakeApi();
    const tokens = memoryTokens();
    let release = () => {};
    const held = new Promise((resolve) => (release = () => resolve(undefined)));
    // The first request leaves with the old token, and its 401 arrives late.
    api.routes['GET /workspaces'] = async (call) => {
      if (call.auth === 'Bearer access-1') {
        await held;
        return reply(401, { error: 'invalid or expired token' });
      }
      return reply(200, { data: [] });
    };
    api.routes['GET /auth/me'] = guarded(api, { id: 'u1' });
    api.routes['POST /auth/refresh'] = rotatingRefresh(api);
    const { client } = clientFor(api, tokens);

    const slow = client.request('/workspaces');
    tokens.access = 'expired';
    await client.request('/auth/me');
    release();

    expect(await slow).toEqual([]);
    expect(api.calls.filter((c) => c.path === '/auth/refresh')).toHaveLength(1);
  });

  it('retries only once, and ends the session when the retry is refused too', async () => {
    const api = fakeApi();
    const tokens = memoryTokens();
    api.routes['GET /auth/me'] = () => reply(401, { error: 'invalid or expired token' });
    api.routes['POST /auth/refresh'] = rotatingRefresh(api);
    const { client, onSessionEnd } = clientFor(api, tokens);

    const failure = await client.request('/auth/me').catch((e) => e);

    expect(failure).toBeInstanceOf(ApiError);
    expect(failure.status).toBe(401);
    expect(api.calls.filter((c) => c.path === '/auth/me')).toHaveLength(2);
    expect(api.calls.filter((c) => c.path === '/auth/refresh')).toHaveLength(1);
    expect(tokens.cleared).toBe(1);
    expect(onSessionEnd).toHaveBeenCalledTimes(1);
  });

  it('ends the session once when the refresh is refused, for every waiting request', async () => {
    const api = fakeApi();
    const tokens = memoryTokens();
    tokens.access = 'expired';
    api.routes['GET /auth/me'] = guarded(api);
    api.routes['GET /workspaces'] = guarded(api);
    api.routes['POST /auth/refresh'] = () => reply(401, { error: 'invalid refresh token' });
    const { client, onSessionEnd } = clientFor(api, tokens);

    const failures = await Promise.all([client.request('/auth/me').catch((e) => e), client.request('/workspaces').catch((e) => e)]);

    for (const failure of failures) {
      expect(failure).toBeInstanceOf(ApiError);
      expect(failure.status).toBe(401);
      expect(failure.message).toMatch(/session has ended/i);
    }
    expect(api.calls.filter((c) => c.path === '/auth/refresh')).toHaveLength(1);
    expect(tokens.cleared).toBe(1);
    expect(tokens.refresh).toBeNull();
    expect(onSessionEnd).toHaveBeenCalledTimes(1);
  });

  it('ends the session without a refresh when there is no refresh token', async () => {
    const api = fakeApi();
    const tokens = memoryTokens('expired', null);
    api.routes['GET /auth/me'] = guarded(api);
    const { client, onSessionEnd } = clientFor(api, tokens);

    await expect(client.request('/auth/me')).rejects.toMatchObject({ status: 401 });
    expect(api.calls.some((c) => c.path === '/auth/refresh')).toBe(false);
    expect(onSessionEnd).toHaveBeenCalledTimes(1);
  });

  it('keeps the session when the refresh cannot reach the server', async () => {
    const api = fakeApi();
    const tokens = memoryTokens();
    tokens.access = 'expired';
    api.routes['GET /auth/me'] = guarded(api);
    api.routes['POST /auth/refresh'] = () => {
      throw new TypeError('Failed to fetch');
    };
    const { client, onSessionEnd } = clientFor(api, tokens);

    await expect(client.request('/auth/me')).rejects.toMatchObject({ status: 0 });
    expect(tokens.cleared).toBe(0);
    expect(tokens.refresh).toBe('refresh-1');
    expect(onSessionEnd).not.toHaveBeenCalled();
  });

  it('keeps the session when the refresh is rate limited', async () => {
    const api = fakeApi();
    const tokens = memoryTokens();
    tokens.access = 'expired';
    api.routes['GET /auth/me'] = guarded(api);
    api.routes['POST /auth/refresh'] = () => reply(429, { error: 'too many requests' }, { 'Retry-After': '12' });
    const { client, onSessionEnd } = clientFor(api, tokens);

    await expect(client.request('/auth/me')).rejects.toMatchObject({ status: 429, retryAfter: 12 });
    expect(tokens.cleared).toBe(0);
    expect(onSessionEnd).not.toHaveBeenCalled();
  });

  it('does not refresh for a 401 on a public route, such as a wrong password', async () => {
    const api = fakeApi();
    api.routes['POST /auth/login'] = () => reply(401, { error: 'invalid email or password' });
    const { client, onSessionEnd } = clientFor(api, memoryTokens());

    await expect(client.request('/auth/login', { method: 'POST', body: {}, auth: false })).rejects.toMatchObject({
      status: 401,
      message: 'invalid email or password'
    });
    expect(api.calls).toHaveLength(1);
    expect(onSessionEnd).not.toHaveBeenCalled();
  });
});

describe('error messages', () => {
  /** @param {() => Response} answer */
  async function failureOf(answer) {
    const api = fakeApi();
    api.routes['GET /workspaces'] = answer;
    const { client } = clientFor(api, memoryTokens());
    return client.request('/workspaces').catch((e) => e);
  }

  it('says how long to wait after a 429', async () => {
    const failure = await failureOf(() => reply(429, { error: 'too many requests' }, { 'Retry-After': '30' }));
    expect(failure).toMatchObject({ status: 429, retryAfter: 30 });
    expect(failure.message).toMatch(/too many requests/i);
    expect(failure.message).toMatch(/30 seconds/);
  });

  it('asks to wait without a number when a 429 has no Retry-After', async () => {
    const failure = await failureOf(() => reply(429, { error: 'too many requests' }));
    expect(failure).toMatchObject({ status: 429, retryAfter: null });
    expect(failure.message).toMatch(/wait a moment/i);
  });

  it('says the server is busy after a 503', async () => {
    const failure = await failureOf(() => reply(503, { error: 'server busy' }, { 'Retry-After': '1' }));
    expect(failure.status).toBe(503);
    expect(failure.message).toMatch(/busy/i);
  });

  it('hides the details of a server error', async () => {
    const failure = await failureOf(() => reply(500, { error: 'internal server error' }));
    expect(failure.status).toBe(500);
    expect(failure.message).toMatch(/something went wrong/i);
  });

  it('handles a body that is not JSON', async () => {
    const failure = await failureOf(() => reply(502, '<html>Bad gateway</html>'));
    expect(failure.status).toBe(502);
    expect(failure.message).toMatch(/something went wrong/i);
  });

  it('passes on the API message for a request it refused', async () => {
    for (const status of [400, 403, 404, 409]) {
      const failure = await failureOf(() => reply(status, { error: 'a name of at most 200 characters is required' }));
      expect(failure).toMatchObject({ status, message: 'a name of at most 200 characters is required' });
    }
  });

  it('says when the server cannot be reached', async () => {
    const failure = await failureOf(() => {
      throw new TypeError('Failed to fetch');
    });
    expect(failure).toBeInstanceOf(ApiError);
    expect(failure.status).toBe(0);
    expect(failure.message).toMatch(/cannot reach the server/i);
  });

  it('lets an aborted request stay an abort', async () => {
    const failure = await failureOf(() => {
      throw new DOMException('The operation was aborted.', 'AbortError');
    });
    expect(failure.name).toBe('AbortError');
  });
});

describe('refresh across tabs', () => {
  /** Storage shared by every tab of one browser. */
  function sharedStorage() {
    const map = new Map();
    return {
      getItem: (/** @type {string} */ key) => (map.has(key) ? map.get(key) : null),
      setItem: (/** @type {string} */ key, /** @type {string} */ value) => void map.set(key, String(value)),
      removeItem: (/** @type {string} */ key) => void map.delete(key)
    };
  }

  /** Web Locks as a browser grants them: one holder per name at a time, in request order. */
  function sharedLocks() {
    /** @type {Map<string, Promise<unknown>>} */
    const tails = new Map();
    const requested = /** @type {string[]} */ ([]);
    return {
      requested,
      /** @template T @param {string} name @param {() => Promise<T>} callback */
      request(name, callback) {
        requested.push(name);
        const run = (tails.get(name) ?? Promise.resolve()).then(() => callback());
        tails.set(name, run.catch(() => undefined));
        return run;
      }
    };
  }

  /** An API that rotates refresh tokens and revokes the family when a spent one returns. */
  function rotatingServer() {
    let n = 1;
    const server = { live: new Set(['refresh-1']), spent: new Set(), revoked: false, rotations: 0 };
    const fetch = vi.fn(async (/** @type {RequestInfo | URL} */ url, /** @type {RequestInit} */ init = {}) => {
      expect(String(url)).toBe(`${BASE}/auth/refresh`);
      const { refresh_token: presented } = JSON.parse(String(init.body));
      // Both tabs' requests are in flight before either answer arrives.
      await new Promise((resolve) => setTimeout(resolve, 5));
      if (server.spent.has(presented)) {
        server.revoked = true;
        server.live.clear();
        return reply(401, { error: 'invalid refresh token' });
      }
      if (!server.live.has(presented)) return reply(401, { error: 'invalid refresh token' });
      server.live.delete(presented);
      server.spent.add(presented);
      n++;
      server.rotations++;
      server.live.add(`refresh-${n}`);
      return reply(200, { data: { token: `access-${n}`, refresh_token: `refresh-${n}`, user: { id: 'u1' } } });
    });
    return { server, fetch };
  }

  /** @param {ReturnType<typeof sharedStorage>} storage @param {ReturnType<typeof rotatingServer>} api @param {unknown} locks */
  function tab(storage, api, locks) {
    const tokens = createTokenStore(storage);
    const onSessionEnd = vi.fn();
    const client = createClient({ baseUrl: BASE, fetch: api.fetch, tokens, onSessionEnd, locks: /** @type {any} */ (locks) });
    return { tokens, client, onSessionEnd };
  }

  it('rotates one at a time across tabs, each with the token the last one stored', async () => {
    const storage = sharedStorage();
    storage.setItem(REFRESH_TOKEN_KEY, 'refresh-1');
    const api = rotatingServer();
    const locks = sharedLocks();
    const first = tab(storage, api, locks);
    const second = tab(storage, api, locks);

    // Two tabs restoring the session at the same moment.
    await Promise.all([first.client.refresh(), second.client.refresh()]);

    expect(api.server.revoked).toBe(false);
    expect(api.server.rotations).toBe(2);
    expect(api.fetch.mock.calls.map(([, init]) => JSON.parse(String(init?.body)).refresh_token)).toEqual(['refresh-1', 'refresh-2']);
    expect(first.tokens.getAccess()).toBe('access-2');
    expect(second.tokens.getAccess()).toBe('access-3');
    expect(storage.getItem(REFRESH_TOKEN_KEY)).toBe('refresh-3');
    expect(first.onSessionEnd).not.toHaveBeenCalled();
    expect(second.onSessionEnd).not.toHaveBeenCalled();
    expect(new Set(locks.requested)).toEqual(new Set(['crm.refresh']));
  });

  it('ends the session in a tab whose lock turn comes after another tab signed out', async () => {
    const storage = sharedStorage();
    storage.setItem(REFRESH_TOKEN_KEY, 'refresh-1');
    const api = rotatingServer();
    const locks = sharedLocks();
    const other = tab(storage, api, locks);
    const here = tab(storage, api, locks);

    // The other tab holds the lock while this one waits; it signs out before releasing it.
    let release = () => {};
    let started = () => {};
    const running = new Promise((resolve) => (started = () => resolve(undefined)));
    const held = locks.request('crm.refresh', () => {
      started();
      return new Promise((resolve) => (release = () => resolve(undefined)));
    });
    await running;
    const waiting = here.client.refresh().catch((e) => e);
    other.tokens.clear();
    release();
    await held;

    expect(await waiting).toMatchObject({ status: 401 });
    expect(api.fetch).not.toHaveBeenCalled();
    expect(here.onSessionEnd).toHaveBeenCalledTimes(1);
  });

  it('still refreshes, one request at a time within the tab, where Web Locks are missing', async () => {
    const storage = sharedStorage();
    storage.setItem(REFRESH_TOKEN_KEY, 'refresh-1');
    const api = rotatingServer();
    const only = tab(storage, api, null);

    await Promise.all([only.client.refresh(), only.client.refresh()]);

    expect(api.fetch).toHaveBeenCalledTimes(1);
    expect(only.tokens.getAccess()).toBe('access-2');
  });
});
