// A sale that is already closed is recorded as a deal moved to the Won stage. The API
// starts every new deal in an open stage, so recording one takes a create and then a move.
// The stage is looked up before the create, so a pipeline that cannot close a sale creates nothing.

import type { Pipeline } from '../../api/crm';
import { t } from '../../i18n';
import type { Deal } from './board';

type Deals = { move: (workspaceId: string, id: string, body: { stage_id: string }) => Promise<Deal> };
type Pipelines = { get: (workspaceId: string, id: string) => Promise<Pipeline> };

/** The id of the first won stage of a pipeline. */
export async function wonStageId(pipelines: Pipelines, workspaceId: string, pipelineId: string): Promise<string> {
  const pipeline = await pipelines.get(workspaceId, pipelineId);
  const won = (pipeline.stages ?? []).find((stage) => stage.kind === 'won');
  if (!won) throw new Error(t('This pipeline has no won stage.'));
  return won.id;
}

/** Moves a new deal to a won stage. */
export function markWon(deals: Deals, workspaceId: string, dealId: string, stageId: string): Promise<Deal> {
  return deals.move(workspaceId, dealId, { stage_id: stageId });
}
