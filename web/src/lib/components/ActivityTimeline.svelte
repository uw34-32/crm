<script>
  // The activities of one deal, contact, or company, newest first and grouped by day.
  // A note, call, meeting, or task is added from the form on top; a task can be
  // completed and reopened by anyone, and the author, an owner, or an admin can edit
  // or delete an activity.
  import { CalendarClock, CircleCheckBig, Circle, Handshake, NotebookPen, Pencil, Phone, Plus, Trash2, ListTodo } from '@lucide/svelte';
  import { tick, untrack } from 'svelte';
  import { activitiesApi } from '$lib/api/crm.js';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import Field from '$lib/components/Field.svelte';
  import { ACTIVITY_KINDS, activityCreateBody, activityPatchBody, toLocalInput, validateActivity } from '$lib/activities/fields.js';
  import { canEditActivity, dueState, groupByDay, memberName } from '$lib/activities/timeline.js';
  import { toSentence } from '$lib/forms.js';
  import { loadMembers, members } from '$lib/members.svelte.js';
  import { createPager, emptyList } from '$lib/records/list.js';
  import { isMissing } from '$lib/records/load.js';
  import { api, session } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';
  import { workspaces } from '$lib/workspaces.svelte.js';

  /**
   * @type {{ workspaceId: string, link: { deal_id: string } | { contact_id: string } | { company_id: string }, subjectName?: string }}
   */
  let { workspaceId, link, subjectName = 'this record' } = $props();

  const activities = activitiesApi(api);
  const kindInfo = {
    note: { label: 'Note', icon: NotebookPen },
    call: { label: 'Call', icon: Phone },
    meeting: { label: 'Meeting', icon: Handshake },
    task: { label: 'Task', icon: ListTodo }
  };

  const list = $state(/** @type {import('$lib/records/list.js').ListState<any>} */ (emptyList()));
  const filterOf = () => /** @type {Record<string, string>} */ (link);
  const pager = createPager(list, ({ limit, offset }) => activities.list(workspaceId, { ...filterOf(), limit, offset }));
  const now = new Date();
  const timeFormat = new Intl.DateTimeFormat(undefined, { timeStyle: 'short' });
  const dueFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });

  $effect(() => {
    const key = Object.values(link).join();
    untrack(() => {
      loadMembers(workspaceId);
      if (key) pager.load('');
    });
    return () => pager.stop();
  });

  const groups = $derived(/** @type {{ key: string, label: string, items: any[] }[]} */ (groupByDay(list.items, now)));
  const actor = $derived({ userId: session.user?.id ?? '', role: workspaces.current?.role ?? 'member' });

  // Composer
  let kind = $state(/** @type {'note' | 'call' | 'meeting' | 'task'} */ ('note'));
  const blank = () => ({ subject: '', body: '', due_at: '', assignee_id: '' });
  let draft = $state(blank());
  let errors = $state(/** @type {Record<string, string>} */ ({}));
  let adding = $state(false);
  let addError = $state('');

  /** @param {SubmitEvent} event */
  async function add(event) {
    event.preventDefault();
    errors = validateActivity({ kind, ...draft });
    const first = ['subject', 'body', 'due_at'].find((field) => errors[field]);
    if (first) {
      await tick();
      document.getElementById(`activity-new-${first}`)?.focus();
      return;
    }
    adding = true;
    addError = '';
    try {
      const created = await activities.create(workspaceId, activityCreateBody({ kind, ...draft }, link));
      list.items = [/** @type {any} */ (created), ...list.items];
      draft = blank();
      showToast(`${kindInfo[kind].label} added`);
    } catch (err) {
      addError = toSentence(err instanceof Error ? err.message : String(err));
    } finally {
      adding = false;
    }
  }

  // Tasks: completing and reopening
  /** @type {Record<string, boolean>} */
  let toggling = $state({});

  /** @param {any} activity */
  async function toggle(activity) {
    if (toggling[activity.id]) return;
    toggling[activity.id] = true;
    try {
      const updated = /** @type {any} */ (
        activity.completed_at ? await activities.reopen(workspaceId, activity.id) : await activities.complete(workspaceId, activity.id)
      );
      list.items = list.items.map((item) => (item.id === activity.id ? updated : item));
    } catch (err) {
      if (isMissing(err)) list.items = list.items.filter((item) => item.id !== activity.id);
      showToast(toSentence(err instanceof Error ? err.message : String(err)), 4000);
    } finally {
      delete toggling[activity.id];
    }
  }

  // Editing one activity at a time
  /** @type {any} */
  let editing = $state(null);
  let editDraft = $state(blank());
  let editErrors = $state(/** @type {Record<string, string>} */ ({}));
  let saving = $state(false);
  let editError = $state('');

  /** @param {any} activity */
  async function startEdit(activity) {
    editing = activity;
    editDraft = {
      subject: activity.subject,
      body: activity.body ?? '',
      due_at: toLocalInput(activity.due_at),
      assignee_id: activity.assignee_id ?? ''
    };
    editErrors = {};
    editError = '';
    await tick();
    document.getElementById('activity-edit-subject')?.focus();
  }

  /** @param {SubmitEvent} event */
  async function saveEdit(event) {
    event.preventDefault();
    if (!editing) return;
    editErrors = validateActivity({ kind: editing.kind, ...editDraft });
    if (Object.keys(editErrors).length > 0) return;
    const body = activityPatchBody(editing, { kind: editing.kind, ...editDraft });
    if (Object.keys(body).length === 0) {
      editing = null;
      return;
    }
    saving = true;
    editError = '';
    try {
      const updated = /** @type {any} */ (await activities.update(workspaceId, editing.id, body));
      list.items = list.items.map((item) => (item.id === updated.id ? updated : item));
      editing = null;
    } catch (err) {
      if (isMissing(err)) {
        list.items = list.items.filter((item) => item.id !== editing.id);
        editing = null;
        showToast('That activity no longer exists.');
      } else {
        editError = toSentence(err instanceof Error ? err.message : String(err));
      }
    } finally {
      saving = false;
    }
  }

  // Deleting
  /** @type {any} */
  let deleting = $state(null);
  let deleteBusy = $state(false);
  let deleteError = $state('');

  async function confirmDelete() {
    if (!deleting) return;
    deleteBusy = true;
    deleteError = '';
    try {
      await activities.remove(workspaceId, deleting.id);
    } catch (err) {
      if (!isMissing(err)) {
        deleteError = toSentence(err instanceof Error ? err.message : String(err));
        deleteBusy = false;
        return;
      }
    }
    const gone = deleting.id;
    list.items = list.items.filter((item) => item.id !== gone);
    deleting = null;
    deleteBusy = false;
    showToast('Activity deleted');
  }

  /** @param {any} activity */
  const stateOf = (activity) => dueState(activity, now);
  /** @param {'overdue' | 'today' | 'upcoming' | 'completed' | 'open' | 'none'} state */
  const stateLabel = (state) => ({ overdue: 'Overdue', today: 'Due today', upcoming: 'Upcoming', completed: 'Done', open: 'Open', none: '' })[state];
