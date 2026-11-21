import { resetWorkspacesOnSignOut } from '../src/bind';
import { createSession } from '../src/session';
import { createWorkspaces } from '../src/workspaces';
import { BASE, memoryStorage, reply } from '../test-support/helpers';

const user = { id: 'u1', email: 'a@b.co', name: 'Ada' };
const pair = { token: 'a', refresh_token: 'r', user };

function setup(fetch: jest.Mock) {
  const session = createSession({ baseUrl: BASE, fetch, storage: memoryStorage() });
  const workspaces = createWorkspaces({ api: { request: jest.fn(async () => []) as never }, storage: memoryStorage() });
  resetWorkspacesOnSignOut(session, workspaces);
  return { session, workspaces };
}

describe('resetWorkspacesOnSignOut', () => {
  it('forgets the workspaces on sign-out, so the same user reloads them on the next sign-in', async () => {
    const { session, workspaces } = setup(jest.fn(async () => reply(200, { data: pair })));
    await session.signIn({ email: 'a@b.co', password: 'x' });
    await workspaces.load('u1');
    expect(workspaces.state.get().status).toBe('ready');
    await session.signOut();
    expect(workspaces.state.get()).toMatchObject({ status: 'idle', list: [], userId: null });
  });

  it('forgets them when the API ends the session', async () => {
    const fetch = jest.fn(async (url: string) => (url.endsWith('/auth/login') ? reply(200, { data: pair }) : reply(401, {})));
    const { session, workspaces } = setup(fetch);
    await session.signIn({ email: 'a@b.co', password: 'x' });
    await workspaces.load('u1');
    await expect(session.api.request('/workspaces')).rejects.toMatchObject({ status: 401 });
    expect(workspaces.state.get().userId).toBeNull();
  });
});

describe('signing out during a refresh', () => {
  it('does not sign the user back in', async () => {
    let answer: (r: Response) => void = () => {};
    const fetch = jest
      .fn()
      .mockResolvedValueOnce(reply(200, { data: pair }))
      .mockResolvedValueOnce(reply(401, {}))
      .mockImplementationOnce(() => new Promise<Response>((resolve) => (answer = resolve)))
      .mockResolvedValue(reply(204));
    const { session } = setup(fetch);
    await session.signIn({ email: 'a@b.co', password: 'x' });
    const request = session.api.request('/workspaces').catch((e: unknown) => e);
    await new Promise((resolve) => setTimeout(resolve, 0));
    await session.signOut();
    answer(reply(200, { data: { token: 'a2', refresh_token: 'r2', user } }));
    await request;
    expect(session.state.get().user).toBeNull();
    expect(session.tokens.getRefresh()).toBeNull();
  });
});
