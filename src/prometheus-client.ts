import type { PrometheusClient } from '@prometheus-io/codemirror-promql';
import type { Envelope } from './domain';
import { invoke } from './bridge';

export type CompletionInvoker = (
  connectionId: string,
  method: string,
  params: Record<string, unknown>,
) => Promise<unknown>;

const defaultInvoker: CompletionInvoker = (connectionId, method, params) => invoke(connectionId, method, params);

export class BridgePrometheusClient implements PrometheusClient {
  private metricNamesRequest?: Promise<string[]>;
  private readonly labelNameRequests = new Map<string, Promise<string[]>>();
  private readonly labelValueRequests = new Map<string, Promise<string[]>>();

  constructor(
    private readonly connectionId: string,
    private readonly call: CompletionInvoker = defaultInvoker,
  ) {}

  async metricNames(prefix = ''): Promise<string[]> {
    this.metricNamesRequest ??= this.readList('prometheus/metric_names', {});
    const metrics = await this.metricNamesRequest;
    return prefix ? metrics.filter(metric => metric.startsWith(prefix)) : metrics;
  }

  labelNames(metricName = ''): Promise<string[]> {
    const key = metricName;
    if (!this.labelNameRequests.has(key)) {
      this.labelNameRequests.set(key, this.readList('prometheus/label_names', metricName ? { metricName } : {}));
    }
    return this.labelNameRequests.get(key)!;
  }

  labelValues(labelName: string, metricName = ''): Promise<string[]> {
    const key = `${metricName}\u0000${labelName}`;
    if (!this.labelValueRequests.has(key)) {
      this.labelValueRequests.set(key, this.readList('prometheus/label_values', { labelName, ...(metricName ? { metricName } : {}) }));
    }
    return this.labelValueRequests.get(key)!;
  }

  metricMetadata(): Promise<Record<string, Array<{ type: string; help: string }>>> {
    return Promise.resolve({});
  }

  series(): Promise<Map<string, string>[]> {
    return Promise.resolve([]);
  }

  flags(): Promise<Record<string, string>> {
    return Promise.resolve({});
  }

  private async readList(method: string, form: Record<string, string>): Promise<string[]> {
    const response = await this.call(this.connectionId, method, { form }) as Envelope<string[]>;
    return Array.isArray(response.data) ? response.data : [];
  }
}
