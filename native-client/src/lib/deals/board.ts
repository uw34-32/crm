// The board: one column per stage, each holding its deals in position order. Every
// function here returns new columns and leaves the ones it was given alone, so a
// screen can keep the old board to put back if the API refuses a move.

export type Stage = { id: string; name: string; position: number; kind: 'open' | 'won' | 'lost'; color: string; probability?: number };
export type Deal = {
  id: string;
  stage_id: string;
  position: number;
  title: string;
  value_minor: number;
  currency: string;
  [key: string]: any;
};
export type Column = { stage: Stage; deals: Deal[]; hasMore: boolean };
export type Total = { currency: string; minor: number };

const renumbered = (deals: Deal[]) => deals.map((deal, position) => (deal.position === position ? deal : { ...deal, position }));

/** hasMore says which stages have deals beyond those loaded. */
export function buildColumns(stages: Stage[], deals: Deal[], { hasMore = {} }: { hasMore?: Record<string, boolean> } = {}): Column[] {
  return [...stages]
    .sort((a, b) => a.position - b.position)
    .map((stage) => ({
      stage,
      deals: deals.filter((deal) => deal.stage_id === stage.id).sort((a, b) => a.position - b.position),
      hasMore: Boolean(hasMore[stage.id])
    }));
}

/** The sum of a column's deals per currency, largest first. */
export function stageTotals(deals: Deal[]): Total[] {
  const sums = new Map<string, number>();
  for (const deal of deals) sums.set(deal.currency, (sums.get(deal.currency) ?? 0) + deal.value_minor);
  return [...sums]
    .map(([currency, minor]) => ({ currency, minor }))
    .sort((a, b) => b.minor - a.minor || a.currency.localeCompare(b.currency));
}

function locate(columns: Column[], dealId: string) {
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
 */
export function planMove(
  columns: Column[],
  dealId: string,
  toStageId: string,
  before: string | null
): { columns: Column[]; body: { stage_id: string; position?: number }; from: { stageId: string; index: number } } | null {
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

/**
 * The board with a deal back where a refused move took it from, so a failed move is
 * undone without undoing moves of other deals made meanwhile. The index is clamped,
 * since the old stage may have fewer deals by now.
 */
export function undoMove(columns: Column[], dealId: string, from: { stageId: string; index: number }) {
  const found = locate(columns, dealId);
  const homeIndex = columns.findIndex((column) => column.stage.id === from.stageId);
  if (!found || homeIndex < 0) return columns;
  const without = removeDeal(columns, dealId);
  const home = [...without[homeIndex].deals];
  home.splice(Math.min(from.index, home.length), 0, { ...found.deal, stage_id: from.stageId });
  return without.map((column, index) => (index === homeIndex ? { ...column, deals: renumbered(home) } : column));
}

/** The board with one deal replaced by what the API returned for it. */
export function replaceDeal(columns: Column[], deal: Deal) {
  return columns.map((column) => ({ ...column, deals: column.deals.map((d) => (d.id === deal.id ? { ...d, ...deal } : d)) }));
}

/** The board without a deal, with the gap closed. */
export function removeDeal(columns: Column[], dealId: string) {
  return columns.map((column) =>
    column.deals.some((deal) => deal.id === dealId) ? { ...column, deals: renumbered(column.deals.filter((deal) => deal.id !== dealId)) } : column
  );
}

/**
 * The board with a later page of deals added to their columns: new deals take their
 * place by position among those shown, deals already shown are skipped, and the
 * named stages learn whether more remain.
 */
export function appendDeals(columns: Column[], deals: Deal[], hasMore: Record<string, boolean>) {
  return columns.map((column) => {
    const shown = new Set(column.deals.map((deal) => deal.id));
    const added = deals.filter((deal) => deal.stage_id === column.stage.id && !shown.has(deal.id));
    const more = column.stage.id in hasMore ? hasMore[column.stage.id] : column.hasMore;
    if (added.length === 0 && more === column.hasMore) return column;
    const merged = [...column.deals, ...added].sort((a, b) => a.position - b.position);
    return { ...column, deals: renumbered(merged), hasMore: more };
  });
}

/** A move into a lost stage needs a reason; a move within one keeps the reason it has. */
export function needsLostReason(deal: Deal, stage: Stage) {
  return stage.kind === 'lost' && deal.stage_id !== stage.id;
}
