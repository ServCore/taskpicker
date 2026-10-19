<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { NButton, NDatePicker, NForm, NFormItem, NInput, NPopconfirm, NPopover, NSpin } from 'naive-ui'
import { PhArrowsClockwise, PhCalendarBlank, PhCopy, PhFolderSimple, PhInfo, PhNotebook, PhSparkle } from '@phosphor-icons/vue'
import { api } from '../../shared/api/api'
import { localDay } from '../tasks/tasks'
import type { DailyAISettings, DailyReport, Project, TaskActivity } from '../../shared/types/types'

const props = defineProps<{ project?: Project }>()
const dailyAPI = {
  get: (date: string) => props.project ? api.GetProjectDailyReport(props.project.id, date) : api.GetDailyReport(date),
  save: (date: string, text: string) => props.project ? api.SaveProjectDailyReport(props.project.id, date, text) : api.SaveDailyReport(date, text),
  generate: (date: string, ai: boolean, supplement: boolean) => props.project
    ? (supplement ? api.ContinueProjectDailyReport(props.project.id, date, ai) : api.GenerateProjectDailyReport(props.project.id, date, ai))
    : (supplement ? api.ContinueDailyReport(date, ai) : api.GenerateDailyReport(date, ai)),
  settings: () => props.project ? api.GetProjectDailyReportSettings(props.project.id) : api.GetDailyAISettings(),
  prompt: (value: string) => props.project ? api.SaveProjectDailyReportPrompt(props.project.id, value) : api.SaveDailyReportPrompt(value),
}
const emit = defineEmits<{ lock: [value: boolean] }>()
const date = ref(localDay()), body = ref(''), dirty = ref(false)
const report = ref<DailyReport | null>(null), config = ref<DailyAISettings | null>(null)
const loading = ref(false), saving = ref(false), generating = ref(false)
const error = ref(''), message = ref(''), toast = ref('')
let toastTimer: ReturnType<typeof setTimeout> | undefined
function notify(value: string) {
  clearTimeout(toastTimer); toast.value = value
  toastTimer = setTimeout(() => { toast.value = '' }, 3000)
}
onUnmounted(() => clearTimeout(toastTimer))
const showDatePicker = ref(false)
const calendarTheme = {
  itemSize: '36px', itemCellWidth: '46px', itemCellHeight: '42px', itemFontSize: '14px',
  calendarDaysHeight: '36px', calendarDaysFontSize: '13px', calendarTitleFontSize: '15px',
  calendarTitleHeight: '40px', calendarTitleGridTempateColumns: '36px 36px 1fr 36px 36px',
  calendarLeftPaddingDate: '10px 16px 12px', arrowSize: '18px',
  scrollItemWidth: '100px', scrollItemHeight: '44px', panelActionPadding: '10px 16px',
}
const showPrompt = ref(false), prompt = ref(''), savingPrompt = ref(false)
const promptChanged = computed(() => !!config.value && prompt.value !== config.value.prompt)
const busy = computed(() => loading.value || saving.value || generating.value || savingPrompt.value)
watch(() => busy.value || promptChanged.value, value => emit('lock', value), { immediate: true, flush: 'sync' })
const taskCount = computed(() => new Set(report.value?.activities.map(a => `${a.document_key}:${a.sheet}:${a.record_id || a.row}`)).size)
const hasNewRecords = computed(() => {
  const current = report.value
  if (!current?.draft.text.trim()) return false
  const included = current.draft.activity_ids
  return included?.length ? current.activities.some(activity => !included.includes(activity.id)) : current.activities.length > current.draft.activity_count
})
const canAI = computed(() => !!config.value?.base_url && !!config.value?.model && !promptChanged.value)
const kindLabels: Record<TaskActivity['kind'], string> = { edit: '本地修改', revert: '撤销修改', synced: '远程已同步', sync_failed: '远程未确认', work: '工作进展' }
function setReport(value: DailyReport) { report.value = value; body.value = value.draft.text; dirty.value = false }
function time(value: string) { return new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }) }
function updateBody(value: string) { body.value = value; dirty.value = true; message.value = '' }
async function save(): Promise<boolean> {
  if (!dirty.value) return true
  saving.value = true; error.value = ''
  const currentText = body.value
  try {
    const draft = await dailyAPI.save(date.value, currentText)
    if (report.value) report.value.draft = draft
    if (body.value === currentText) dirty.value = false
    message.value = '日报已保存到本机'; return true
  } catch (exc) { error.value = String(exc); return false }
  finally { saving.value = false }
}
async function load(next = date.value) {
  if (busy.value) return
  if (!(await save())) return
  loading.value = true; error.value = ''; message.value = ''
  try { const value = await dailyAPI.get(next); date.value = next; setReport(value) }
  catch (exc) { error.value = String(exc) }
  finally { loading.value = false }
}
async function changeDate(value: string | null) {
  showDatePicker.value = false
  if (value) await load(value)
}
async function generate(useAI: boolean, supplement = false) {
  if (busy.value || !(await save())) return
  generating.value = true; error.value = ''; message.value = ''
  try {
    const value = await dailyAPI.generate(date.value, useAI, supplement)
    setReport(value)
    if (value.notice) { notify(value.notice); return }
    message.value = supplement ? '新增工作已追加并保存，原正文已保留，请核对后使用' : useAI ? 'AI 日报已生成并保存，请核对后使用' : '本地草稿已生成并保存，可继续编辑'
  }
  catch (exc) { error.value = String(exc) }
  finally { generating.value = false }
}
async function cancel() { try { await api.CancelDailyReport() } catch (exc) { error.value = String(exc) } }
async function copy() { try { await api.CopyText(body.value); message.value = '日报正文已复制' } catch (exc) { error.value = String(exc) } }
async function prepareToLeave() {
  if (busy.value || promptChanged.value) return false
  return await save()
}
defineExpose({ prepareToLeave })
async function savePrompt() {
  if (busy.value || !config.value) return
  savingPrompt.value = true; error.value = ''; message.value = ''
  try {
    const saved = await dailyAPI.prompt(prompt.value)
    config.value.prompt = saved; prompt.value = saved; message.value = '日报提示词已保存'
  } catch (exc) { error.value = String(exc) }
  finally { savingPrompt.value = false }
}
function discardPrompt() { if (config.value) prompt.value = config.value.prompt; showPrompt.value = false }
onMounted(async () => {
  loading.value = true
  const [daily, settings] = await Promise.allSettled([dailyAPI.get(date.value), dailyAPI.settings()])
  if (daily.status === 'fulfilled') setReport(daily.value); else error.value = String(daily.reason)
  if (settings.status === 'fulfilled') { config.value = settings.value; prompt.value = settings.value.prompt } else error.value = String(settings.reason)
  loading.value = false
})
</script>

