import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { debounce } from './debounce.js';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('debounce', () => {
  it('runs once with the last arguments after a quiet moment', () => {
    const fn = vi.fn();
    const later = debounce(fn, 300);

    later('a');
    vi.advanceTimersByTime(200);
    later('ab');
    vi.advanceTimersByTime(299);
    expect(fn).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(fn).toHaveBeenCalledExactlyOnceWith('ab');
  });

  it('can be cancelled, as when the screen closes', () => {
    const fn = vi.fn();
    const later = debounce(fn, 300);

    later('a');
    later.cancel();
    vi.advanceTimersByTime(1000);

    expect(fn).not.toHaveBeenCalled();
  });
});
