<script>
  import { page } from '$app/state';
</script>

<svelte:head>
  <title>{page.status === 404 ? 'Page not found' : 'Error'} · CRM</title>
</svelte:head>

<main class="error-page">
  <div class="error-card animate-in">
    <h1 class="error-title">{page.status === 404 ? 'This page does not exist' : 'Something went wrong'}</h1>
    <p class="error-message">
      {page.status === 404 ? 'Check the address, or go back to the dashboard.' : page.error?.message || 'Try again in a moment.'}
    </p>
    <a href="/dashboard" class="btn btn-primary">Go to the dashboard</a>
  </div>
</main>

<style>
  .error-page {
    min-height: 80vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px 16px;
  }

  .error-card {
    text-align: center;
    padding: 36px 28px;
    background: var(--surface);
    border-radius: 16px;
    border: 1px solid var(--border);
    max-width: 400px;
    width: 100%;
  }

  .error-title {
    font-size: 1.375rem;
    font-weight: 700;
    margin-bottom: 8px;
  }

  .error-message {
    font-size: 0.9375rem;
    color: var(--text2);
    margin-bottom: 24px;
    word-break: break-word;
  }
</style>
