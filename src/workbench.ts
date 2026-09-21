import type { Alert, QueryResult, Rule, RuleGroup, Target } from './domain';

export type PanelView = 'table' | 'graph';
export type QueryPanel = {
  id: string;
  expression: string;
  view: PanelView;
  result?: QueryResult;
  loading: boolean;
  error: string;
  warnings: string[];
};

export type ConnectionExperience = {
  connected: boolean;
  title: string;
  description: string;
};

export type FilterOption = {
  value: string;
  label: string;
  description?: string;
  searchText?: string;
};

export function connectionExperience(connectionId: string): ConnectionExperience {
  if (connectionId.trim()) return { connected: true, title: '', description: '' };
  return {
    connected: false,
    title: '先打开 Prometheus 连接',
    description: '当前标签页未绑定连接。请在 DBX 左侧连接列表中双击 Prometheus 连接，打开对应的工作台。',
  };
}

export function createPanel(id: string, expression = ''): QueryPanel {
  return { id, expression, view: 'graph', loading: false, error: '', warnings: [] };
}

export function addHistoryEntry(history: string[], expression: string, limit = 30): string[] {
  const value = expression.trim();
  if (!value) return history;
  return [value, ...history.filter(item => item !== value)].slice(0, limit);
}

export function queryWindow(evaluationTime: Date, hours: number): { start: number; end: number } {
  const end = Math.floor(evaluationTime.getTime() / 1000);
  return { start: end - hours * 3600, end };
}

export function shiftEvaluationTime(value: Date, minutes: number): Date {
  return new Date(value.getTime() + minutes * 60_000);
}

export function filterScrapePools(scrapePools: string[], search: string): string[] {
  const query = search.trim().toLowerCase();
  return [...new Set(scrapePools.map(item => item.trim()).filter(Boolean))]
    .filter(item => !query || item.toLowerCase().includes(query))
    .sort((a, b) => a.localeCompare(b));
}

export function toggleScrapePool(selected: string[], scrapePool: string, limit = 20): string[] {
  const value = scrapePool.trim();
  if (!value) return selected;
  if (selected.includes(value)) return selected.filter(item => item !== value);
  if (selected.length >= limit) return selected;
  return [...selected, value];
}

export function discoveryPageRange(page: number, pageSize: number, total: number) {
  if (total <= 0) return { from: 0, to: 0, canPrevious: false, canNext: false };
  return {
    from: (page - 1) * pageSize + 1,
    to: Math.min(page * pageSize, total),
    canPrevious: page > 1,
    canNext: page * pageSize < total,
  };
}

function compact(parts: Array<string | undefined>): string[] {
  return parts.map(item => item?.trim()).filter((item): item is string => Boolean(item));
}

function distinctOptions(options: FilterOption[]): FilterOption[] {
  return options.filter((option, index) => options.findIndex(item => item.value === option.value) === index);
}

function targetOptionValue(target: Target): string {
  return JSON.stringify(compact([target.scrapePool, target.labels?.instance, target.scrapeUrl]));
}

export function targetFilterOptions(targets: Target[]): FilterOption[] {
  return distinctOptions(targets.map(target => {
    const label = target.labels?.instance || target.scrapeUrl || target.scrapePool || '未命名目标';
    return {
      value: targetOptionValue(target),
      label,
      description: compact([target.scrapePool, target.health]).join(' · '),
      searchText: compact([label, target.scrapePool, target.labels?.job, target.scrapeUrl, target.health]).join(' '),
    };
  }));
}

export function filterTargetsByOption(targets: Target[], selected: string): Target[] {
  return selected ? targets.filter(target => targetOptionValue(target) === selected) : targets;
}

function alertOptionValue(alert: Alert): string {
  return JSON.stringify(compact([alert.labels?.alertname, alert.labels?.instance, alert.state, alert.activeAt]));
}

export function alertFilterOptions(alerts: Alert[]): FilterOption[] {
  return distinctOptions(alerts.map(alert => {
    const name = alert.labels?.alertname || '未命名告警';
    const instance = alert.labels?.instance;
    return {
      value: alertOptionValue(alert),
      label: compact([name, instance]).join(' · '),
      description: compact([alert.labels?.severity, alert.state]).join(' · '),
      searchText: compact([name, instance, alert.labels?.severity, alert.state, alert.annotations?.summary]).join(' '),
    };
  }));
}

export function filterAlertsByOption(alerts: Alert[], selected: string): Alert[] {
  return selected ? alerts.filter(alert => alertOptionValue(alert) === selected) : alerts;
}

function ruleOptionValue(group: RuleGroup, rule: Rule): string {
  return JSON.stringify(compact([group.name, group.file, rule.name, rule.type, rule.query]));
}

export function ruleFilterOptions(groups: RuleGroup[]): FilterOption[] {
  return distinctOptions(groups.flatMap(group => (group.rules || []).map(rule => ({
    value: ruleOptionValue(group, rule),
    label: rule.name || '未命名规则',
    description: compact([group.name, rule.type, rule.health || rule.state]).join(' · '),
    searchText: compact([rule.name, group.name, group.file, rule.type, rule.query]).join(' '),
  }))));
}

export function filterRuleGroupsByOption(groups: RuleGroup[], selected: string): RuleGroup[] {
  if (!selected) return groups;
  return groups.flatMap(group => {
    const rules = (group.rules || []).filter(rule => ruleOptionValue(group, rule) === selected);
    return rules.length ? [{ ...group, rules }] : [];
  });
}
