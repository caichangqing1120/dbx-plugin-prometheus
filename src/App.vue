<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Activity, Search, Server, Bell, ListFilter, RefreshCw, Play, AlertCircle, Clock3 } from '@lucide/vue';
import { invoke, type Context } from './bridge';
import { plot, seriesName, numericValue, type Alert, type Envelope, type QueryResult, type RuleGroup, type Series, type Target } from './domain';

type Tab = 'query' | 'targets' | 'alerts' | 'rules';
type Info = { name: string; baseUrl: string; environment: string; build: Envelope<{ version?: string }> };
const tabs = [
  { id: 'query', title: 'PromQL', icon: Search },
  { id: 'targets', title: '采集目标', icon: Server },
  { id: 'alerts', title: '活动告警', icon: Bell },
  { id: 'rules', title: '规则', icon: ListFilter },
] as const;
const tab = ref<Tab>('query');
const connectionId = ref(''), info = ref<Info>();
const query = ref('up'), mode = ref<'instant' | 'range'>('instant'), hours = ref(1), step = ref(30);
const result = ref<QueryResult>(), targets = ref<Target[]>([]), alerts = ref<Alert[]>([]), groups = ref<RuleGroup[]>([]);
const filter = ref(''), loading = ref(false), error = ref(''), warnings = ref<string[]>([]), fetchedAt = ref('');
let generation = 0, unsubscribe: (() => void) | undefined;
watch([query, mode, hours, step], () => { generation++; result.value = undefined; loading.value = false; warnings.value = []; fetchedAt.value = ''; });
const samples = computed(() => {
  if (!result.value) return [] as Series[];
  if (result.value.resultType === 'vector' || result.value.resultType === 'matrix') return result.value.result as Series[];
  return [{ metric: {}, value: result.value.result as [number, string] }];
});
const visibleTargets = computed(() => targets.value.filter(t => `${t.scrapePool} ${t.labels?.instance} ${t.scrapeUrl}`.toLowerCase().includes(filter.value.toLowerCase())));
const visibleAlerts = computed(() => alerts.value.filter(a => `${a.labels?.alertname} ${a.labels?.severity} ${a.annotations?.summary}`.toLowerCase().includes(filter.value.toLowerCase())));
const visibleGroups = computed(() => groups.value.filter(g => `${g.name} ${g.file} ${g.rules?.map(r => r.name).join(' ')}`.toLowerCase().includes(filter.value.toLowerCase())));
const healthy = computed(() => targets.value.filter(t => t.health === 'up').length);
const currentTitle = computed(() => tabs.find(t => t.id === tab.value)?.title);
function labelMap(value?: Record<string, string>): string { return Object.entries(value || {}).filter(([k]) => k !== '__name__').map(([key, val]) => `${key}=${val}`).join(' · '); }
function date(value?: string): string { if (!value) return '—'; const d = new Date(value); return Number.isNaN(d.getTime()) ? value : d.toLocaleString(); }
function showError(err: unknown): string { return err instanceof Error ? err.message : String(err); }
async function load() {
  const id = connectionId.value, gen = ++generation;
  if (!id) { error.value = '请从 DBX 连接打开工作台'; return; }
  loading.value = true; error.value = ''; warnings.value = [];
  try {
    if (tab.value === 'query') {
      const form: Record<string, unknown> = { query: query.value };
      if (mode.value === 'range') {
        const end = Math.floor(Date.now() / 1000);
        Object.assign(form, { start: end - hours.value * 3600, end, step: Number(step.value) });
      }
      const response = await invoke<Envelope<QueryResult>>(id, mode.value === 'range' ? 'prometheus/query_range' : 'prometheus/query', { form });
      if (gen !== generation) return;
      result.value = response.data; warnings.value = response.warnings || [];
    } else if (tab.value === 'targets') {
      const response = await invoke<Envelope<{ activeTargets?: Target[] }>>(id, 'prometheus/targets');
      if (gen !== generation) return;
      targets.value = response.data.activeTargets || []; warnings.value = response.warnings || [];
    } else if (tab.value === 'alerts') {
      const response = await invoke<Envelope<{ alerts?: Alert[] }>>(id, 'prometheus/alerts');
      if (gen !== generation) return;
      alerts.value = response.data.alerts || []; warnings.value = response.warnings || [];
    } else {
      const response = await invoke<Envelope<{ groups?: RuleGroup[] }>>(id, 'prometheus/rules');
      if (gen !== generation) return;
      groups.value = response.data.groups || []; warnings.value = response.warnings || [];
    }
    fetchedAt.value = new Date().toLocaleTimeString();
  } catch (err) {
    if (gen === generation) { error.value = showError(err); result.value = undefined; targets.value = []; alerts.value = []; groups.value = []; }
  } finally { if (gen === generation) loading.value = false; }
}
function select(next: Tab) { if (tab.value === next) return; tab.value = next; filter.value = ''; load(); }
async function connect(context: Context) {
  generation++;
  loading.value = false;
  connectionId.value = context.connectionId || '';
  info.value = undefined; result.value = undefined; targets.value = []; alerts.value = []; groups.value = [];
  if (!connectionId.value) { error.value = '请从 DBX 连接打开工作台'; return; }
  const id = connectionId.value;
  try { const response = await invoke<Info>(id, 'prometheus/info'); if (connectionId.value === id) { info.value = response; load(); } }
  catch (err) { if (connectionId.value === id) error.value = showError(err); }
}
onMounted(async () => {
  if (!window.dbxPlugin) { error.value = '请从 DBX 连接打开工作台'; return; }
  await window.dbxPlugin.ready;
  unsubscribe = window.dbxPlugin.onContext(connect);
  connect(window.dbxPlugin.context);
});
onBeforeUnmount(() => { generation++; unsubscribe?.(); });
</script>

