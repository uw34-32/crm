import { describe, expect, it, vi } from 'vitest';
import { markWon, wonStageId } from './won.js';

const pipeline = { stages: [{ id: 's1', kind: 'open' }, { id: 's2', kind: 'open' }, { id: 'w', kind: 'won' }, { id: 'l', kind: 'lost' }] };

describe('finding the won stage', () => {
  it("returns the pipeline's won stage", async () => {
    const pipelines = { get: vi.fn(async () => pipeline) };
    expect(await wonStageId(pipelines, 'ws', 'p1')).toBe('w');
    expect(pipelines.get).toHaveBeenCalledWith('ws', 'p1');
  });

  it('fails when the pipeline has no won stage', async () => {
    await expect(wonStageId({ get: async () => ({ stages: [{ id: 's1', kind: 'open' }] }) }, 'ws', 'p1')).rejects.toThrow('no won stage');
  });

  it('passes on a failed lookup', async () => {
    const down = new Error('down');
    await expect(wonStageId({ get: async () => Promise.reject(down) }, 'ws', 'p1')).rejects.toBe(down);
  });
});

describe('recording a closed sale', () => {
  it('moves the deal to the given stage', async () => {
    const deals = { move: vi.fn(async () => ({ id: 'd1' })) };
    expect(await markWon(deals, 'ws', 'd1', 'w')).toEqual({ id: 'd1' });
    expect(deals.move).toHaveBeenCalledWith('ws', 'd1', { stage_id: 'w' });
  });
});
