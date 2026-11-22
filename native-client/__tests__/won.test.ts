import type { Pipeline } from '../src/api/crm';
import type { Deal } from '../src/lib/deals/board';
import { markWon } from '../src/lib/deals/won';

const pipeline = { id: 'p1', name: 'Sales', stages: [{ id: 's1', kind: 'open' }, { id: 's2', kind: 'open' }, { id: 'w', kind: 'won' }, { id: 'l', kind: 'lost' }] } as unknown as Pipeline;

describe('recording a closed sale', () => {
  it("moves the deal to the pipeline's won stage", async () => {
    const moved = { id: 'd1', stage_id: 'w' } as unknown as Deal;
    const deals = { move: jest.fn(async () => moved) };
    const pipelines = { get: jest.fn(async () => pipeline) };
    expect(await markWon(deals, pipelines, 'ws', 'd1', 'p1')).toBe(moved);
    expect(pipelines.get).toHaveBeenCalledWith('ws', 'p1');
    expect(deals.move).toHaveBeenCalledWith('ws', 'd1', { stage_id: 'w' });
  });

  it('fails without moving when the pipeline has no won stage', async () => {
    const deals = { move: jest.fn() };
    const pipelines = { get: jest.fn(async () => ({ id: 'p1', name: 'Sales', stages: [{ id: 's1', kind: 'open' }] }) as unknown as Pipeline) };
    await expect(markWon(deals, pipelines, 'ws', 'd1', 'p1')).rejects.toThrow('no won stage');
    expect(deals.move).not.toHaveBeenCalled();
  });

  it('passes on a failed lookup or move', async () => {
    const down = new Error('down');
    await expect(markWon({ move: jest.fn() }, { get: jest.fn(async () => Promise.reject(down)) }, 'ws', 'd1', 'p1')).rejects.toBe(down);
    await expect(markWon({ move: jest.fn(async () => Promise.reject(down)) }, { get: jest.fn(async () => pipeline) }, 'ws', 'd1', 'p1')).rejects.toBe(down);
  });
});
