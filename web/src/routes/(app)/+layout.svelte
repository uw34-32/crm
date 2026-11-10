<script>
  import { Building2, Kanban, LayoutDashboard, LogOut, Users } from '@lucide/svelte';
  import { tick } from 'svelte';
  import { afterNavigate, goto } from '$app/navigation';
  import { page } from '$app/state';
  import BrandMark from '$lib/components/BrandMark.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import { focusMainHeading, movedToAnotherPage } from '$lib/focus.js';
  import WorkspaceSwitcher from '$lib/components/WorkspaceSwitcher.svelte';
  import { session, signOut } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';
  import { loadWorkspaces, resetWorkspaces, workspaces } from '$lib/workspaces.svelte.js';

  let { children } = $props();

  const navItems = [
    { href: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { href: '/contacts', label: 'Contacts', icon: Users },
    { href: '/companies', label: 'Companies', icon: Building2 },
    { href: '/pipeline', label: 'Pipeline', icon: Kanban, also: '/deals' }
  ];

  /** @param {string} path @param {string} href */
  const under = (path, href) => path === href || path.startsWith(`${href}/`);

  /** A deal's own pages belong to the pipeline. @param {(typeof navItems)[number]} item */
  const isCurrent = (item) => under(page.url.pathname, item.href) || (item.also !== undefined && under(page.url.pathname, item.also));

  /** @type {HTMLElement | null} */
  let main = $state(null);

  // A new page starts keyboard and screen reader users at its heading.
  afterNavigate(async ({ from, to }) => {
    if (!movedToAnotherPage(from, to)) return;
    await tick();
    focusMainHeading(main);
  });

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
      <nav class="topnav" aria-label="Main">
        {#each navItems as item (item.href)}
          <a href={item.href} class="topnav-link" aria-current={isCurrent(item) ? 'page' : undefined}>{item.label}</a>
        {/each}
      </nav>
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

  <main id="main" class:wide={page.url.pathname === '/pipeline'} tabindex="-1" bind:this={main}>
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

  <nav class="tabbar" aria-label="Main">
    {#each navItems as item (item.href)}
      {@const Icon = item.icon}
      <a href={item.href} class="tab" aria-current={isCurrent(item) ? 'page' : undefined}>
        <Icon size={22} aria-hidden="true" />
        <span>{item.label}</span>
      </a>
    {/each}
  </nav>
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

  .topnav {
    display: flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
  }

  .topnav-link {
    display: inline-flex;
    align-items: center;
    min-height: 44px;
    padding: 6px 12px;
    border-radius: 8px;
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--text2);
    transition: background 0.15s, color 0.15s;
  }

  .topnav-link:hover {
    color: var(--text);
    background: var(--surface2);
    text-decoration: none;
  }

  .topnav-link[aria-current='page'] {
    color: var(--accent-text);
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }

  .tabbar {
    display: none;
  }

  .tab {
    flex: 1;
    min-width: 0;
    min-height: 56px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 2px;
    font-size: 0.6875rem;
    font-weight: 500;
    color: var(--text2);
  }

  .tab:hover {
    text-decoration: none;
  }

  /* Long labels at large text sizes truncate instead of colliding. */
  .tab > span {
    max-width: 100%;
    padding: 0 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tab[aria-current='page'] {
    color: var(--accent-text);
  }

  .tab:focus-visible {
    outline-offset: -3px;
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

  /* A board of six columns needs more than a form's width. */
  main.wide {
    max-width: 1680px;
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

  @media (max-width: 820px) {
    .account-name {
      display: none;
    }
  }

  /* Phones navigate from a bottom tab bar, as in contacts; the top bar keeps the
     workspace switcher and the account. */
  @media (max-width: 600px) {
    .brand-name,
    .account-name,
    .topnav {
      display: none;
    }

    .tabbar {
      position: fixed;
      left: 0;
      right: 0;
      bottom: 0;
      z-index: var(--z-nav);
      display: flex;
      padding-bottom: env(safe-area-inset-bottom);
      background: color-mix(in srgb, var(--surface) 92%, transparent);
      backdrop-filter: blur(20px);
      -webkit-backdrop-filter: blur(20px);
      border-top: 1px solid var(--border);
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
      padding-bottom: calc(56px + 32px + env(safe-area-inset-bottom));
    }
  }
</style>