<template>
    <main class="workspace daily-panel" aria-label="工作日报">
      <Transition name="toast"><div v-if="toast" class="daily-toast" role="status" aria-live="polite"><PhInfo :size="17" />{{ toast }}</div></Transition>
      <header class="workspace-header daily-header"><div class="document-heading"><div class="breadcrumb" aria-label="日报路径"><PhFolderSimple :size="14" /><span class="breadcrumb-root">{{ project?.name || '项目' }}</span><span class="breadcrumb-separator" aria-hidden="true">/</span><h1>工作日报</h1></div><p class="document-subline">按日期整理项目进展与任务记录</p></div></header>
      <div class="daily-toolbar"><div class="daily-date"><label for="daily-date">日期</label><NPopover trigger="click" :show="showDatePicker" :to="false" placement="bottom-start" :show-arrow="false" raw @update:show="showDatePicker = !busy && $event"><template #trigger><button id="daily-date" class="daily-date-trigger" aria-label="选择日报日期" aria-haspopup="dialog" :aria-expanded="showDatePicker" :disabled="busy"><PhCalendarBlank :size="17" />{{ date }}</button></template><div class="daily-calendar" role="dialog" aria-label="选择日报日期"><NDatePicker v-if="showDatePicker" panel type="date" :formatted-value="date" value-format="yyyy-MM-dd" :theme-overrides="calendarTheme" :actions="['now']" @update:formatted-value="changeDate" /></div></NPopover><NButton size="small" quaternary :disabled="busy" aria-label="刷新操作记录" @click="load()"><template #icon><PhArrowsClockwise :size="16" /></template></NButton></div><div class="daily-toolbar-actions"><NButton size="small" :type="showPrompt ? 'primary' : 'default'" :disabled="busy || !config" @click="showPrompt = !showPrompt">日报提示词</NButton></div></div>
      <div v-if="error" class="daily-error" role="alert">{{ error }}</div>
      <div v-if="message" class="daily-message" role="status">{{ message }}</div>
      <NSpin :show="loading" class="daily-main">
        <div v-if="showPrompt" class="daily-prompt-settings">
          <h3>日报提示词</h3><p class="daily-hint">仅用于本项目的 AI 日报，适用于本项目各日期的日报。模型连接请在独立的「AI 设置」页面配置。</p>
          <NForm label-placement="top" @submit.prevent="savePrompt">
            <NFormItem label="自定义日报提示词"><NInput v-model:value="prompt" type="textarea" aria-label="自定义日报提示词" :autosize="{ minRows: 8, maxRows: 12 }" :maxlength="20000" :disabled="busy" /></NFormItem>
            <div class="daily-prompt-actions"><NButton :disabled="busy || !config" @click="prompt = config!.default_prompt">恢复默认提示词</NButton><div><div class="daily-return-action"><NButton :disabled="busy" aria-describedby="daily-prompt-return-hint" @click="discardPrompt">返回</NButton><small id="daily-prompt-return-hint">直接返回不会保存</small></div><NButton type="primary" attr-type="submit" :loading="savingPrompt" :disabled="busy && !savingPrompt">保存日报提示词</NButton></div></div>
            <p v-if="promptChanged" class="daily-hint">有未保存的提示词修改，请保存或返回后切换页面。</p>
          </NForm>
        </div>
        <div v-else class="daily-content">
          <aside class="daily-activity"><div class="daily-section-heading"><h3>当天操作</h3><span>{{ report?.activities.length || 0 }} 次 · {{ taskCount }} 项任务</span></div><p class="daily-hint">自动记录状态、备注修改与回写结果，补充的工作进展也在这里。仅浏览任务不会计为工作。</p>
            <div v-if="!report?.activities.length" class="daily-empty"><PhNotebook :size="30" /><strong>这一天还没有记录</strong><p>修改任务后会自动记录；也可以在任务详情填写进展，点击「记入日报」。历史操作不会补录。</p></div>
            <ol v-else class="daily-timeline"><li v-for="activity in report.activities" :key="activity.id"><div class="activity-meta"><time>{{ time(activity.at) }}</time><span :class="activity.kind">{{ kindLabels[activity.kind] }}</span></div><small>{{ activity.document_title }} · {{ activity.sheet }} · #{{ activity.row }}</small><strong>{{ activity.context || activity.task }}</strong><p v-if="activity.context && activity.context !== activity.task" class="activity-task">{{ activity.task }}</p><p v-if="activity.note">{{ activity.note }}</p><div v-for="(change, index) in activity.changes" :key="index" class="activity-change"><span>{{ change.field }}</span><p>{{ change.before || '（空白）' }} → {{ change.after || '（空白）' }}</p></div></li></ol>
          </aside>
          <section class="daily-editor"><div class="daily-section-heading"><h3>日报正文</h3><span>{{ dirty ? '有未保存修改' : report?.draft.saved_at ? '已保存到本机' : '尚未生成' }}</span></div><p class="daily-hint">从当天记录整理，可修改后复制。切换日期或页面时自动保存正文修改。</p><p v-if="hasNewRecords" class="daily-new-records">有新增操作记录，可以续写到现有日报。</p>
            <NInput :value="body" type="textarea" class="daily-body" aria-label="日报正文" placeholder="先生成本地草稿，或配置 AI 后一键整理成日报。也可以直接填写。" :disabled="busy || !report" :maxlength="100000" @update:value="updateBody" />
            <div class="daily-generate-actions">
              <NButton v-for="choice in [{ ai: false, label: body.trim() ? '补充本地草稿' : '生成本地草稿' }, { ai: true, label: body.trim() ? 'AI 续写日报' : 'AI 生成日报' }]" :key="String(choice.ai)" :type="choice.ai ? 'primary' : 'default'" :loading="generating" :disabled="busy || !report || (!body.trim() && !report.activities.length) || (choice.ai && !canAI)" @click="generate(choice.ai, !!body.trim())"><template v-if="choice.ai" #icon><PhSparkle :size="16" /></template>{{ choice.label }}</NButton>
              <NButton v-if="generating" @click="cancel">取消生成</NButton>
            </div>
            <p v-if="body.trim()" class="daily-hint">续写会读取最新记录，只追加新增工作，保留现有正文。</p>
            <div v-if="body.trim()" class="daily-regenerate-actions"><span>重新生成：</span><NPopconfirm v-for="choice in [{ ai: false, label: '本地草稿' }, { ai: true, label: 'AI 日报' }]" :key="String(choice.ai)" positive-text="重新生成" negative-text="保留当前正文" :disabled="busy || !report?.activities.length || (choice.ai && !canAI)" @positive-click="generate(choice.ai)"><template #trigger><NButton size="tiny" quaternary :disabled="busy || !report?.activities.length || (choice.ai && !canAI)">{{ choice.label }}</NButton></template>重新生成将替换当前日报正文，是否继续？</NPopconfirm></div>
            <p class="daily-hint">{{ promptChanged ? '日报提示词有未保存修改，请先保存或放弃。' : canAI ? `AI：${config?.model} · 生成内容请核对后使用` : '尚未配置 AI，可先生成本地草稿；自定义提示词仅用于 AI 生成。' }}</p>
          </section>
        </div>
      </NSpin>
      <footer class="daily-footer"><span>操作记录与日报保存在本机，仅汇总本项目的文档</span><div><NButton :disabled="busy || !dirty" @click="save">保存修改</NButton><NButton :disabled="busy || !body.trim()" @click="copy"><template #icon><PhCopy :size="15" /></template>复制日报</NButton></div></footer>
    </main>
