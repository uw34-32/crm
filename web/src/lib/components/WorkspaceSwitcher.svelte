<script>
  import { Plus } from '@lucide/svelte';
  import { showToast } from '$lib/toast.svelte.js';
  import { selectWorkspace, workspaces } from '$lib/workspaces.svelte.js';

  /** @param {Event & { currentTarget: HTMLSelectElement }} event */
  function change(event) {
    selectWorkspace(event.currentTarget.value);
    if (workspaces.current) showToast(`Switched to ${workspaces.current.name}`);
  }
</script>

<div class="switcher">
  {#if workspaces.list.length > 0}
    <label for="workspace-switcher" class="sr-only">Workspace</label>
    <select id="workspace-switcher" class="select switcher-select" value={workspaces.current?.id} onchange={change}>
      {#each workspaces.list as workspace (workspace.id)}
        <option value={workspace.id}>{workspace.name}</option>
      {/each}
    </select>
  {/if}
  <a href="/workspaces/new" class="icon-btn" aria-label="Create a workspace" title="Create a workspace">
    <Plus size={20} aria-hidden="true" />
  </a>
</div>

<style>
  .switcher {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
  }

  .switcher-select {
    min-width: 0;
    width: 100%;
    max-width: 260px;
    min-height: 44px;
    padding: 8px 36px 8px 12px;
    font-size: 0.9375rem;
    font-weight: 600;
    text-overflow: ellipsis;
  }
</style>
