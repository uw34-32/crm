// A sale that is already closed is recorded as a deal moved to the Won stage. The API
// starts every new deal in an open stage, so recording one takes a create and then a move.

import type { Pipeline } from '../../api/crm';
import type { Deal } from './board';
import { t } from '../../i18n';

type Deals = { move: (workspaceId: string, id: string, body: { stage_id: string }) => Promise<Deal> };
type Pipelines = { get: (workspaceId: string, id: string) => Promise<Pipeline> };

/** Moves a new deal to the first won stage of its pipeline, and returns the deal as the move returned it. */
export async function markWon(deals: Deals, pipelines: Pipelines, workspaceId: string, dealId: string, pipelineId: string): Promise<Deal> {
  const pipeline = await pipelines.get(workspaceId, pipelineId);
  const won = (pipeline.stages ?? []).find((stage) => stage.kind === 'won');
  if (!won) throw new Error(t('This pipeline has no won stage.'));
  return deals.move(workspaceId, dealId, { stage_id: won.id });
}
