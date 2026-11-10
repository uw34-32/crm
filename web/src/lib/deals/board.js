// The board: one column per stage, each holding its deals in position order. Every
// function here returns new columns and leaves the ones it was given alone, so a
// screen can keep the old board to put back if the API refuses a move.

/**
 * @typedef {{ id: string, name: string, position: number, kind: 'open' | 'won' | 'lost', color: string, probability?: number }} Stage
 * @typedef {{ id: string, stage_id: string, position: number, title: string, value_minor: number, currency: string, [key: string]: any }} Deal
 * @typedef {{ stage: Stage, deals: Deal[], hasMore: boolean }} Column
 * @typedef {{ currency: string, minor: number }} Total
 */

/** @param {Deal[]} deals */
const renumbered = (deals) => deals.map((deal, position) => (deal.position === position ? deal : { ...deal, position }));

/**
 * @param {Stage[]} stages
 * @param {Deal[]} deals
 * @param {{ hasMore?: Record<string, boolean> }} [options] which stages have deals beyond those loaded
 * @returns {Column[]}
 */
export function buildColumns(stages, deals, { hasMore = {} } = {}) {
  return [...stages]
    .sort((a, b) => a.position - b.position)
    .map((stage) => ({
      stage,
      deals: deals.filter((deal) => deal.stage_id === stage.id).sort((a, b) => a.position - b.position),
      hasMore: Boolean(hasMore[stage.id])
    }));
}

/** The sum of a column's deals per currency, largest first. @param {Deal[]} deals @returns {Total[]} */
export function stageTotals(deals) {
  /** @type {Map<string, number>} */
  const sums = new Map();
  for (const deal of deals) sums.set(deal.currency, (sums.get(deal.currency) ?? 0) + deal.value_minor);
  return [...sums]
    .map(([currency, minor]) => ({ currency, minor }))
    .sort((a, b) => b.minor - a.minor || a.currency.localeCompare(b.currency));
}

/** @param {Column[]} columns @param {string} dealId */
function locate(columns, dealId) {
  for (const [columnIndex, column] of columns.entries()) {
    const index = column.deals.findIndex((deal) => deal.id === dealId);
    if (index >= 0) return { columnIndex, index, deal: column.deals[index] };
  }
  return null;
}

/**
 * Where a deal goes when it is dropped in a stage before another deal, or at the end
 * when before is null, and the request that puts it there. The position is its dense
 * index in the target once it has left its old place, which is what the API wants.
 * Appending to a column whose later pages are not loaded sends no position, so the
 * API puts the deal last rather than in the middle of what is not shown.
 * @param {Column[]} columns
 * @param {string} dealId
 * @param {string} toStageId
 * @param {string | null} before
 * @returns {{ columns: Column[], body: { stage_id: string, position?: number }, from: { stageId: string, index: number } } | null}
 */
export function planMove(columns, dealId, toStageId, before) {
  const found = locate(columns, dealId);
  const targetIndex = columns.findIndex((column) => column.stage.id === toStageId);
  if (!found || targetIndex < 0) return null;
  const source = columns[found.columnIndex];
  const sourceDeals = source.deals.filter((deal) => deal.id !== dealId);
  const targetDeals = targetIndex === found.columnIndex ? sourceDeals : [...columns[targetIndex].deals];
  const target = columns[targetIndex];

  let position = targetDeals.length;
  if (before !== null) {
    const index = targetDeals.findIndex((deal) => deal.id === before);
    if (index < 0) return null;
    position = index;
  }
  if (target.stage.id === source.stage.id && position === found.index) return null;

  const moved = { ...found.deal, stage_id: target.stage.id };
  targetDeals.splice(position, 0, moved);

  const next = columns.map((column, index) => {
    if (index === targetIndex) return { ...column, deals: renumbered(targetDeals) };
    if (index === found.columnIndex) return { ...column, deals: renumbered(sourceDeals) };
    return column;
  });
  const appendingBeyondLoaded = before === null && target.hasMore;
  const body = appendingBeyondLoaded ? { stage_id: target.stage.id } : { stage_id: target.stage.id, position };
  return { columns: next, body, from: { stageId: source.stage.id, index: found.index } };
}

/** The board with one deal replaced by what the API returned for it. @param {Column[]} columns @param {Deal} deal */
export function replaceDeal(columns, deal) {
  return columns.map((column) => ({ ...column, deals: column.deals.map((d) => (d.id === deal.id ? { ...d, ...deal } : d)) }));
}

/** The board without a deal, with the gap closed. @param {Column[]} columns @param {string} dealId */
export function removeDeal(columns, dealId) {
  return columns.map((column) =>
    column.deals.some((deal) => deal.id === dealId) ? { ...column, deals: renumbered(column.deals.filter((deal) => deal.id !== dealId)) } : column
  );
}

/** A move into a lost stage needs a reason; a move within one keeps the reason it has. @param {Deal} deal @param {Stage} stage */
export function needsLostReason(deal, stage) {
  return stage.kind === 'lost' && deal.stage_id !== stage.id;
}
