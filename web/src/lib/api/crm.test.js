import { describe, expect, it, vi } from 'vitest';
import { activitiesApi, dealsApi, membersApi, pipelinesApi } from './crm.js';

function client() {
  return { request: vi.fn(async () => null) };
}

/** @param {{ request: import('vitest').Mock }} c */
const calls = (c) => c.request.mock.calls;

describe('pipelines API', () => {
  it('lists the first page of pipelines and reads one with its stages', async () => {
    const c = client();
    const pipelines = pipelinesApi(c);

    await pipelines.list('w 1');
    await pipelines.get('w1', 'p/1');

    expect(calls(c)).toEqual([['/workspaces/w%201/pipelines?limit=200&offset=0'], ['/workspaces/w1/pipelines/p%2F1']]);
  });
});

describe('deals API', () => {
  it('lists with limit and offset and only the filters that are set', async () => {
    const c = client();
    const deals = dealsApi(c);

    await deals.list('w1', { limit: 200, offset: 0 });
    await deals.list('w1', { pipeline_id: 'p1', status: 'open', stage_id: '', q: 'acme & co', limit: 50, offset: 100 });

    expect(calls(c).map((call) => call[0])).toEqual([
      '/workspaces/w1/deals?limit=200&offset=0',
      '/workspaces/w1/deals?limit=50&offset=100&pipeline_id=p1&status=open&q=acme+%26+co'
    ]);
  });

  it('reads, creates, updates, moves, and deletes one deal', async () => {
    const c = client();
    const deals = dealsApi(c);

    await deals.get('w1', 'd1');
    await deals.create('w1', { title: 'Acme renewal', currency: 'EUR', pipeline_id: 'p1' });
    await deals.update('w1', 'd1', { expected_close_date: null });
    await deals.move('w1', 'd1', { stage_id: 's2', position: 0 });
    await deals.remove('w1', 'd/1');

    expect(calls(c)).toEqual([
      ['/workspaces/w1/deals/d1'],
      ['/workspaces/w1/deals', { method: 'POST', body: { title: 'Acme renewal', currency: 'EUR', pipeline_id: 'p1' } }],
      ['/workspaces/w1/deals/d1', { method: 'PATCH', body: { expected_close_date: null } }],
      ['/workspaces/w1/deals/d1/move', { method: 'POST', body: { stage_id: 's2', position: 0 } }],
      ['/workspaces/w1/deals/d%2F1', { method: 'DELETE' }]
    ]);
  });
});

describe('activities API', () => {
  it('lists the timeline of one record, newest first as the API orders it', async () => {
    const c = client();
    const activities = activitiesApi(c);

    await activities.list('w1', { deal_id: 'd1', limit: 50, offset: 0 });
    await activities.list('w1', { contact_id: 'c1', limit: 50, offset: 50 });

    expect(calls(c).map((call) => call[0])).toEqual([
      '/workspaces/w1/activities?limit=50&offset=0&deal_id=d1',
      '/workspaces/w1/activities?limit=50&offset=50&contact_id=c1'
    ]);
  });

  it('creates, updates, completes, reopens, and deletes one activity', async () => {
    const c = client();
    const activities = activitiesApi(c);

    await activities.create('w1', { kind: 'note', subject: 'Called back', deal_id: 'd1' });
    await activities.update('w1', 'a1', { subject: 'Called twice' });
    await activities.complete('w1', 'a1');
    await activities.reopen('w1', 'a1');
    await activities.remove('w1', 'a1');

    expect(calls(c)).toEqual([
      ['/workspaces/w1/activities', { method: 'POST', body: { kind: 'note', subject: 'Called back', deal_id: 'd1' } }],
      ['/workspaces/w1/activities/a1', { method: 'PATCH', body: { subject: 'Called twice' } }],
      ['/workspaces/w1/activities/a1/complete', { method: 'POST' }],
      ['/workspaces/w1/activities/a1/reopen', { method: 'POST' }],
      ['/workspaces/w1/activities/a1', { method: 'DELETE' }]
    ]);
  });
});

describe('members API', () => {
  it('lists the members of a workspace', async () => {
    const c = client();
    await membersApi(c).list('w1');
    expect(calls(c)).toEqual([['/workspaces/w1/members']]);
  });
});
