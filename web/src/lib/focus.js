// Where focus goes after a navigation. A new page moves focus to its heading, so a
// keyboard or screen reader user starts at the content instead of the top bar. A
// change of search on the same page leaves focus in the search field.

/**
 * @param {{ url: URL } | null} from
 * @param {{ url: URL } | null} to
 */
export function movedToAnotherPage(from, to) {
  return Boolean(from && to && from.url.pathname !== to.url.pathname);
}

/**
 * Focuses the main heading, or main itself when the page has none yet, unless the
 * page already put focus somewhere inside main, such as a form's first field.
 * @param {HTMLElement | null} main
 */
export function focusMainHeading(main) {
  if (!main) return;
  const active = document.activeElement;
  if (active && active !== document.body && active !== main && main.contains(active)) return;
  const heading = main.querySelector('h1');
  if (heading instanceof HTMLElement) {
    heading.tabIndex = -1;
    heading.focus({ preventScroll: true });
  } else {
    main.focus({ preventScroll: true });
  }
}
