import { describe, expect, it } from 'vitest';
import { BridgePrometheusClient, type CompletionInvoker } from './prometheus-client';

describe('BridgePrometheusClient', () => {
  it('loads metric names once and filters them by prefix', async () => {
    const calls: Array<{ method: string; params: Record<string, unknown> }> = [];
    const call: CompletionInvoker = async (_connectionId, method, params) => {
      calls.push({ method, params });
      return { data: ['up', 'node_cpu_seconds_total', 'node_memory_MemAvailable_bytes'] };
    };
    const client = new BridgePrometheusClient('demo', call);

    await expect(client.metricNames('node_cpu')).resolves.toEqual(['node_cpu_seconds_total']);
    await expect(client.metricNames('node_memory')).resolves.toEqual(['node_memory_MemAvailable_bytes']);
    expect(calls).toEqual([{ method: 'prometheus/metric_names', params: { form: {} } }]);
  });

  it('requests labels in the context of the selected metric', async () => {
    const calls: Array<{ method: string; params: Record<string, unknown> }> = [];
    const call: CompletionInvoker = async (_connectionId, method, params) => {
      calls.push({ method, params });
      return { data: method === 'prometheus/label_names' ? ['instance', 'job', 'mode'] : ['idle', 'system', 'user'] };
    };
    const client = new BridgePrometheusClient('demo', call);

    await expect(client.labelNames('node_cpu_seconds_total')).resolves.toContain('mode');
    await expect(client.labelValues('mode', 'node_cpu_seconds_total')).resolves.toEqual(['idle', 'system', 'user']);
    expect(calls).toEqual([
      { method: 'prometheus/label_names', params: { form: { metricName: 'node_cpu_seconds_total' } } },
      { method: 'prometheus/label_values', params: { form: { labelName: 'mode', metricName: 'node_cpu_seconds_total' } } },
    ]);
  });
});
