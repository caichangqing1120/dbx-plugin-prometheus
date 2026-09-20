import { describe, expect, it } from 'vitest';
import { plot, seriesName } from './domain';

describe('Prometheus presentation', () => {
  it('shows metric labels in a stable order', () => {
    expect(seriesName({ instance: 'node:9090', __name__: 'up', job: 'prom' })).toBe('up{instance="node:9090", job="prom"}');
  });
  it('handles a flat range and filters non-finite samples', () => {
    expect(plot([[10, '2'], [20, '2']])).toBe('0.00,230.00 900.00,230.00');
    expect(plot([[10, 'NaN']])).toBe('');
    expect(plot([])).toBe('');
  });
});
