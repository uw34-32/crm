import { createTokenStore, REFRESH_TOKEN_KEY } from '../src/api/tokens';
import { failingStorage, memoryStorage } from '../test-support/helpers';

describe('token store', () => {
  it('keeps the access token in memory and the refresh token in storage', async () => {
    const storage = memoryStorage();
    const tokens = createTokenStore(storage);
    tokens.save({ token: 'a', refresh_token: 'r' });
    await tokens.settled();
    expect(tokens.getAccess()).toBe('a');
    expect(storage.data.get(REFRESH_TOKEN_KEY)).toBe('r');
    expect([...storage.data.values()]).not.toContain('a');
  });

  it('loads the saved refresh token', async () => {
    const tokens = createTokenStore(memoryStorage({ [REFRESH_TOKEN_KEY]: 'saved' }));
    expect(tokens.getRefresh()).toBeNull();
    await tokens.load();
    expect(tokens.getRefresh()).toBe('saved');
  });

  it('is not overtaken: a clear right after a save leaves nothing stored', async () => {
    const storage = memoryStorage();
    const tokens = createTokenStore(storage);
    tokens.save({ token: 'a', refresh_token: 'r' });
    tokens.clear();
    await tokens.settled();
    expect(storage.data.has(REFRESH_TOKEN_KEY)).toBe(false);
    expect(tokens.getRefresh()).toBeNull();
  });

  it('works from memory when storage throws', async () => {
    const tokens = createTokenStore(failingStorage);
    await tokens.load();
    tokens.save({ token: 'a', refresh_token: 'r' });
    await tokens.settled();
    expect(tokens.getRefresh()).toBe('r');
    tokens.clear();
    await tokens.settled();
    expect(tokens.getRefresh()).toBeNull();
  });
});
