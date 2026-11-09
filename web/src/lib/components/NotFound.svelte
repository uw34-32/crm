<script>
  // The answer for a record that was deleted, never existed, or belongs to another
  // workspace: the API does not say which, and neither does this.
  /** @type {{ title: string, backHref: string, backLabel: string }} */
  let { title, backHref, backLabel } = $props();
</script>

<div class="empty-state not-found animate-in">
  <h1 class="page-title not-found-title">{title}</h1>
  <p class="empty-state-text">It may have been deleted, or it belongs to another workspace than the one that is open.</p>
  <a class="btn btn-secondary" href={backHref}>{backLabel}</a>
</div>

<style>
  .not-found-title {
    margin-bottom: 8px;
  }
</style>
