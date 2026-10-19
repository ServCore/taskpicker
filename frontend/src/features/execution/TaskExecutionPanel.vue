<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { NButton, NInput, NSelect, NTabPane, NTabs } from 'naive-ui'
import { PhArrowSquareOut, PhArrowsClockwise, PhPlay, PhSparkle, PhStop } from '@phosphor-icons/vue'
import { api } from '../../shared/api/api'
import MarkdownDocument from '../../shared/components/MarkdownDocument.vue'
import type { TaskExecution, TaskExecutionContext, TaskExecutionProgress } from '../../shared/types/types'

const props = defineProps<{ documentKey: string; sheet: string; row: number; disabled: boolean; launch?: typeof api.StartTaskExecution; continueLaunch?: typeof api.ContinueTaskExecution }>()
const emit = defineEmits<{ started: [run: TaskExecution] }>()
const context = ref<TaskExecutionContext | null>(null), runs = ref<TaskExecution[]>([])
const selectedID = ref(''), repository = ref(''), instructions = ref(''), report = ref(''), reportPath = ref('')
const loading = ref(true), starting = ref(false), stopping = ref(false), refreshing = ref(false), error = ref('')
const reportLoading = ref(false), copied = ref(false), progress = ref<TaskExecutionProgress | null>(null), progressCopied = ref(false), launchingMode = ref<'new' | 'continue'>('new')
const run = computed(() => runs.value.find(item => item.id === selectedID.value))
const labels: Record<TaskExecution['status'], string> = { preparing: '准备工作区', running: '正在执行', stopping: '正在停止', succeeded: '执行结束 · 待核对', failed: '执行失败', stopped: '已停止', interrupted: '执行中断' }
const active = computed(() => !!run.value && isActive(run.value))
const runningRun = computed(() => runs.value.find(isActive))
const anyActive = computed(() => !!runningRun.value)
const newerWorkspaceRun = computed(() => run.value ? runs.value.find(item => item.workspace === run.value!.workspace && item.started_at > run.value!.started_at) : undefined)
const canContinue = computed(() => !!run.value?.workspace && !active.value && !newerWorkspaceRun.value && !!context.value?.repositories.some(repo => repo.path === run.value?.repository))
const repositories = computed(() => context.value?.repositories.map(item => ({ label: `${item.path}${item.codegraph ? ' · CodeGraph' : ''}`, value: item.path })) ?? [])
let disposed = false, timer: ReturnType<typeof setTimeout> | undefined, selectionVersion = 0, requestVersion = 0
function isActive(item: TaskExecution) { return ['preparing', 'running', 'stopping'].includes(item.status) }
function dateTime(value: string) { return new Date(value).toLocaleString('zh-CN', { hour12: false }) }
function schedule() { clearTimeout(timer); if (!disposed && anyActive.value) timer = setTimeout(poll, 1500) }
function replaceRun(value: TaskExecution) { runs.value = [value, ...runs.value.filter(item => item.id !== value.id)].sort((a,b) => b.started_at.localeCompare(a.started_at)) }
async function poll() {
  if (disposed) return
  clearTimeout(timer)
  const id = selectedID.value, version = selectionVersion, request = ++requestVersion
  refreshing.value = true
  try {
    // Read history first, then its document so a finishing run cannot leave a stale final report.
    const values = await api.GetTaskExecutions(props.documentKey, props.sheet, props.row)
    if (disposed || version !== selectionVersion || request !== requestVersion) return
    const [document, currentProgress] = await Promise.all([
      id ? api.GetTaskExecutionReport(id) : Promise.resolve(null),
      api.GetTaskExecutionProgress(props.documentKey, props.sheet, props.row),
    ])
    if (disposed || version !== selectionVersion || request !== requestVersion) return
    runs.value = values; progress.value = currentProgress
    if (document) { report.value = document.markdown; reportPath.value = document.path }
  } catch (exc) { if (!disposed && version === selectionVersion && request === requestVersion) error.value = String(exc) }
  finally {
    if (!disposed && version === selectionVersion && request === requestVersion) { refreshing.value = false; reportLoading.value = false; schedule() }
  }
}
async function selectRun(id: string) {
  selectionVersion++; selectedID.value = id; report.value = ''; reportPath.value = ''; error.value = ''; copied.value = false; reportLoading.value = true
  await poll()
}
async function load() {
  loading.value = true; error.value = ''
  const [settings, history] = await Promise.allSettled([api.GetTaskExecutionContext(props.documentKey, props.sheet, props.row), api.GetTaskExecutions(props.documentKey, props.sheet, props.row)])
  if (disposed) return
  if (settings.status === 'fulfilled') { context.value = settings.value; repository.value = settings.value.repositories[0]?.path ?? '' }
  else error.value = String(settings.reason)
  if (history.status === 'fulfilled') runs.value = history.value
  else error.value = String(history.reason)
  loading.value = false
  if (runs.value[0]) await selectRun(runs.value[0].id)
  else schedule()
}
async function start(mode: 'new' | 'continue' = 'new') {
  if (starting.value || anyActive.value || props.disabled || !context.value?.available) return
  if (mode === 'continue' ? !canContinue.value : !repository.value) return
  starting.value = true; launchingMode.value = mode; error.value = ''
  try {
    const value = mode === 'continue'
      ? await (props.continueLaunch ?? api.ContinueTaskExecution)(props.documentKey, props.sheet, props.row, run.value!.id, instructions.value)
      : await (props.launch ?? api.StartTaskExecution)(props.documentKey, props.sheet, props.row, repository.value, instructions.value)
    if (disposed) return
    emit('started', value); instructions.value = ''; progressCopied.value = false
    replaceRun(value); await selectRun(value.id)
  } catch (exc) { if (!disposed) error.value = String(exc) }
  finally { starting.value = false }
}
async function stop() {
  if (!runningRun.value || stopping.value) return
  stopping.value = true; error.value = ''
  try { await api.StopTaskExecution(runningRun.value.id); await poll() }
  catch (exc) { if (!disposed) error.value = String(exc) }
  finally { stopping.value = false }
}
async function openWorkspace() {
  if (!run.value) return
  try { await api.OpenTaskExecutionWorkspace(run.value.id) }
  catch (exc) { error.value = String(exc) }
}
async function openReport() {
  if (!run.value || reportLoading.value) return
  try { await api.OpenTaskExecutionReport(run.value.id) }
  catch (exc) { error.value = String(exc) }
}
async function copyReport() {
  if (!report.value) return
  try { await api.CopyText(report.value); copied.value = true }
  catch (exc) { error.value = String(exc) }
}
async function openProgress() {
  try { await api.OpenTaskExecutionProgress(props.documentKey, props.sheet, props.row) }
  catch (exc) { error.value = String(exc) }
}
async function copyProgress() {
  if (!progress.value?.markdown) return
  try { await api.CopyText(progress.value.markdown); progressCopied.value = true }
  catch (exc) { error.value = String(exc) }
}
onMounted(load)
onUnmounted(() => { disposed = true; selectionVersion++; clearTimeout(timer) })
</script>

