<script>
  // Picks a company or a contact by searching the workspace's records. The text is only
  // for searching: a record is linked by choosing it from the list, and unlinked with
  // the remove button or by clearing the field. Leaving the field with unmatched text
  // puts back the linked record's name.
  import { X } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { contactName } from '$lib/records/format.js';
  import { debounce } from '$lib/debounce.js';
  import { api } from '$lib/session.svelte.js';

  /** @typedef {{ id: string, name: string, detail?: string }} Choice */
  /** @type {{ id: string, kind?: 'companies' | 'contacts', label?: string, workspaceId: string, value: Choice | null, error?: string }} */
  let { id, kind = 'companies', label = kind === 'contacts' ? 'Contact' : 'Company', workspaceId, value = $bindable(null), error = '' } = $props();

  // The kind is fixed for the life of a picker.
  const records = untrack(() => recordsApi(api, kind));
  const noun = untrack(() => (kind === 'contacts' ? 'contacts' : 'companies'));
  /** What the picker shows for a record: a contact's full name, a company's name and domain. @param {any} record @returns {Choice} */
  const choiceOf = (record) =>
    kind === 'contacts'
      ? { id: record.id, name: contactName(record), detail: record.company_name ?? record.email ?? '' }
      : { id: record.id, name: record.name, detail: record.domain ?? '' };
  const RESULTS = 8;
  const listId = $derived(`${id}-list`);
  const hintId = $derived(`${id}-hint`);
  const statusId = $derived(`${id}-status`);

  let query = $state(value?.name ?? '');
  let open = $state(false);
  let active = $state(-1);
  /** @type {Choice[]} */
  let options = $state([]);
  let status = $state(/** @type {'idle' | 'loading' | 'ready' | 'error'} */ ('idle'));
  let generation = 0;
  /** @type {HTMLInputElement | null} */
  let input = $state(null);

  async function search(/** @type {string} */ text) {
    const mine = ++generation;
    status = 'loading';
    try {
      const page = await records.list(workspaceId, { q: text.trim(), limit: RESULTS, offset: 0 });
      if (mine !== generation) return;
      options = page.items.map(choiceOf);
      active = -1;
      status = 'ready';
    } catch {
      if (mine !== generation) return;
      options = [];
      status = 'error';
    }
  }
  const searchLater = debounce(search, 250);

  $effect(() => () => {
    searchLater.cancel();
    generation++;
  });

  function openList() {
    open = true;
    if (status === 'idle') search(query);
  }

  /** @param {Choice} choice */
  function choose(choice) {
    value = { id: choice.id, name: choice.name };
    query = choice.name;
    open = false;
    active = -1;
  }

  function unlink() {
    value = null;
    query = '';
    input?.focus();
  }

  function oninput() {
    // Unlink right away so a submit before the blur handler runs never sends the old record.
    if (!query.trim()) value = null;
    open = true;
    searchLater(query);
  }

  function onblur() {
    // Let a click on an option land first.
    setTimeout(() => {
      open = false;
      if (!query.trim()) value = null;
      else query = value?.name ?? '';
    }, 120);
  }

  /** @param {KeyboardEvent} event */
  function onkeydown(event) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (!open) openList();
      else active = Math.min(active + 1, options.length - 1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      active = Math.max(active - 1, 0);
    } else if (event.key === 'Enter' && open && options[active]) {
      event.preventDefault();
      choose(options[active]);
    } else if (event.key === 'Escape' && open) {
      event.preventDefault();
      event.stopPropagation();
      open = false;
    }
  }

  let message = $derived(
    status === 'loading'
      ? `Searching ${noun}…`
      : status === 'error'
        ? `The ${noun} could not be loaded. Keep typing to try again.`
        : status === 'ready' && options.length === 0
          ? `No ${noun} match. Add one first, then link it here.`
          : ''
  );
  let describedBy = $derived([hintId, error && `${id}-error`].filter(Boolean).join(' '));
</script>

<div class="form-group picker">
  <label class="form-label" for={id}>{label}</label>
  <div class="picker-field">
    <input
      bind:this={input}
      {id}
      class="input"
      type="text"
      role="combobox"
      autocomplete="off"
      spellcheck="false"
      aria-autocomplete="list"
      aria-expanded={open}
      aria-controls={listId}
      aria-activedescendant={open && active >= 0 ? `${id}-option-${active}` : undefined}
      aria-describedby={describedBy}
      aria-invalid={error ? 'true' : undefined}
      placeholder="Search {noun}"
      bind:value={query}
      onfocus={openList}
      {oninput}
      {onblur}
      {onkeydown}
    />
    {#if value}
      <button type="button" class="icon-btn unlink" onclick={unlink} aria-label="Remove the link to {value.name}" title="Remove the link">
        <X size={18} aria-hidden="true" />
      </button>
    {/if}
  </div>
  <ul class="picker-list" id={listId} role="listbox" aria-label={noun} hidden={!open || options.length === 0}>
    {#each options as option, i (option.id)}
      <li
        id="{id}-option-{i}"
        role="option"
        aria-selected={value?.id === option.id}
        class="picker-option"
        class:active={i === active}
        onmousedown={(event) => {
          event.preventDefault();
          choose(option);
        }}
      >
        <span class="option-name">{option.name}</span>
        {#if option.detail}<span class="option-detail">{option.detail}</span>{/if}
      </li>
    {/each}
  </ul>
  <p class="form-hint" id={hintId}>
    {value ? `Linked to ${value.name}.` : `No ${kind === 'contacts' ? 'contact' : 'company'} chosen.`}
  </p>
  <p class="sr-only" id={statusId} role="status">{open ? message : ''}</p>
  {#if open && message && status !== 'loading'}
    <p class="form-hint" aria-hidden="true">{message}</p>
  {/if}
  {#if error}
    <p class="form-error" id="{id}-error">{error}</p>
  {/if}
</div>

<style>
  .picker {
    position: relative;
  }

  .picker-field {
    position: relative;
  }

  .picker-field .input {
    padding-right: 52px;
  }

  .unlink {
    position: absolute;
    right: 2px;
    top: 50%;
    transform: translateY(-50%);
  }

  .picker-list {
    position: absolute;
    left: 0;
    right: 0;
    z-index: var(--z-sticky);
    margin-top: 4px;
    padding: 4px;
    list-style: none;
    max-height: 264px;
    overflow-y: auto;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
  }

  .picker-list[hidden] {
    display: none;
  }

  .picker-option {
    display: flex;
    flex-direction: column;
    justify-content: center;
    min-height: 44px;
    padding: 6px 12px;
    border-radius: 8px;
    cursor: pointer;
  }

  .picker-option.active,
  .picker-option:hover {
    background: var(--surface2);
  }

  .picker-option[aria-selected='true'] .option-name {
    color: var(--accent-text);
    font-weight: 600;
  }

  .option-name {
    font-size: 0.9375rem;
    overflow-wrap: anywhere;
  }

  .option-detail {
    font-size: 0.8125rem;
    color: var(--text2);
  }
</style>
