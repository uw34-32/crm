<script>
  // An announced error with an optional retry. It takes focus when a form fails, so a
  // keyboard or screen reader user lands on what went wrong. Styling lives in app.css.
  /** @type {{ message: string, onretry?: () => void, retryLabel?: string, element?: HTMLElement | null }} */
  let { message, onretry, retryLabel = 'Try again', element = $bindable(null) } = $props();
</script>

<div class="load-error" role="alert" tabindex="-1" bind:this={element}>
  <span>{message}</span>
  {#if onretry}
    <button type="button" class="btn btn-secondary" onclick={onretry}>{retryLabel}</button>
  {/if}
</div>
