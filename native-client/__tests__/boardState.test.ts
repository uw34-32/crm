import { ApiError } from '../src/api/client';
import { createBoard } from '../src/lib/deals/boardState';
import type { Deal, Stage } from '../src/lib/deals/board';

const stages: Stage[] = [
  { id: 's1', name: 'Lead', position: 0, kind: 'open', color: '#000' },
  { id: 's2', name: 'Qualified', position: 1, kind: 'open', color: '#000' },
  { id: 'won', name: 'Won', position: 2, kind: 'won', color: '#000' },
  { id: 'lost', name: 'Lost', position: 3, kind: 'lost', color: '#000' }
];
const deal = (id: string, stage_id: string, position: number): Deal => ({ id, stage_id, position, title: id, value_minor: 100, currency: 'EUR' });
const page = (items: Deal[], has_more = false) => ({ items, limit: 200, offset: 0, has_more });

function setup(overrides: { list?: jest.Mock; move?: jest.Mock } = {}) {
  const pipelines = {
    list: jest.fn(async () => ({ items: [{ id: 'p1', name: 'Sales' }, { id: 'p2', name: 'Other' }], limit: 200, offset: 0, has_more: false })),
    get: jest.fn(async (_w: string, id: string) => ({ id, name: id, stages }))
  };
  const list =
    overrides.list ??
    jest.fn(async (_w: string, q: { status?: string }) =>
      q.status === 'open' ? page([deal('a', 's1', 0), deal('b', 's1', 1), deal('c', 's2', 0)]) : q.status === 'won' ? page([deal('w', 'won', 0)], true) : page([])
    );
  const move = overrides.move ?? jest.fn(async (_w: string, id: string, body: { stage_id: string }) => deal(id, body.stage_id, 0));
  const board = createBoard({ pipelines: pipelines as never, deals: { list, move } as never });
  return { board, pipelines, list, move };
}

describe('loading', () => {
  it('opens the first pipeline with its columns, and the one asked for when it exists', async () => {
    const { board, pipelines } = setup();
    await board.load('w1');
    expect(board.store.get()).toMatchObject({ status: 'ready', pipeline: { id: 'p1' } });
    expect(board.store.get().columns.map((c) => c.deals.map((d) => d.id))).toEqual([['a', 'b'], ['c'], ['w'], []]);
    expect(board.store.get().columns[2].hasMore).toBe(true);
    await board.load('w1', 'p2');
    expect(pipelines.get).toHaveBeenLastCalledWith('w1', 'p2');
  });

  it('is ready and empty when the workspace has no pipeline', async () => {
    const { board, pipelines } = setup();
    pipelines.list.mockResolvedValueOnce({ items: [], limit: 200, offset: 0, has_more: false });
    await board.load('w1');
    expect(board.store.get()).toMatchObject({ status: 'ready', pipeline: null, columns: [] });
  });

  it('reports a failed load, and drops an answer that a newer load overtook', async () => {
    const { board, pipelines } = setup();
    pipelines.list.mockRejectedValueOnce(new ApiError(0, 'Cannot reach the server.'));
    await board.load('w1');
    expect(board.store.get()).toMatchObject({ status: 'error', error: 'Cannot reach the server.' });
    const slow = board.load('w1');
    board.stop();
    await slow;
    expect(board.store.get().status).toBe('loading');
  });
});

describe('moving', () => {
  it('shows the move at once and keeps the answer', async () => {
    const { board, move } = setup();
    await board.load('w1');
    const moving = board.move('a', 's2', null);
    expect(board.store.get().columns[1].deals.map((d) => d.id)).toEqual(['c', 'a']);
    expect(board.store.get().busy).toEqual({ a: true });
    await moving;
    expect(move).toHaveBeenCalledWith('w1', 'a', { stage_id: 's2', position: 1 });
    expect(board.store.get().busy).toEqual({});
  });

  it('sends the lost reason', async () => {
    const { board, move } = setup();
    await board.load('w1');
    await board.move('a', 'lost', null, 'Too pricey');
    expect(move).toHaveBeenCalledWith('w1', 'a', { stage_id: 'lost', position: 0, lost_reason: 'Too pricey' });
  });

  it('puts the deal back and says why when the API refuses', async () => {
    const { board } = setup({ move: jest.fn().mockRejectedValue(new ApiError(409, 'a lost reason is required')) });
    await board.load('w1');
    await board.move('a', 's2', null);
    expect(board.store.get().columns[0].deals.map((d) => d.id)).toEqual(['a', 'b']);
    expect(board.store.get().notice).toBe('The move was undone. A lost reason is required.');
  });

  it('reloads the board when the deal or the stage is gone', async () => {
    const { board, pipelines } = setup({ move: jest.fn().mockRejectedValue(new ApiError(404, 'not found')) });
    await board.load('w1');
    await board.move('a', 's2', null);
    await Promise.resolve();
    expect(pipelines.list).toHaveBeenCalledTimes(2);
  });

  it('cannot move a deal that is already moving', async () => {
    let finish: (d: Deal) => void = () => {};
    const { board, move } = setup({ move: jest.fn(() => new Promise<Deal>((resolve) => (finish = resolve))) });
    await board.load('w1');
    const first = board.move('a', 's2', null);
    await board.move('a', 'won', null);
    finish(deal('a', 's2', 1));
    await first;
    expect(move).toHaveBeenCalledTimes(1);
  });
});

describe('closed deals and forgetting', () => {
  it('loads the next page of won deals into their column', async () => {
    const { board, list } = setup();
    await board.load('w1');
    list.mockResolvedValueOnce(page([deal('w2', 'won', 1)], false));
    await board.loadMore('won');
    expect(list).toHaveBeenLastCalledWith('w1', { pipeline_id: 'p1', status: 'won', limit: 50, offset: 1 });
    expect(board.store.get().columns[2]).toMatchObject({ hasMore: false });
    expect(board.store.get().columns[2].deals.map((d) => d.id)).toEqual(['w', 'w2']);
  });

  it('reports a failed page without losing the ones shown', async () => {
    const { board, list } = setup();
    await board.load('w1');
    list.mockRejectedValueOnce(new Error('down'));
    await board.loadMore('won');
    expect(board.store.get().closed.won).toMatchObject({ error: 'down', loading: false, hasMore: true });
    expect(board.store.get().columns[2].deals).toHaveLength(1);
  });

  it('takes a deleted deal off the board', async () => {
    const { board } = setup();
    await board.load('w1');
    board.forget('a');
    expect(board.store.get().columns[0].deals.map((d) => d.id)).toEqual(['b']);
  });
});

describe('quiet reload', () => {
  it('keeps the board on screen while it loads, and when it fails', async () => {
    const { board, pipelines } = setup();
    await board.load('w1');
    const seen: string[] = [];
    board.store.subscribe(() => seen.push(board.store.get().status));
    await board.load('w1', 'p1', { quiet: true });
    expect(seen.every((status) => status === 'ready')).toBe(true);
    pipelines.list.mockRejectedValueOnce(new Error('down'));
    await board.load('w1', 'p1', { quiet: true });
    expect(board.store.get()).toMatchObject({ status: 'ready', error: '' });
    expect(board.store.get().columns[0].deals).toHaveLength(2);
  });
});
