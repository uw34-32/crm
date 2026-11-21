import { checkEmail, checkName, checkPassword, toSentence } from '../src/lib/forms';

describe('forms', () => {
  it('checks the shape of an email', () => {
    expect(checkEmail('')).not.toBe('');
    expect(checkEmail('ada')).not.toBe('');
    expect(checkEmail(' ada@example.com ')).toBe('');
  });

  it('holds only a new password to the limits', () => {
    expect(checkPassword('x', { creating: false })).toBe('');
    expect(checkPassword('', { creating: false })).not.toBe('');
    expect(checkPassword('short', { creating: true })).toMatch(/at least 8/);
    expect(checkPassword('é'.repeat(40), { creating: true })).toMatch(/too long/);
    expect(checkPassword('longenough', { creating: true })).toBe('');
  });

  it('checks a name and phrases API messages as sentences', () => {
    expect(checkName('  ', 'Enter a name.')).toBe('Enter a name.');
    expect(checkName('a'.repeat(201), 'x')).toMatch(/at most 200/);
    expect(toSentence('invalid credentials')).toBe('Invalid credentials.');
    expect(toSentence('Already done.')).toBe('Already done.');
  });
});
