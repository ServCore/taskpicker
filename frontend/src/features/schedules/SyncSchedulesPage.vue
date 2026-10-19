<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NForm, NFormItem, NInputNumber, NPopconfirm, NSelect, NSwitch } from 'naive-ui'
import { PhArrowsClockwise, PhCaretLeft, PhClock, PhPlus } from '@phosphor-icons/vue'
import { api } from '../../shared/api/api'
import type { Document, Project, SyncSchedule, SyncScheduleInput } from '../../shared/types/types'

const props = defineProps<{
  projects: Project[]; documents: Document[]; schedules: SyncSchedule[]; runningID: string
  blocked: boolean; loadError: string; embedded?: boolean
}>()
const emit = defineEmits<{ close: []; updated: [items: SyncSchedule[]]; refresh: []; run: [id: string]; lock: [value: boolean] }>()
const editingID = ref<string | null>(null), editingRevision = ref(''), projectID = ref('')
const interval = ref<number | null>(30), unit = ref(1), enabled = ref(true)
const saving = ref(false), error = ref('')
const baseline = ref('')
const draft = () => JSON.stringify([projectID.value, interval.value, unit.value, enabled.value])
const changed = computed(() => editingID.value !== null && draft() !== baseline.value)
watch(() => saving.value || changed.value, value => emit('lock', value), { immediate: true, flush: 'sync' })
const projectOptions = computed(() => props.projects.map(project => ({
  label: project.name, value: project.id,
  disabled: props.schedules.some(item => item.project_id === project.id && item.id !== editingID.value),
})))
const units = [{ label: '分钟', value: 1 }, { label: '小时', value: 60 }, { label: '天', value: 1440 }]
const projectName = (id: string) => props.projects.find(project => project.id === id)?.name ?? '项目已不可用'
const documentCount = (id: string) => {
  const keys = props.projects.find(project => project.id === id)?.document_keys ?? []
  return props.documents.filter(doc => keys.includes(doc.key) && doc.source_url).length
}
function formatTime(value: string) {
  return value ? new Date(value).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) : '—'
}
function formatInterval(minutes: number) {
  return minutes % 1440 === 0 ? `${minutes / 1440} 天` : minutes % 60 === 0 ? `${minutes / 60} 小时` : `${minutes} 分钟`
}
function edit(item?: SyncSchedule) {
  editingID.value = item?.id ?? ''; editingRevision.value = item?.revision ?? ''
  projectID.value = item?.project_id ?? projectOptions.value.find(option => !option.disabled)?.value ?? ''
  const minutes = item?.interval_minutes ?? 30
  unit.value = minutes % 1440 === 0 ? 1440 : minutes % 60 === 0 ? 60 : 1
  interval.value = minutes / unit.value; enabled.value = item?.enabled ?? true
  baseline.value = draft(); error.value = ''
}
function discard() { editingID.value = null; error.value = '' }
async function save() {
  if (saving.value) return
  const minutes = Number(interval.value) * unit.value
  if (!projectID.value || interval.value == null || !Number.isInteger(interval.value) || !Number.isInteger(minutes) || minutes < 1 || minutes > 43200) {
    error.value = '请选择项目，循环间隔应为 1 分钟至 30 天，填写整数。'; return
  }
  await mutate(() => api.SaveSyncSchedule({ id: editingID.value ?? '', revision: editingRevision.value, project_id: projectID.value, interval_minutes: minutes, enabled: enabled.value }), true)
}
async function mutate(action: () => Promise<SyncSchedule[]>, close = false) {
  saving.value = true; error.value = ''
  try { emit('updated', await action()); if (close) editingID.value = null }
  catch (exc) { error.value = String(exc) }
  finally { saving.value = false }
}
function remove(item: SyncSchedule) { return mutate(() => api.DeleteSyncSchedule(item.id, item.revision)) }
function toggle(item: SyncSchedule, value: boolean) {
  const input: SyncScheduleInput = { ...item, enabled: value }
  void mutate(() => api.SaveSyncSchedule(input))
}
</script>

