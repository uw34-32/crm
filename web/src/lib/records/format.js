// How contacts and companies read on screen.

/** @param {{ first_name?: string | null, last_name?: string | null }} contact */
export function contactName(contact) {
  return [contact.first_name, contact.last_name].map((part) => (part ?? '').trim()).filter(Boolean).join(' ');
}

/** Up to two capitals from the first two words, or ? for a blank name. @param {string} name */
export function initials(name) {
  const letters = name
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => [...word][0].toUpperCase())
    .join('');
  return letters || '?';
}

/** @param {{ title?: string | null, company_name?: string | null }} contact */
export function contactSubtitle(contact) {
  return [contact.title, contact.company_name].map((part) => (part ?? '').trim()).filter(Boolean).join(' · ');
}
