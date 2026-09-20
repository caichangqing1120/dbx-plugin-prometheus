import { describe, expect, it } from 'vitest';
import { addHistoryEntry, connectionExperience, createPanel, queryWindow, shiftEvaluationTime } from './workbench';

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
});
