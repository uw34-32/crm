// Ties the workspaces to the session: whoever signs out, or whatever ends the session,
// the next user starts with no workspaces of the last. A component cannot do this, because
// the screens that would are unmounted when the user goes.

import type { SessionService } from './session';
import type { WorkspacesService } from './workspaces';

/** Returns a function that stops listening. */
export function resetWorkspacesOnSignOut(session: Pick<SessionService, 'state'>, workspaces: Pick<WorkspacesService, 'state' | 'reset'>) {
  return session.state.subscribe(() => {
    if (session.state.get().user === null && workspaces.state.get().userId !== null) workspaces.reset();
  });
}
