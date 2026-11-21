/** Calls fn once input has been quiet for ms, with the last arguments. */
export function debounce<A extends unknown[]>(fn: (...args: A) => void, ms: number) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const later = (...args: A) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
  later.cancel = () => clearTimeout(timer);
  return later;
}
