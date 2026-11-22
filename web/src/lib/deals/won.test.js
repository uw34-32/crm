import { describe, expect, it, vi } from 'vitest';
import { markWon } from './won.js';

const pipeline = { stages: [{ id: 's1', kind: 'open' }, { id: 's2', kind: 'open' }, { id: 'w', kind: 'won' }, { id: 'l', kind: 'lost' }] };

describe('recording a closed sale', () => {
  it('moves the deal to the pipeline\'s won stage', async () => {
    const deals = { move: vi.fn(async () => ({ id: 'd1', stage_id: 'w' })) };
    const pipelines = { get: vi.fn(async () => pipeline) };
    expect(await markWon(deals, pipelines, 'ws', 'd1', 'p1')).toEqual({ id: 'd1', stage_id: 'w' });
    expect(pipelines.get).toHaveBeenCalledWith('ws', 'p1');
    expect(deals.move).toHaveBeenCalledWith('ws', 'd1', { stage_id: 'w' });
  });

  it('fails without moving when the pipeline has no won stage', async () => {
    const deals = { move: vi.fn() };
    const pipelines = { get: vi.fn(async () => ({ stages: [{ id: 's1', kind: 'open' }] })) };
    await expect(markWon(deals, pipelines, 'ws', 'd1', 'p1')).rejects.toThrow('no won stage');
    expect(deals.move).not.toHaveBeenCalled();
  });

  it('passes on a failed lookup or move', async () => {
    const down = new Error('down');
    await expect(markWon({ move: vi.fn() }, { get: vi.fn(async () => Promise.reject(down)) }, 'ws', 'd1', 'p1')).rejects.toBe(down);
    await expect(markWon({ move: vi.fn(async () => Promise.reject(down)) }, { get: vi.fn(async () => pipeline) }, 'ws', 'd1', 'p1')).rejects.toBe(down);
  });
});