</script>

<section class="section-card" aria-labelledby="timeline-title">
  <div class="section-card-header">
    <h2 id="timeline-title" class="section-card-title">Activity</h2>
  </div>

  <form class="composer" novalidate onsubmit={add}>
    <div class="kinds" role="radiogroup" aria-label="Kind of activity">
      {#each ACTIVITY_KINDS as k (k)}
        {@const Icon = kindInfo[k].icon}
        <label class="kind" class:selected={kind === k}>
          <input type="radio" name="activity-kind" value={k} bind:group={kind} class="sr-only" />
          <Icon size={16} aria-hidden="true" />
          {kindInfo[k].label}
        </label>
      {/each}
    </div>
    <Field id="activity-new-subject" label={kind === 'task' ? 'What needs doing' : 'Subject'} bind:value={draft.subject} error={errors.subject} required />
    <div class="form-group">
      <label class="form-label" for="activity-new-body">Details (optional)</label>
      <textarea id="activity-new-body" class="input" rows="2" bind:value={draft.body} aria-invalid={errors.body ? 'true' : undefined}></textarea>
      {#if errors.body}<p class="form-error">{errors.body}</p>{/if}
    </div>
    {#if kind === 'task'}
      <div class="task-row">
        <div class="form-group">
          <label class="form-label" for="activity-new-due_at">Due</label>
          <input id="activity-new-due_at" class="input" type="datetime-local" bind:value={draft.due_at} aria-invalid={errors.due_at ? 'true' : undefined} />
          {#if errors.due_at}<p class="form-error">{errors.due_at}</p>{/if}
        </div>
        <div class="form-group">
          <label class="form-label" for="activity-new-assignee">Assignee</label>
          <select id="activity-new-assignee" class="select" bind:value={draft.assignee_id}>
            <option value="">Nobody</option>
            {#each members.list as member (member.user_id)}
              <option value={member.user_id}>{member.name || member.email}</option>
            {/each}
          </select>
        </div>
      </div>
    {/if}
    {#if addError}<p class="form-error" role="alert">{addError}</p>{/if}
    <div class="form-actions">
      <button class="btn btn-primary" type="submit" disabled={adding} aria-busy={adding}>
        <Plus size={18} aria-hidden="true" />
        {adding ? 'Adding…' : `Add ${kindInfo[kind].label.toLowerCase()}`}
      </button>
    </div>
  </form>

  {#if list.status === 'error'}
    <div class="section-card-body"><ErrorNotice message={list.error} onretry={() => pager.load('')} /></div>
  {:else if list.status !== 'ready' && list.items.length === 0}
    <div class="section-card-body" aria-busy="true">
      <p class="sr-only" role="status">Loading the activity</p>
      <span class="skeleton skeleton-line" style="display:block;width:70%;margin-bottom:10px"></span>
      <span class="skeleton skeleton-line" style="display:block;width:45%"></span>
    </div>
  {:else if list.items.length === 0}
    <p class="section-card-body muted">Nothing logged for {subjectName} yet. Add a note after your next call.</p>
  {:else}
    {#each groups as group (group.key)}
      <div class="day">
        <h3 class="day-label">{group.label}</h3>
        <ul class="items">
          {#each group.items as activity (activity.id)}
            {@const info = kindInfo[/** @type {keyof typeof kindInfo} */ (activity.kind)]}
            {@const Icon = info.icon}
            {@const state = stateOf(activity)}
            <li class="item" class:done={state === 'completed'}>
              {#if activity.kind === 'task'}
                <button
                  type="button"
                  class="check"
                  onclick={() => toggle(activity)}
                  disabled={Boolean(toggling[activity.id])}
                  aria-pressed={Boolean(activity.completed_at)}
                  aria-label={activity.completed_at ? `Reopen ${activity.subject}` : `Complete ${activity.subject}`}
                >
                  {#if activity.completed_at}<CircleCheckBig size={22} aria-hidden="true" />{:else}<Circle size={22} aria-hidden="true" />{/if}
                </button>
              {:else}
                <span class="kind-icon"><Icon size={18} aria-hidden="true" /></span>
              {/if}
              <div class="item-main">
                {#if editing?.id === activity.id}
                  <form class="edit" novalidate onsubmit={saveEdit}>
                    <Field id="activity-edit-subject" label="Subject" bind:value={editDraft.subject} error={editErrors.subject} required />
                    <div class="form-group">
                      <label class="form-label" for="activity-edit-body">Details</label>
                      <textarea id="activity-edit-body" class="input" rows="2" bind:value={editDraft.body}></textarea>
                      {#if editErrors.body}<p class="form-error">{editErrors.body}</p>{/if}
                    </div>
                    {#if activity.kind === 'task'}
                      <div class="task-row">
                        <div class="form-group">
                          <label class="form-label" for="activity-edit-due">Due</label>
                          <input id="activity-edit-due" class="input" type="datetime-local" bind:value={editDraft.due_at} />
                          {#if editErrors.due_at}<p class="form-error">{editErrors.due_at}</p>{/if}
                        </div>
                        <div class="form-group">
                          <label class="form-label" for="activity-edit-assignee">Assignee</label>
                          <select id="activity-edit-assignee" class="select" bind:value={editDraft.assignee_id}>
                            <option value="">Nobody</option>
                            {#each members.list as member (member.user_id)}
                              <option value={member.user_id}>{member.name || member.email}</option>
                            {/each}
                          </select>
                        </div>
                      </div>
                    {/if}
                    {#if editError}<p class="form-error" role="alert">{editError}</p>{/if}
                    <div class="form-actions">
                      <button type="button" class="btn btn-secondary" onclick={() => (editing = null)}>Cancel</button>
                      <button type="submit" class="btn btn-primary" disabled={saving} aria-busy={saving}>{saving ? 'Saving…' : 'Save changes'}</button>
                    </div>
                  </form>
                {:else}
                  <p class="subject"><span class="sr-only">{info.label}: </span>{activity.subject}</p>
                  {#if activity.body}<p class="body">{activity.body}</p>{/if}
                  <p class="meta">
                    {timeFormat.format(new Date(activity.created_at))} · {memberName(members.list, activity.author_id)}
                    {#if activity.kind === 'task'}
                      {#if activity.assignee_id} · For {memberName(members.list, activity.assignee_id)}{/if}
                      {#if activity.due_at}
                        · <span class="due {state}"><CalendarClock size={13} aria-hidden="true" /> {dueFormat.format(new Date(activity.due_at))}{#if state === 'overdue' || state === 'today'} ({stateLabel(state).toLowerCase()}){/if}</span>
                      {/if}
                      {#if activity.completed_at} · Done{/if}
                    {/if}
                  </p>
                {/if}
              </div>
              {#if editing?.id !== activity.id && canEditActivity(activity, actor)}
                <div class="item-actions">
                  <button type="button" class="icon-btn" onclick={() => startEdit(activity)} aria-label="Edit {activity.subject}" title="Edit">
                    <Pencil size={16} aria-hidden="true" />
                  </button>
                  <button type="button" class="icon-btn" onclick={() => (deleting = activity)} aria-label="Delete {activity.subject}" title="Delete">
                    <Trash2 size={16} aria-hidden="true" />
                  </button>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/each}
    <div class="more">
      {#if list.moreError}<ErrorNotice message={list.moreError} />{/if}
      {#if list.hasMore}
        <button type="button" class="btn btn-secondary" onclick={pager.more} disabled={list.loadingMore} aria-busy={list.loadingMore}>
          {list.loadingMore ? 'Loading…' : 'Load older activity'}
        </button>
      {/if}
    </div>
  {/if}
</section>

<ConfirmDialog
  open={deleting !== null}
  title="Delete this {deleting?.kind ?? 'activity'}?"
  message="It leaves the timeline for everyone in the workspace."
  confirmLabel="Delete {deleting?.kind ?? 'activity'}"
  busyLabel="Deleting…"
  busy={deleteBusy}
  error={deleteError}
  onconfirm={confirmDelete}
  oncancel={() => {
    deleting = null;
    deleteError = '';
  }}
/>

<style>
  .composer {
    padding: 14px 16px 4px;
    border-bottom: 1px solid var(--border);
  }

  .kinds {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 14px;
  }

  .kind {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-height: 44px;
    padding: 8px 14px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--surface2);
    color: var(--text2);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }

  .kind.selected {
    color: var(--accent-text);
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }

  .kind:has(input:focus-visible) {
    outline: 3px solid color-mix(in srgb, var(--accent) 45%, transparent);
    outline-offset: 2px;
  }

  textarea.input {
    resize: vertical;
    min-height: 64px;
  }

  .task-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
    column-gap: 12px;
  }

  .muted {
    color: var(--text2);
    font-size: 0.9375rem;
  }

  .day-label {
    padding: 12px 16px 4px;
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--text2);
  }

  .items {
    list-style: none;
  }

  .item {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 10px 8px 10px 16px;
  }

  .item + .item {
    border-top: 1px solid var(--border);
  }

  .kind-icon,
  .check {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    margin-top: -2px;
    color: var(--text2);
  }

  .check {
    min-width: 44px;
    min-height: 44px;
    margin: -8px -6px -8px -6px;
    border: none;
    border-radius: 10px;
    background: transparent;
    cursor: pointer;
  }

  .check:hover:not(:disabled) {
    background: var(--surface2);
    color: var(--accent-text);
  }

  .done .check {
    color: var(--accent-text);
  }

  .done .subject {
    text-decoration: line-through;
    color: var(--text2);
  }

  .item-main {
    flex: 1;
    min-width: 0;
  }

  .subject {
    font-size: 0.9375rem;
    font-weight: 500;
    overflow-wrap: anywhere;
  }

  .body {
    margin-top: 2px;
    font-size: 0.875rem;
    white-space: pre-line;
    overflow-wrap: anywhere;
  }

  .meta {
    margin-top: 3px;
    font-size: 0.8125rem;
    color: var(--text2);
  }

  .due {
    display: inline-flex;
    align-items: center;
    gap: 3px;
  }

  .due.overdue {
    color: var(--danger-text);
    font-weight: 600;
  }

  .due.today {
    color: var(--accent-text);
    font-weight: 600;
  }

  .item-actions {
    display: flex;
    flex-shrink: 0;
  }

  .more {
    padding: 8px 16px 16px;
    text-align: center;
  }

  .edit {
    padding: 4px 0;
  }
</style>
