<script>
  import { t } from '$lib/i18n.svelte.js';
  import '../app.css';
  import { ModeWatcher } from 'mode-watcher';
  import { onMount } from 'svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import { startLocale } from '$lib/i18n.svelte.js';
  import { restoreSession, session, signOut } from '$lib/session.svelte.js';
  import { toast } from '$lib/toast.svelte.js';

  let { children } = $props();

  // Restore the previous visit's session before any route decides where to go.
  onMount(() => {
    startLocale();
    void restoreSession();
  });
</script>

<ModeWatcher themeColors={{ dark: '#000000', light: '#f2f2f7' }} />

{#if !session.ready}
  <p class="sr-only" role="status">{t('Loading')}</p>
{:else if session.restoreError}
  <main class="restore-error">
    <h1 class="sr-only">{t('CRM')}</h1>
    <ErrorNotice message={session.restoreError} onretry={restoreSession} />
    <button type="button" class="btn-ghost" onclick={signOut}>{t('Sign out on this device')}</button>
  </main>
{:else}
  {@render children()}
{/if}

<div class={toast.message ? 'toast' : 'sr-only'} role="status" aria-live="polite">{toast.message}</div>

<style>
  .restore-error {
    max-width: 480px;
    margin: 0 auto;
    padding: 20vh 16px 20px;
  }
</style>
