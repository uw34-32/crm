import { ApiError } from '../src/api/client';
import { loadRecord } from '../src/lib/records/load';

describe('loading one record', () => {
  it('answers the record', async () => {
    expect(await loadRecord(async () => ({ id: 'c1' }))).toEqual({ status: 'ready', record: { id: 'c1' }, error: '' });
  });

  it('calls a 404 missing, whether deleted, absent, or in another workspace', async () => {
    const got = await loadRecord(async () => {
      throw new ApiError(404, 'contact not found');
    });

    expect(got).toEqual({ status: 'missing', record: null, error: '' });
  });

  it('reports any other failure, so it can be retried', async () => {
    const got = await loadRecord(async () => {
      throw new ApiError(0, 'Cannot reach the server.');
    });

    expect(got).toEqual({ status: 'error', record: null, error: 'Cannot reach the server.' });
  });
});
