<script>
  // Picks the company a contact or deal is linked to; see RecordPicker.
  import RecordPicker from './RecordPicker.svelte';

  /** @type {{ id: string, workspaceId: string, value: { id: string, name: string } | null, error?: string }} */
  let { id, workspaceId, value = $bindable(null), error = '' } = $props();
</script>

<RecordPicker {id} kind="companies" {workspaceId} bind:value {error} />
