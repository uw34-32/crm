<script>
  import { Search } from '@lucide/svelte';
  import { MAX_SEARCH_CHARS } from '$lib/records/url.js';

  /** @type {{ id: string, label: string, value: string, placeholder?: string, oninput: (value: string) => void }} */
  let { id, label, value, placeholder = '', oninput } = $props();
</script>

<div class="search-bar" role="search">
  <label class="sr-only" for={id}>{label}</label>
  <span class="search-icon" aria-hidden="true"><Search size={18} /></span>
  <input
    {id}
    class="input search-input"
    type="search"
    autocomplete="off"
    spellcheck="false"
    maxlength={MAX_SEARCH_CHARS}
    {placeholder}
    {value}
    oninput={(event) => oninput(event.currentTarget.value)}
  />
</div>

<style>
  .search-bar {
    position: sticky;
  }

  .search-icon {
    position: absolute;
    left: 14px;
    top: 50%;
    transform: translateY(-50%);
    display: inline-flex;
    color: var(--text2);
    pointer-events: none;
  }

  .search-input {
    padding-left: 40px;
  }
</style>
