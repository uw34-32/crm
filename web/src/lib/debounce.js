/**
 * Calls fn once input has been quiet for ms, with the last arguments.
 * @template {unknown[]} A
 * @param {(...args: A) => void} fn
 * @param {number} ms
 */
export function debounce(fn, ms) {
  /** @type {ReturnType<typeof setTimeout> | undefined} */
  let timer;
  /** @param {A} args */
  const later = (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
  later.cancel = () => clearTimeout(timer);
  return later;
}
