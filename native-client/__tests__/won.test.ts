import type { Pipeline } from '../src/api/crm';
import type { Deal } from '../src/lib/deals/board';
import { markWon, wonStageId } from '../src/lib/deals/won';

const pipeline = { id: 'p1', name: 'Sales', stages: [{ id: 's1', kind: 'open' }, { id: 's2', kind: 'open' }, { id: 'w', kind: 'won' }, { id: 'l', kind: 'lost' }] } as unknown as Pipeline;

describe('finding the won stage', () => {
  it("returns the pipeline's won stage", async () => {
    const pipelines = { get: jest.fn(async () => pipeline) };
    expect(await wonStageId(pipelines, 'ws', 'p1')).toBe('w');
    expect(pipelines.get).toHaveBeenCalledWith('ws', 'p1');
  });

  it('fails when the pipeline has no won stage', async () => {
    const pipelines = { get: jest.fn(async () => ({ id: 'p1', name: 'Sales', stages: [{ id: 's1', kind: 'open' }] }) as unknown as Pipeline) };
    await expect(wonStageId(pipelines, 'ws', 'p1')).rejects.toThrow('no won stage');
  });

  it('passes on a failed lookup', async () => {
    const down = new Error('down');
    await expect(wonStageId({ get: jest.fn(async () => Promise.reject(down)) }, 'ws', 'p1')).rejects.toBe(down);
  });
});

describe('recording a closed sale', () => {
  it('moves the deal to the given stage', async () => {
    const moved = { id: 'd1' } as unknown as Deal;
    const deals = { move: jest.fn(async () => moved) };
    expect(await markWon(deals, 'ws', 'd1', 'w')).toBe(moved);
    expect(deals.move).toHaveBeenCalledWith('ws', 'd1', { stage_id: 'w' });
  });
});
