<script>
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount, tick } from 'svelte';
  import AuthCard from '$lib/components/AuthCard.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import Field from '$lib/components/Field.svelte';
  import { checkEmail, checkPassword, safeNext, toSentence } from '$lib/forms.js';
  import { session, signIn } from '$lib/session.svelte.js';

  let email = $state('');
  let password = $state('');
  let errors = $state({ email: '', password: '' });
  let formError = $state('');
  let busy = $state(false);
  /** @type {HTMLElement | null} */
  let notice = $state(null);

  // Signed in already, or just now: go where the user was headed.
  $effect(() => {
    if (session.user) goto(safeNext(page.url.searchParams.get('next')), { replaceState: true });
  });

  onMount(() => document.getElementById('email')?.focus());

  /** @param {SubmitEvent} event */
  async function submit(event) {
    event.preventDefault();
    formError = '';
    errors = { email: checkEmail(email), password: checkPassword(password, { creating: false }) };
    const invalid = Object.entries(errors).find(([, message]) => message);
    if (invalid) {
      document.getElementById(invalid[0])?.focus();
      return;
    }
    busy = true;
    try {
      await signIn({ email: email.trim(), password });
    } catch (err) {
      formError = toSentence(err instanceof Error ? err.message : String(err));
      await tick();
      notice?.focus();
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>Sign in · CRM</title>
</svelte:head>

<AuthCard title="Sign in">
  {#if session.ended && !formError}
    <p class="ended" role="status">Your session has ended. Sign in again to continue.</p>
  {/if}
  {#if formError}
    <ErrorNotice message={formError} bind:element={notice} />
  {/if}

  <form novalidate onsubmit={submit}>
    <Field id="email" label="Email" type="email" autocomplete="email" required bind:value={email} error={errors.email} />
    <Field
      id="password"
      label="Password"
      type="password"
      autocomplete="current-password"
      required
      bind:value={password}
      error={errors.password}
    />
    <button class="btn btn-primary btn-full" type="submit" disabled={busy} aria-busy={busy}>
      {busy ? 'Signing in…' : 'Sign in'}
    </button>
  </form>

  {#snippet footer()}
    New here? <a href="/register{page.url.search}">Create an account</a>
  {/snippet}
</AuthCard>

<style>
  .ended {
    padding: 12px 16px;
    margin-bottom: 16px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface2);
    font-size: 0.9375rem;
  }
</style>
