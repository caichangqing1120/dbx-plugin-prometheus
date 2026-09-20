export type Point = [number, string];
export type Series = { metric: Record<string, string>; value?: Point; values?: Point[] };
export type QueryResult = { resultType: 'vector' | 'matrix' | 'scalar' | 'string'; result: Series[] | Point };
export type Envelope<T> = { data: T; warnings?: string[] };
export type Target = { scrapePool?: string; health?: string; labels?: Record<string, string>; discoveredLabels?: Record<string, string>; lastScrape?: string; lastError?: string; scrapeUrl?: string };
export type Alert = { labels?: Record<string, string>; annotations?: Record<string, string>; state?: string; activeAt?: string; value?: string };
export type Rule = { name?: string; type?: string; query?: string; state?: string; health?: string; lastError?: string; alerts?: Alert[] };
export type RuleGroup = { name?: string; file?: string; rules?: Rule[] };
export const seriesName = (metric: Record<string, string>): string => {
  const { __name__, ...labels } = metric;
  const parts = Object.entries(labels).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => `${key}="${value}"`);
  return `${__name__ || 'series'}${parts.length ? `{${parts.join(', ')}}` : ''}`;
};
export const numericValue = (value?: Point): string => value ? value[1] : '—';
export function plot(values: Point[], width = 900, height = 230): string {
  const points = values.map(([x, y]) => [Number(x), Number(y)]).filter(([x, y]) => Number.isFinite(x) && Number.isFinite(y));
  if (!points.length) return '';
  const xs = points.map(p => p[0]), ys = points.map(p => p[1]);
  const loX = Math.min(...xs), hiX = Math.max(...xs), loY = Math.min(...ys), hiY = Math.max(...ys);
  return points.map(([x, y]) => `${((x - loX) / (hiX - loX || 1) * width).toFixed(2)},${(height - (y - loY) / (hiY - loY || 1) * height).toFixed(2)}`).join(' ');
}
