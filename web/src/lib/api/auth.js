/**
 * Signs out: revokes the refresh token's family on the API, then forgets the tokens.
 * The tokens are forgotten even when the API cannot be reached, because signing out
 * on this device must not depend on the network.
 * @param {{ request: (path: string, options: { method: string, body: unknown, auth: boolean }) => Promise<unknown> }} client
 * @param {{ getRefresh: () => string | null, clear: () => void }} tokens
 */
export async function logout(client, tokens) {
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
