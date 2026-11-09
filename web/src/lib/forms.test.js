import { describe, expect, it } from 'vitest';
import { checkEmail, checkName, checkPassword, safeNext } from './forms.js';

describe('checkEmail', () => {
  it.each([
    ['', 'Enter your email address.'],
    ['   ', 'Enter your email address.'],
    ['ada', 'Enter an email address such as ada@example.com.'],
    ['ada@', 'Enter an email address such as ada@example.com.'],
    ['@example.com', 'Enter an email address such as ada@example.com.'],
    ['ada@example', 'Enter an email address such as ada@example.com.']
  ])('refuses %j', (value, message) => {
    expect(checkEmail(value)).toBe(message);
  });

  it('accepts an address', () => {
    expect(checkEmail(' ada@example.com ')).toBe('');
  });
});

describe('checkPassword', () => {
  it('needs a password', () => {
    expect(checkPassword('', { creating: false })).toBe('Enter your password.');
  });

  it('only checks the length of a new password', () => {
    expect(checkPassword('short', { creating: false })).toBe('');
    expect(checkPassword('short', { creating: true })).toBe('Use at least 8 characters.');
  });

  it('counts bytes, as the API does, for the upper limit', () => {
    expect(checkPassword('x'.repeat(72), { creating: true })).toBe('');
    expect(checkPassword('x'.repeat(73), { creating: true })).toMatch(/too long/i);
    // 37 two-byte letters are 74 bytes.
    expect(checkPassword('é'.repeat(37), { creating: true })).toMatch(/too long/i);
  });
});

describe('checkName', () => {
  it('needs a name of at most 200 characters', () => {
    expect(checkName('  ', 'Enter your name.')).toBe('Enter your name.');
    expect(checkName('x'.repeat(201), 'Enter your name.')).toBe('Use at most 200 characters.');
    expect(checkName('é'.repeat(200), 'Enter your name.')).toBe('');
  });
});

describe('safeNext', () => {
  it.each([
    ['/dashboard', '/dashboard'],
    ['/workspaces/new?x=1', '/workspaces/new?x=1'],
    [null, '/dashboard'],
    ['', '/dashboard'],
    ['https://evil.example', '/dashboard'],
    ['//evil.example', '/dashboard'],
    ['/\\evil.example', '/dashboard'],
    ['/login', '/dashboard'],
    ['/register', '/dashboard']
  ])('sends %j to %j', (next, want) => {
    expect(safeNext(next)).toBe(want);
  });
});
