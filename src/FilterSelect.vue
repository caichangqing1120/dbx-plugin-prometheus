<script setup lang="ts">
import { computed, nextTick, ref } from 'vue';
import { Check, ChevronDown, Search } from '@lucide/vue';
import type { FilterOption } from './workbench';

const props = withDefaults(defineProps<{
  modelValue: string;
  options: FilterOption[];
  placeholder: string;
  allLabel?: string;
}>(), { allLabel: '全部' });
const emit = defineEmits<{ 'update:modelValue': [value: string] }>();

const root = ref<HTMLElement>(), searchInput = ref<HTMLInputElement>();
const open = ref(false), query = ref(''), activeIndex = ref(0);
const selected = computed(() => props.options.find(item => item.value === props.modelValue));
const matchingOptions = computed(() => {
  const keyword = query.value.trim().toLowerCase();
  return props.options.filter(item => !keyword || `${item.label} ${item.description || ''} ${item.searchText || ''}`.toLowerCase().includes(keyword));
});
const visibleOptions = computed(() => matchingOptions.value.slice(0, 200));

async function showOptions() {
  open.value = true;
  query.value = '';
  activeIndex.value = 0;
  await nextTick();
  searchInput.value?.focus();
}
function hideOptions() { open.value = false; query.value = ''; }
function select(value: string) { emit('update:modelValue', value); hideOptions(); }
function move(step: number) {
  if (!visibleOptions.value.length) return;
  activeIndex.value = (activeIndex.value + step + visibleOptions.value.length) % visibleOptions.value.length;
}
function selectActive() {
  const option = visibleOptions.value[activeIndex.value];
  if (option) select(option.value);
}
function handleFocusOut(event: FocusEvent) {
  const next = event.relatedTarget;
  if (!(next instanceof Node) || !root.value?.contains(next)) hideOptions();
}
</script>

<template>
  <div ref="root" class="filter-select" @focusout="handleFocusOut" @keydown.esc="hideOptions">
    <button
      class="filter-select-trigger"
      type="button"
      role="combobox"
      :aria-expanded="open"
      aria-haspopup="listbox"
      @click="open ? hideOptions() : showOptions()"
      @keydown.down.prevent="showOptions"
    >
      <span><small>{{ placeholder }}</small><strong>{{ selected?.label || allLabel }}</strong></span>
      <ChevronDown :size="16" :class="{ rotated: open }" />
    </button>
    <div v-if="open" class="filter-select-menu">
      <label class="filter-select-search"><Search :size="15" /><input ref="searchInput" v-model="query" type="search" :placeholder="`搜索${placeholder}`" @input="activeIndex = 0" @keydown.down.prevent="move(1)" @keydown.up.prevent="move(-1)" @keydown.enter.prevent="selectActive" /></label>
      <div class="filter-select-options" role="listbox">
        <button type="button" role="option" :aria-selected="!modelValue" :class="{ selected: !modelValue }" @mousedown.prevent @click="select('')"><Check :size="15" /><span><strong>{{ allLabel }}</strong></span></button>
        <button v-for="(option, index) in visibleOptions" :key="option.value" type="button" role="option" :aria-selected="modelValue === option.value" :class="{ selected: modelValue === option.value, active: activeIndex === index }" @mouseenter="activeIndex = index" @mousedown.prevent @click="select(option.value)"><Check :size="15" /><span><strong>{{ option.label }}</strong><small v-if="option.description">{{ option.description }}</small></span></button>
        <div v-if="!visibleOptions.length" class="filter-select-empty">没有匹配项</div>
      </div>
      <footer v-if="matchingOptions.length > visibleOptions.length">还有 {{ matchingOptions.length - visibleOptions.length }} 项，请输入关键字缩小范围</footer>
    </div>
  </div>
</template>
