<script>
  // A placeholder mark: three bars that narrow like a pipeline. Decorative, so the
  // name next to it, or the link around it, carries the meaning.
  let { size = 28 } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" focusable="false">
  <rect width="32" height="32" rx="8" fill="var(--accent-strong)" />
  <rect x="8" y="9" width="16" height="3" rx="1.5" fill="#fff" />
  <rect x="8" y="14.5" width="11" height="3" rx="1.5" fill="#fff" />
  <rect x="8" y="20" width="6" height="3" rx="1.5" fill="#fff" />
</svg>
