<script>
  import BrandMark from './BrandMark.svelte';

  /** @type {{ title: string, children: import('svelte').Snippet, footer: import('svelte').Snippet }} */
  let { title, children, footer } = $props();
</script>

<main class="auth-page">
  <div class="auth-card animate-in">
    <div class="auth-brand">
      <BrandMark size={36} />
      <span>CRM</span>
    </div>
    <h1 class="auth-title">{title}</h1>
    {@render children()}
    <p class="auth-footer">{@render footer()}</p>
  </div>
</main>

<style>
  .auth-page {
    min-height: 100vh;
    min-height: 100dvh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px 16px;
  }

  .auth-card {
    width: 100%;
    max-width: 380px;
    padding: 32px 28px;
    background: var(--surface);
    border-radius: 16px;
    border: 1px solid var(--border);
  }

  .auth-brand {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    margin-bottom: 20px;
    font-size: 1.125rem;
    font-weight: 700;
    letter-spacing: -0.3px;
  }

  .auth-title {
    font-size: 1.5rem;
    font-weight: 700;
    line-height: 1.25;
    text-align: center;
    margin-bottom: 24px;
    letter-spacing: -0.3px;
  }

  .auth-footer {
    text-align: center;
    margin-top: 20px;
    font-size: 0.875rem;
    color: var(--text2);
  }

  @media (max-width: 400px) {
    .auth-card {
      padding: 28px 20px;
    }
  }
</style>