<template>
  <main class="workspace schedules-page" aria-label="定时任务">
    <header class="workspace-header">
      <div class="breadcrumb"><PhClock :size="17" /><template v-if="!embedded"><span>设置</span><span class="breadcrumb-separator">/</span></template><h1>定时任务</h1></div>
      <NButton v-if="!embedded" size="small" :disabled="saving || changed" @click="emit('close')"><template #icon><PhCaretLeft :size="15" /></template>返回</NButton>
    </header>
    <div class="schedules-scroll">
      <div class="schedules-content">
        <div class="schedules-heading"><div><h2>定时拉取文档</h2><p>按项目循环同步在线文档，沿用每份文档的拉取条件。</p></div><NButton type="primary" size="small" :disabled="saving || editingID !== null || !!loadError || !projects.length" @click="edit()"><template #icon><PhPlus :size="15" /></template>新建定时任务</NButton></div>
        <div v-if="error || loadError" class="schedule-error" role="alert">{{ error || loadError }} <NButton v-if="loadError" size="tiny" :disabled="saving" @click="emit('refresh')">重新加载</NButton></div>
        <NForm v-if="editingID !== null" class="schedule-editor" label-placement="top" @submit.prevent="save">
          <h3>{{ editingID ? '编辑定时任务' : '新建定时任务' }}</h3>
          <div class="schedule-form-row"><NFormItem label="项目"><NSelect v-model:value="projectID" aria-label="定时任务项目" :options="projectOptions" placeholder="选择项目" :disabled="saving" /></NFormItem><NFormItem label="每隔"><div class="schedule-interval"><NInputNumber v-model:value="interval" :input-props="{ 'aria-label': '循环间隔' }" :min="1" :max="Math.floor(43200 / unit)" :precision="0" :disabled="saving" /><NSelect v-model:value="unit" aria-label="间隔单位" :options="units" :disabled="saving" /></div></NFormItem></div>
          <div class="schedule-enabled"><NSwitch v-model:value="enabled" aria-label="启用定时任务" :disabled="saving" /><span>启用循环拉取</span></div>
          <p class="schedule-help">项目当前有 {{ documentCount(projectID) }} 份在线文档。保存或恢复后等待一个间隔再执行，也可立即拉取。</p>
          <div class="schedule-editor-actions"><div class="schedule-return"><NButton :disabled="saving" @click="discard">返回</NButton><small>直接返回不会保存</small></div><NButton type="primary" attr-type="submit" :loading="saving" :disabled="saving">保存定时任务</NButton></div>
        </NForm>
        <div v-if="!schedules.length && editingID === null && !loadError" class="schedules-empty"><PhClock :size="30" /><strong>还没有定时任务</strong><span>{{ projects.length ? '选择一个项目，设置文档拉取间隔。' : '请先创建项目并添加文档。' }}</span></div>
        <article v-for="item in schedules" :key="item.id" class="schedule-item">
          <div class="schedule-item-heading"><div><strong>{{ projectName(item.project_id) }}</strong><span class="schedule-kind">文档拉取</span></div><div class="schedule-item-toggle"><span>{{ runningID === item.id ? '正在拉取' : item.enabled ? '已启用' : '已暂停' }}</span><NSwitch :value="item.enabled" :aria-label="`${projectName(item.project_id)} 定时任务开关`" :disabled="saving || editingID !== null" @update:value="toggle(item, $event)" /></div></div>
          <p class="schedule-scope">每 {{ formatInterval(item.interval_minutes) }} · {{ documentCount(item.project_id) }} 份在线文档</p>
          <dl><div><dt>下次执行</dt><dd>{{ item.enabled ? formatTime(item.next_run_at) : '已暂停' }}</dd></div><div><dt>上次完成</dt><dd>{{ formatTime(item.last_run_at) }}</dd></div></dl>
          <p v-if="item.enabled && blocked && runningID !== item.id" class="schedule-help">到期后会等待当前同步、编辑或弹窗操作结束。</p>
          <p class="schedule-result" :title="item.last_result">{{ item.last_result || '尚未执行' }}</p>
          <div class="schedule-item-actions"><NButton size="small" :loading="runningID === item.id" :disabled="blocked || !!runningID || saving || editingID !== null || !!loadError || !projects.some(project => project.id === item.project_id)" @click="emit('run', item.id)"><template #icon><PhArrowsClockwise :size="14" /></template>立即拉取</NButton><NButton size="small" :disabled="saving || editingID !== null" @click="edit(item)">编辑</NButton><NPopconfirm @positive-click="remove(item)"><template #trigger><NButton size="small" quaternary :disabled="saving || editingID !== null">删除</NButton></template>删除这个定时任务？项目、文档和历史记录会保留。</NPopconfirm></div>
        </article>
      </div>
    </div>
    <footer class="schedules-footer">软件运行期间生效；每轮结束后计算下一次时间。多个任务依次执行，休眠或退出期间的错过轮次不补跑。暂停不打断当前轮次，可在底部状态栏取消。仅拉取在线文档，不自动执行 AI 或回写远程。</footer>
  </main>
</template>

<style scoped>
.schedules-scroll { flex: 1; min-height: 0; overflow: auto; }
.schedules-content { max-width: 960px; margin: 0 auto; padding: 24px 30px; }
.schedules-heading, .schedule-item-heading, .schedule-item-heading > div, .schedule-item-toggle, .schedule-item-actions, .schedule-enabled, .schedule-editor-actions { display: flex; align-items: center; gap: 10px; }
.schedules-heading, .schedule-item-heading, .schedule-editor-actions { justify-content: space-between; }
h2 { font-size: 16px; margin: 0 0 6px; } h3 { font-size: 14px; margin: 0 0 20px; }
.schedules-heading p, .schedule-help, .schedule-scope, dt, .schedule-item-toggle, .schedule-kind { color: var(--muted); font-size: 12px; line-height: 1.7; }
.schedules-heading p { margin: 0; } .schedules-heading { margin-bottom: 22px; }
.schedule-editor { padding: 22px; background: var(--surface); border: 1px solid var(--line); border-radius: 8px; margin-bottom: 20px; }
.schedule-form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
.schedule-interval { display: grid; grid-template-columns: minmax(80px, 1fr) 92px; gap: 8px; width: 100%; }
.schedule-item { padding: 20px 0; border-bottom: 1px solid var(--line); } .schedule-item-heading strong { font-size: 14px; }
.schedule-kind { border-left: 1px solid var(--line); padding-left: 10px; } .schedule-scope { margin: 8px 0 14px; }
dl { display: flex; gap: 44px; margin: 0; } dl div { display: flex; align-items: baseline; gap: 12px; } dd { font-size: 12px; margin: 0; font-variant-numeric: tabular-nums; }
.schedule-result { color: var(--muted); font-size: 12px; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.8; margin: 12px 0; }
.schedule-error { background: #fff1f0; color: #c7352c; padding: 12px; border-radius: 6px; margin-bottom: 18px; font-size: 12px; }
.schedule-return { display: flex; align-items: center; gap: 12px; } .schedule-return small { color: var(--subtle); font-size: 11px; }
.schedules-empty { min-height: 240px; display: flex; flex-direction: column; justify-content: center; align-items: center; gap: 12px; color: var(--muted); } .schedules-empty span { font-size: 12px; }
.schedules-footer { color: var(--subtle); font-size: 11px; line-height: 1.8; border-top: 1px solid var(--line); padding: 12px 28px; }
</style>
