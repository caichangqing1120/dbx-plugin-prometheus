<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue';
import {
  Activity, AlertCircle, Bell, Braces, Check, ChevronLeft, ChevronRight, Clock3, History, ListFilter,
  Monitor, Moon, Play, Plus, RefreshCw, Search, Server, Sun, Trash2, X,
} from '@lucide/vue';
import prometheusLogo from '../assets/plugin.svg?inline';
import { invoke, waitForPluginReady, type Context } from './bridge';
import PromQLEditor from './PromQLEditor.vue';
import FilterSelect from './FilterSelect.vue';
import { numericValue, plot, seriesName, type Alert, type BuildInfo, type Envelope, type QueryResult, type RuleGroup, type RuntimeInfo, type Series, type StatusEntry, type Target, type TSDBStatus } from './domain';
import { BridgePrometheusClient } from './prometheus-client';
import { browserStorage, readStorage, writeStorage } from './storage';
import {
  addHistoryEntry,
  connectionExperience,
  createPanel,
  discoveryPageRange,
  alertFilterOptions,
  filterAlertsByOption,
  filterRuleGroupsByOption,
  filterScrapePools,
  filterTargetsByOption,
  queryWindow,
  ruleFilterOptions,
  shiftEvaluationTime,
  targetFilterOptions,
  toggleScrapePool,
  type QueryPanel,
} from './workbench';

type Tab = 'query' | 'targets' | 'alerts' | 'rules' | 'status';
type StatusView = 'runtime' | 'tsdb' | 'flags' | 'config' | 'discovery';
type Theme = 'light' | 'dark' | 'system';
type Info = { name: string; baseUrl: string; environment: string; build: Envelope<BuildInfo> };
const tabs = [
  { id: 'query', title: 'PromQL', icon: Search },
  { id: 'targets', title: '采集目标', icon: Server },
  { id: 'alerts', title: '活动告警', icon: Bell },
  { id: 'rules', title: '规则', icon: ListFilter },
  { id: 'status', title: '运行状态', icon: Activity },
] as const;
const statusViews: Array<{ id: StatusView; title: string }> = [
  { id: 'runtime', title: '运行与构建' },
  { id: 'tsdb', title: 'TSDB' },
  { id: 'flags', title: '启动参数' },
  { id: 'config', title: '配置' },
  { id: 'discovery', title: '服务发现' },
];

const tab = ref<Tab>('query');
const connectionId = ref(''), info = ref<Info>();
const panels = ref<QueryPanel[]>([createPanel('panel-1', 'up')]);
const targets = ref<Target[]>([]), alerts = ref<Alert[]>([]), groups = ref<RuleGroup[]>([]);
const statusView = ref<StatusView>('runtime'), runtimeInfo = ref<RuntimeInfo>({}), tsdbStatus = ref<TSDBStatus>({});
const flags = ref<Record<string, string>>({}), configYaml = ref('');
const targetFilter = ref(''), alertFilter = ref(''), ruleFilter = ref(''), statusFilter = ref('');
const pageLoading = ref(false), pageError = ref(''), fetchedAt = ref('');
const localTime = ref(true), historyEnabled = ref(true), autocomplete = ref(true), highlighting = ref(true), linter = ref(true);
const history = ref<string[]>([]), historyPanel = ref('');
const completionClient = shallowRef<BridgePrometheusClient>();
const metricPanel = ref(''), metricFilter = ref(''), metrics = ref<string[]>([]), metricsLoading = ref(false), metricError = ref('');
const scrapePools = ref<string[]>([]), discoverySearch = ref(''), selectedScrapePools = ref<string[]>([]);
const discoveryTargets = ref<Target[]>([]), discoveryState = ref<'active' | 'dropped'>('active');
const discoveryPage = ref(1), discoveryPageSize = ref(20), discoveryTotal = ref(0), discoveryHasNext = ref(false);
const discoverySuggestionsOpen = ref(false), discoverySuggestionIndex = ref(0);
const rangeHours = ref(1), step = ref(30), evaluationTime = ref(new Date());
const theme = ref<Theme>('system'), systemDark = ref(false);
let generation = 0, panelSequence = 1, unsubscribe: (() => void) | undefined, media: MediaQueryList | undefined;

