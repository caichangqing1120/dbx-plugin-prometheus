import { describe, expect, it } from 'vitest';
import {
  addHistoryEntry,
  alertFilterOptions,
  connectionExperience,
  createPanel,
  discoveryPageRange,
  filterAlertsByOption,
  filterRuleGroupsByOption,
  filterScrapePools,
  filterTargetsByOption,
  queryWindow,
  ruleFilterOptions,
  shiftEvaluationTime,
  targetFilterOptions,
  toggleScrapePool,
} from './workbench';
import type { Alert, RuleGroup, Target } from './domain';

describe('PromQL workbench state', () => {
  it('creates independent query panels with stable defaults', () => {
    expect(createPanel('panel-2')).toMatchObject({ id: 'panel-2', expression: '', view: 'graph', loading: false });
  });

  it('keeps recent unique query history', () => {
    expect(addHistoryEntry(['rate(http_requests_total[5m])', 'up'], 'up', 3)).toEqual(['up', 'rate(http_requests_total[5m])']);
    expect(addHistoryEntry(['b', 'c', 'd'], 'a', 3)).toEqual(['a', 'b', 'c']);
    expect(addHistoryEntry(['up'], '   ', 3)).toEqual(['up']);
  });

  it('uses the selected evaluation time for range queries', () => {
    expect(queryWindow(new Date('2026-09-20T08:00:00Z'), 1)).toEqual({ start: 1789887600, end: 1789891200 });
    expect(shiftEvaluationTime(new Date('2026-09-20T08:00:00Z'), -5).toISOString()).toBe('2026-09-20T07:55:00.000Z');
  });

  it('shows a connection-opening guide for an unbound workbench tab', () => {
    expect(connectionExperience('')).toEqual({
      connected: false,
      title: '先打开 Prometheus 连接',
      description: '当前标签页未绑定连接。请在 DBX 左侧连接列表中双击 Prometheus 连接，打开对应的工作台。',
    });
    expect(connectionExperience('connection-1')).toEqual({ connected: true, title: '', description: '' });
  });

  it('filters service discovery scrape pools without loading targets', () => {
    expect(filterScrapePools(['node', 'kubernetes-pods', 'node', 'serviceMonitor/gateway/0'], 'node')).toEqual(['node']);
    expect(filterScrapePools(['node', 'kubernetes-pods', 'serviceMonitor/gateway/0'], 'service')).toEqual(['serviceMonitor/gateway/0']);
    expect(filterScrapePools(Array.from({ length: 15 }, (_, index) => `service-${index}`), '')).toHaveLength(15);
  });

  it('keeps selection separate from the service search keyword', () => {
    expect(toggleScrapePool([], 'node')).toEqual(['node']);
    expect(toggleScrapePool(['node'], 'kubernetes-pods')).toEqual(['node', 'kubernetes-pods']);
    expect(toggleScrapePool(['node', 'kubernetes-pods'], 'node')).toEqual(['kubernetes-pods']);
  });

  it('describes discovery pagination boundaries', () => {
    expect(discoveryPageRange(2, 20, 45)).toEqual({ from: 21, to: 40, canPrevious: true, canNext: true });
    expect(discoveryPageRange(3, 20, 45)).toEqual({ from: 41, to: 45, canPrevious: true, canNext: false });
    expect(discoveryPageRange(1, 20, 0)).toEqual({ from: 0, to: 0, canPrevious: false, canNext: false });
  });

  it('builds exact selectable options for scrape targets', () => {
    const items: Target[] = [
      { scrapePool: 'node', labels: { instance: 'node-a:9100', job: 'node' }, health: 'up' },
      { scrapePool: 'node', labels: { instance: 'node-a:9100', job: 'node' }, health: 'up' },
      { scrapePool: 'pods', scrapeUrl: 'http://10.0.0.2:9253/metrics', health: 'down' },
    ];
    const options = targetFilterOptions(items);
    expect(options.map(item => ({ label: item.label, description: item.description }))).toEqual([
      { label: 'node-a:9100', description: 'node · up' },
      { label: 'http://10.0.0.2:9253/metrics', description: 'pods · down' },
    ]);
    expect(filterTargetsByOption(items, options[1].value)).toEqual([items[2]]);
    expect(filterTargetsByOption(items, '')).toEqual(items);
  });

  it('offers alert targets without requiring a typed keyword', () => {
    const items: Alert[] = [
      { labels: { alertname: 'InstanceDown', instance: 'node-b:9100' }, state: 'firing', activeAt: '2026-09-21T04:00:00Z' },
      { labels: { alertname: 'HighCPU' }, state: 'pending', activeAt: '2026-09-21T04:01:00Z' },
    ];
    const options = alertFilterOptions(items);
    expect(options.map(item => item.label)).toEqual(['InstanceDown · node-b:9100', 'HighCPU']);
    expect(filterAlertsByOption(items, options[0].value)).toEqual([items[0]]);
  });

  it('filters a selected rule without showing unrelated rules in its group', () => {
    const items: RuleGroup[] = [{
      name: 'node.rules',
      file: 'rules/node.yml',
      rules: [
        { name: 'InstanceDown', type: 'alerting', health: 'ok' },
        { name: 'NodeCPU', type: 'recording', health: 'ok' },
      ],
    }];
    const options = ruleFilterOptions(items);
    expect(options.map(item => item.label)).toEqual(['InstanceDown', 'NodeCPU']);
    expect(filterRuleGroupsByOption(items, options[1].value)).toEqual([{ ...items[0], rules: [items[0].rules![1]] }]);
    expect(filterRuleGroupsByOption(items, '')).toEqual(items);
  });
});
