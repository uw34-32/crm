import { describe, expect, it } from 'vitest';
import { canEditActivity, dayLabel, dueState, groupByDay, memberName } from './timeline.js';

/** A local time on a given day. @param {number} y @param {number} m 1-based @param {number} d @param {number} [h] */
const at = (y, m, d, h = 10) => new Date(y, m - 1, d, h, 0, 0);

const now = at(2026, 11, 9, 15);

/** @typedef {import('./timeline.js').Activity} Activity */

/** @param {string} id @param {Date} created @param {Partial<Activity>} [rest] @returns {Activity} */
const activity = (id, created, rest = {}) => ({ id, kind: 'note', subject: id, created_at: created.toISOString(), ...rest });

describe('grouping by day', () => {
  it('keeps the newest-first order and starts a group at each new local day', () => {
    const groups = groupByDay(
      [activity('a', at(2026, 11, 9, 14)), activity('b', at(2026, 11, 9, 9)), activity('c', at(2026, 11, 8, 23)), activity('d', at(2026, 10, 1))],
      now,
      'en-US'
    );
    expect(groups.map((group) => [group.label, group.items.map((item) => item.id)])).toEqual([
      ['Today', ['a', 'b']],
      ['Yesterday', ['c']],
      ['Thu, Oct 1', ['d']]
    ]);
  });

  it('adds the year to a day in another year', () => {
    expect(dayLabel(at(2025, 12, 31), now, 'en-US')).toBe('Wed, Dec 31, 2025');
  });

  it('is empty for no activities', () => {
    expect(groupByDay([], now, 'en-US')).toEqual([]);
  });
});

describe('the state of a task', () => {
  /** @param {Partial<Activity>} rest @returns {Activity} */
  const task = (rest) => ({ id: 't', kind: 'task', due_at: null, completed_at: null, ...rest });

  it('is none for anything but a task, and for a task with no due time', () => {
    expect(dueState({ id: 'n', kind: 'note' }, now)).toBe('none');
    expect(dueState(task({}), now)).toBe('open');
  });

  it('is completed once done, whatever the due time', () => {
    expect(dueState(task({ due_at: at(2026, 11, 1).toISOString(), completed_at: now.toISOString() }), now)).toBe('completed');
  });

  it('is overdue after the due time, today on the same local day, and upcoming before that', () => {
    expect(dueState(task({ due_at: at(2026, 11, 9, 9).toISOString() }), now)).toBe('overdue');
    expect(dueState(task({ due_at: at(2026, 11, 9, 18).toISOString() }), now)).toBe('today');
    expect(dueState(task({ due_at: at(2026, 11, 10, 9).toISOString() }), now)).toBe('upcoming');
  });
});

describe('who may edit an activity', () => {
  /** @type {Activity} */
  const note = { id: 'a', kind: 'note', author_id: 'u1' };

  it('lets the author, an owner, and an admin edit, and nobody else', () => {
    expect(canEditActivity(note, { userId: 'u1', role: 'member' })).toBe(true);
    expect(canEditActivity(note, { userId: 'u2', role: 'member' })).toBe(false);
    expect(canEditActivity(note, { userId: 'u2', role: 'admin' })).toBe(true);
    expect(canEditActivity(note, { userId: 'u2', role: 'owner' })).toBe(true);
  });

  it('keeps an activity whose author left to owners and admins', () => {
    expect(canEditActivity({ ...note, author_id: null }, { userId: 'u1', role: 'member' })).toBe(false);
    expect(canEditActivity({ ...note, author_id: null }, { userId: 'u1', role: 'admin' })).toBe(true);
  });
});

describe('naming a member', () => {
  const members = [
    { user_id: 'u1', name: 'Ada Lovelace', email: 'ada@example.com' },
    { user_id: 'u2', name: '', email: 'grace@example.com' }
  ];

  it('uses the name, then the email, and says when the member is gone', () => {
    expect(memberName(members, 'u1')).toBe('Ada Lovelace');
    expect(memberName(members, 'u2')).toBe('grace@example.com');
    expect(memberName(members, 'u9')).toBe('Former member');
    expect(memberName(members, null)).toBe('Former member');
  });
});
