<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NInput, NSelect } from 'naive-ui'
import { PhPlus, PhX } from '@phosphor-icons/vue'
import { text } from '../tasks/tasks'
import { needsValue, operatorOptions } from './pullConditions'
import type { Document, DocumentFilter, DocumentPullConditions } from '../../shared/types/types'
const props = defineProps<{ document: Document; modelValue: DocumentPullConditions; disabled: boolean; pendingEdits: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: DocumentPullConditions] }>()
const fields = computed(() => props.document.sheets.flatMap(sheet => sheet.headers.map((header, i) => ({
  label: `${props.document.sheets.length > 1 ? `${sheet.name} / ` : ''}${text(header) || '空白表头'} · 第 ${i + 1} 列`,
  value: `${sheet.name}:${i + 1}`, sheet: sheet.name, column: i + 1, header: String(header ?? ''),
}))))
const locked = computed(() => props.disabled || props.pendingEdits)
function update(filters: DocumentFilter[]) { emit('update:modelValue', { ...props.modelValue, filters }) }
function add() {
  const field = fields.value[0]
  if (field) update([...(props.modelValue.filters ?? []), { sheet: field.sheet, column: field.column, header: field.header, operator: 'equals', value: '' }])
}
function change(index: number, patch: Partial<DocumentFilter>) {
  update((props.modelValue.filters ?? []).map((rule, i) => i === index ? { ...rule, ...patch } : rule))
}
function choose(index: number, value: string) {
  const field = fields.value.find(option => option.value === value)
  if (field) change(index, { sheet: field.sheet, column: field.column, header: field.header })
}
</script>

<template>
  <section class="pull-editor" aria-label="编辑拉取条件">
    <div class="pull-editor-heading"><strong>拉取条件</strong><span>全部满足 · 按单元格文字匹配</span></div>
    <p class="pull-help">选择原表任意列，保存后在下次手动或定时同步时生效。</p>
    <p v-if="pendingEdits" class="pull-pending">请先回写或撤销待同步的状态、备注修改，再更改拉取条件。</p>
    <div v-if="modelValue.owner" class="legacy-condition"><span>原有负责人条件：{{ modelValue.owner }}</span><NButton size="tiny" :disabled="locked" @click="emit('update:modelValue', { ...modelValue, owner: '' })">移除</NButton></div>
    <div v-for="(rule, index) in modelValue.filters" :key="index" class="pull-rule">
      <NSelect :value="`${rule.sheet}:${rule.column}`" :options="fields" :disabled="locked" :aria-label="`条件 ${index + 1} 字段`" size="small" @update:value="choose(index, $event)" />
      <NSelect :value="rule.operator" :options="operatorOptions" :disabled="locked" :aria-label="`条件 ${index + 1} 关系`" size="small" @update:value="change(index, { operator: $event })" />
      <NInput v-if="needsValue(rule)" :value="rule.value" :input-props="{ 'aria-label': `条件 ${index + 1} 内容` }" placeholder="匹配内容" :maxlength="1000" :disabled="locked" size="small" @update:value="change(index, { value: $event })" />
      <span v-else class="pull-no-value">无需填写内容</span>
      <button type="button" class="icon-button" :aria-label="`删除条件 ${index + 1}`" :disabled="locked" @click="update((modelValue.filters ?? []).filter((_, i) => i !== index))"><PhX :size="15" /></button>
    </div>
    <p v-if="!modelValue.filters?.length && !modelValue.owner" class="pull-help">未添加条件，将拉取全部任务。</p>
    <NButton size="small" quaternary :disabled="locked || !fields.length || (modelValue.filters?.length ?? 0) >= 30" @click="add"><template #icon><PhPlus :size="14" /></template>添加条件</NButton>
  </section>
</template>

<style scoped>
.pull-editor { margin: 0 0 24px; padding-top: 18px; border-top: 1px solid var(--line); }
.pull-editor-heading, .legacy-condition { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.pull-editor-heading strong { font-size: 13px; font-weight: 500; }
.pull-editor-heading > span, .pull-help { color: var(--muted); font-size: 11px; line-height: 1.7; }
.pull-rule { display: grid; grid-template-columns: minmax(130px, 1fr) 115px minmax(120px, 1fr) 24px; gap: 8px; align-items: center; margin-bottom: 10px; }
.pull-no-value { font-size: 11px; color: var(--subtle); }
.pull-pending { color: #a86600; font-size: 12px; line-height: 1.7; }
.legacy-condition { font-size: 12px; margin: 10px 0; }
@media (max-width: 640px) { .pull-rule { grid-template-columns: minmax(0, 1fr) 110px 24px; } .pull-rule > :nth-child(3) { grid-row: 2; grid-column: 1 / 3; } .pull-rule > :last-child { grid-row: 1; grid-column: 3; } }
</style>