<template>
  <section class="task-execution" aria-label="AI 执行任务">
    <div class="execution-heading"><h3><PhSparkle :size="17" />AI 执行任务</h3><span v-if="run" class="execution-status" :class="run.status" role="status">{{ labels[run.status] }}</span></div>
    <p class="execution-hint">手动启动单个任务，使用本机 Codex 的登录与模型配置。修改保存在独立 Git 工作区，执行后可检查结果。</p>
    <p v-if="context?.notice" class="execution-notice">{{ context.notice }}</p>
    <div class="execution-form">
      <label>{{ runs.length ? '重新执行时使用的仓库' : '执行仓库' }}<NSelect v-model:value="repository" :options="repositories" placeholder="选择项目下的代码仓库" aria-label="AI 执行仓库" :disabled="loading || starting || anyActive" /></label>
      <NInput v-model:value="instructions" type="textarea" aria-label="AI 任务补充要求" placeholder="本次要求（可选），例如：接着上次的修改，补充异常场景测试" :autosize="{ minRows: 2, maxRows: 5 }" :maxlength="4000" :disabled="starting || anyActive" />
      <div class="execution-actions">
        <span>{{ anyActive ? '执行期间可切换查看历史记录。' : runs.length ? '重新执行会创建新工作区；继续处理会保留所选工作区的修改。' : '基于仓库当前提交执行，请先整理未提交修改。' }}</span>
        <NButton v-if="anyActive" size="small" :loading="stopping" :disabled="stopping" @click="stop"><template #icon><PhStop :size="15" /></template>停止执行</NButton>
        <template v-else>
          <NButton size="small" :type="runs.length ? 'default' : 'primary'" :loading="starting && launchingMode === 'new'" :disabled="loading || starting || disabled || !context?.available || !repository" @click="start('new')"><template #icon><PhPlay :size="15" weight="fill" /></template>{{ runs.length ? '重新执行' : '启动执行' }}</NButton>
          <NButton v-if="runs.length" size="small" type="primary" :loading="starting && launchingMode === 'continue'" :disabled="loading || starting || disabled || !context?.available || !canContinue" @click="start('continue')">继续处理</NButton>
        </template>
      </div>
      <p v-if="run && !anyActive" class="execution-resume-hint">{{ newerWorkspaceRun ? '所选工作区已有后续执行，请选择该工作区的最新记录继续。' : canContinue ? `继续当前选中的执行（${dateTime(run.started_at)}），自动带入进展摘要、历史日志入口和本次要求。仓库：${run.repository}` : '当前记录无法继续，请检查项目绑定目录和执行工作区。' }}</p>
    </div>
    <p v-if="error" class="execution-error" role="alert">{{ error }} <NButton v-if="!context && !loading" size="tiny" quaternary @click="load">重试</NButton></p>
    <template v-if="runs.length">
      <div class="execution-history-heading"><span>执行记录 · {{ runs.length }} 次</span><NButton size="small" quaternary aria-label="刷新执行记录" :loading="refreshing" @click="poll"><template #icon><PhArrowsClockwise :size="16" /></template></NButton></div>
      <div class="execution-history" role="group" aria-label="任务执行历史">
        <button v-for="(item, index) in runs" :key="item.id" type="button" :class="{ selected: selectedID === item.id }" :aria-pressed="selectedID === item.id" @click="selectRun(item.id)">
          <span class="execution-history-title">第 {{ runs.length - index }} 次{{ item.previous_id ? ' · 继续处理' : ' · 独立执行' }} <span>{{ labels[item.status] }}</span></span>
          <span class="execution-history-date">{{ dateTime(item.started_at) }} · {{ item.id.slice(0, 8) }} <span>查看记录 →</span></span>
        </button>
      </div>
      <template v-if="run">
        <p v-if="run.previous_id" class="execution-resume-hint">已承接上一轮摘要，沿用工作区 {{ run.root_id?.slice(0, 8) }}；本轮日志独立保存。</p>
        <p class="execution-time">开始 {{ dateTime(run.started_at) }}<template v-if="run.finished_at"> · 结束 {{ dateTime(run.finished_at) }}</template></p>
        <NTabs type="line" size="small" animated>
          <NTabPane name="report" tab="执行文档">
            <div class="execution-report-actions"><span>每次执行独立保存为 Markdown{{ active ? ' · 持续更新中' : '' }}</span><NButton size="tiny" :disabled="reportLoading || !report" @click="copyReport">{{ copied ? '已复制' : '复制 Markdown' }}</NButton><NButton size="tiny" :disabled="reportLoading || !report" @click="openReport"><template #icon><PhArrowSquareOut :size="14" /></template>打开 MD 文件</NButton></div>
            <p v-if="reportLoading" class="execution-hint" role="status">正在读取执行文档…</p>
            <MarkdownDocument v-else-if="report" :source="report" class="execution-report" />
            <p v-else class="execution-hint">执行文档暂未读取，请刷新重试。</p>
            <p v-if="reportPath" class="execution-report-path">{{ reportPath }}</p>
          </NTabPane>
          <NTabPane name="progress" tab="任务进展">
            <p class="execution-hint">跟随最近一次执行更新。继续处理会读取所选记录所在链路的摘要，重新执行会建立新的处理链路。</p>
            <div v-if="progress?.markdown" class="execution-report-actions"><span>任务累计进展与待办</span><NButton size="tiny" @click="copyProgress">{{ progressCopied ? '已复制' : '复制进展摘要' }}</NButton><NButton size="tiny" @click="openProgress">打开进展 MD</NButton></div>
            <MarkdownDocument v-if="progress?.markdown" :source="progress.markdown" class="execution-report" />
            <p v-else class="execution-hint">暂无任务进展摘要，请刷新重试。</p>
            <p v-if="progress?.path" class="execution-report-path">{{ progress.path }}</p>
          </NTabPane>
          <NTabPane name="changes" tab="修改文件"><pre class="execution-result">{{ run.changed_files || (active ? '执行结束后汇总修改文件。' : '没有记录到文件修改。') }}</pre><p class="execution-hint">文件清单为当轮结束时的工作区累计修改。继续处理会更新同一工作区，历史文档保留当轮快照。<br>工作区：{{ run.workspace }}<br>分支：{{ run.branch }}</p><NButton size="small" @click="openWorkspace"><template #icon><PhArrowSquareOut :size="15" /></template>打开执行工作区</NButton></NTabPane>
        </NTabs>
      </template>
    </template>
    <p v-else-if="!loading" class="execution-empty">这条任务还没有执行记录</p>
  </section>
