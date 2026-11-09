// The API client: JSON in and out, the access token on every private request, and one
// refresh of an expired token shared by every request that needs it.

/**
 * @typedef {{ token: string, refresh_token: string, user?: unknown }} Session
 * @typedef {{
 *   getAccess: () => string,
 *   getRefresh: () => string | null,
 *   save: (session: Session) => void,
 *   clear: () => void
 * }} Tokens
 * @typedef {{ method?: string, body?: unknown, auth?: boolean, signal?: AbortSignal }} RequestOptions
 */

const MSG_NETWORK = 'Cannot reach the server. Check your connection and try again.';
const MSG_SERVER = 'Something went wrong on the server. Try again in a moment.';
const MSG_BUSY = 'The server is busy. Try again in a moment.';
const MSG_SESSION_ENDED = 'Your session has ended. Sign in again.';

/** A failed request. status is 0 when the server could not be reached. */
export class ApiError extends Error {
  /** @param {number} status @param {string} message @param {number | null} [retryAfter] */
  constructor(status, message, retryAfter = null) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    /** Seconds the server asked to wait, from Retry-After. */
    this.retryAfter = retryAfter;
  }
}

/** @param {Response} res */
function retryAfterOf(res) {
  const seconds = Number.parseInt(res.headers.get('Retry-After') ?? '', 10);
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : null;
}

/**
 * Turns a failed response into an error with a message a person can act on. The API's
 * own messages for a refused request are written for people, so they pass through;
 * server errors keep their details to the logs.
 * @param {Response} res
 */
async function errorOf(res) {
  const raw = await res.json().catch(() => null);
  const apiMessage = typeof raw?.error === 'string' && raw.error ? raw.error : '';
  const retryAfter = retryAfterOf(res);
  if (res.status === 429) {
    const wait = retryAfter === null ? 'Wait a moment' : `Wait ${retryAfter} ${retryAfter === 1 ? 'second' : 'seconds'}`;
    return new ApiError(429, `Too many requests. ${wait} and try again.`, retryAfter);
  }
  if (res.status === 503) return new ApiError(503, MSG_BUSY, retryAfter);
  if (res.status >= 500) return new ApiError(res.status, MSG_SERVER, retryAfter);
  return new ApiError(res.status, apiMessage || MSG_SERVER, retryAfter);
}

/** @param {unknown} err */
function isAbort(err) {
  return err instanceof Error && err.name === 'AbortError';
}

/**
 * @param {{
 *   baseUrl: string,
 *   fetch: typeof globalThis.fetch,
 *   tokens: Tokens,
 *   onSessionEnd: () => void
 * }} options
 */
export function createClient({ baseUrl, fetch, tokens, onSessionEnd }) {
  const base = baseUrl.replace(/\/+$/, '');
  /** @type {Promise<void> | null} */
  let refreshing = null;

  /** @param {string} path @param {RequestOptions} options @param {string} access */
  async function send(path, { method = 'GET', body, signal }, access) {
    /** @type {Record<string, string>} */
    const headers = { Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (access) headers.Authorization = `Bearer ${access}`;
    try {
      return await fetch(base + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), signal });
    } catch (err) {
      if (isAbort(err)) throw err;
      throw new ApiError(0, MSG_NETWORK);
    }
  }

  function endSession() {
    tokens.clear();
    onSessionEnd();
    return new ApiError(401, MSG_SESSION_ENDED);
  }

  /**
   * Trades the refresh token for a new pair. A refused token ends the session; a
   * failure that says nothing about the token, such as a lost connection or a rate
   * limit, keeps it for the next try.
   */
  async function rotate() {
    const refreshToken = tokens.getRefresh();
    if (!refreshToken) throw endSession();
    const res = await send('/auth/refresh', { method: 'POST', body: { refresh_token: refreshToken } }, '');
    if (res.status === 401 || res.status === 400) throw endSession();
    if (!res.ok) throw await errorOf(res);
    const raw = await res.json().catch(() => null);
    if (!raw?.data?.token || !raw.data.refresh_token) throw new ApiError(res.status, MSG_SERVER);
    tokens.save(raw.data);
  }

  /** Every caller waiting at once shares one rotation, so the token rotates once. */
  function refresh() {
    refreshing ??= rotate().finally(() => {
      refreshing = null;
    });
    return refreshing;
  }

  /**
   * Sends a request and answers its data, or null for 204. A private request refused
   * with 401 is retried once with a fresh token: right away if another request has
   * already refreshed it meanwhile, otherwise after one shared refresh.
   * @param {string} path
   * @param {RequestOptions} [options]
   */
  async function request(path, options = {}) {
    const auth = options.auth ?? true;
    let access = auth ? tokens.getAccess() : '';
    let res = await send(path, options, access);
    if (auth && res.status === 401) {
      if (tokens.getAccess() === access || !tokens.getAccess()) await refresh();
      access = tokens.getAccess();
      res = await send(path, options, access);
      if (res.status === 401) throw endSession();
    }
    if (!res.ok) throw await errorOf(res);
    if (res.status === 204) return null;
    const raw = await res.json().catch(() => null);
    return raw?.data ?? null;
  }

  return { request, refresh };
}
