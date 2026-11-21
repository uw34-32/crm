// The fields of an activity: checks that mirror the API's limits, and the request
// bodies. A due time lives in the form as the local text of a datetime-local field and
// travels as an instant. Only a task has a due time and an assignee.

export type ActivityKind = 'note' | 'call' | 'meeting' | 'task';
export type ActivityDraft = { kind: ActivityKind; subject?: string; body?: string; due_at?: string; assignee_id?: string | null };
export type ActivityLink = { deal_id: string } | { contact_id: string } | { company_id: string };

export const ACTIVITY_KINDS = ['note', 'call', 'meeting', 'task'] as const;
export const ACTIVITY_LIMITS = { subject: 200, body: 10000 };

const MIN_YEAR = 1900;
const MAX_YEAR = 2200;
const LOCAL_INPUT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2})?$/;

const text = (value: unknown) => (typeof value === 'string' ? value.trim() : '');

const tooLong = (value: string, max: number) => ([...value].length > max ? `Use at most ${max} characters.` : '');

/** An instant as the local value a datetime-local field holds, to the minute. */
export function toLocalInput(iso: string | null | undefined) {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** The instant a datetime-local value names, in UTC; null for a blank or malformed one. */
export function fromLocalInput(value: string) {
  const local = text(value);
  if (!LOCAL_INPUT.test(local)) return null;
  const date = new Date(local);
  return Number.isNaN(date.getTime()) ? null : date.toISOString();
}

function checkDue(value: string) {
  if (!value) return '';
  const iso = fromLocalInput(value);
  if (!iso) return 'Enter a date and time.';
  const year = new Date(iso).getUTCFullYear();
  return year < MIN_YEAR || year > MAX_YEAR ? 'Use a date from 1900 to 2200.' : '';
}

const compact = (errors: Record<string, string>) => Object.fromEntries(Object.entries(errors).filter(([, message]) => message));

export function validateActivity(draft: ActivityDraft): Record<string, string> {
  const subject = text(draft.subject);
  return compact({
    subject: subject ? tooLong(subject, ACTIVITY_LIMITS.subject) : 'Enter a subject.',
    body: tooLong(text(draft.body), ACTIVITY_LIMITS.body),
    due_at: draft.kind === 'task' ? checkDue(text(draft.due_at)) : ''
  });
}

export function activityCreateBody(draft: ActivityDraft, link: ActivityLink) {
  const body: Record<string, string> = { kind: draft.kind, subject: text(draft.subject) };
  if (text(draft.body)) body.body = text(draft.body);
  if (draft.kind === 'task') {
    const due = fromLocalInput(text(draft.due_at));
    if (due) body.due_at = due;
    if (text(draft.assignee_id)) body.assignee_id = text(draft.assignee_id);
  }
  return { ...body, ...link };
}

/** original is the activity as the API sent it. */
export function activityPatchBody(original: Record<string, any>, draft: ActivityDraft) {
  const body: Record<string, string | null> = {};
  const subject = text(draft.subject);
  if (subject !== text(original.subject)) body.subject = subject;
  const content = text(draft.body);
  if (content !== text(original.body)) body.body = content;
  if (original.kind === 'task') {
    const due = fromLocalInput(text(draft.due_at));
    const before = original.due_at ? new Date(original.due_at).toISOString() : null;
    if (due !== before) body.due_at = due;
    const assignee = text(draft.assignee_id) || null;
    if (assignee !== (original.assignee_id ?? null)) body.assignee_id = assignee;
  }
  return body;
}
