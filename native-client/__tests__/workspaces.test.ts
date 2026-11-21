import { createWorkspaces, pickWorkspace, type Workspace } from '../src/workspaces';
import { failingStorage, memoryStorage } from '../test-support/helpers';

const ws = (id: string, name = id): Workspace => ({ id, name, role: 'owner', created_at: '', updated_at: '' });

describe('pickWorkspace', () => {
  it('prefers the saved one, then the first, then none', () => {
    expect(pickWorkspace([ws('a'), ws('b')], 'b')?.id).toBe('b');
    expect(pickWorkspace([ws('a'), ws('b')], 'gone')?.id).toBe('a');
    expect(pickWorkspace([], null)).toBeNull();
  });
});

describe('workspaces service', () => {
  it('loads the list and opens the saved workspace of this user', async () => {
    const storage = memoryStorage({ 'crm.workspace.u1': 'b' });
    const service = createWorkspaces({ api: { request: jest.fn(async () => [ws('a'), ws('b')]) as never }, storage });
    await service.load('u1');
    expect(service.state.get()).toMatchObject({ status: 'ready', current: { id: 'b' }, userId: 'u1' });
  });

  it('does not use another user\'s saved choice', async () => {
    const storage = memoryStorage({ 'crm.workspace.u1': 'b' });
    const service = createWorkspaces({ api: { request: jest.fn(async () => [ws('a'), ws('b')]) as never }, storage });
    await service.load('u2');
    expect(service.state.get().current?.id).toBe('a');
  });

  it('saves the selection per user', async () => {
    const storage = memoryStorage();
    const service = createWorkspaces({ api: { request: jest.fn(async () => [ws('a'), ws('b')]) as never }, storage });
    await service.load('u1');
    service.select('b');
    await Promise.resolve();
    expect(storage.data.get('crm.workspace.u1')).toBe('b');
  });

  it('reports a failed load and can retry', async () => {
    const request = jest.fn().mockRejectedValueOnce(new Error('Cannot reach the server.')).mockResolvedValueOnce([ws('a')]);
    const service = createWorkspaces({ api: { request: request as never }, storage: memoryStorage() });
    await service.load('u1');
    expect(service.state.get()).toMatchObject({ status: 'error', error: 'Cannot reach the server.' });
    await service.load('u1');
    expect(service.state.get().status).toBe('ready');
  });

  it('ignores a load that finishes after the user changed', async () => {
    let finish: (list: Workspace[]) => void = () => {};
    const request = jest.fn(() => new Promise<Workspace[]>((resolve) => (finish = resolve)));
    const service = createWorkspaces({ api: { request: request as never }, storage: memoryStorage() });
    const slow = service.load('u1');
    service.reset();
    finish([ws('a')]);
    await slow;
    expect(service.state.get()).toMatchObject({ list: [], current: null, userId: null });
  });

  it('creates a workspace and opens it', async () => {
    const created = ws('new', 'Sales');
    const request = jest.fn().mockResolvedValueOnce([]).mockResolvedValueOnce(created);
    const service = createWorkspaces({ api: { request: request as never }, storage: memoryStorage() });
    await service.load('u1');
    await service.create('Sales');
    expect(service.state.get()).toMatchObject({ list: [created], current: created });
    expect(request).toHaveBeenLastCalledWith('/workspaces', { method: 'POST', body: { name: 'Sales' } });
  });

  it('keeps working in memory when storage throws', async () => {
    const service = createWorkspaces({ api: { request: jest.fn(async () => [ws('a'), ws('b')]) as never }, storage: failingStorage });
    await service.load('u1');
    service.select('b');
    expect(service.state.get().current?.id).toBe('b');
  });
});
