import { describe, expect, it } from 'vitest';
import { pipelineFromUrl, withPipeline } from './url.js';

describe('the pipeline in the URL', () => {
  it('reads ?pipeline=, or nothing', () => {
    expect(pipelineFromUrl(new URL('http://x/pipeline?pipeline=p1'))).toBe('p1');
    expect(pipelineFromUrl(new URL('http://x/pipeline'))).toBe('');
  });

  it('writes the pipeline, keeping other parameters, and removes it when blank', () => {
    expect(withPipeline(new URL('http://x/pipeline?x=1'), 'p2')).toBe('/pipeline?x=1&pipeline=p2');
    expect(withPipeline(new URL('http://x/pipeline?pipeline=p1'), '')).toBe('/pipeline');
  });
});
