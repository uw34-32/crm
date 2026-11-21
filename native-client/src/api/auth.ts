import type { Client, Tokens } from './client';

/**
 * Signs out: revokes the refresh token's family on the API, then forgets the tokens.
 * The tokens are forgotten even when the API cannot be reached, because signing out
 * on this device must not depend on the network.
 */
export async function logout(client: Pick<Client, 'request'>, tokens: Pick<Tokens, 'getRefresh' | 'clear'>) {
  const refreshToken = tokens.getRefresh();
  try {
    if (refreshToken) {
      await client.request('/auth/logout', { method: 'POST', body: { refresh_token: refreshToken }, auth: false });
    }
  } catch {
    // The token expires on its own; this device forgets it either way.
  } finally {
    tokens.clear();
  }
}
