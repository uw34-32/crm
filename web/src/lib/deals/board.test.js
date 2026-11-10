import { describe, expect, it } from 'vitest';
import { buildColumns, needsLostReason, planMove, removeDeal, replaceDeal, stageTotals, undoMove, appendDeals } from './board.js';

/** @type {import('./board.js').Stage[]} */
const stages = [
  { id: 's2', name: 'Qualified', position: 1, kind: 'open', color: '#0a84ff' },
  { id: 's1', name: 'Lead', position: 0, kind: 'open', color: '#64748b' },
  { id: 'won', name: 'Won', position: 2, kind: 'won', color: '#30d158' },
  { id: 'lost', name: 'Lost', position: 3, kind: 'lost', color: '#ff453a' }
];

/** @param {string} id @param {string} stage_id @param {number} position @param {Partial<any>} [rest] */
const deal = (id, stage_id, position, rest = {}) => ({ id, stage_id, position, title: id, value_minor: 100, currency: 'EUR', ...rest });

const deals = [deal('b', 's1', 1), deal('a', 's1', 0), deal('c', 's2', 0, { value_minor: 250 }), deal('w', 'won', 0, { currency: 'USD' })];

describe('building the board', () => {
  it('makes one column per stage in stage order, with its deals in position order', () => {
    const columns = buildColumns(stages, deals);
    expect(columns.map((column) => column.stage.id)).toEqual(['s1', 's2', 'won', 'lost']);
    expect(columns[0].deals.map((d) => d.id)).toEqual(['a', 'b']);
    expect(columns[1].deals.map((d) => d.id)).toEqual(['c']);
    expect(columns[3].deals).toEqual([]);
  });

  it('drops a deal whose stage is not on the board, such as one just deleted', () => {
    const columns = buildColumns(stages, [deal('x', 'gone', 0)]);
    expect(columns.every((column) => column.deals.length === 0)).toBe(true);
  });

  it('remembers when a column has more deals than were loaded', () => {
    const columns = buildColumns(stages, deals, { hasMore: { won: true } });
    expect(columns.map((column) => column.hasMore)).toEqual([false, false, true, false]);
  });
});

describe('column totals', () => {
  it('sums minor units per currency, largest first', () => {
    expect(stageTotals([deal('a', 's1', 0), deal('b', 's1', 1, { value_minor: 50, currency: 'USD' }), deal('c', 's1', 2, { value_minor: 300 })])).toEqual([
      { currency: 'EUR', minor: 400 },
      { currency: 'USD', minor: 50 }
    ]);
  });

  it('is empty for an empty column', () => {
    expect(stageTotals([])).toEqual([]);
  });
});

describe('planning a move', () => {
  const columns = buildColumns(stages, deals);

  it('moves a deal to another stage before a given deal, with the dense position there', () => {
    const plan = planMove(columns, 'c', 's1', 'b');
    expect(plan?.body).toEqual({ stage_id: 's1', position: 1 });
    expect(plan?.columns[0].deals.map((d) => d.id)).toEqual(['a', 'c', 'b']);
    expect(plan?.columns[1].deals).toEqual([]);
    expect(plan?.columns[0].deals.map((d) => d.position)).toEqual([0, 1, 2]);
  });

  it('appends to the end of a stage when no deal follows', () => {
    const plan = planMove(columns, 'a', 's2', null);
    expect(plan?.body).toEqual({ stage_id: 's2', position: 1 });
    expect(plan?.columns[1].deals.map((d) => d.id)).toEqual(['c', 'a']);
  });

  it('reorders within a stage, counting positions after the deal has left its old place', () => {
    expect(planMove(columns, 'a', 's1', null)?.body).toEqual({ stage_id: 's1', position: 1 });
    const plan = planMove(columns, 'b', 's1', 'a');
    expect(plan?.body).toEqual({ stage_id: 's1', position: 0 });
    expect(plan?.columns[0].deals.map((d) => d.id)).toEqual(['b', 'a']);
  });

  it('is null when the deal would land where it already is', () => {
    expect(planMove(columns, 'a', 's1', 'b')).toBeNull();
    expect(planMove(columns, 'b', 's1', null)).toBeNull();
    expect(planMove(columns, 'c', 's2', 'c')).toBeNull();
  });

  it('sends no position when appending to a column whose later pages are not loaded, so the API puts it last', () => {
    const partial = buildColumns(stages, deals, { hasMore: { won: true } });
    expect(planMove(partial, 'a', 'won', null)?.body).toEqual({ stage_id: 'won' });
  });

  it('is null for a deal or a stage that is not on the board', () => {
    expect(planMove(columns, 'nope', 's1', null)).toBeNull();
    expect(planMove(columns, 'a', 'nope', null)).toBeNull();
  });

  it('leaves the columns it was given untouched', () => {
    planMove(columns, 'c', 's1', 'b');
    expect(columns[1].deals.map((d) => d.id)).toEqual(['c']);
  });
});

