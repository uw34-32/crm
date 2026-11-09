<script>
  /** @type {{ label: string, rows?: number, shape?: 'person' | 'organization' }} */
  let { label, rows = 6, shape = 'person' } = $props();
</script>

<div aria-busy="true">
  <p class="sr-only" role="status">{label}</p>
  <ul class="record-list" aria-hidden="true">
    {#each Array.from({ length: rows }, (_, i) => i) as i (i)}
      <li class="list-item">
        <span class="skeleton" class:skeleton-circle={shape === 'person'} style="width:44px;height:44px;border-radius:{shape === 'person' ? '50%' : '11px'}"></span>
        <span class="list-item-content">
          <span class="skeleton skeleton-line" style="width:{45 + ((i * 13) % 30)}%;height:16px;margin-bottom:8px"></span>
          <span class="skeleton skeleton-line" style="width:{30 + ((i * 11) % 25)}%"></span>
        </span>
      </li>
    {/each}
  </ul>
</div>
