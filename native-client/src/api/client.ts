// The API client: JSON in and out, the access token on every private request, and one
// refresh of an expired token shared by every request that needs it. A device runs one
// app instance, so single-flight inside the process is enough; no cross-tab lock.

export type Session = { token: string; refresh_token: string; user?: unknown };

export type Tokens = {
  getAccess: () => string;
  getRefresh: () => string | null;
  save: (session: Session) => void;
  clear: () => void;
};

export type RequestOptions = { method?: string; body?: unknown; auth?: boolean; signal?: AbortSignal };

/** How long a refresh may take before it is given up. */
export const REFRESH_TIMEOUT_MS = 15000;

const MSG_NETWORK = 'Cannot reach the server. Check your connection and try again.';
const MSG_SERVER = 'Something went wrong on the server. Try again in a moment.';
const MSG_BUSY = 'The server is busy. Try again in a moment.';
const MSG_SESSION_ENDED = 'Your session has ended. Sign in again.';

/** A failed request. status is 0 when the server could not be reached. */
export class ApiError extends Error {
  status: number;
  /** Seconds the server asked to wait, from Retry-After. */
  retryAfter: number | null;

  constructor(status: number, message: string, retryAfter: number | null = null) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.retryAfter = retryAfter;
  }
}

function retryAfterOf(res: Response): number | null {
  const seconds = Number.parseInt(res.headers.get('Retry-After') ?? '', 10);
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : null;
}

/**
 * Turns a failed response into an error with a message a person can act on. The API's
 * own messages for a refused request are written for people, so they pass through;
 * server errors keep their details to the logs.
 */
async function errorOf(res: Response): Promise<ApiError> {
  const raw = (await res.json().catch(() => null)) as { error?: unknown } | null;
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

function isAbort(err: unknown): boolean {
  return err instanceof Error && err.name === 'AbortError';
}

export type ClientOptions = {
  baseUrl: string;
  fetch: typeof globalThis.fetch;
  tokens: Tokens;
  onSessionEnd: () => void;
};

export function createClient({ baseUrl, fetch, tokens, onSessionEnd }: ClientOptions) {
  const base = baseUrl.replace(/\/+$/, '');
  let refreshing: Promise<void> | null = null;

  async function send(path: string, { method = 'GET', body, signal }: RequestOptions, access: string) {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (access) headers.Authorization = `Bearer ${access}`;
    try {
      return await fetch(base + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), signal });
    } catch (err) {
      if (isAbort(err)) throw err;
      throw new ApiError(0, MSG_NETWORK);
    }
  }

  function endSession(): ApiError {
    tokens.clear();
    onSessionEnd();
    return new ApiError(401, MSG_SESSION_ENDED);
  }

  /**
   * Trades the refresh token for a new pair. A refused token ends the session; a
   * failure that says nothing about the token, such as a lost connection, a timeout, or
   * a rate limit, keeps it for the next try.
   */
  async function rotate() {
    const refreshToken = tokens.getRefresh();
    if (!refreshToken) throw endSession();
    // The timer covers the whole exchange, body included.
    const abort = new AbortController();
    const timer = setTimeout(() => abort.abort(), REFRESH_TIMEOUT_MS);
    try {
      const res = await send('/auth/refresh', { method: 'POST', body: { refresh_token: refreshToken }, signal: abort.signal }, '');
      if (res.status === 401 || res.status === 400) throw endSession();
      if (!res.ok) throw await errorOf(res);
      const raw = (await res.json().catch(() => null)) as { data?: Session } | null;
      // A body that stalls past the timeout reads as null, so say what went wrong.
      if (abort.signal.aborted) throw new ApiError(0, MSG_NETWORK);
      if (!raw?.data?.token || !raw.data.refresh_token) throw new ApiError(res.status, MSG_SERVER);
      // Signing out, or signing in as someone else, while this was in flight replaced
      // the token it started with; its answer belongs to a session that is gone.
      if (tokens.getRefresh() !== refreshToken) throw new ApiError(401, MSG_SESSION_ENDED);
      tokens.save(raw.data);
    } catch (err) {
      if (abort.signal.aborted && isAbort(err)) throw new ApiError(0, MSG_NETWORK);
      throw err;
    } finally {
      clearTimeout(timer);
    }
  }

  /** Every caller waiting at once shares one rotation, so the token rotates once. */
  function refresh(): Promise<void> {
    refreshing ??= rotate().finally(() => {
      refreshing = null;
    });
    return refreshing;
  }

  /**
   * Sends a request and answers its data, or null for 204. A private request refused
   * with 401 is retried once with a fresh token: right away if another request has
   * already refreshed it meanwhile, otherwise after one shared refresh.
   */
  async function request<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
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
    if (res.status === 204) return null as T;
    const raw = (await res.json().catch(() => null)) as { data?: T } | null;
    return (raw?.data ?? null) as T;
  }

  return { request, refresh };
}

export type Client = ReturnType<typeof createClient>;
