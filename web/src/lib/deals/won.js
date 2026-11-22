// A sale that is already closed is recorded as a deal moved to the Won stage. The API
// starts every new deal in an open stage, so recording one takes a create and then a move.

import { t } from '../i18n.svelte.js';

/**
 * Moves a new deal to the first won stage of its pipeline.
 * @param {{ move: (workspaceId: string, id: string, body: { stage_id: string }) => Promise<unknown> }} deals
 * @param {{ get: (workspaceId: string, id: string) => Promise<unknown> }} pipelines
 * @param {string} workspaceId
 * @param {string} dealId
 * @param {string} pipelineId
 * @returns {Promise<unknown>} the deal as the move returned it
 */
export async function markWon(deals, pipelines, workspaceId, dealId, pipelineId) {
  const pipeline = /** @type {{ stages?: { id: string, kind: string }[] }} */ (await pipelines.get(workspaceId, pipelineId));
  const won = (pipeline.stages ?? []).find((stage) => stage.kind === 'won');
  if (!won) throw new Error(t('This pipeline has no won stage.'));
  return deals.move(workspaceId, dealId, { stage_id: won.id });
}
