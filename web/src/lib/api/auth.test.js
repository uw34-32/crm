import { describe, expect, it, vi } from 'vitest';
import { logout } from './auth.js';
import { ApiError } from './client.js';

function tokensWith(/** @type {string | null} */ refresh) {
  return { getRefresh: () => refresh, clear: vi.fn() };
}

describe('logout', () => {
  it('revokes the refresh token on the API, then clears the session', async () => {
    const request = vi.fn(async () => null);
    const tokens = tokensWith('refresh-1');

    await logout({ request }, tokens);

    expect(request).toHaveBeenCalledWith('/auth/logout', { method: 'POST', body: { refresh_token: 'refresh-1' }, auth: false });
    expect(tokens.clear).toHaveBeenCalledTimes(1);
  });

  it('clears the session even when the API call fails', async () => {
    const request = vi.fn(async () => {
      throw new ApiError(0, 'Cannot reach the server.');
    });
    const tokens = tokensWith('refresh-1');

    await expect(logout({ request }, tokens)).resolves.toBeUndefined();
    expect(tokens.clear).toHaveBeenCalledTimes(1);
  });

  it('skips the API call when there is no refresh token', async () => {
    const request = vi.fn(async () => null);
    const tokens = tokensWith(null);

    await logout({ request }, tokens);

    expect(request).not.toHaveBeenCalled();
    expect(tokens.clear).toHaveBeenCalledTimes(1);
  });
});
