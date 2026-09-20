import type { Alert, Envelope, QueryResult, RuleGroup, Target } from './domain';

const now = () => Math.floor(Date.now() / 1000);
const targets: Target[] = [
  { scrapePool: 'prometheus', labels: { instance: 'localhost:9090', job: 'prometheus' }, health: 'up', lastScrape: new Date().toISOString() },
  { scrapePool: 'kubernetes-pods', scrapeUrl: 'http://172.16.1.3:9253/metrics', labels: { instance: '172.16.1.3:9253', job: 'kubernetes-pods', kubernetes_namespace: 'kube-system', pod: 'nodelocaldns-tsmsn' }, discoveredLabels: { __address__: '172.16.1.3:9253', __meta_kubernetes_namespace: 'kube-system', __meta_kubernetes_pod_name: 'nodelocaldns-tsmsn' }, health: 'up', lastScrape: new Date().toISOString() },
  { scrapePool: 'node', labels: { instance: 'node-a.example:9100', job: 'node' }, health: 'up', lastScrape: new Date().toISOString() },
  { scrapePool: 'node', labels: { instance: 'node-b.example:9100', job: 'node' }, health: 'down', lastScrape: new Date().toISOString(), lastError: 'context deadline exceeded' },
];
const alerts: Alert[] = [
  { labels: { alertname: 'InstanceDown', severity: 'critical', instance: 'node-b.example:9100' }, annotations: { summary: '采集目标不可达（演示）' }, state: 'firing', activeAt: new Date(Date.now() - 8 * 60_000).toISOString(), value: '1' },
  { labels: { alertname: 'HighCPU', severity: 'warning' }, annotations: { summary: 'CPU 使用率超过阈值（演示）' }, state: 'pending', activeAt: new Date(Date.now() - 2 * 60_000).toISOString(), value: '0.83' },
];
const groups: RuleGroup[] = [
  { name: 'node.rules', file: 'rules/node.yml', rules: [
    { name: 'InstanceDown', type: 'alerting', query: 'up == 0', health: 'ok', state: 'firing' },
    { name: 'job:node_cpu_usage:rate5m', type: 'recording', query: 'rate(node_cpu_seconds_total[5m])', health: 'ok' },
  ] },
];
const metrics = [
  'go_goroutines',
  'http_requests_total',
  'job:node_cpu_usage:rate5m',
  'node_cpu_seconds_total',
  'node_filesystem_avail_bytes',
  'node_memory_MemAvailable_bytes',
  'process_cpu_seconds_total',
  'prometheus_engine_queries',
  'up',
];
const runtimeInfo = {
  startTime: new Date(Date.now() - 3 * 24 * 60 * 60_000).toISOString(),
  cwd: '/opt/prometheus',
  reloadConfigSuccess: true,
  lastConfigTime: new Date(Date.now() - 2 * 60 * 60_000).toISOString(),
  corruptionCount: 0,
  goroutineCount: 86,
  GOMAXPROCS: 8,
  GOMEMLIMIT: 7516192768,
  GOGC: '75',
};
const tsdbStatus = {
  headStats: { numSeries: 280800, numLabelPairs: 560810, chunkCount: 842400, minTime: Date.now() - 90 * 60_000, maxTime: Date.now() },
  seriesCountByMetricName: metrics.slice(0, 6).map((name, index) => ({ name, value: 4018 - index * 377 })),
  labelValueCountByLabelName: ['__name__', 'name', 'id', 'job', 'instance', 'device'].map((name, index) => ({ name, value: 4018 - index * 521 })),
  memoryInBytesByLabelName: ['__name__', 'instance', 'job', 'pod'].map((name, index) => ({ name, value: 18874368 - index * 2097152 })),
  seriesCountByLabelValuePair: ['job=node', 'job=prometheus', 'mode=idle', 'mode=user'].map((name, index) => ({ name, value: 3200 - index * 580 })),
};
const flags = {
  'config.file': '/etc/prometheus/prometheus.yml',
  'storage.tsdb.path': '/prometheus',
  'storage.tsdb.retention.time': '15d',
  'web.enable-lifecycle': 'true',
  'web.listen-address': '0.0.0.0:9090',
};
const configYaml = `global:\n  scrape_interval: 15s\n  evaluation_interval: 15s\nscrape_configs:\n  - job_name: prometheus\n    static_configs:\n      - targets: [localhost:9090]\n  - job_name: node\n    static_configs:\n      - targets: [node-a.example:9100]\n`;
const droppedTargets: Target[] = [
  { discoveredLabels: { __address__: 'node-retired.example:9100', job: 'node', reason: 'relabel_drop' } },
];
const scrapePools = ['kubernetes-pods', 'kubernetes-summary', 'node', 'prometheus', 'serviceMonitor/nanzang-parking/gateway-servicemonitor/0'];
const discoveryActiveTargets: Target[] = Array.from({ length: 47 }, (_, index) => ({
  scrapePool: 'kubernetes-pods',
  scrapeUrl: `http://172.16.1.${index + 10}:9253/metrics`,
  labels: { job: 'kubernetes-pods', instance: `172.16.1.${index + 10}:9253`, kubernetes_namespace: index % 2 ? 'monitoring' : 'kube-system' },
  discoveredLabels: { __address__: `172.16.1.${index + 10}:9253`, __meta_kubernetes_pod_name: `metrics-agent-${String(index + 1).padStart(2, '0')}` },
  health: index % 9 ? 'up' : 'down',
  lastScrape: new Date(Date.now() - index * 3000).toISOString(),
}));
const discoveryDroppedTargets: Target[] = Array.from({ length: 23 }, (_, index) => ({
  scrapePool: 'kubernetes-pods',
  discoveredLabels: { __address__: `10.20.0.${index + 1}:9100`, job: 'kubernetes-pods', reason: 'relabel_drop' },
}));

