import { describe, expect, it } from 'vitest';
import { activityCreateBody, activityPatchBody, fromLocalInput, toLocalInput, validateActivity } from './fields.js';

const local = new Date(2027, 2, 14, 9, 30);

describe('local time fields', () => {
  it('round-trips an instant through the value a datetime-local field holds', () => {
    const text = toLocalInput(local.toISOString());
    expect(text).toBe('2027-03-14T09:30');
    expect(fromLocalInput(text)).toBe(local.toISOString());
  });

  it('is blank for no time, and null for text that is not a time', () => {
    expect(toLocalInput(null)).toBe('');
    expect(fromLocalInput('')).toBeNull();
    expect(fromLocalInput('tomorrow')).toBeNull();
  });
});

describe('activity validation', () => {
  it('requires a subject within 200 characters and a body within 10000', () => {
    expect(validateActivity({ kind: 'note', subject: ' ', body: '' })).toEqual({ subject: 'Enter a subject.' });
    expect(validateActivity({ kind: 'note', subject: 'x'.repeat(201), body: '' })).toEqual({ subject: 'Use at most 200 characters.' });
    expect(validateActivity({ kind: 'note', subject: 'Hi', body: 'x'.repeat(10001) })).toEqual({ body: 'Use at most 10000 characters.' });
    expect(validateActivity({ kind: 'note', subject: 'Hi', body: '' })).toEqual({});
  });

  it('checks a task due time is a time within 1900 to 2200', () => {
    expect(validateActivity({ kind: 'task', subject: 'Call', body: '', due_at: '' })).toEqual({});
    expect(validateActivity({ kind: 'task', subject: 'Call', body: '', due_at: '2027-03-14T09:30' })).toEqual({});
    expect(validateActivity({ kind: 'task', subject: 'Call', body: '', due_at: 'soon' })).toEqual({ due_at: 'Enter a date and time.' });
    expect(validateActivity({ kind: 'task', subject: 'Call', body: '', due_at: '2201-01-01T00:00' })).toEqual({ due_at: 'Use a date from 1900 to 2200.' });
  });
});

describe('the create body', () => {
  it('sends the kind, the subject, the link, and a body only when there is one', () => {
    expect(activityCreateBody({ kind: 'note', subject: ' Called back ', body: '' }, { deal_id: 'd1' })).toEqual({
      kind: 'note',
      subject: 'Called back',
      deal_id: 'd1'
    });
  });

  it('adds the due time as an instant and the assignee for a task', () => {
    expect(activityCreateBody({ kind: 'task', subject: 'Send proposal', body: 'PDF', due_at: '2027-03-14T09:30', assignee_id: 'u2' }, { contact_id: 'c1' })).toEqual({
      kind: 'task',
      subject: 'Send proposal',
      body: 'PDF',
      due_at: local.toISOString(),
      assignee_id: 'u2',
      contact_id: 'c1'
    });
  });

  it('ignores a due time and an assignee on anything but a task, which the API refuses', () => {
    expect(activityCreateBody({ kind: 'call', subject: 'Intro', body: '', due_at: '2027-03-14T09:30', assignee_id: 'u2' }, { deal_id: 'd1' })).toEqual({
      kind: 'call',
      subject: 'Intro',
      deal_id: 'd1'
    });
  });
});

describe('the patch body', () => {
  /** @type {import('./fields.js').ActivityDraft & Record<string, unknown>} */
  const task = { id: 'a1', kind: 'task', subject: 'Send proposal', body: 'PDF', due_at: local.toISOString(), assignee_id: 'u2' };

  it('is empty when nothing changed', () => {
    expect(activityPatchBody(task, { kind: 'task', subject: 'Send proposal', body: 'PDF', due_at: '2027-03-14T09:30', assignee_id: 'u2' })).toEqual({});
  });

  it('names the changed fields, clearing the due time and assignee with null and the body with an empty string', () => {
    expect(activityPatchBody(task, { kind: 'task', subject: 'Send the proposal', body: '', due_at: '', assignee_id: '' })).toEqual({
      subject: 'Send the proposal',
      body: '',
      due_at: null,
      assignee_id: null
    });
  });

  it('changes neither the kind nor the links', () => {
    expect(activityPatchBody(task, { kind: 'note', subject: 'Send proposal', body: 'PDF', due_at: '2027-03-14T09:30', assignee_id: 'u2' })).toEqual({});
  });
});