const currentTitle = computed(() => tabs.find(item => item.id === tab.value)?.title);
const targetOptions = computed(() => targetFilterOptions(targets.value));
const alertOptions = computed(() => alertFilterOptions(alerts.value));
const ruleOptions = computed(() => ruleFilterOptions(groups.value));
const visibleTargets = computed(() => filterTargetsByOption(targets.value, targetFilter.value));
const visibleAlerts = computed(() => filterAlertsByOption(alerts.value, alertFilter.value));
const visibleGroups = computed(() => filterRuleGroupsByOption(groups.value, ruleFilter.value));
const healthy = computed(() => targets.value.filter(item => item.health === 'up').length);
const visibleFlags = computed(() => Object.entries(flags.value).filter(([key, value]) => `${key} ${value}`.toLowerCase().includes(statusFilter.value.toLowerCase())));
const discoverySuggestionItems = computed(() => filterScrapePools(scrapePools.value, discoverySearch.value));
const discoveryRange = computed(() => discoveryPageRange(discoveryPage.value, discoveryPageSize.value, discoveryTotal.value));
const tsdbTables = computed<Array<{ title: string; items: StatusEntry[]; bytes?: boolean }>>(() => [
  { title: '按指标名统计序列', items: tsdbStatus.value.seriesCountByMetricName || [] },
  { title: '按标签名统计值', items: tsdbStatus.value.labelValueCountByLabelName || [] },
  { title: '按标签名统计内存', items: tsdbStatus.value.memoryInBytesByLabelName || [], bytes: true },
  { title: '按标签值对统计序列', items: tsdbStatus.value.seriesCountByLabelValuePair || [] },
]);
const visibleMetrics = computed(() => {
  const search = metricFilter.value.trim().toLowerCase();
  return metrics.value.filter(metric => !search || metric.toLowerCase().includes(search)).slice(0, 500);
});
const dark = computed(() => theme.value === 'dark' || (theme.value === 'system' && systemDark.value));
const connectionState = computed(() => connectionExperience(connectionId.value));
const evaluationInput = computed({
  get: () => toInputValue(evaluationTime.value),
  set: value => { evaluationTime.value = fromInputValue(value); clearPanelResults(); },
});

function panelSamples(panel: QueryPanel): Series[] {
  if (!panel.result) return [];
  if (panel.result.resultType === 'vector' || panel.result.resultType === 'matrix') return panel.result.result as Series[];
  return [{ metric: {}, value: panel.result.result as [number, string] }];
}
function labelMap(value?: Record<string, string>): string { return Object.entries(value || {}).filter(([key]) => key !== '__name__').map(([key, val]) => `${key}=${val}`).join(' · '); }
async function toggleDiscoverySuggestion(value: string) {
  const next = toggleScrapePool(selectedScrapePools.value, value);
  if (next === selectedScrapePools.value) {
    pageError.value = '一次最多选择 20 个服务';
    return;
  }
  selectedScrapePools.value = next;
  discoverySearch.value = '';
  discoverySuggestionsOpen.value = true;
  discoverySuggestionIndex.value = 0;
  discoveryPage.value = 1;
  if (next.length) await loadDiscoveryTargets();
  else {
    discoveryTargets.value = [];
    discoveryTotal.value = 0;
    discoveryHasNext.value = false;
  }
}
function moveDiscoverySuggestion(offset: number) {
  const count = discoverySuggestionItems.value.length;
  if (!count) return;
  discoverySuggestionsOpen.value = true;
  discoverySuggestionIndex.value = (discoverySuggestionIndex.value + offset + count) % count;
}
function submitDiscoverySearch() {
  const suggestions = discoverySuggestionItems.value;
  const exact = scrapePools.value.find(item => item === discoverySearch.value.trim());
  const selected = discoverySuggestionsOpen.value ? suggestions[discoverySuggestionIndex.value] : exact || suggestions[0];
  if (selected) void toggleDiscoverySuggestion(selected);
}
function handleDiscoveryInput() {
  discoverySuggestionIndex.value = 0;
  discoverySuggestionsOpen.value = true;
}
async function setDiscoveryState(state: 'active' | 'dropped') {
  if (discoveryState.value === state) return;
  discoveryState.value = state;
  discoveryPage.value = 1;
  if (selectedScrapePools.value.length) await loadDiscoveryTargets();
}
async function setDiscoveryPageSize() {
  discoveryPage.value = 1;
  if (selectedScrapePools.value.length) await loadDiscoveryTargets();
}
async function moveDiscoveryPage(offset: number) {
  const next = discoveryPage.value + offset;
  if (next < 1 || (offset > 0 && !discoveryHasNext.value)) return;
  discoveryPage.value = next;
  await loadDiscoveryTargets();
}
function statusEntries(value?: Record<string, unknown>): Array<[string, unknown]> { return Object.entries(value || {}).sort(([a], [b]) => a.localeCompare(b)); }
function statusValue(value: unknown): string { return value === undefined || value === null || value === '' ? '—' : typeof value === 'object' ? JSON.stringify(value) : String(value); }
function formatBytes(value?: number): string {
  if (!Number.isFinite(value)) return '—';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']; let size = Number(value), unit = 0;
  while (Math.abs(size) >= 1024 && unit < units.length - 1) { size /= 1024; unit++; }
  return `${size.toFixed(unit ? 1 : 0)} ${units[unit]}`;
}
function formatMilliseconds(value?: number): string { return value ? formatDate(value / 1000) : '—'; }
function showError(error: unknown): string { return error instanceof Error ? error.message : String(error); }
function formatDate(value?: string | number): string {
  if (value === undefined || value === '') return '—';
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium', ...(localTime.value ? {} : { timeZone: 'UTC' }) }).format(date) + (localTime.value ? '' : ' UTC');
}
function toInputValue(value: Date): string {
  const shifted = localTime.value ? new Date(value.getTime() - value.getTimezoneOffset() * 60_000) : value;
  return shifted.toISOString().slice(0, 16);
}
function fromInputValue(value: string): Date {
  const parsed = new Date(localTime.value ? value : `${value}Z`);
  return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
}
function persistHistory() { writeStorage(browserStorage(), 'prometheus-query-history', JSON.stringify(history.value)); }
function clearPanelResults() { panels.value.forEach(panel => { panel.result = undefined; panel.error = ''; panel.warnings = []; }); }
function addPanel() { panelSequence++; panels.value.push(createPanel(`panel-${panelSequence}`)); }
function removePanel(id: string) {
  if (panels.value.length === 1) return;
  panels.value = panels.value.filter(panel => panel.id !== id);
  if (historyPanel.value === id) historyPanel.value = '';
}
function useHistory(panel: QueryPanel, expression: string) { panel.expression = expression; historyPanel.value = ''; }
async function openMetricExplorer(panel: QueryPanel) {
  metricPanel.value = panel.id; metricFilter.value = ''; metricError.value = ''; historyPanel.value = '';
  if (metrics.value.length || metricsLoading.value) return;
  if (!completionClient.value) { metricError.value = 'Prometheus 连接尚未就绪'; return; }
  metricsLoading.value = true;
  try { metrics.value = (await completionClient.value.metricNames()).slice().sort((a, b) => a.localeCompare(b)); }
  catch (error) { metricError.value = showError(error); }
  finally { metricsLoading.value = false; }
}
function useMetric(metric: string) {
  const panel = panels.value.find(item => item.id === metricPanel.value);
  if (panel) panel.expression = metric;
  metricPanel.value = '';
}
function setPanelView(panel: QueryPanel, view: 'table' | 'graph') { panel.view = view; panel.result = undefined; panel.error = ''; panel.warnings = []; }
function shiftEvaluation(minutes: number) {
  const shifted = shiftEvaluationTime(evaluationTime.value, minutes);
  evaluationTime.value = shifted > new Date() ? new Date() : shifted;
  clearPanelResults();
}
function useNow() { evaluationTime.value = new Date(); clearPanelResults(); }
function select(next: Tab) { if (tab.value === next) return; tab.value = next; historyPanel.value = ''; if (next !== 'query') loadPage(); }
function selectStatus(next: StatusView) { if (statusView.value === next) return; statusView.value = next; statusFilter.value = ''; loadPage(); }