<template>
  <div class="workspace">
    <aside class="sidebar">
      <div class="brand"><span class="logo"><Activity :size="22" /></span><div><strong>Prometheus</strong><small>DBX 工作台</small></div></div>
      <nav aria-label="工作台导航">
        <button v-for="item in tabs" :key="item.id" type="button" :class="{ active: tab === item.id }" @click="select(item.id)"><component :is="item.icon" :size="17" /><span>{{ item.title }}</span></button>
      </nav>
      <div class="sidebar-meta"><span class="status-dot" :class="{ disconnected: !info }"></span><span>{{ info?.environment || info?.name || '未连接' }}</span><small>{{ info?.build.data?.version || '' }}</small></div>
    </aside>
    <main>
      <header class="topbar"><div><span class="breadcrumb">监控 / {{ currentTitle }}</span><h1>{{ currentTitle }}</h1></div><div class="top-actions"><span v-if="fetchedAt" class="updated">更新于 {{ fetchedAt }}</span><button class="icon-button" type="button" aria-label="刷新" title="刷新" :disabled="loading || !connectionId" @click="load"><RefreshCw :size="17" :class="{ spinning: loading }" /></button></div></header>
      <div v-if="error" class="message error" role="alert"><AlertCircle :size="16" />{{ error }}</div>
      <div v-for="warning in warnings" :key="warning" class="message warning" role="status"><AlertCircle :size="16" />{{ warning }}</div>

      <template v-if="tab === 'query'">
        <form class="query-form" @submit.prevent="load"><label for="promql">PromQL</label><div class="editor"><textarea id="promql" v-model="query" rows="2" spellcheck="false" placeholder="up" :disabled="loading"></textarea><button type="submit" class="run" :disabled="loading || !connectionId || !query.trim()"><Play :size="15" fill="currentColor" />运行</button></div>
          <div class="query-options"><div class="segmented" role="group" aria-label="查询方式"><button type="button" :class="{ selected: mode === 'instant' }" @click="mode = 'instant'">即时</button><button type="button" :class="{ selected: mode === 'range' }" @click="mode = 'range'">区间</button></div><template v-if="mode === 'range'"><label for="range">时间范围</label><select id="range" v-model.number="hours"><option :value="1">最近 1 小时</option><option :value="6">最近 6 小时</option><option :value="24">最近 24 小时</option></select><label for="step">步长</label><select id="step" v-model.number="step"><option :value="30">30 秒</option><option :value="60">1 分钟</option><option :value="300">5 分钟</option></select></template></div>
        </form>
        <section class="results"><div class="section-head"><h2>查询结果</h2><span>{{ samples.length }} 条序列</span></div><div v-if="!loading && result && samples.length === 0" class="empty">当前查询无数据</div><div v-if="!result && !loading && !error" class="empty">运行查询以查看结果</div>
          <div v-if="result && (result.resultType === 'scalar' || result.resultType === 'string')" class="scalar"><span>{{ result.resultType }}</span><strong>{{ numericValue((result.result as [number, string])) }}</strong></div>
          <div v-for="(series, index) in (result?.resultType === 'scalar' || result?.resultType === 'string' ? [] : samples.slice(0, 100))" :key="index" class="series"><div class="series-header"><code :title="seriesName(series.metric)">{{ seriesName(series.metric) }}</code><strong v-if="series.value">{{ numericValue(series.value) }}</strong><strong v-else>{{ numericValue(series.values?.at(-1)) }}</strong></div><div v-if="series.values?.length" class="chart"><svg viewBox="0 0 900 230" preserveAspectRatio="none" role="img" :aria-label="`${seriesName(series.metric)} 时序曲线`"><line x1="0" y1="229" x2="900" y2="229" stroke="currentColor" opacity=".18" /><polyline :points="plot(series.values)" fill="none" stroke="currentColor" stroke-width="2" vector-effect="non-scaling-stroke" /></svg><div class="chart-axis"><span>{{ date(new Date(series.values[0][0] * 1000).toISOString()) }}</span><span>{{ date(new Date(series.values.at(-1)![0] * 1000).toISOString()) }}</span></div></div></div><div v-if="samples.length > 100" class="limit">仅展示前 100 条序列；请用 PromQL 缩小范围。</div>
        </section>
      </template>
      <template v-else>
        <div class="list-tools"><label class="search"><Search :size="16" /><input v-model="filter" type="search" :placeholder="`筛选${currentTitle}`" :aria-label="`筛选${currentTitle}`" /></label><span v-if="tab === 'targets'">{{ healthy }} / {{ targets.length }} 正常</span><span v-else-if="tab === 'alerts'">{{ alerts.length }} 条活动告警</span><span v-else>{{ groups.length }} 个规则组</span></div>
        <div v-if="tab === 'targets'" class="table-wrap"><table><thead><tr><th>目标 / 任务</th><th>状态</th><th>最近采集</th><th>错误</th></tr></thead><tbody><tr v-for="(item, i) in visibleTargets" :key="i"><td><strong>{{ item.labels?.instance || item.scrapeUrl || '—' }}</strong><small>{{ item.scrapePool }} · {{ labelMap(item.labels) }}</small></td><td><span class="badge" :class="item.health === 'up' ? 'ok' : 'bad'">{{ item.health || 'unknown' }}</span></td><td>{{ date(item.lastScrape) }}</td><td class="error-text">{{ item.lastError || '—' }}</td></tr><tr v-if="!visibleTargets.length"><td colspan="4" class="empty">{{ loading ? '加载中…' : '没有匹配的采集目标' }}</td></tr></tbody></table></div>
        <div v-if="tab === 'alerts'" class="table-wrap"><table><thead><tr><th>告警</th><th>状态</th><th>开始时间</th><th>值</th></tr></thead><tbody><tr v-for="(item, i) in visibleAlerts" :key="i"><td><strong>{{ item.labels?.alertname || '未命名告警' }}</strong><small>{{ item.annotations?.summary || labelMap(item.labels) }}</small></td><td><span class="badge" :class="item.state === 'firing' ? 'bad' : 'pending'">{{ item.state || 'unknown' }}</span></td><td>{{ date(item.activeAt) }}</td><td>{{ item.value || '—' }}</td></tr><tr v-if="!visibleAlerts.length"><td colspan="4" class="empty">{{ loading ? '加载中…' : '没有活动告警' }}</td></tr></tbody></table></div>
        <div v-if="tab === 'rules'" class="rule-list"><section v-for="(group, i) in visibleGroups" :key="i" class="rule-group"><div class="group-head"><h2>{{ group.name || '未命名规则组' }}</h2><small>{{ group.file || '' }}</small></div><div v-for="(rule, j) in group.rules || []" :key="j" class="rule"><div><strong>{{ rule.name || '未命名规则' }}</strong><code>{{ rule.query || '' }}</code><small v-if="rule.lastError" class="error-text">{{ rule.lastError }}</small></div><span class="badge" :class="rule.health === 'ok' ? 'ok' : 'bad'">{{ rule.health || rule.state || rule.type || '—' }}</span></div></section><div v-if="!visibleGroups.length" class="empty">{{ loading ? '加载中…' : '没有匹配的规则组' }}</div></div>
      </template>
      <footer><Clock3 :size="13" />只读 Prometheus HTTP API</footer>
    </main>
  </div>
</template>
