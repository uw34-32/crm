<script>
  // A labelled text field whose hint and error are tied to the input, so a screen
  // reader reads them with it. Forms set novalidate and check fields themselves.
  /**
   * @type {{
   *   id: string,
   *   label: string,
   *   value?: string,
   *   type?: 'text' | 'email' | 'password' | 'tel',
   *   error?: string,
   *   hint?: string,
   *   autocomplete?: import('svelte/elements').FullAutoFill,
   *   required?: boolean,
   *   disabled?: boolean
   * }}
   */
  let {
    id,
    label,
    value = $bindable(''),
    type = 'text',
    error = '',
    hint = '',
    autocomplete,
    required = false,
    disabled = false
  } = $props();

  let describedBy = $derived([hint && `${id}-hint`, error && `${id}-error`].filter(Boolean).join(' ') || undefined);
</script>

<div class="form-group">
  <label class="form-label" for={id}>{label}</label>
  <input
    {id}
    name={id}
    class="input"
    {type}
    bind:value
    {autocomplete}
    {disabled}
    spellcheck={type === 'text' ? undefined : false}
    autocapitalize={type === 'text' || type === 'tel' ? undefined : 'off'}
    aria-required={required}
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}
  />
  {#if hint}
    <p class="form-hint" id="{id}-hint">{hint}</p>
  {/if}
  {#if error}
    <p class="form-error" id="{id}-error">{error}</p>
  {/if}
</div>
