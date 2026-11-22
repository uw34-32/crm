<script>
  import { t } from '$lib/i18n.svelte.js';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount, tick } from 'svelte';
  import AuthCard from '$lib/components/AuthCard.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import Field from '$lib/components/Field.svelte';
  import { ApiError } from '$lib/api/client.js';
  import { checkEmail, checkName, checkPassword, safeNext, toSentence } from '$lib/forms.js';
  import { register, session } from '$lib/session.svelte.js';

  let name = $state('');
  let email = $state('');
  let password = $state('');
  let errors = $state({ name: '', email: '', password: '' });
  let formError = $state('');
  let busy = $state(false);
  /** @type {HTMLElement | null} */
  let notice = $state(null);

  $effect(() => {
    if (session.user) goto(safeNext(page.url.searchParams.get('next')), { replaceState: true });
  });

  onMount(() => document.getElementById('name')?.focus());

  /** @param {SubmitEvent} event */
  async function submit(event) {
    event.preventDefault();
    formError = '';
    errors = {
      name: checkName(name, t('Enter your name.')),
      email: checkEmail(email),
      password: checkPassword(password, { creating: true })
    };
    const invalid = Object.entries(errors).find(([, message]) => message);
    if (invalid) {
      document.getElementById(invalid[0])?.focus();
      return;
    }
    busy = true;
    try {
      await register({ name: name.trim(), email: email.trim(), password });
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        errors.email = t('An account with this email already exists. Sign in instead.');
        document.getElementById('email')?.focus();
        return;
      }
      formError = toSentence(err instanceof Error ? err.message : String(err));
      await tick();
      notice?.focus();
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>{t('Create an account · CRM')}</title>
</svelte:head>

<AuthCard title={t('Create an account')}>
  {#if formError}
    <ErrorNotice message={formError} bind:element={notice} />
  {/if}

  <form novalidate onsubmit={submit}>
    <Field id="name" label={t('Name')} autocomplete="name" required bind:value={name} error={errors.name} />
    <Field id="email" label={t('Email')} type="email" autocomplete="email" required bind:value={email} error={errors.email} />
    <Field
      id="password"
      label={t('Password')}
      type="password"
      autocomplete="new-password"
      required
      hint={t('At least 8 characters.')}
      bind:value={password}
      error={errors.password}
    />
    <button class="btn btn-primary btn-full" type="submit" disabled={busy} aria-busy={busy}>
      {busy ? 'Creating your account…' : 'Create account'}
    </button>
  </form>

  {#snippet footer()}
    {t('Already have an account?')} <a href="/login{page.url.search}">{t('Sign in')}</a>
  {/snippet}
</AuthCard>
