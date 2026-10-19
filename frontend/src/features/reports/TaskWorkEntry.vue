<script setup lang="ts">
import { ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import { PhNotebook } from '@phosphor-icons/vue'
import { api } from '../../shared/api/api'

const props = defineProps<{ documentKey: string; sheet: string; row: number; disabled: boolean }>()
const note = ref(''), saving = ref(false), error = ref(''), saved = ref(false)
async function record() {
  if (!note.value.trim() || saving.value || props.disabled) return
  saving.value = true; error.value = ''; saved.value = false
  try {
    await api.RecordTaskWork(props.documentKey, props.sheet, props.row, note.value)
    note.value = ''; saved.value = true
  } catch (exc) { error.value = String(exc) }
  finally { saving.value = false }
}
</script>

<template>
  <section class="work-entry" aria-label="记录任务工作进展">
    <h3><PhNotebook :size="16" />工作进展</h3>
    <p>状态、备注修改会自动记录；实际做了什么，也可以在这里补充。</p>
    <NInput v-model:value="note" type="textarea" :autosize="{ minRows: 2, maxRows: 6 }" :maxlength="10000"
      placeholder="例如：完成联调，发现登录接口超时，等待后端排查" aria-label="今日工作进展" :disabled="saving || disabled" @update:value="saved = false" />
    <div class="work-entry-actions"><span>仅保存在本机，用于日报整理</span><NButton size="small" :loading="saving" :disabled="disabled || !note.trim()" @click="record">记入日报</NButton></div>
    <p v-if="error" class="work-entry-error" role="alert">{{ error }}</p>
    <p v-else-if="saved" class="work-entry-saved" role="status">已记入今天的操作记录，可在侧栏「工作日报」查看。</p>
  </section>
</template>

<style scoped>
.work-entry { margin-top: 24px; border-top: 1px solid var(--line); padding-top: 24px; }
.work-entry p { font-size: 12px; color: var(--muted); line-height: 1.8; margin: 0 0 12px; }
.work-entry-actions { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-top: 10px; }
.work-entry-actions > span { color: var(--subtle); font-size: 11px; }
.work-entry .work-entry-error { color: var(--danger); margin-top: 12px; }
.work-entry .work-entry-saved { color: var(--success); margin-top: 12px; }
</style>
