export const toast = $state({ message: '' });

/** @type {ReturnType<typeof setTimeout> | undefined} */
let timer;

/** Confirms an action for a moment. @param {string} message */
export function showToast(message, ms = 2500) {
  toast.message = message;
  clearTimeout(timer);
  timer = setTimeout(() => {
    toast.message = '';
  }, ms);
}