async function runPanel(panel: QueryPanel) {
  const id = connectionId.value;
  const expression = panel.expression.trim();
  if (!id || !expression) return;
  panel.loading = true; panel.error = ''; panel.warnings = [];
  try {
    const window = queryWindow(evaluationTime.value, rangeHours.value);
    const form = panel.view === 'graph' ? { query: expression, ...window, step: Number(step.value) } : { query: expression, time: window.end };
    const response = await invoke<Envelope<QueryResult>>(id, panel.view === 'graph' ? 'prometheus/query_range' : 'prometheus/query', { form });
    panel.result = response.data; panel.warnings = response.warnings || [];
    if (historyEnabled.value) { history.value = addHistoryEntry(history.value, expression); persistHistory(); }
    fetchedAt.value = new Date().toLocaleTimeString();
  } catch (error) { panel.error = showError(error); panel.result = undefined; }
  finally { panel.loading = false; }
}
async function loadPage() {
  const id = connectionId.value, gen = ++generation;
  if (!id) { pageError.value = '请从 DBX 连接打开工作台'; return; }
  pageLoading.value = true; pageError.value = '';
  try {
    if (tab.value === 'targets') {
      const response = await invoke<Envelope<{ activeTargets?: Target[] }>>(id, 'prometheus/targets');
      if (gen === generation) {
        targets.value = response.data.activeTargets || [];
        if (!targetFilterOptions(targets.value).some(item => item.value === targetFilter.value)) targetFilter.value = '';
      }
    } else if (tab.value === 'alerts') {
      const response = await invoke<Envelope<{ alerts?: Alert[] }>>(id, 'prometheus/alerts');
      if (gen === generation) {
        alerts.value = response.data.alerts || [];
        if (!alertFilterOptions(alerts.value).some(item => item.value === alertFilter.value)) alertFilter.value = '';
      }
    } else if (tab.value === 'rules') {
      const response = await invoke<Envelope<{ groups?: RuleGroup[] }>>(id, 'prometheus/rules');
      if (gen === generation) {
        groups.value = response.data.groups || [];
        if (!ruleFilterOptions(groups.value).some(item => item.value === ruleFilter.value)) ruleFilter.value = '';
      }
    } else if (tab.value === 'status') {
      if (statusView.value === 'runtime') {
        const response = await invoke<Envelope<RuntimeInfo>>(id, 'prometheus/status_runtime');
        if (gen === generation) runtimeInfo.value = response.data || {};
      } else if (statusView.value === 'tsdb') {
        const response = await invoke<Envelope<TSDBStatus>>(id, 'prometheus/status_tsdb');
        if (gen === generation) tsdbStatus.value = response.data || {};
      } else if (statusView.value === 'flags') {
        const response = await invoke<Envelope<Record<string, string>>>(id, 'prometheus/status_flags');
        if (gen === generation) flags.value = response.data || {};
      } else if (statusView.value === 'config') {
        const response = await invoke<Envelope<{ yaml?: string }>>(id, 'prometheus/status_config');
        if (gen === generation) configYaml.value = response.data.yaml || '';
      } else {
        const response = await invoke<Envelope<{ scrapePools?: string[] }>>(id, 'prometheus/service_discovery_services');
        if (gen === generation) {
          scrapePools.value = response.data.scrapePools || [];
          const selected = selectedScrapePools.value.filter(item => scrapePools.value.includes(item));
          if (selected.length !== selectedScrapePools.value.length) {
            selectedScrapePools.value = selected;
            discoveryTargets.value = [];
            discoveryTotal.value = 0;
          }
        }
      }
    }
    if (gen === generation) fetchedAt.value = new Date().toLocaleTimeString();
  } catch (error) { if (gen === generation) pageError.value = showError(error); }
  finally { if (gen === generation) pageLoading.value = false; }
}
async function loadDiscoveryTargets() {
  const id = connectionId.value, scrapePools = [...selectedScrapePools.value], gen = ++generation;
  if (!id || !scrapePools.length) return;
  pageLoading.value = true; pageError.value = '';
  try {
    const response = await invoke<Envelope<{ items?: Target[]; total?: number; page?: number; pageSize?: number; hasNext?: boolean }>>(id, 'prometheus/service_discovery', {
      form: { scrapePools, state: discoveryState.value, page: discoveryPage.value, pageSize: discoveryPageSize.value },
    });
    if (gen === generation) {
      discoveryTargets.value = response.data.items || [];
      discoveryTotal.value = response.data.total || 0;
      discoveryHasNext.value = Boolean(response.data.hasNext);
      fetchedAt.value = new Date().toLocaleTimeString();
    }
  } catch (error) { if (gen === generation) pageError.value = showError(error); }
  finally { if (gen === generation) pageLoading.value = false; }
}
async function refresh() {
  if (tab.value === 'query') await Promise.all(panels.value.filter(panel => panel.expression.trim()).map(runPanel));
  else if (tab.value === 'status' && statusView.value === 'discovery' && selectedScrapePools.value.length) await loadDiscoveryTargets();
  else await loadPage();
}
async function connect(context: Context) {
  generation++; connectionId.value = context.connectionId || ''; info.value = undefined;
  completionClient.value = connectionId.value ? new BridgePrometheusClient(connectionId.value) : undefined;
  metrics.value = []; metricPanel.value = ''; metricError.value = '';
  targets.value = []; alerts.value = []; groups.value = []; targetFilter.value = ''; alertFilter.value = ''; ruleFilter.value = ''; statusFilter.value = '';
  scrapePools.value = []; discoverySearch.value = ''; selectedScrapePools.value = []; discoveryTargets.value = []; discoveryTotal.value = 0; discoveryPage.value = 1;
  panels.value.forEach(panel => { panel.result = undefined; panel.error = ''; });
  if (!connectionId.value) { pageError.value = ''; return; }
  const id = connectionId.value;
  try {
    const response = await invoke<Info>(id, 'prometheus/info');
    if (connectionId.value === id) { info.value = response; pageError.value = ''; await runPanel(panels.value[0]); }
  } catch (error) { if (connectionId.value === id) pageError.value = showError(error); }
}
function setTheme(next: Theme) { theme.value = next; writeStorage(browserStorage(), 'prometheus-theme', next); }