describe('after the API answers', () => {
  it('replaces the moved deal with what the API returned, in its place', () => {
    const columns = buildColumns(stages, deals);
    const updated = replaceDeal(columns, { ...deal('a', 's1', 0), title: 'A renamed' });
    expect(updated[0].deals[0].title).toBe('A renamed');
    expect(columns[0].deals[0].title).toBe('a');
  });

  it('removes a deleted deal and closes the gap', () => {
    const columns = removeDeal(buildColumns(stages, deals), 'a');
    expect(columns[0].deals.map((d) => [d.id, d.position])).toEqual([['b', 0]]);
  });
});

describe('asking for a lost reason', () => {
  it('asks when a deal enters a lost stage, and not when it moves within one', () => {
    const lost = stages[3];
    expect(needsLostReason(deal('a', 's1', 0), lost)).toBe(true);
    expect(needsLostReason(deal('a', 'lost', 0), lost)).toBe(false);
    expect(needsLostReason(deal('a', 's1', 0), stages[2])).toBe(false);
  });
});

describe('undoing a refused move', () => {
  it('puts the deal back where it came from, whatever moved meanwhile', () => {
    const columns = buildColumns(stages, deals);
    const plan = planMove(columns, 'a', 's2', null);
    if (!plan) throw new Error('expected a plan');
    const undone = undoMove(plan.columns, 'a', plan.from);
    expect(undone[0].deals.map((d) => [d.id, d.stage_id, d.position])).toEqual([['a', 's1', 0], ['b', 's1', 1]]);
    expect(undone[1].deals.map((d) => d.id)).toEqual(['c']);
  });

  it('clamps to the end when the old stage has fewer deals than before', () => {
    const columns = buildColumns(stages, deals);
    const plan = planMove(columns, 'b', 's2', null);
    if (!plan) throw new Error('expected a plan');
    const shrunk = removeDeal(plan.columns, 'a');
    expect(undoMove(shrunk, 'b', plan.from)[0].deals.map((d) => [d.id, d.position])).toEqual([['b', 0]]);
  });

  it('leaves the board alone when the deal or its old stage is gone', () => {
    const columns = buildColumns(stages, deals);
    expect(undoMove(columns, 'nope', { stageId: 's1', index: 0 })).toBe(columns);
    expect(undoMove(columns, 'a', { stageId: 'gone', index: 0 })).toBe(columns);
  });
});

describe('appending a later page of closed deals', () => {
  it('adds deals to their columns in position order, skips ones already shown, and updates what has more', () => {
    const columns = buildColumns(stages, deals, { hasMore: { won: true } });
    const next = appendDeals(columns, [deal('w2', 'won', 2), deal('w', 'won', 0), deal('w1', 'won', 1)], { won: false });
    expect(next[2].deals.map((d) => [d.id, d.position])).toEqual([['w', 0], ['w1', 1], ['w2', 2]]);
    expect(next[2].hasMore).toBe(false);
    expect(columns[2].deals.length).toBe(1);
  });
});
