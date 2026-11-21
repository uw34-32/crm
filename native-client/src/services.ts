// The app's one session and workspace service, wired to the device.

import { deviceStorage } from './lib/storage';
import { createSession } from './session';
import { createWorkspaces } from './workspaces';

/** The API's address. Android emulators reach the host at 10.0.2.2, not localhost. */
export const API_URL = process.env.EXPO_PUBLIC_API_URL || 'http://localhost:8085';

export const session = createSession({
  baseUrl: API_URL,
  fetch: (input, init) => fetch(input, init),
  storage: deviceStorage
});

export const workspaces = createWorkspaces({ api: session.api, storage: deviceStorage });