</template>

<style scoped>
.daily-panel { color: var(--ink); overflow: hidden; }
.daily-header, .daily-toolbar, .daily-footer, .daily-error, .daily-message { flex-shrink: 0; }
.daily-main { flex: 1; min-height: 0; overflow: hidden; }
.daily-main :deep(.n-spin-content) { height: 100%; }
.daily-toolbar { display: flex; justify-content: space-between; align-items: center; padding: 14px 24px; gap: 16px; }
.daily-date { display: flex; gap: 10px; align-items: center; font-size: 12px; color: var(--muted); }
.daily-date-trigger { display: flex; align-items: center; gap: 10px; min-height: 36px; padding: 6px 12px; border: 1px solid #dee0e3; border-radius: 6px; color: var(--ink); background: #fff; font-size: 13px; }
.daily-date-trigger:hover:not(:disabled) { border-color: var(--primary); background: #f7f9ff; }
.daily-date-trigger svg { color: var(--muted); }
.daily-content { display: grid; grid-template-columns: minmax(240px, 32%) minmax(0,1fr); height: 100%; min-height: 0; border-block: 1px solid var(--line); }
.daily-activity { padding: 20px 22px; overflow: auto; border-right: 1px solid var(--line); background: #f8f9fb; }
.daily-section-heading { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 8px; }
.daily-section-heading h3 { margin: 0; }
.daily-section-heading > span { font-size: 11px; color: var(--muted); }
.daily-hint { font-size: 11px; color: var(--subtle); line-height: 1.8; margin: 0 0 14px; }
.daily-editor { padding: 20px 24px; display: flex; flex-direction: column; min-height: 0; overflow: auto; }
.daily-body { flex: 1; min-height: 140px; }
.daily-body :deep(.n-input-wrapper), .daily-body :deep(.n-input__textarea) { height: 100%; }
.daily-body :deep(textarea) { height: 100% !important; line-height: 1.9; font-size: 13px; }
.daily-generate-actions { display: flex; gap: 9px; margin-top: 14px; margin-bottom: 9px; flex-wrap: wrap; }
.daily-editor > .daily-hint:last-child { margin-bottom: 0; }
.daily-timeline { padding: 0; margin: 20px 0 0; list-style: none; }
.daily-timeline li { padding: 0 0 18px 16px; margin-left: 3px; border-left: 1px solid #d8dfe9; position: relative; overflow-wrap: anywhere; }
.daily-timeline li::before { content: ''; width: 7px; height: 7px; background: #8eafff; border-radius: 50%; position: absolute; left: -4px; top: 5px; }
.activity-meta { display: flex; gap: 8px; align-items: center; font-size: 11px; margin-bottom: 7px; color: var(--subtle); }
.activity-meta > span { color: var(--primary-pressed); background: var(--primary-light); padding: 1px 5px; border-radius: 3px; }
.activity-meta .synced { color: var(--success); background: #e9f5ed; }
.activity-meta .sync_failed, .activity-meta .revert { color: #a86600; background: #fff3db; }
.daily-timeline small { display: block; color: var(--subtle); font-size: 10px; line-height: 1.7; margin-bottom: 5px; }
.daily-timeline strong { display: block; font-size: 12px; font-weight: 500; line-height: 1.7; }
.daily-timeline p { margin: 6px 0 0; white-space: pre-wrap; font-size: 12px; line-height: 1.8; color: var(--text); }
.daily-timeline .activity-task { font-size: 11px; color: var(--muted); }
.activity-change { padding-top: 8px; }
.activity-change > span { font-size: 10px; color: var(--subtle); }
.activity-change p { margin-top: 2px; }
.daily-empty { display: flex; flex-direction: column; align-items: center; text-align: center; padding: 50px 8px; color: var(--subtle); gap: 12px; }
.daily-empty strong { font-size: 13px; color: var(--muted); }
.daily-empty p { font-size: 12px; line-height: 1.9; margin: 0; }
.daily-footer { display: flex; justify-content: space-between; align-items: center; padding: 16px 24px; gap: 16px; }
.daily-footer > span { font-size: 11px; color: var(--subtle); }
.daily-footer > div { display: flex; gap: 9px; }
.daily-error, .daily-message { margin: 0 24px 12px; font-size: 12px; line-height: 1.8; padding: 8px 12px; border-radius: 5px; overflow-wrap: anywhere; }
.daily-error { color: var(--danger); background: #fff2f0; }
.daily-message { color: var(--success); background: #edf8f0; }
.daily-new-records { color: #a86600; background: #fff8e8; padding: 6px 10px; font-size: 11px; margin: 0 0 10px; }
.daily-toolbar-actions, .daily-prompt-actions, .daily-prompt-actions > div { display: flex; gap: 9px; align-items: center; }
.daily-prompt-settings { padding: 20px 28px; height: 100%; min-height: 0; overflow: auto; border-block: 1px solid var(--line); }
.daily-prompt-settings h3 { margin: 0 0 10px; }
.daily-prompt-actions { align-items: flex-start; justify-content: space-between; margin-bottom: 16px; }
.daily-prompt-actions > div { align-items: flex-start; }
.daily-return-action { display: flex; flex-direction: column; align-items: center; gap: 5px; }
.daily-return-action small { color: var(--subtle); font-size: 11px; line-height: 1.6; white-space: nowrap; }
.daily-regenerate-actions { display: flex; align-items: center; gap: 5px; margin-bottom: 9px; color: var(--subtle); font-size: 11px; }
.daily-toast { position: fixed; right: 24px; bottom: 56px; z-index: 3000; display: flex; align-items: center; gap: 9px; max-width: calc(100vw - 48px); padding: 12px 18px; border: 1px solid var(--line); border-radius: 8px; background: #fff; color: var(--ink); box-shadow: 0 6px 24px #1f35521a; font-size: 13px; pointer-events: none; }
.daily-toast svg { color: var(--primary); flex-shrink: 0; }
</style>
