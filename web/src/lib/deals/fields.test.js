import { describe, expect, it } from 'vitest';
import { dealCreateBody, dealPatchBody, validateDeal } from './fields.js';

const draft = {
  title: 'Acme renewal',
  value: '1,250.50',
  currency: 'eur',
  pipeline_id: 'p1',
  stage_id: 's1',
  expected_close_date: '2027-03-14',
  company_id: 'k1',
  contact_id: null,
  owner_id: 'u1'
};

const original = {
  id: 'd1',
  title: 'Acme renewal',
  value_minor: 125050,
  currency: 'EUR',
  pipeline_id: 'p1',
  stage_id: 's1',
  expected_close_date: '2027-03-14',
  company_id: 'k1',
  contact_id: null,
  owner_id: 'u1'
};

describe('deal validation', () => {
  it('passes a complete draft', () => {
    expect(validateDeal(draft)).toEqual({});
  });

  it('requires a title within 200 characters, a three-letter currency, and a pipeline', () => {
    expect(validateDeal({ ...draft, title: ' ' })).toEqual({ title: 'Enter a title.' });
    expect(validateDeal({ ...draft, title: 'x'.repeat(201) })).toEqual({ title: 'Use at most 200 characters.' });
    expect(validateDeal({ ...draft, currency: 'eu' })).toEqual({ currency: 'Enter a three-letter currency code such as EUR.' });
    expect(validateDeal({ ...draft, currency: 'E1R' })).toHaveProperty('currency');
    expect(validateDeal({ ...draft, pipeline_id: '' })).toEqual({ pipeline_id: 'Choose a pipeline.' });
  });

  it('checks the amount against the currency', () => {
    expect(validateDeal({ ...draft, value: '12.345' })).toEqual({ value: 'EUR amounts have at most 2 decimals.' });
    expect(validateDeal({ ...draft, value: '' })).toEqual({});
  });

  it('checks the close date is a real date between 1900 and 2200', () => {
    expect(validateDeal({ ...draft, expected_close_date: '' })).toEqual({});
    expect(validateDeal({ ...draft, expected_close_date: '2027-02-30' })).toEqual({ expected_close_date: 'Enter a date such as 2027-03-14.' });
    expect(validateDeal({ ...draft, expected_close_date: '1899-12-31' })).toEqual({ expected_close_date: 'Use a date from 1900 to 2200.' });
    expect(validateDeal({ ...draft, expected_close_date: '14/03/2027' })).toHaveProperty('expected_close_date');
  });
});

describe('the create body', () => {
  it('sends the amount in minor units, the currency in uppercase, and only the links that are set', () => {
    expect(dealCreateBody(draft)).toEqual({
      title: 'Acme renewal',
      value_minor: 125050,
      currency: 'EUR',
      pipeline_id: 'p1',
      stage_id: 's1',
      expected_close_date: '2027-03-14',
      company_id: 'k1',
      owner_id: 'u1'
    });
  });

  it('lets the API pick the first open stage and the caller as owner when they are blank', () => {
    const body = dealCreateBody({ ...draft, stage_id: '', expected_close_date: '', company_id: null, owner_id: '' , value: ''});
    expect(body).toEqual({ title: 'Acme renewal', value_minor: 0, currency: 'EUR', pipeline_id: 'p1' });
  });
});

describe('the patch body', () => {
  it('is empty when nothing changed, however the amount was typed', () => {
    expect(dealPatchBody(original, { ...draft, value: '1250.5' })).toEqual({});
  });

  it('names only the changed fields and clears a date or a link with null', () => {
    expect(dealPatchBody(original, { ...draft, title: 'Acme renewal 2027', value: '2000', expected_close_date: '', company_id: null, contact_id: 'c1', owner_id: '' })).toEqual({
      title: 'Acme renewal 2027',
      value_minor: 200000,
      expected_close_date: null,
      company_id: null,
      contact_id: 'c1',
      owner_id: null
    });
  });

  it('never sends the pipeline or the stage, which change only through a move', () => {
    expect(dealPatchBody(original, { ...draft, pipeline_id: 'p2', stage_id: 's9' })).toEqual({});
  });
});
