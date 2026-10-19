<script setup lang="ts">
import { conditionSummary, conditionsPending } from './pullConditions'
import type { Document } from '../../shared/types/types'
defineProps<{ document: Document }>()
</script>

<template>
  <div class="document-pull-bar" aria-label="文档拉取条件">
    <span class="pull-label">拉取条件</span>
    <span class="pull-summary" :title="conditionSummary(document)">{{ conditionSummary(document) }}</span>
    <span v-if="conditionsPending(document)" class="pull-pending">{{ document.setup_pending ? '已识别表头，请在文档设置中设置条件后同步' : '条件已修改，待同步' }}</span>
  </div>
</template>

<style scoped>
.document-pull-bar { display: flex; align-items: center; flex-wrap: wrap; flex: 1; min-width: 0; gap: 10px; font-size: 12px; }
.pull-label { flex-shrink: 0; color: var(--muted); }
.pull-summary { max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pull-pending { color: #a86600; font-size: 11px; }
</style>
