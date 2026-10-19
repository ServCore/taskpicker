<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
const props = defineProps<{ source: string }>()
// AI and task text are untrusted. Render Markdown with no executable HTML.
const html = computed(() => DOMPurify.sanitize(marked.parse(props.source, { async: false, breaks: true }), { USE_PROFILES: { html: true }, FORBID_TAGS: ['img', 'style', 'form', 'input', 'button', 'video', 'audio'], FORBID_ATTR: ['style', 'id', 'name'] }))
</script>

<template><div class="markdown-document" v-html="html" /></template>

<style scoped>
.markdown-document { font-size: 12px; line-height: 1.85; overflow-wrap: anywhere; }
.markdown-document :deep(h1) { font-size: 18px; line-height: 1.4; margin: 0 0 16px; font-weight: 600; }
.markdown-document :deep(h2) { font-size: 14px; line-height: 1.5; margin: 24px 0 10px; border-bottom: 1px solid var(--line); padding-bottom: 7px; font-weight: 600; }
.markdown-document :deep(h3) { font-size: 12px; margin: 16px 0 6px; font-weight: 600; }
.markdown-document :deep(p) { margin: 7px 0; }
.markdown-document :deep(ul), .markdown-document :deep(ol) { padding-left: 20px; margin: 8px 0; }
.markdown-document :deep(li) { margin: 4px 0; }
.markdown-document :deep(blockquote) { border-left: 3px solid #d7e5ff; background: #f7f9fc; margin: 8px 0; padding: 6px 12px; color: var(--muted); }
.markdown-document :deep(pre) { padding: 10px; background: #f7f9fc; overflow: auto; white-space: pre-wrap; }
.markdown-document :deep(code) { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }
.markdown-document :deep(table) { border-collapse: collapse; width: 100%; font-size: 11px; }
.markdown-document :deep(th), .markdown-document :deep(td) { border: 1px solid var(--line); padding: 5px 8px; text-align: left; }
.markdown-document :deep(a) { color: var(--primary); }
</style>
