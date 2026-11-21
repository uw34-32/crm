// How contacts and companies read on screen.

type Nullable = string | null | undefined;

export function contactName(contact: { first_name?: Nullable; last_name?: Nullable }) {
  return [contact.first_name, contact.last_name].map((part) => (part ?? '').trim()).filter(Boolean).join(' ');
}

/** Up to two capitals from the first two words, or ? for a blank name. */
export function initials(name: string) {
  const letters = name
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => [...[...word][0].toUpperCase()][0])
    .join('');
  return letters || '?';
}

export function contactSubtitle(contact: { title?: Nullable; company_name?: Nullable }) {
  return [contact.title, contact.company_name].map((part) => (part ?? '').trim()).filter(Boolean).join(' · ');
}
