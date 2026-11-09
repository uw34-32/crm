<script>
  // A confirmation on the native modal dialog: the page behind is inert, Tab stays
  // inside, and Escape cancels. It opens on the safe choice, Cancel, so Enter never
  // confirms by accident, and focus returns to what opened it when it is cancelled.
  import { tick } from 'svelte';

  /**
   * @type {{
   *   open: boolean,
   *   title: string,
   *   message: string,
   *   confirmLabel: string,
   *   busyLabel?: string,
   *   busy?: boolean,
   *   error?: string,
   *   onconfirm: () => void,
   *   oncancel: () => void
   * }}
   */
  let { open, title, message, confirmLabel, busyLabel = 'Working…', busy = false, error = '', onconfirm, oncancel } = $props();

  /** @type {HTMLDialogElement | null} */
  let dialog = $state(null);
  /** @type {HTMLButtonElement | null} */
  let cancelButton = $state(null);
  /** @type {HTMLElement | null} */
  let opener = null;

  $effect(() => {
    if (!dialog) return;
    if (open && !dialog.open) {
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      dialog.showModal();
      tick().then(() => cancelButton?.focus());
    } else if (!open && dialog.open) {
      dialog.close();
    }
  });

  function cancel() {
    if (busy) return;
    oncancel();
    opener?.focus();
  }

  /** Escape fires cancel; while the action runs, the dialog stays. @param {Event} event */
  function onCancelEvent(event) {
    event.preventDefault();
    cancel();
  }

  /** A click on the backdrop lands on the dialog element itself. @param {MouseEvent} event */
  function onClick(event) {
    if (event.target === dialog) cancel();
  }
</script>

<dialog
  bind:this={dialog}
  class="modal"
  aria-labelledby="confirm-title"
  aria-describedby="confirm-text"
  oncancel={onCancelEvent}
  onclick={onClick}
>
  <h2 class="modal-title" id="confirm-title">{title}</h2>
  <p class="modal-text" id="confirm-text">{message}</p>
  {#if error}
    <p class="form-error dialog-error" role="alert">{error}</p>
  {/if}
  <div class="modal-actions">
    <button type="button" class="btn btn-secondary" bind:this={cancelButton} onclick={cancel} disabled={busy}>Cancel</button>
    <button type="button" class="btn btn-danger" onclick={onconfirm} disabled={busy} aria-busy={busy}>
      {busy ? busyLabel : confirmLabel}
    </button>
  </div>
</dialog>

<style>
  .dialog-error {
    margin: -8px 0 16px;
  }
</style>
