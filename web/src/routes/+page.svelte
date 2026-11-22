<script>
  import { t } from '$lib/i18n.svelte.js';
  import { goto } from '$app/navigation';
  import { session } from '$lib/session.svelte.js';

  $effect(() => {
    goto(session.user ? '/dashboard' : '/login', { replaceState: true });
  });
</script>

<h1 class="sr-only">{t('CRM')}</h1>
