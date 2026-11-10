<script>
  // Picks the contact a deal is linked to; see RecordPicker.
  import RecordPicker from './RecordPicker.svelte';

  /** @type {{ id: string, workspaceId: string, value: { id: string, name: string } | null, error?: string }} */
  let { id, workspaceId, value = $bindable(null), error = '' } = $props();
</script>

<RecordPicker {id} kind="contacts" {workspaceId} bind:value {error} />
