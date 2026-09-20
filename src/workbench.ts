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
