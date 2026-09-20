import type { QueryResult } from './domain';

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