onMounted(async () => {
  media = window.matchMedia('(prefers-color-scheme: dark)'); systemDark.value = media.matches;
  media.addEventListener('change', event => { systemDark.value = event.matches; });
  const storage = browserStorage();
  theme.value = readStorage(storage, 'prometheus-theme', 'system') as Theme;
  try { history.value = JSON.parse(readStorage(storage, 'prometheus-query-history', '[]')); } catch { history.value = []; }
  if (!window.dbxPlugin) { pageError.value = '请从 DBX 连接打开工作台'; return; }
  try {
    const plugin = window.dbxPlugin;
    const context = await waitForPluginReady(plugin);
    unsubscribe = plugin.onContext(connect);
    await connect(context);
  } catch (error) {
    pageError.value = showError(error);
  }
});
onBeforeUnmount(() => { generation++; unsubscribe?.(); });
</script>

<template>
  <div class="app-shell" :data-theme="dark ? 'dark' : 'light'">
    <aside class="sidebar">
      <div class="brand"><img class="logo" :src="prometheusLogo" alt="" /><div><strong>Prometheus</strong><small>DBX 工作台</small></div></div>
      <nav aria-label="工作台导航"><button v-for="item in tabs" :key="item.id" type="button" :disabled="!connectionState.connected" :class="{ active: tab === item.id }" @click="select(item.id)"><component :is="item.icon" :size="17" /><span>{{ item.title }}</span></button></nav>
      <div class="sidebar-meta"><span class="status-dot" :class="{ disconnected: !info }"></span><span>{{ info?.environment || info?.name || '未连接' }}</span><small>{{ info?.build.data?.version || '' }}</small></div>
    </aside>
    <main>
      <header class="topbar"><div><span class="breadcrumb">监控 / {{ currentTitle }}</span><h1>{{ currentTitle }}</h1></div><div class="top-actions"><span v-if="fetchedAt" class="updated">更新于 {{ fetchedAt }}</span><div class="theme-switch" aria-label="主题"><button title="浅色" :class="{ selected: theme === 'light' }" @click="setTheme('light')"><Sun :size="15" /></button><button title="跟随系统" :class="{ selected: theme === 'system' }" @click="setTheme('system')"><Monitor :size="15" /></button><button title="深色" :class="{ selected: theme === 'dark' }" @click="setTheme('dark')"><Moon :size="15" /></button></div><button class="icon-button" type="button" aria-label="刷新" title="刷新" :disabled="pageLoading || !connectionId" @click="refresh"><RefreshCw :size="17" :class="{ spinning: pageLoading }" /></button></div></header>
      <section v-if="!connectionState.connected" class="connection-empty" aria-live="polite">
        <span class="connection-empty-icon"><Server :size="25" /></span>
        <h2>{{ connectionState.title }}</h2>
        <p>{{ connectionState.description }}</p>
      </section>

      <template v-else>
      <div v-if="pageError" class="message error" role="alert"><AlertCircle :size="16" />{{ pageError }}</div>

      <template v-if="tab === 'query'">
        <section class="query-settings" aria-label="查询设置"><label><input v-model="localTime" type="checkbox" />使用本地时间</label><label><input v-model="historyEnabled" type="checkbox" />启用查询历史</label><label><input v-model="autocomplete" type="checkbox" />启用自动补全</label><label><input v-model="highlighting" type="checkbox" />启用高亮</label><label><input v-model="linter" type="checkbox" />启用 linter</label></section>
        <section class="evaluation-bar"><div class="evaluation-control"><button class="icon-button" title="向前 5 分钟" @click="shiftEvaluation(-5)"><ChevronLeft :size="17" /></button><label for="evaluation-time">评估时间</label><input id="evaluation-time" v-model="evaluationInput" type="datetime-local" /><button class="icon-button" title="向后 5 分钟" @click="shiftEvaluation(5)"><ChevronRight :size="17" /></button><button class="text-button" @click="useNow">现在</button></div><div class="range-control"><label for="range-hours">图表范围</label><select id="range-hours" v-model.number="rangeHours" @change="clearPanelResults"><option :value="1">1 小时</option><option :value="6">6 小时</option><option :value="24">24 小时</option></select><label for="range-step">步长</label><select id="range-step" v-model.number="step" @change="clearPanelResults"><option :value="30">30 秒</option><option :value="60">1 分钟</option><option :value="300">5 分钟</option></select></div></section>

        <section v-for="(panel, panelIndex) in panels" :key="panel.id" class="query-panel">
          <div class="query-row"><div class="editor-shell"><Search :size="19" class="editor-search" /><PromQLEditor v-model="panel.expression" :dark="dark" :disabled="panel.loading" :autocomplete="autocomplete" :highlighting="highlighting" :linter="linter" :prometheus-client="completionClient" @execute="runPanel(panel)" /></div><div class="query-tools"><button class="square-button" type="button" title="指标浏览器" aria-label="指标浏览器" @click="openMetricExplorer(panel)"><Braces :size="17" /></button><div class="history-wrap"><button class="square-button" type="button" title="查询历史" aria-label="查询历史" @click="historyPanel = historyPanel === panel.id ? '' : panel.id"><History :size="17" /></button><div v-if="historyPanel === panel.id" class="history-menu"><strong>最近查询</strong><button v-for="item in history" :key="item" @click="useHistory(panel, item)"><code>{{ item }}</code></button><span v-if="!history.length">暂无历史记录</span></div></div></div><button class="run" type="button" :disabled="panel.loading || !connectionId || !panel.expression.trim()" @click="runPanel(panel)"><Play :size="15" fill="currentColor" />{{ panel.loading ? '执行中' : '执行' }}</button></div>
          <div class="panel-tabs"><button :class="{ active: panel.view === 'table' }" @click="setPanelView(panel, 'table')">Table</button><button :class="{ active: panel.view === 'graph' }" @click="setPanelView(panel, 'graph')">Graph</button><button class="remove-panel" :disabled="panels.length === 1" @click="removePanel(panel.id)"><Trash2 :size="14" />移除面板</button></div>
          <div v-if="panel.error" class="message error"><AlertCircle :size="16" />{{ panel.error }}</div><div v-for="warning in panel.warnings" :key="warning" class="message warning"><AlertCircle :size="16" />{{ warning }}</div>
          <div class="panel-results"><div v-if="!panel.result && !panel.loading && !panel.error" class="empty">面板 {{ panelIndex + 1 }} 尚未查询数据</div><div v-if="panel.result && !panelSamples(panel).length" class="empty">当前查询无数据</div>
            <template v-if="panel.view === 'table' && panel.result"><div v-if="panel.result.resultType === 'scalar' || panel.result.resultType === 'string'" class="scalar"><span>{{ panel.result.resultType }}</span><strong>{{ numericValue(panel.result.result as [number, string]) }}</strong></div><div v-else class="query-table-wrap"><table class="query-table"><thead><tr><th>指标</th><th>时间</th><th>值</th></tr></thead><tbody><tr v-for="(series, index) in panelSamples(panel)" :key="index"><td><code>{{ seriesName(series.metric) }}</code></td><td>{{ formatDate((series.value || series.values?.at(-1))?.[0]) }}</td><td><strong>{{ numericValue(series.value || series.values?.at(-1)) }}</strong></td></tr></tbody></table></div></template>
            <template v-if="panel.view === 'graph' && panel.result"><div v-for="(series, index) in panelSamples(panel).slice(0, 100)" :key="index" class="series"><div class="series-header"><code>{{ seriesName(series.metric) }}</code><strong>{{ numericValue(series.value || series.values?.at(-1)) }}</strong></div><div v-if="series.values?.length" class="chart"><svg viewBox="0 0 900 230" preserveAspectRatio="none" role="img" :aria-label="`${seriesName(series.metric)} 时序曲线`"><line x1="0" y1="229" x2="900" y2="229" stroke="currentColor" opacity=".18" /><polyline :points="plot(series.values)" fill="none" stroke="currentColor" stroke-width="2" vector-effect="non-scaling-stroke" /></svg><div class="chart-axis"><span>{{ formatDate(series.values[0][0]) }}</span><span>{{ formatDate(series.values.at(-1)![0]) }}</span></div></div></div><div v-if="panelSamples(panel).length > 100" class="limit">仅展示前 100 条序列；请用 PromQL 缩小范围。</div></template>
          </div>
        </section>
        <button class="add-panel" type="button" @click="addPanel"><Plus :size="17" />新增面板</button>
      </template>

      <template v-else-if="tab !== 'status'"><div class="list-tools"><FilterSelect v-if="tab === 'targets'" v-model="targetFilter" :options="targetOptions" placeholder="采集目标" all-label="全部采集目标" /><FilterSelect v-else-if="tab === 'alerts'" v-model="alertFilter" :options="alertOptions" placeholder="活动告警" all-label="全部活动告警" /><FilterSelect v-else v-model="ruleFilter" :options="ruleOptions" placeholder="规则" all-label="全部规则" /><span v-if="tab === 'targets'">{{ healthy }} / {{ targets.length }} 正常</span><span v-else-if="tab === 'alerts'">{{ alerts.length }} 条活动告警</span><span v-else>{{ groups.length }} 个规则组</span></div>
        <div v-if="tab === 'targets'" class="table-wrap responsive-table"><table><thead><tr><th>目标 / 任务</th><th>状态</th><th>最近采集</th><th>错误</th></tr></thead><tbody><tr v-for="(item, index) in visibleTargets" :key="index"><td data-label="目标 / 任务"><strong>{{ item.labels?.instance || item.scrapeUrl || '—' }}</strong><small>{{ item.scrapePool }} · {{ labelMap(item.labels) }}</small></td><td data-label="状态"><span class="badge" :class="item.health === 'up' ? 'ok' : 'bad'">{{ item.health || 'unknown' }}</span></td><td data-label="最近采集">{{ formatDate(item.lastScrape) }}</td><td data-label="错误" class="error-text">{{ item.lastError || '—' }}</td></tr><tr v-if="!visibleTargets.length"><td colspan="4" class="empty">{{ pageLoading ? '加载中…' : '没有匹配的采集目标' }}</td></tr></tbody></table></div>
        <div v-if="tab === 'alerts'" class="table-wrap responsive-table"><table><thead><tr><th>告警</th><th>状态</th><th>开始时间</th><th>值</th></tr></thead><tbody><tr v-for="(item, index) in visibleAlerts" :key="index"><td data-label="告警"><strong>{{ item.labels?.alertname || '未命名告警' }}</strong><small>{{ item.annotations?.summary || labelMap(item.labels) }}</small></td><td data-label="状态"><span class="badge" :class="item.state === 'firing' ? 'bad' : 'pending'">{{ item.state || 'unknown' }}</span></td><td data-label="开始时间">{{ formatDate(item.activeAt) }}</td><td data-label="值">{{ item.value || '—' }}</td></tr><tr v-if="!visibleAlerts.length"><td colspan="4" class="empty">{{ pageLoading ? '加载中…' : '没有活动告警' }}</td></tr></tbody></table></div>
        <div v-if="tab === 'rules'" class="rule-list"><section v-for="(group, index) in visibleGroups" :key="index" class="rule-group"><div class="group-head"><h2>{{ group.name || '未命名规则组' }}</h2><small>{{ group.file || '' }}</small></div><div v-for="(rule, ruleIndex) in group.rules || []" :key="ruleIndex" class="rule"><div><strong>{{ rule.name || '未命名规则' }}</strong><code>{{ rule.query || '' }}</code><small v-if="rule.lastError" class="error-text">{{ rule.lastError }}</small></div><span class="badge" :class="rule.health === 'ok' ? 'ok' : 'bad'">{{ rule.health || rule.state || rule.type || '—' }}</span></div></section><div v-if="!visibleGroups.length" class="empty">{{ pageLoading ? '加载中…' : '没有匹配的规则组' }}</div></div>
      </template>

      <template v-else>
        <nav class="status-tabs" aria-label="运行状态分类"><button v-for="item in statusViews" :key="item.id" type="button" :class="{ active: statusView === item.id }" @click="selectStatus(item.id)">{{ item.title }}</button></nav>
        <div v-if="pageLoading" class="status-loading"><RefreshCw :size="17" class="spinning" />正在读取 Prometheus 状态</div>

        <div v-else-if="statusView === 'runtime'" class="status-content">
          <section class="status-section"><div class="section-heading"><h2>Runtime &amp; Build Information</h2><span>{{ info?.baseUrl }}</span></div><div class="status-columns"><dl><template v-for="([key, value]) in statusEntries(info?.build.data)" :key="key"><dt>{{ key }}</dt><dd>{{ statusValue(value) }}</dd></template></dl><dl><template v-for="([key, value]) in statusEntries(runtimeInfo)" :key="key"><dt>{{ key }}</dt><dd>{{ statusValue(value) }}</dd></template></dl></div></section>
        </div>

        <div v-else-if="statusView === 'tsdb'" class="status-content">
          <section class="status-section"><div class="section-heading"><h2>Head Stats</h2></div><div class="stat-strip"><div><span>Series</span><strong>{{ tsdbStatus.headStats?.numSeries ?? '—' }}</strong></div><div><span>Label pairs</span><strong>{{ tsdbStatus.headStats?.numLabelPairs ?? '—' }}</strong></div><div><span>Chunks</span><strong>{{ tsdbStatus.headStats?.chunkCount ?? '—' }}</strong></div><div><span>Min time</span><strong>{{ formatMilliseconds(tsdbStatus.headStats?.minTime) }}</strong></div><div><span>Max time</span><strong>{{ formatMilliseconds(tsdbStatus.headStats?.maxTime) }}</strong></div></div></section>
          <section class="status-section"><div class="section-heading"><h2>Head Cardinality Stats</h2><span>Prometheus 返回的前 10 项</span></div><div class="status-table-grid"><div v-for="table in tsdbTables" :key="table.title" class="compact-table"><h3>{{ table.title }}</h3><table><thead><tr><th>名称</th><th>值</th></tr></thead><tbody><tr v-for="item in table.items" :key="item.name"><td><code>{{ item.name }}</code></td><td>{{ table.bytes ? formatBytes(item.value) : item.value }}</td></tr><tr v-if="!table.items.length"><td colspan="2" class="empty compact">暂无数据</td></tr></tbody></table></div></div></section>
        </div>

        <div v-else-if="statusView === 'flags'" class="status-content"><div class="list-tools"><label class="search"><Search :size="16" /><input v-model="statusFilter" type="search" placeholder="筛选启动参数" aria-label="筛选启动参数" /></label><span>{{ visibleFlags.length }} 项</span></div><div class="table-wrap"><table><thead><tr><th>参数</th><th>值</th></tr></thead><tbody><tr v-for="([key, value]) in visibleFlags" :key="key"><td><code>--{{ key }}</code></td><td>{{ value }}</td></tr><tr v-if="!visibleFlags.length"><td colspan="2" class="empty">没有匹配的启动参数</td></tr></tbody></table></div></div>

        <div v-else-if="statusView === 'config'" class="status-content"><section class="status-section"><div class="section-heading"><h2>Configuration</h2><span>当前 Prometheus 已加载配置，只读</span></div><pre class="config-source"><code>{{ configYaml || 'Prometheus 未返回配置内容' }}</code></pre></section></div>

        <div v-else class="status-content">
          <div class="discovery-toolbar">
            <div class="discovery-search-wrap">
              <label class="search"><Search :size="16" /><input v-model="discoverySearch" type="search" role="combobox" aria-controls="discovery-suggestions" :aria-expanded="discoverySuggestionsOpen && discoverySuggestionItems.length > 0" :aria-activedescendant="discoverySuggestionsOpen && discoverySuggestionItems.length ? `discovery-suggestion-${discoverySuggestionIndex}` : undefined" placeholder="搜索并添加服务" aria-label="搜索并添加服务" @input="handleDiscoveryInput" @focus="discoverySuggestionsOpen = true" @blur="discoverySuggestionsOpen = false" @keydown.down.prevent="moveDiscoverySuggestion(1)" @keydown.up.prevent="moveDiscoverySuggestion(-1)" @keydown.esc="discoverySuggestionsOpen = false" @keydown.enter.prevent="submitDiscoverySearch" /></label>
              <div v-if="discoverySuggestionsOpen && discoverySuggestionItems.length" id="discovery-suggestions" class="search-suggestions" role="listbox" aria-multiselectable="true">
                <button v-for="(suggestion, index) in discoverySuggestionItems" :id="`discovery-suggestion-${index}`" :key="suggestion" type="button" role="option" :aria-selected="selectedScrapePools.includes(suggestion)" :class="{ active: index === discoverySuggestionIndex, selected: selectedScrapePools.includes(suggestion) }" @mousedown.prevent @click="toggleDiscoverySuggestion(suggestion)">
                  <Check v-if="selectedScrapePools.includes(suggestion)" :size="14" /><Server v-else :size="14" />
                  <span>{{ suggestion }}</span>
                  <small>{{ selectedScrapePools.includes(suggestion) ? '已选' : index === discoverySuggestionIndex ? 'Enter' : '' }}</small>
                </button>
              </div>
            </div>
            <div class="discovery-state" aria-label="目标状态"><button type="button" :class="{ selected: discoveryState === 'active' }" @click="setDiscoveryState('active')">活动</button><button type="button" :class="{ selected: discoveryState === 'dropped' }" @click="setDiscoveryState('dropped')">已丢弃</button></div>
            <label class="page-size">每页<select v-model.number="discoveryPageSize" :disabled="!selectedScrapePools.length" @change="setDiscoveryPageSize"><option :value="20">20</option><option :value="50">50</option><option :value="100">100</option></select>条</label>
          </div>
          <div v-if="selectedScrapePools.length" class="selected-services" aria-label="已选服务">
            <span>已选 {{ selectedScrapePools.length }} 个</span>
            <button v-for="service in selectedScrapePools" :key="service" type="button" :title="`移除 ${service}`" @click="toggleDiscoverySuggestion(service)"><span>{{ service }}</span><X :size="13" /></button>
          </div>
          <div v-if="!selectedScrapePools.length" class="discovery-empty"><Server :size="28" /><strong>请先选择服务</strong><span>可连续选择多个服务，再合并查看目标并分页。</span></div>
          <section v-else class="status-section discovery-section">
            <div class="section-heading"><div><h2>{{ discoveryState === 'active' ? '活动目标' : '已丢弃目标' }}</h2><small>已选择 {{ selectedScrapePools.length }} 个服务</small></div><span>{{ discoveryRange.from }}–{{ discoveryRange.to }} / {{ discoveryTotal }} 项</span></div>
            <div v-for="(item, index) in discoveryTargets" :key="`${item.scrapePool}-${item.scrapeUrl || item.discoveredLabels?.__address__}-${index}`" class="discovery-row" :class="{ dropped: discoveryState === 'dropped' }">
              <div><strong>{{ item.labels?.job || item.discoveredLabels?.job || item.scrapePool || '未命名目标' }}</strong><span class="target-service">{{ item.scrapePool || '未知服务' }}</span><small>{{ item.scrapeUrl || item.labels?.instance || item.discoveredLabels?.__address__ || '—' }}</small></div>
              <div v-if="discoveryState === 'active'"><span>最终标签</span><code>{{ labelMap(item.labels) || '—' }}</code></div>
              <div><span>发现标签</span><code>{{ labelMap(item.discoveredLabels) || '—' }}</code></div>
              <span v-if="discoveryState === 'active'" class="badge" :class="item.health === 'up' ? 'ok' : 'bad'">{{ item.health || 'unknown' }}</span>
            </div>
            <div v-if="!pageLoading && !discoveryTargets.length" class="empty">所选服务没有{{ discoveryState === 'active' ? '活动' : '已丢弃' }}目标</div>
            <div class="pagination"><button type="button" :disabled="pageLoading || !discoveryRange.canPrevious" @click="moveDiscoveryPage(-1)"><ChevronLeft :size="15" />上一页</button><span>第 {{ discoveryPage }} 页</span><button type="button" :disabled="pageLoading || !discoveryHasNext" @click="moveDiscoveryPage(1)">下一页<ChevronRight :size="15" /></button></div>
          </section>
        </div>
      </template>
      <footer><Clock3 :size="13" />只读 Prometheus HTTP API</footer>
      </template>
    </main>

    <div v-if="metricPanel" class="modal-backdrop" role="presentation" @click.self="metricPanel = ''"><section class="metric-dialog" role="dialog" aria-modal="true" aria-labelledby="metric-dialog-title"><header><div><h2 id="metric-dialog-title">指标浏览器</h2><span>{{ metrics.length }} 个指标，最多显示前 500 条</span></div><button class="icon-button" type="button" title="关闭" aria-label="关闭指标浏览器" @click="metricPanel = ''"><X :size="18" /></button></header><label class="metric-search"><Search :size="17" /><input v-model="metricFilter" type="search" placeholder="按指标名搜索" aria-label="按指标名搜索" autofocus /></label><div class="metric-list"><div v-if="metricsLoading" class="empty">正在读取指标名称…</div><div v-else-if="metricError" class="message error"><AlertCircle :size="16" />{{ metricError }}</div><button v-for="metric in visibleMetrics" :key="metric" type="button" @click="useMetric(metric)"><code>{{ metric }}</code><span>使用</span></button><div v-if="!metricsLoading && !metricError && !visibleMetrics.length" class="empty">没有匹配的指标</div></div></section></div>
  </div>
</template>