</template>

<style scoped>
.task-execution { margin-top: 24px; border-top: 1px solid var(--line); padding-top: 24px; }
.execution-heading, .execution-actions, .execution-history-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.execution-heading h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 12px; }
.execution-hint, .execution-notice, .execution-time, .execution-actions > span, .execution-empty { color: var(--muted); font-size: 11px; line-height: 1.8; margin: 0 0 12px; overflow-wrap: anywhere; }
.execution-notice { color: #a86600; background: #fff8e8; padding: 8px 10px; border-radius: 5px; }
.execution-form { display: flex; flex-direction: column; gap: 12px; }
.execution-form label { display: flex; flex-direction: column; gap: 6px; font-size: 12px; color: var(--muted); }
.execution-actions { flex-wrap: wrap; }
.execution-actions > span { margin: 0; flex: 1; min-width: 140px; }
.execution-resume-hint { font-size: 11px; color: var(--muted); line-height: 1.7; overflow-wrap: anywhere; margin: 0; }
.execution-actions > .n-button { flex-shrink: 0; }
.execution-status { font-size: 11px; color: var(--muted); background: #f0f1f3; padding: 3px 8px; border-radius: 4px; white-space: nowrap; margin-bottom: 12px; }
.execution-status.running, .execution-status.preparing, .execution-status.stopping { color: var(--primary); background: var(--primary-light); }
.execution-status.succeeded { color: var(--success); background: #edf8f0; }
.execution-status.failed, .execution-status.interrupted { color: var(--danger); background: #fff2f0; }
.execution-error { color: var(--danger); font-size: 12px; line-height: 1.8; overflow-wrap: anywhere; margin: 12px 0; }
.execution-history-heading { margin: 20px 0 8px; font-size: 12px; color: var(--muted); }
.execution-history { display: flex; flex-direction: column; gap: 6px; max-height: 190px; overflow: auto; margin-bottom: 14px; }
.execution-history button { border: 1px solid var(--line); background: var(--surface, #fff); color: inherit; border-radius: 6px; padding: 10px 12px; cursor: pointer; text-align: left; font: inherit; }
.execution-history button:hover { border-color: #a8c4ff; }
.execution-history button.selected { background: var(--primary-light); border-color: var(--primary); }
.execution-history button:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
.execution-history-title, .execution-history-date { display: flex; justify-content: space-between; gap: 12px; }
.execution-history-title { font-size: 12px; font-weight: 500; }
.execution-history-title > span { font-size: 11px; color: var(--muted); font-weight: 400; }
.execution-history-date { color: var(--muted); font-size: 10px; margin-top: 5px; }
.execution-history-date > span { color: var(--primary); white-space: nowrap; }
.execution-time { margin-bottom: 0; }
.execution-report-actions { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.execution-report-actions > span { flex: 1; font-size: 10px; color: var(--muted); }
.execution-report { max-height: 460px; overflow: auto; border: 1px solid var(--line); border-radius: 6px; padding: 18px; background: var(--surface, #fff); }
.execution-report-path { font-size: 10px; line-height: 1.6; color: var(--subtle); overflow-wrap: anywhere; }
.execution-result { font-size: 12px; line-height: 1.8; white-space: pre-wrap; overflow-wrap: anywhere; font-family: inherit; }
.execution-result { max-height: 420px; overflow: auto; margin: 0 0 12px; }
.execution-empty { margin-top: 16px; margin-bottom: 0; }
</style>
