<script>
  import { goto } from '$app/navigation';
  import { session } from '$lib/session.svelte.js';

  $effect(() => {
    goto(session.user ? '/dashboard' : '/login', { replaceState: true });
  });
</script>

<h1 class="sr-only">CRM</h1>
