import { REFRESH_TOKEN_KEY } from '../src/api/tokens';
import { createSession } from '../src/session';
import { BASE, memoryStorage, reply } from '../test-support/helpers';

const user = { id: 'u1', email: 'ada@example.com', name: 'Ada' };
const pair = { token: 'access-2', refresh_token: 'refresh-2', user };

function make(fetch: jest.Mock, storage = memoryStorage()) {
  return { storage, service: createSession({ baseUrl: BASE, fetch, storage }) };
}

describe('restore', () => {
  it('signs a returning user in with the saved refresh token', async () => {
    const fetch = jest.fn(async () => reply(200, { data: pair }));
    const { service, storage } = make(fetch, memoryStorage({ [REFRESH_TOKEN_KEY]: 'refresh-1' }));
    await service.restore();
    expect(service.state.get()).toMatchObject({ ready: true, user, restoreError: '' });
    await service.tokens.settled();
    expect(storage.data.get(REFRESH_TOKEN_KEY)).toBe('refresh-2');
  });

  it('is ready and signed out with nothing saved, without calling the API', async () => {
    const fetch = jest.fn();
    const { service } = make(fetch);
    await service.restore();
    expect(service.state.get()).toMatchObject({ ready: true, user: null });
    expect(fetch).not.toHaveBeenCalled();
  });

  it('keeps the token and reports the error when the server is unreachable', async () => {
    const fetch = jest.fn(async () => Promise.reject(new TypeError('offline')));
    const { service } = make(fetch, memoryStorage({ [REFRESH_TOKEN_KEY]: 'refresh-1' }));
    await service.restore();
    expect(service.state.get().restoreError).toMatch(/Cannot reach the server/);
    expect(service.tokens.getRefresh()).toBe('refresh-1');
  });

  it('leaves the user signed out, without an error screen, when the token is refused', async () => {
    const fetch = jest.fn(async () => reply(401, { error: 'invalid' }));
    const { service, storage } = make(fetch, memoryStorage({ [REFRESH_TOKEN_KEY]: 'refresh-1' }));
    await service.restore();
    await service.tokens.settled();
    expect(service.state.get()).toMatchObject({ ready: true, user: null, restoreError: '' });
    expect(storage.data.has(REFRESH_TOKEN_KEY)).toBe(false);
  });
});

describe('sign in, register, and sign out', () => {
  it('signs in and clears the ended flag', async () => {
    const fetch = jest.fn(async () => reply(200, { data: pair }));
    const { service } = make(fetch);
    service.state.set({ ended: true });
    await service.signIn({ email: 'ada@example.com', password: 'pw' });
    expect(service.state.get()).toMatchObject({ user, ended: false });
    expect((fetch.mock.calls[0] as unknown as [string])[0]).toBe(`${BASE}/auth/login`);
  });

  it('registers', async () => {
    const fetch = jest.fn(async () => reply(201, { data: pair }));
    const { service } = make(fetch);
    await service.register({ name: 'Ada', email: 'ada@example.com', password: 'longenough' });
    expect(service.state.get().user).toEqual(user);
    expect((fetch.mock.calls[0] as unknown as [string])[0]).toBe(`${BASE}/auth/register`);
  });

  it('stays signed out and rethrows when the API refuses', async () => {
    const { service } = make(jest.fn(async () => reply(401, { error: 'invalid credentials' })));
    await expect(service.signIn({ email: 'a@b.co', password: 'x' })).rejects.toMatchObject({ message: 'invalid credentials' });
    expect(service.state.get().user).toBeNull();
  });

  it('signs out even when the API cannot be reached', async () => {
    const fetch = jest.fn().mockResolvedValueOnce(reply(200, { data: pair })).mockRejectedValueOnce(new TypeError('offline'));
    const { service, storage } = make(fetch);
    await service.signIn({ email: 'a@b.co', password: 'x' });
    await service.signOut();
    await service.tokens.settled();
    expect(service.state.get().user).toBeNull();
    expect(storage.data.has(REFRESH_TOKEN_KEY)).toBe(false);
  });

  it('marks the session ended when the API ends it', async () => {
    const fetch = jest.fn(async (url: string) => (url.endsWith('/auth/login') ? reply(200, { data: pair }) : reply(401, {})));
    const { service } = make(fetch);
    await service.signIn({ email: 'a@b.co', password: 'x' });
    await expect(service.api.request('/workspaces')).rejects.toMatchObject({ status: 401 });
    expect(service.state.get()).toMatchObject({ user: null, ended: true });
  });
});
