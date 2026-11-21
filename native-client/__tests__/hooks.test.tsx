import { act, renderHook, waitFor } from '@testing-library/react-native';

const mockDismissTo = jest.fn();
jest.mock('expo-router', () => ({ router: { dismissTo: (...a: unknown[]) => mockDismissTo(...a) } }));

import { ApiError } from '../src/api/client';
import { recordsChanged } from '../src/lib/records/changes';
import { useLeaveOnWorkspaceSwitch, useRecord } from '../src/lib/records/hooks';
import { workspaces } from '../src/services';
import type { Workspace } from '../src/workspaces';

const ws = (id: string): Workspace => ({ id, name: id, role: 'owner', created_at: '', updated_at: '' });
const open = (id: string | null) => act(async () => workspaces.state.set({ current: id ? ws(id) : null }));

beforeEach(() => {
  mockDismissTo.mockClear();
  workspaces.state.set({ current: null });
});

describe('useLeaveOnWorkspaceSwitch', () => {
  it('leaves for the list when another workspace opens', async () => {
    await open('a');
    await renderHook(() => useLeaveOnWorkspaceSwitch('/contacts'));
    expect(mockDismissTo).not.toHaveBeenCalled();
    await open('b');
    expect(mockDismissTo).toHaveBeenCalledWith('/contacts');
  });

  it('counts the first workspace that arrives after the screen mounted', async () => {
    await renderHook(() => useLeaveOnWorkspaceSwitch('/companies'));
    await open('a');
    expect(mockDismissTo).not.toHaveBeenCalled();
    await open('b');
    expect(mockDismissTo).toHaveBeenCalledWith('/companies');
  });
});

describe('useRecord', () => {
  it('keeps the record while an edit reloads it, and not another record', async () => {
    await open('a');
    let release: (value: { id: string }) => void = () => {};
    const fetchOne = jest.fn((_w: string, id: string) => (id === 'x' ? Promise.resolve({ id: 'x' }) : new Promise<{ id: string }>((resolve) => (release = resolve))));
    const { result, rerender } = await renderHook(({ id }: { id: string }) => useRecord(fetchOne, id), { initialProps: { id: 'x' } });
    await waitFor(() => expect(result.current.state.status).toBe('ready'));

    await act(async () => recordsChanged());
    expect(result.current.state.status).toBe('ready');

    await rerender({ id: 'y' });
    expect(result.current.state).toMatchObject({ status: 'loading', record: null });
    await act(async () => release({ id: 'y' }));
    await waitFor(() => expect(result.current.state.record).toEqual({ id: 'y' }));
  });

  it('does not load again once paused, so a deleted record is not asked for', async () => {
    await open('a');
    const fetchOne = jest.fn(async () => ({ id: 'x' }));
    const { result, rerender } = await renderHook(({ paused }: { paused: boolean }) => useRecord(fetchOne, 'x', { paused }), { initialProps: { paused: false } });
    await waitFor(() => expect(result.current.state.status).toBe('ready'));
    await rerender({ paused: true });
    await act(async () => recordsChanged());
    expect(fetchOne).toHaveBeenCalledTimes(1);
  });

  it('reads a 404 as missing', async () => {
    await open('a');
    const { result } = await renderHook(() =>
      useRecord(async () => {
        throw new ApiError(404, 'not found');
      }, 'x')
    );
    await waitFor(() => expect(result.current.state.status).toBe('missing'));
  });
});
