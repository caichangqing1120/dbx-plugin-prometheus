<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { basicSetup } from 'codemirror';
import { EditorState } from '@codemirror/state';
import { EditorView, keymap, placeholder as placeholderExtension } from '@codemirror/view';
import { oneDark } from '@codemirror/theme-one-dark';
import { PromQLExtension, type PrometheusClient } from '@prometheus-io/codemirror-promql';

const props = withDefaults(defineProps<{
  modelValue: string;
  dark?: boolean;
  disabled?: boolean;
  autocomplete?: boolean;
  highlighting?: boolean;
  linter?: boolean;
  placeholder?: string;
  prometheusClient?: PrometheusClient;
}>(), {
  dark: false,
  disabled: false,
  autocomplete: true,
  highlighting: true,
  linter: true,
  placeholder: '输入 PromQL 表达式',
});
const emit = defineEmits<{ 'update:modelValue': [value: string]; execute: [] }>();
const host = ref<HTMLElement>();
let editor: EditorView | undefined;

function mountEditor() {
  if (!host.value) return;
  editor?.destroy();
  const promQL = new PromQLExtension().activateCompletion(props.autocomplete).activateLinter(props.linter);
  if (props.prometheusClient) promQL.setComplete({ remote: props.prometheusClient, maxMetricsMetadata: 0 });
  editor = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        basicSetup,
        promQL.asExtension(),
        placeholderExtension(props.placeholder),
        EditorState.readOnly.of(props.disabled),
        keymap.of([{ key: 'Shift-Enter', run: () => { emit('execute'); return true; } }]),
        EditorView.updateListener.of(update => {
          if (update.docChanged) emit('update:modelValue', update.state.doc.toString());
        }),
        EditorView.theme({
          '&': { minHeight: '46px', backgroundColor: 'transparent' },
          '.cm-scroller': { fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', lineHeight: '1.5' },
          '.cm-content': { padding: '11px 0' },
          '.cm-line': { padding: '0 12px' },
          '.cm-gutters': { display: 'none' },
          '&.cm-focused': { outline: 'none' },
        }),
        ...(props.dark ? [oneDark] : []),
      ],
    }),
  });
  editor.dom.classList.toggle('no-highlight', !props.highlighting);
}

onMounted(mountEditor);
onBeforeUnmount(() => editor?.destroy());
watch(() => [props.dark, props.disabled, props.autocomplete, props.highlighting, props.linter, props.prometheusClient], mountEditor);
watch(() => props.modelValue, value => {
  if (!editor || editor.state.doc.toString() === value) return;
  editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value } });
});
</script>

<template><div ref="host" class="promql-editor" :class="{ disabled }"></div></template>
