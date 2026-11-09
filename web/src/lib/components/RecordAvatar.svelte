<script>
  // Initials in a tinted disc for a person, or a rounded square for an organization.
  import { initials } from '$lib/records/format.js';

  /** @type {{ name: string, size?: number, shape?: 'person' | 'organization' }} */
  let { name, size = 44, shape = 'person' } = $props();
</script>

<span class="avatar" class:organization={shape === 'organization'} style:--size="{size}px" aria-hidden="true">
  {initials(name)}
</span>

<style>
  .avatar {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--size);
    height: var(--size);
    border-radius: 50%;
    background: var(--surface2);
    color: var(--accent-text);
    font-size: calc(var(--size) * 0.36);
    font-weight: 600;
    line-height: 1;
    overflow: hidden;
  }

  .organization {
    border-radius: 25%;
  }
</style>
