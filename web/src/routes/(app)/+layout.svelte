<script>
  import { LogOut } from '@lucide/svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import BrandMark from '$lib/components/BrandMark.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import WorkspaceSwitcher from '$lib/components/WorkspaceSwitcher.svelte';
  import { session, signOut } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';
  import { loadWorkspaces, resetWorkspaces, workspaces } from '$lib/workspaces.svelte.js';

  let { children } = $props();

  // Signing out on purpose goes to a plain sign-in page; a session that ended, or a
  // visit with none, comes back here afterwards.
  let leaving = false;

  // The guard: the API is the real one, and this keeps the screens to signed-in users.
  $effect(() => {
    if (!session.user) {
      resetWorkspaces();
      const next = page.url.pathname + page.url.search;
      goto(leaving ? '/login' : `/login?next=${encodeURIComponent(next)}`, { replaceState: true });
      return;
    }
    if (workspaces.userId !== session.user.id) loadWorkspaces();
  });

  // A user with no workspace has nothing to open until they create one.
  $effect(() => {
    if (workspaces.status === 'ready' && workspaces.list.length === 0 && page.url.pathname !== '/workspaces/new') {
      goto('/workspaces/new', { replaceState: true });
    }
  });

  async function handleSignOut() {
    leaving = true;
    await signOut();
    showToast('Signed out');
  }
</script>

{#if session.user}
  <a class="skip-link" href="#main">Skip to content</a>
  <header class="topbar">
    <div class="topbar-inner">
      <a href="/dashboard" class="brand" aria-label="CRM dashboard">
        <BrandMark />
        <span class="brand-name" aria-hidden="true">CRM</span>
      </a>
      <WorkspaceSwitcher />
      <div class="account">
        <span class="account-name" title={session.user.email}>{session.user.name}</span>
        <button type="button" class="btn-ghost sign-out" onclick={handleSignOut}>
          <LogOut size={18} aria-hidden="true" />
          <span class="sign-out-label">Sign out</span>
        </button>
      </div>
    </div>
  </header>

  <main id="main" tabindex="-1">
    {#if workspaces.status === 'error'}
      <ErrorNotice message={workspaces.error} onretry={loadWorkspaces} />
    {:else if workspaces.status === 'ready'}
      {@render children()}
    {:else}
      <div class="loading" aria-busy="true">
        <p class="sr-only" role="status">Loading your workspaces</p>
        <span class="skeleton title-skeleton"></span>
        <span class="skeleton skeleton-line line-skeleton"></span>
        <span class="skeleton card-skeleton"></span>
      </div>
    {/if}
  </main>
{/if}

<style>
  .skip-link {
    position: absolute;
    left: 16px;
    top: -100px;
    z-index: var(--z-modal);
    padding: 10px 16px;
    border-radius: var(--radius);
    background: var(--accent-strong);
    color: #fff;
    font-weight: 600;
  }

  .skip-link:focus {
    top: 8px;
  }

  .topbar {
    position: sticky;
    top: 0;
    z-index: var(--z-nav);
    height: 64px;
    display: flex;
    align-items: center;
    background: color-mix(in srgb, var(--surface) 92%, transparent);
    backdrop-filter: blur(20px);
    -webkit-backdrop-filter: blur(20px);
    border-bottom: 1px solid var(--border);
  }

  .topbar-inner {
    width: 100%;
    max-width: 960px;
    margin: 0 auto;
    padding: 0 16px;
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .brand {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    min-height: 44px;
    flex-shrink: 0;
    color: var(--text);
    font-size: 1.25rem;
    font-weight: 700;
    letter-spacing: -0.3px;
  }

  .brand:hover {
    text-decoration: none;
  }

  .topbar-inner > :global(.switcher) {
    flex: 1;
  }

  .account {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-shrink: 0;
    margin-left: auto;
  }

  .account-name {
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.875rem;
    color: var(--text2);
  }

  main {
    max-width: 960px;
    margin: 0 auto;
    padding: 24px 16px calc(40px + env(safe-area-inset-bottom));
  }

  main:focus {
    outline: none;
  }

  .loading {
    display: grid;
    gap: 12px;
  }

  .title-skeleton {
    width: 40%;
    height: 28px;
  }

  .line-skeleton {
    width: 60%;
  }

  .card-skeleton {
    height: 140px;
    border-radius: var(--radius);
    margin-top: 12px;
  }

  @media (max-width: 600px) {
    .brand-name,
    .account-name {
      display: none;
    }

    .sign-out {
      min-width: 44px;
      padding: 10px;
    }

    /* The label stays for screen readers; the icon carries it on screen. */
    .sign-out-label {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }

    main {
      padding-top: 20px;
    }
  }
</style>
