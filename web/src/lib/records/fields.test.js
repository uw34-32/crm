import { describe, expect, it } from 'vitest';
import { companyCreateBody, companyPatchBody, contactCreateBody, contactPatchBody, validateCompany, validateContact } from './fields.js';

const contact = {
  id: 'c1',
  first_name: 'Ada',
  last_name: 'Lovelace',
  email: 'ada@example.com',
  phone: '+44 20 7946 0000',
  title: 'Analyst',
  company_id: 'k1',
  company_name: 'Engines Ltd'
};

const company = { id: 'k1', name: 'Engines Ltd', domain: 'engines.example', phone: '', industry: 'Computing', address: '' };

describe('contact validation', () => {
  it('requires a first name', () => {
    expect(validateContact({ first_name: '  ' })).toEqual({ first_name: 'Enter a first name.' });
  });

  it('holds each field to the API limit, counting characters rather than bytes', () => {
    const errors = validateContact({
      first_name: 'é'.repeat(101),
      last_name: 'x'.repeat(101),
      title: 'x'.repeat(201),
      email: 'not an email',
      phone: 'call me'
    });

    expect(Object.keys(errors).sort()).toEqual(['email', 'first_name', 'last_name', 'phone', 'title']);
    expect(validateContact({ first_name: 'é'.repeat(100), title: 'x'.repeat(200) })).toEqual({});
  });

  it('accepts a phone with a digit up to 50 characters, as typed', () => {
    expect(validateContact({ first_name: 'Ada', phone: '(555) 010-2030 ext. 4' })).toEqual({});
    expect(validateContact({ first_name: 'Ada', phone: '1'.repeat(51) })).toHaveProperty('phone');
  });

  it('leaves optional fields alone when blank', () => {
    expect(validateContact({ first_name: 'Ada', last_name: '', email: ' ', phone: '', title: '' })).toEqual({});
  });
});

describe('company validation', () => {
  it('requires a name of at most 200 characters', () => {
    expect(validateCompany({ name: '' })).toEqual({ name: 'Enter a company name.' });
    expect(validateCompany({ name: 'x'.repeat(201) })).toHaveProperty('name');
  });

  it('takes a domain as a bare host name, refusing a scheme or a single label', () => {
    expect(validateCompany({ name: 'A', domain: 'Acme.Example' })).toEqual({});
    expect(validateCompany({ name: 'A', domain: 'https://acme.example' })).toHaveProperty('domain');
    expect(validateCompany({ name: 'A', domain: 'localhost' })).toHaveProperty('domain');
    expect(validateCompany({ name: 'A', domain: '-acme.example' })).toHaveProperty('domain');
  });

  it('holds industry and address to 100 and 500 characters', () => {
    const errors = validateCompany({ name: 'A', industry: 'x'.repeat(101), address: 'x'.repeat(501) });
    expect(Object.keys(errors).sort()).toEqual(['address', 'industry']);
  });
});

describe('create bodies', () => {
  it('trims text and leaves out blank optional fields', () => {
    expect(contactCreateBody({ first_name: ' Ada ', last_name: '', email: ' ada@example.com ', phone: '', title: '', company_id: null })).toEqual({
      first_name: 'Ada',
      email: 'ada@example.com'
    });
  });

  it('links the chosen company', () => {
    expect(contactCreateBody({ first_name: 'Ada', company_id: 'k1' })).toEqual({ first_name: 'Ada', company_id: 'k1' });
  });

  it('builds a company body the same way', () => {
    expect(companyCreateBody({ name: ' Acme ', domain: 'ACME.example', phone: '', industry: ' ', address: '' })).toEqual({
      name: 'Acme',
      domain: 'acme.example'
    });
  });
});

describe('PATCH bodies', () => {
  it('sends nothing when nothing changed, ignoring surrounding spaces', () => {
    expect(contactPatchBody(contact, { ...contact, first_name: ' Ada ' })).toEqual({});
  });

  it('sends only the fields that changed', () => {
    expect(contactPatchBody(contact, { ...contact, title: 'Lead analyst' })).toEqual({ title: 'Lead analyst' });
  });

  it('clears an optional field with an empty string, never null', () => {
    const body = contactPatchBody(contact, { ...contact, phone: '   ', email: '' });

    expect(body).toEqual({ phone: '', email: '' });
    expect(Object.values(body)).not.toContain(null);
  });

  it('unlinks the company with null, and links another by ID', () => {
    expect(contactPatchBody(contact, { ...contact, company_id: null })).toEqual({ company_id: null });
    expect(contactPatchBody(contact, { ...contact, company_id: 'k2' })).toEqual({ company_id: 'k2' });
  });

  it('treats a record field the API left null as blank', () => {
    const unlinked = { ...contact, company_id: null, title: /** @type {any} */ (null) };

    expect(contactPatchBody(unlinked, { ...unlinked, title: '' })).toEqual({});
  });

  it('compares a domain the way the API stores it, in lowercase', () => {
    expect(companyPatchBody(company, { ...company, domain: 'Engines.Example' })).toEqual({});
    expect(companyPatchBody(company, { ...company, domain: 'engines.test', industry: '' })).toEqual({ domain: 'engines.test', industry: '' });
  });
});
