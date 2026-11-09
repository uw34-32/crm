import { describe, expect, it, vi } from 'vitest';
import { recordsApi } from './records.js';

function client() {
  return { request: vi.fn(async () => null) };
}

describe('records API', () => {
  it('lists a page with limit and offset, and q only when there is a search', async () => {
    const c = client();
    const contacts = recordsApi(c, 'contacts');

    await contacts.list('w 1', { q: '', limit: 50, offset: 0 });
    await contacts.list('w1', { q: 'ada & co', limit: 50, offset: 100 });

    expect(c.request.mock.calls.map((call) => /** @type {unknown[]} */ (call)[0])).toEqual([
      '/workspaces/w%201/contacts?limit=50&offset=0',
      '/workspaces/w1/contacts?limit=50&offset=100&q=ada+%26+co'
    ]);
  });

  it('reads, creates, updates, and deletes one record', async () => {
    const c = client();
    const companies = recordsApi(c, 'companies');

    await companies.get('w1', 'k1');
    await companies.create('w1', { name: 'Acme' });
    await companies.update('w1', 'k1', { industry: '' });
    await companies.remove('w1', 'k/1');

    expect(c.request.mock.calls).toEqual([
      ['/workspaces/w1/companies/k1'],
      ['/workspaces/w1/companies', { method: 'POST', body: { name: 'Acme' } }],
      ['/workspaces/w1/companies/k1', { method: 'PATCH', body: { industry: '' } }],
      ['/workspaces/w1/companies/k%2F1', { method: 'DELETE' }]
    ]);
  });
});
