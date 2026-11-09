<script>
  // Picks the company a contact works at, by searching the workspace's companies. The
  // text is only for searching: a company is linked by choosing it from the list, and
  // unlinked with the remove button or by clearing the field. Leaving the field with
  // unmatched text puts back the linked company's name.
  import { X } from '@lucide/svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { debounce } from '$lib/debounce.js';
  import { api } from '$lib/session.svelte.js';

  /** @typedef {{ id: string, name: string, domain?: string }} CompanyChoice */
  /** @type {{ id: string, workspaceId: string, value: CompanyChoice | null, error?: string }} */
  let { id, workspaceId, value = $bindable(null), error = '' } = $props();

  const companies = recordsApi(api, 'companies');
  const RESULTS = 8;
  const listId = $derived(`${id}-list`);
  const hintId = $derived(`${id}-hint`);
  const statusId = $derived(`${id}-status`);

  let query = $state(value?.name ?? '');
  let open = $state(false);
  let active = $state(-1);
  /** @type {CompanyChoice[]} */
  let options = $state([]);
  let status = $state(/** @type {'idle' | 'loading' | 'ready' | 'error'} */ ('idle'));
  let generation = 0;
  /** @type {HTMLInputElement | null} */
  let input = $state(null);

  async function search(/** @type {string} */ text) {
    const mine = ++generation;
    status = 'loading';
    try {
      const page = await companies.list(workspaceId, { q: text.trim(), limit: RESULTS, offset: 0 });
      if (mine !== generation) return;
      options = page.items;
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

  /** @param {CompanyChoice} company */
  function choose(company) {
    value = { id: company.id, name: company.name };
    query = company.name;
    open = false;
    active = -1;
  }

  function unlink() {
    value = null;
    query = '';
    input?.focus();
  }

  function oninput() {
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
      ? 'Searching companies…'
      : status === 'error'
        ? 'Companies could not be loaded. Keep typing to try again.'
        : status === 'ready' && options.length === 0
          ? 'No companies match. Add the company first, then link it here.'
          : ''
  );
  let describedBy = $derived([hintId, error && `${id}-error`].filter(Boolean).join(' '));
</script>

<div class="form-group picker">
  <label class="form-label" for={id}>Company</label>
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
      placeholder="Search companies"
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
  <ul class="picker-list" id={listId} role="listbox" aria-label="Companies" hidden={!open || options.length === 0}>
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
        {#if option.domain}<span class="option-domain">{option.domain}</span>{/if}
      </li>
    {/each}
  </ul>
  <p class="form-hint" id={hintId}>
    {value ? `Linked to ${value.name}.` : 'Not linked to a company.'}
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

  .option-domain {
    font-size: 0.8125rem;
    color: var(--text2);
  }
</style>
