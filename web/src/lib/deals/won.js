// A sale that is already closed is recorded as a deal moved to the Won stage. The API
// starts every new deal in an open stage, so recording one takes a create and then a move.
// The stage is looked up before the create, so a pipeline that cannot close a sale creates nothing.

import { t } from '../i18n.svelte.js';

/**
 * The id of the first won stage of a pipeline.
 * @param {{ get: (workspaceId: string, id: string) => Promise<unknown> }} pipelines
 * @param {string} workspaceId
 * @param {string} pipelineId
 * @returns {Promise<string>}
 */
export async function wonStageId(pipelines, workspaceId, pipelineId) {
  const pipeline = /** @type {{ stages?: { id: string, kind: string }[] }} */ (await pipelines.get(workspaceId, pipelineId));
  const won = (pipeline.stages ?? []).find((stage) => stage.kind === 'won');
  if (!won) throw new Error(t('This pipeline has no won stage.'));
  return won.id;
}

/**
 * Moves a new deal to a won stage.
 * @param {{ move: (workspaceId: string, id: string, body: { stage_id: string }) => Promise<unknown> }} deals
 * @param {string} workspaceId
 * @param {string} dealId
 * @param {string} stageId
 */
export function markWon(deals, workspaceId, dealId, stageId) {
  return deals.move(workspaceId, dealId, { stage_id: stageId });
}
