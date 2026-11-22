<script>
  // The language of the screens: each one is named in its own language, so it can be found
  // from the other. The choice is kept on this device.
  import { i18n, setLocale, t } from '$lib/i18n.svelte.js';

  const names = { en: 'English', es: 'Español' };

  /** @param {Event & { currentTarget: HTMLSelectElement }} event */
  function change(event) {
    setLocale(/** @type {'en' | 'es'} */ (event.currentTarget.value));
  }
</script>

<label class="sr-only" for="language">{t('Language')}</label>
<select id="language" class="select language" value={i18n.locale} onchange={change}>
  {#each Object.entries(names) as [code, name] (code)}
    <option value={code}>{name}</option>
  {/each}
</select>

<style>
  .language {
    width: auto;
    min-width: 0;
    padding-block: 6px;
    font-size: 0.875rem;
  }
</style>
