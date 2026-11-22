// How a timeline reads: activities grouped by the local day they were created, the
// state of a task against its due time, and who may change an activity.

import { t } from '../../i18n';

export type Activity = {
  id: string;
  kind: 'note' | 'call' | 'meeting' | 'task';
  created_at?: string;
  due_at?: string | null;
  completed_at?: string | null;
  author_id?: string | null;
  [key: string]: any;
};
export type DayGroup = { key: string; label: string; items: Activity[] };
export type DueState = 'none' | 'open' | 'completed' | 'overdue' | 'today' | 'upcoming';

const DAY_MS = 24 * 60 * 60 * 1000;

/** The local calendar day of a date, as YYYY-MM-DD. */
export function dayKey(date: Date) {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** Midnight at the start of a date's local day. */
const startOfDay = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate());

/** Today, Yesterday, or the day; the year only when it is not this one. */
export function dayLabel(date: Date, now: Date, locale?: string) {
  const days = Math.round((startOfDay(now).getTime() - startOfDay(date).getTime()) / DAY_MS);
  if (days === 0) return t('Today');
  if (days === 1) return t('Yesterday');
  const sameYear = date.getFullYear() === now.getFullYear();
  return new Intl.DateTimeFormat(locale, { weekday: 'short', month: 'short', day: 'numeric', ...(sameYear ? {} : { year: 'numeric' }) }).format(date);
}

/**
 * Activities in the order given, newest first as the API sends them, in one group per
 * local day.
 */
export function groupByDay(activities: Activity[], now: Date, locale?: string): DayGroup[] {
  const groups: DayGroup[] = [];
  for (const activity of activities) {
    const date = new Date(activity.created_at ?? 0);
    const key = dayKey(date);
    const last = groups.at(-1);
    if (last && last.key === key) last.items.push(activity);
    else groups.push({ key, label: dayLabel(date, now, locale), items: [activity] });
  }
  return groups;
}

/**
 * none for anything but a task; open for a task with no due time; otherwise where the
 * task stands against its due time.
 */
export function dueState(activity: Activity, now: Date): DueState {
  if (activity.kind !== 'task') return 'none';
  if (activity.completed_at) return 'completed';
  if (!activity.due_at) return 'open';
  const due = new Date(activity.due_at);
  if (due.getTime() < now.getTime()) return 'overdue';
  return dayKey(due) === dayKey(now) ? 'today' : 'upcoming';
}

/**
 * The author, an owner, or an admin may edit or delete an activity, as the API rules.
 */
export function canEditActivity(activity: Activity, { userId, role }: { userId: string; role: string }) {
  if (role === 'owner' || role === 'admin') return true;
  return Boolean(activity.author_id) && activity.author_id === userId;
}

/**
 * A member's name, or their email when they have given no name, or a note that they
 * have left the workspace.
 */
export function memberName(members: { user_id: string; name?: string; email: string }[], userId: string | null | undefined) {
  const member = userId ? members.find((m) => m.user_id === userId) : undefined;
  if (!member) return t('Former member');
  return member.name?.trim() || member.email;
}