function queryResult(method: string, form: Record<string, unknown>): Envelope<QueryResult> {
  const range = method === 'prometheus/query_range';
  const end = Number(form.end) || now();
  const start = Number(form.start) || end - 3600;
  const count = Math.min(60, Math.max(2, Math.floor((end - start) / (Number(form.step) || 30))));
  const series = [
    { metric: { __name__: 'up', job: 'prometheus', instance: 'localhost:9090' }, value: '1' },
    { metric: { __name__: 'up', job: 'node', instance: 'node-a.example:9100' }, value: '1' },
    { metric: { __name__: 'up', job: 'node', instance: 'node-b.example:9100' }, value: '0' },
  ];
  return { data: range
    ? { resultType: 'matrix', result: series.map((s, i) => ({ metric: s.metric, values: Array.from({ length: count }, (_, j) => [start + (end - start) * j / (count - 1), i === 2 && j % 9 === 0 ? '1' : s.value] as [number, string]) })) }
    : { resultType: 'vector', result: series.map(s => ({ metric: s.metric, value: [end, s.value] as [number, string] })) } };
}

const disconnected = new URLSearchParams(window.location.search).get('disconnected') === '1';

window.dbxPlugin = {
  ready: Promise.resolve(),
  context: disconnected ? {} : { connectionId: 'preview-only' },
  onContext: () => () => {},
  async invoke<T>(method: string, params: Record<string, unknown>): Promise<T> {
    const form = (params.form || {}) as Record<string, unknown>;
    let response: unknown;
    switch (method) {
      case 'prometheus/info': response = { name: 'Prometheus 演示连接', baseUrl: 'http://localhost:9090', environment: '演示环境', build: { data: { version: '3.0.0-demo' } } }; break;
      case 'prometheus/query':
      case 'prometheus/query_range': response = queryResult(method, params.form as Record<string, unknown>); break;
      case 'prometheus/targets': response = { data: { activeTargets: targets } }; break;
      case 'prometheus/alerts': response = { data: { alerts } }; break;
      case 'prometheus/rules': response = { data: { groups } }; break;
      case 'prometheus/metric_names': response = { data: metrics }; break;
      case 'prometheus/label_names': response = { data: form.metricName === 'node_cpu_seconds_total' ? ['cpu', 'instance', 'job', 'mode'] : ['instance', 'job'] }; break;
      case 'prometheus/label_values': response = { data: form.labelName === 'mode' ? ['idle', 'iowait', 'system', 'user'] : form.labelName === 'job' ? ['node', 'prometheus'] : ['localhost:9090', 'node-a.example:9100'] }; break;
      case 'prometheus/status_runtime': response = { data: runtimeInfo }; break;
      case 'prometheus/status_tsdb': response = { data: tsdbStatus }; break;
      case 'prometheus/status_flags': response = { data: flags }; break;
      case 'prometheus/status_config': response = { data: { yaml: configYaml } }; break;
      case 'prometheus/service_discovery_services': response = { data: { scrapePools } }; break;
      case 'prometheus/service_discovery': {
        const stateTargets = form.state === 'dropped' ? discoveryDroppedTargets : discoveryActiveTargets;
        const selected = Array.isArray(form.scrapePools) ? form.scrapePools.map(String) : [String(form.scrapePool || '')].filter(Boolean);
        const matching = selected.flatMap(scrapePool => scrapePool === 'kubernetes-pods'
          ? stateTargets
          : stateTargets.slice(0, scrapePool === 'node' ? 7 : 3).map(item => ({ ...item, scrapePool, labels: { ...item.labels, job: scrapePool } })));
        const page = Number(form.page) || 1;
        const pageSize = Number(form.pageSize) || 20;
        const start = (page - 1) * pageSize;
        response = { data: { items: matching.slice(start, start + pageSize), total: matching.length, page, pageSize, hasNext: start + pageSize < matching.length, scrapePools: selected, state: form.state } };
        break;
      }
      default: throw new Error(`未知演示接口: ${method}`);
    }
    return response as T;
  },
};

void import('./main');
