<script setup lang="ts">
import { cloneVNode, computed, defineAsyncComponent, h, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  NAlert, NButton, NConfigProvider, NDrawer, NDropdown, NForm, NFormItem, NImage, NImageGroup,
  NInput, NModal, NSelect, NSkeleton, NTooltip, dateZhCN, zhCN,
} from 'naive-ui'
import type { GlobalThemeOverrides, ImageGroupRenderToolbar, InputInst, SelectOption } from 'naive-ui'
import {
  PhAlarm, PhArrowSquareOut, PhArrowsClockwise, PhCaretLeft,
  PhCaretRight, PhCheck, PhCheckCircle, PhCopy, PhFileXls, PhFolderSimple, PhGear,
  PhImage, PhMagnifyingGlass, PhNotebook, PhPlus, PhSlidersHorizontal, PhSquaresFour,
  PhSparkle, PhStack, PhWifiSlash, PhX,
} from '@phosphor-icons/vue'
import { api, onProgress } from '../shared/api/api'
import ResourceMonitor from '../features/status-bar/ResourceMonitor.vue'
import WorkdayCountdown from '../features/status-bar/WorkdayCountdown.vue'
import TaskWorkEntry from '../features/reports/TaskWorkEntry.vue'
import PriorityBadges from '../features/tasks/PriorityBadges.vue'
import DocumentPullBar from '../features/documents/DocumentPullBar.vue'
import DocumentPullSettings from '../features/documents/DocumentPullSettings.vue'
import { editableConditions, needsValue } from '../features/documents/pullConditions'
import { copyTask, documentStats, formatDate, imageURL, isCompleted, isDueToday, localDay, priorityColumns, taskStatusChoices, tasksFor, text, tone } from '../features/tasks/tasks'
import { createScheduleRunner } from '../features/schedules/runner'
import type { SyncSchedule, Document, DocumentPullConditions, Project, RemoteAuthState, Task, TaskExecution } from '../shared/types/types'

const DailyReportPanel = defineAsyncComponent(() => import('../features/reports/DailyReportPanel.vue'))
const TaskExecutionPanel = defineAsyncComponent(() => import('../features/execution/TaskExecutionPanel.vue'))
const ProjectSettingsPanel = defineAsyncComponent(() => import('../features/projects/ProjectSettingsPanel.vue'))
const SettingsDialog = defineAsyncComponent(() => import('../features/settings/SettingsDialog.vue'))
const projects = ref<Project[]>([])
const collapsedProjects = ref<string[]>(loadCollapsedProjects())
const showProjectSettings = ref(false)
const editingProject = ref<Project | null>(null)
const draftProjectID = ref('')
const reportProject = ref<Project | null>(null)
const currentPage = ref<'tasks' | 'report'>('tasks')
const showAppSettings = ref(false)
const settingsEditingLocked = ref(false)
const reportNavigationLocked = ref(false), navigating = ref(false)
const pageNavigationLocked = computed(() => reportNavigationLocked.value || navigating.value)
const navigationLocked = computed(() => showAppSettings.value || showSettings.value || pageNavigationLocked.value)
const reportPage = ref<{ prepareToLeave: () => Promise<boolean> } | null>(null)
const executionRun = ref<TaskExecution | null>(null)
let executionTimer: ReturnType<typeof setTimeout> | undefined
const executionLabel = computed(() => executionRun.value ? ({ preparing: '准备中', running: '执行中', stopping: '停止中', succeeded: '结束 · 待核对', failed: '失败', stopped: '已停止', interrupted: '已中断' } as const)[executionRun.value.status] : '')
async function launchTaskExecution(key: string, sheet: string, row: number, repository: string, instructions: string) {
  const run = await api.StartTaskExecution(key, sheet, row, repository, instructions)
  if (!disposed) trackExecution(run)
  return run
}
async function continueTaskExecution(key: string, sheet: string, row: number, previousID: string, instructions: string) {
  const run = await api.ContinueTaskExecution(key, sheet, row, previousID, instructions)
  if (!disposed) trackExecution(run)
  return run
}
function trackExecution(run: TaskExecution) {
  executionRun.value = run; clearTimeout(executionTimer)
  if (['preparing', 'running', 'stopping'].includes(run.status)) executionTimer = setTimeout(async () => {
    try {
      const active = await api.GetActiveTaskExecution()
      const value = active?.id === run.id ? active : (await api.GetTaskExecution(run.id, run.log_count)).run
      if (!disposed) trackExecution(value)
    }
    catch { if (!disposed) trackExecution(run) }
  }, 2000)
}
async function openExecution() {
  const run = executionRun.value
  if (!run || navigationLocked.value) return
  await chooseDocument(run.document_key)
  if (currentPage.value !== 'tasks') return
  sheetName.value = run.sheet; await nextTick()
  const task = filtered.value.find(item => run.record_id ? item.row.record_id === run.record_id : item.row.number === run.row)
  if (task) chooseTask(task)
  else notify('执行记录保存在对应任务下，请调整筛选条件后查看。')
}
async function canLeavePage() {
  if (navigationLocked.value) return false
  if (currentPage.value !== 'report') return true
  navigating.value = true
  try { return await reportPage.value?.prepareToLeave() ?? false }
  finally { navigating.value = false }
}
async function openSettings() {
  if (showAppSettings.value || !(await canLeavePage())) return
  showDetails.value = false
  showAppSettings.value = true
}
function leaveSettings() {
  showAppSettings.value = false
  settingsEditingLocked.value = false
}
async function openDailyReport(project: Project) {
  if (project.daily_report_enabled === false || (currentPage.value === 'report' && reportProject.value?.id === project.id) || !(await canLeavePage())) return
  showDetails.value = false
  reportNavigationLocked.value = true
  reportProject.value = project
  currentPage.value = 'report'
}

const theme: GlobalThemeOverrides = {
  common: {
    primaryColor: '#3370ff', primaryColorHover: '#4e83fd', primaryColorPressed: '#245bdb',
    primaryColorSuppl: '#3370ff', borderRadius: '6px', fontSize: '13px',
    fontFamily: '-apple-system, BlinkMacSystemFont, "Helvetica Neue", "PingFang SC", "Microsoft YaHei", sans-serif',
    textColorBase: '#1f2329', textColor1: '#1f2329', textColor2: '#4e5969', textColor3: '#646a73',
    placeholderColor: '#8f959e', borderColor: '#dee0e3', hoverColor: '#f5f6f7', dividerColor: '#e8e9eb',
    successColor: '#25834b', warningColor: '#a86600', errorColor: '#d83931', infoColor: '#3370ff',
    modalColor: '#ffffff', bodyColor: '#f5f6f7',
  },
  Button: { fontWeight: '500', heightMedium: '34px', boxShadowFocus: '0 0 0 3px rgba(51,112,255,.15)' },
  Input: {
    color: '#f5f6f7', colorFocus: '#ffffff', heightMedium: '36px',
    border: '1px solid #f5f6f7', borderHover: '1px solid #b4ccff', boxShadowFocus: '0 0 0 2px rgba(51,112,255,.12)',
  },
  Select: { peers: { InternalSelection: { heightMedium: '32px', border: '1px solid #dee0e3', color: '#ffffff' } } },
  Tooltip: { color: '#353b45', textColor: '#ffffff', borderRadius: '6px' },
}

const documents = ref<Document[]>([])
const selectedKey = ref('')
const loading = ref(true)
const loadError = ref('')
const libraryWarnings = ref<string[]>([])
const sheetName = ref('')
const search = ref('')
const searchExpanded = ref(false)
const searchToggle = ref<HTMLButtonElement | null>(null)
const today = ref(localDay())
const defaultTaskStatuses = localGet('include-completed') === 'true' ? [] : ['unfinished']
const selectedStatuses = ref<string[]>([...defaultTaskStatuses])
const selectedTaskID = ref('')
const busy = ref(false)
const cancelling = ref(false)
const progress = ref('')
const syncingTitle = ref('')
const queuePosition = ref(0)
const queueTotal = ref(0)
const syncSummary = ref('')
const syncIssues = ref<string[]>([])
let queueStopped = false
let disposed = false
const syncStatusText = computed(() => busy.value
  ? `${queueTotal.value ? `正在同步 ${queuePosition.value}/${queueTotal.value}` : '正在同步'}「${syncingTitle.value}」 · ${progress.value}`
  : authorizing.value ? '正在授权腾讯文档…'
  : statusBusy.value ? `正在更新「${active.value?.title || '当前文档'}」的状态与备注…`
  : syncSummary.value)

const error = ref('')
const feedback = ref('')
const showSettings = ref(false)
const showDetails = ref(false)
const imagePreviewOpen = ref(false)
const isNew = ref(false)
const editingDocumentKey = ref('')
const draftPullConditions = ref<DocumentPullConditions>({ owner: '', include_completed: true, filters: [] })
const initialPullConditions = ref('')
const editingDocument = computed(() => documents.value.find(doc => doc.key === editingDocumentKey.value))
const editingHasPendingChanges = computed(() => editingDocument.value?.sheets.some(sheet => sheet.rows.some(row => row.status_edit)) ?? false)
const savingConditions = ref(false)
const draftURL = ref('')
const formError = ref('')
const remoteAuth = ref<RemoteAuthState>({ authorized: false, expires_at: 0 })
const statusBusy = ref(false)
const authorizing = ref(false)
const statusDraft = ref('')
const noteDrafts = ref<Record<number, string>>({})
const statusError = ref('')
const statusMenuOpen = ref(false)
const searchInput = ref<InputInst | null>(null)
const detail = ref<HTMLElement | null>(null)
const taskList = ref<HTMLElement | null>(null)
const active = computed(() => documents.value.find(doc => doc.key === selectedKey.value))
const activeProject = computed(() => projects.value.find(project => project.document_keys.includes(selectedKey.value)))
const projectOptions = computed(() => [{ label: '未分组', value: '' }, ...projects.value.map(project => ({ label: project.name, value: project.id }))])
const documentGroups = computed(() => {
  const assigned = new Set(projects.value.flatMap(project => project.document_keys))
  const groups: { id: string; name: string; project: Project | null; documents: Document[] }[] = projects.value.filter(project => project.enabled !== false).map(project => ({ id: project.id, name: project.name, project, documents: documents.value.filter(doc => project.document_keys.includes(doc.key)) }))
  const unassigned = documents.value.filter(doc => !assigned.has(doc.key))
  if (unassigned.length) groups.push({ id: '', name: '未分组', project: null, documents: unassigned })
  return groups
})
function loadCollapsedProjects(): string[] {
  try {
    const value: unknown = JSON.parse(localGet('collapsed-projects') || '[]')
    return Array.isArray(value) ? value.filter((id): id is string => typeof id === 'string') : []
  } catch { return [] }
}
function projectExpanded(id: string) { return !collapsedProjects.value.includes(id) }
function toggleProject(id: string) {
  collapsedProjects.value = projectExpanded(id) ? [...collapsedProjects.value, id] : collapsedProjects.value.filter(item => item !== id)
  localSet('collapsed-projects', JSON.stringify(collapsedProjects.value))
}
function projectSettings(project: Project | null = null) {
  if (navigationLocked.value) return
  editingProject.value = project; showProjectSettings.value = true
}
function projectSaved(updated: Project[]) {
  projects.value = updated; showProjectSettings.value = false; notify('项目已保存')
}
function projectPaths(project: Project): string[] { return project.local_paths ?? (project.local_path ? [project.local_path] : []) }
function directoryOptions(project: Project) { return projectPaths(project).map((path, index) => ({ label: path, key: index })) }
async function openProjectDirectory(project: Project, index = 0) {
  try { if (index === 0) await api.OpenProjectDirectory(project.id); else await api.OpenProjectDirectoryAt(project.id, index) } catch (exc) { error.value = String(exc) }
}
const activeSheet = computed(() => active.value?.sheets.find(sheet => sheet.name === sheetName.value) ?? active.value?.sheets[0])
const tasks = computed(() => activeSheet.value ? tasksFor(activeSheet.value) : [])
const priorityFields = computed(() => priorityColumns(activeSheet.value))
const priorityColumnLabel = computed(() => priorityFields.value.length === 1 ? priorityFields.value[0]!.label : '紧急 / 优先级')
const stats = computed(() => active.value ? documentStats(active.value) : { tasks: 0, pictures: 0 })
const options = computed(() => [{ label: '未完成任务', value: 'unfinished' },
  ...Array.from(new Set([...taskStatusChoices(activeSheet.value), ...tasks.value.map(task => task.status), ...selectedStatuses.value])).filter(value => value !== 'unfinished').map(value => ({ label: value, value })),
])
const selectedStatusLabels = computed(() => selectedStatuses.value.map(value => options.value.find(option => option.value === value)?.label ?? value))
function renderStatusSummary({ option }: { option: SelectOption }) {
  if (option.value !== selectedStatuses.value[0]) return null
  const labels = selectedStatusLabels.value
  return h('span', { class: 'status-selection-summary', title: labels.join('、') }, [
    h('span', { class: 'status-selection-names' }, labels.length <= 2 ? labels.join('、') : labels[0]),
    labels.length > 2 ? h('span', { class: 'status-selection-count' }, `等 ${labels.length} 项`) : null,
  ])
}
const unfinishedOnly = computed(() => selectedStatuses.value.length === 1 && selectedStatuses.value[0] === 'unfinished')
const filtered = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return tasks.value.filter(task => (!query || task.search.includes(query))
    && (!selectedStatuses.value.length || selectedStatuses.value.some(value => value === 'unfinished'
      ? (!isCompleted(task.status) || !!task.row.status_edit) : task.status === value)))
    .sort((a, b) => Number(isDueToday(b, today.value)) - Number(isDueToday(a, today.value)))
})
const listOrderLabel = computed(() => filtered.value.some(task => isDueToday(task, today.value)) ? '今日截止优先' : '按原表顺序')
const selected = computed(() => filtered.value.find(task => task.id === selectedTaskID.value) ?? filtered.value[0])
const selectedIndex = computed(() => filtered.value.findIndex(task => task.id === selected.value?.id))
const sheetOptions = computed(() => active.value?.sheets.map(sheet => ({ label: sheet.name, value: sheet.name })) ?? [])
const hasFilters = computed(() => !!search.value || (selectedStatuses.value.length > 0 && !unfinishedOnly.value))
const warnings = computed(() => [...libraryWarnings.value, ...(active.value?.warnings ?? [])])
const shortTitle = (doc: Document) => doc.title.replace(/\s*任务清单$/, '').replace(/（数智）$/, '')
const pendingCount = computed(() => active.value?.sheets.reduce((count, sheet) => count + sheet.rows.filter(row => row.status_edit).length, 0) ?? 0)
const editableStatus = computed(() => !!active.value?.source_url && !!selected.value?.sheet.headers.some(header => text(header).includes('状态')))
const noteFields = computed(() => selected.value?.sheet.headers.flatMap((header, column) => text(header).includes('备注')
  ? [{ column, label: text(header), value: String(selected.value?.row.values[column] ?? '') }] : []) ?? [])
const detailsChanged = computed(() => !!selected.value && (statusDraft.value !== selected.value.status
  || noteFields.value.some(field => (noteDrafts.value[field.column] ?? '') !== field.value)))
function resetDetailDrafts(task = selected.value) {
  statusDraft.value = task?.status ?? ''
  noteDrafts.value = Object.fromEntries((task?.sheet.headers ?? []).flatMap((header, column) => text(header).includes('备注')
    ? [[column, String(task?.row.values[column] ?? '')]] : []))
}
const statusChoices = computed(() => {
  const task = selected.value
  if (!task) return []
  const values = taskStatusChoices(task.sheet)
  return Array.from(new Set([task.status, ...values])).map(value => ({ label: value, value }))
})
const selectedOwner = computed(() => {
  const task = selected.value
  const column = task?.sheet.headers.findIndex(header => ['负责人', '负责人员', '责任人'].includes(text(header).replace(/\s/g, ''))) ?? -1
  return column >= 0 ? text(task?.row.values[column]) : ''
})
const currentImages = computed(() => selected.value?.row.pictures ?? [])
const imageToolbar: ImageGroupRenderToolbar = ({ nodes }) => {
  const buttons: [keyof typeof nodes, string][] = [
    ...(currentImages.value.length > 1 ? [['prev', '上一张图片'], ['next', '下一张图片']] as [keyof typeof nodes, string][] : []),
    ['rotateCounterclockwise', '向左旋转'], ['rotateClockwise', '向右旋转'],
    ['resizeToOriginalSize', '原始尺寸'], ['zoomOut', '缩小图片'], ['zoomIn', '放大图片'], ['close', '关闭图片预览'],
  ]
  return buttons.map(([key, label]) => cloneVNode(nodes[key], {
    role: 'button', 'aria-label': label, title: label, tabindex: 0,
    onKeydown: (event: KeyboardEvent) => {
      if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); (event.currentTarget as HTMLElement).click() }
    },
  }))
}
let focusSearchAfterClose = false
let offProgress = () => {}
let feedbackTimer: ReturnType<typeof setTimeout> | undefined
let dayTimer: ReturnType<typeof setTimeout> | undefined
function refreshToday() {
  clearTimeout(dayTimer)
  today.value = localDay()
  const now = new Date()
  const midnight = new Date(now)
  midnight.setHours(24, 0, 0, 0)
  dayTimer = setTimeout(refreshToday, midnight.getTime() - now.getTime() + 100)
}
function refreshVisibleDay() { if (!document.hidden) refreshToday() }

function localGet(key: string) { try { return window.localStorage.getItem(key) ?? '' } catch { return '' } }
function localSet(key: string, value: string) { try { window.localStorage.setItem(key, value) } catch { /* Library remains usable without web storage. */ } }
function statusFilterKey() { return `task-statuses:${selectedKey.value}:${activeSheet.value?.name ?? ''}` }
function loadStatusFilter(): string[] {
  try {
    const saved = localGet(statusFilterKey())
    if (saved) {
      const value: unknown = JSON.parse(saved)
      if (Array.isArray(value) && value.every((item): item is string => typeof item === 'string' && !!item.trim())) return [...new Set(value)]
    }
  } catch { /* Use the default when a saved preference is invalid. */ }
  return [...defaultTaskStatuses]
}
function updateStatusFilter(value: string[] | null) {
  selectedStatuses.value = value ?? []
  if (selectedKey.value && activeSheet.value) localSet(statusFilterKey(), JSON.stringify(selectedStatuses.value))
}
function resetFilters() { searchExpanded.value = false; search.value = ''; selectedStatuses.value = loadStatusFilter() }
function openSearch() {
  searchExpanded.value = true
  nextTick(() => searchInput.value?.focus())
}
function closeSearch() {
  search.value = ''; searchExpanded.value = false
  nextTick(() => searchToggle.value?.focus())
}
function clearAllFilters() { searchExpanded.value = false; search.value = ''; updateStatusFilter([]) }
function notify(message: string) {
  feedback.value = message
  clearTimeout(feedbackTimer)
  feedbackTimer = setTimeout(() => { feedback.value = '' }, 2400)
}
async function chooseDocument(key: string) {
  if (!(await canLeavePage())) return
  currentPage.value = 'tasks'
  reportNavigationLocked.value = false
  selectedKey.value = key
  api.SelectDocument(key).catch(exc => { error.value = `无法记录上次打开的文档：${String(exc)}` })
}
function chooseTask(task: Task) { statusError.value = ''; resetDetailDrafts(task); selectedTaskID.value = task.id; showDetails.value = true }
function restoreTaskFocus() {
  // Restore focus after the drawer has finished leaving the page.
  requestAnimationFrame(() => {
    if (showAppSettings.value || showSettings.value || showProjectSettings.value || showDetails.value || currentPage.value !== 'tasks') return
    if (focusSearchAfterClose) { focusSearchAfterClose = false; searchInput.value?.focus(); return }
    const row = Array.from(taskList.value?.querySelectorAll<HTMLButtonElement>('.task-item') ?? [])
      .find(button => button.dataset.taskId === selectedTaskID.value)
    row?.focus({ preventScroll: true })
  })
}
function moveTask(direction: number) {
  const index = Math.min(filtered.value.length - 1, Math.max(0, selectedIndex.value + direction))
  const task = filtered.value[index]
  if (task) { chooseTask(task); nextTick(() => taskList.value?.querySelector<HTMLElement>('[aria-current="true"]')?.scrollIntoView({ block: 'nearest' })) }
}
function listKey(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault(); moveTask(event.key === 'ArrowDown' ? 1 : -1)
  }
}
function focusDetails() {
  if (imagePreviewOpen.value) return
  document.querySelector<HTMLButtonElement>('.task-drawer [aria-label="关闭任务详情"]')?.focus()
}
function globalKey(event: KeyboardEvent) {
  if (showAppSettings.value || showProjectSettings.value || currentPage.value !== 'tasks') return
  if (statusMenuOpen.value && (event.key === 'Escape' || event.key === 'Tab')) return
  if (event.key === 'Tab' && showDetails.value && !showSettings.value) {
    const container = document.querySelector(imagePreviewOpen.value ? '.n-image-preview-toolbar' : '.task-drawer')
    const controls = Array.from(container?.querySelectorAll<HTMLElement>('button:not(:disabled), [tabindex="0"]') ?? [])
      .filter(element => element.getClientRects().length > 0)
    const first = controls[0], last = controls[controls.length - 1]
    if (first && last && (!container?.contains(document.activeElement) || (event.shiftKey ? document.activeElement === first : document.activeElement === last))) {
      event.preventDefault()
      const target = event.shiftKey ? last : first
      target.focus()
    }
  }
  if (event.key === 'Escape' && imagePreviewOpen.value) {
    event.preventDefault()
    event.stopImmediatePropagation()
    imagePreviewOpen.value = false
    return
  }
  if (event.key === 'Escape' && showDetails.value && !showSettings.value) {
    event.preventDefault()
    event.stopImmediatePropagation()
    showDetails.value = false
    return
  }
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'f' && !showSettings.value) {
    event.preventDefault()
    if (showDetails.value) { searchExpanded.value = true; focusSearchAfterClose = true; showDetails.value = false }
    else openSearch()
  }
}
function settings(newDocument = false, projectID?: string, documentKey?: string) {
  if (navigationLocked.value) return
  const doc = documents.value.find(item => item.key === (documentKey ?? selectedKey.value))
  editingDocumentKey.value = newDocument ? '' : doc?.key ?? ''
  draftProjectID.value = projectID ?? projects.value.find(project => project.document_keys.includes(doc?.key ?? ''))?.id ?? ''
  draftPullConditions.value = doc && !newDocument ? editableConditions(doc) : { owner: '', include_completed: true, filters: [] }
  initialPullConditions.value = JSON.stringify(draftPullConditions.value)
  isNew.value = newDocument
  draftURL.value = newDocument ? '' : doc?.source_url ?? ''
  formError.value = ''
  showSettings.value = true
}
function validateForm() {
  if (!isNew.value) return ''
  try {
    const url = new URL(draftURL.value.trim())
    if (url.protocol !== 'https:' || url.host !== 'docs.qq.com' || url.username || url.password
      || !/^\/(sheet|smartsheet)\/[A-Za-z0-9_-]+$/.test(url.pathname)) throw new Error()
  } catch { return '请填写完整的腾讯文档表格链接。' }
  return ''
}
async function perform(action: () => Promise<Document | null>, title: string, projectID?: string) {
  if (busy.value || statusBusy.value || authorizing.value) return
  busy.value = true; cancelling.value = false; error.value = ''; progress.value = '正在读取任务…'
  syncingTitle.value = title; syncSummary.value = ''; syncIssues.value = []; queueTotal.value = 0
  try {
    const doc = await action()
    if (!doc) return
    syncSummary.value = `已同步「${doc.title}」`
    const index = documents.value.findIndex(item => item.key === doc.key)
    if (index >= 0) documents.value[index] = doc
    else documents.value.push(doc)
    showDetails.value = false
    selectedKey.value = doc.key
    resetFilters()
    if (projectID !== undefined) {
      try { projects.value = await api.AssignDocumentProject(doc.key, projectID) }
      catch (exc) { error.value = `文档已保存，但项目归属未保存：${String(exc)}。可在项目设置中重新分配。` }
    }
    notify(`已保存 ${documentStats(doc).tasks} 条任务、${documentStats(doc).pictures} 张图片`)
  } catch (exc) {
    const message = String(exc)
    if (cancelling.value && message.includes('已取消')) { syncSummary.value = '已取消同步，本地内容已保留'; notify(syncSummary.value) }
    else { error.value = message; syncSummary.value = `「${title}」同步失败，本地内容已保留`; syncIssues.value = [message] }
  } finally { busy.value = false; cancelling.value = false; progress.value = '' }
}
async function submitSettings() {
  if (savingConditions.value || busy.value || statusBusy.value || authorizing.value) return
  formError.value = validateForm()
  if (formError.value) return
  const conditionsChanged = !isNew.value && JSON.stringify(draftPullConditions.value) !== initialPullConditions.value
  if (conditionsChanged && editingHasPendingChanges.value) { formError.value = '请先回写或撤销待同步修改，再更改拉取条件。'; return }
  if (conditionsChanged && draftPullConditions.value.filters?.some(rule => needsValue(rule) && !rule.value.trim())) {
    formError.value = '请填写匹配内容，或删除未填写的条件。'; return
  }
  savingConditions.value = true
  if (!isNew.value) {
    try {
      if (conditionsChanged) {
        replaceDocument(await api.SaveDocumentFilters(editingDocumentKey.value, draftPullConditions.value))
        initialPullConditions.value = JSON.stringify(draftPullConditions.value)
      }
      projects.value = await api.AssignDocumentProject(editingDocumentKey.value, draftProjectID.value)
      showSettings.value = false
      notify('文档设置已保存')
    }
    catch (exc) { formError.value = String(exc) }
    finally { savingConditions.value = false }
    return
  }
  busy.value = true; cancelling.value = false; progress.value = '正在识别文档表头…'; syncingTitle.value = '新文档'; syncSummary.value = ''; syncIssues.value = []
  try {
    const doc = await api.PrepareOnlineDocument(draftURL.value.trim())
    const index = documents.value.findIndex(item => item.key === doc.key)
    if (index >= 0) documents.value[index] = doc; else documents.value.push(doc)
    selectedKey.value = doc.key; showDetails.value = false; currentPage.value = 'tasks'
    try { projects.value = await api.AssignDocumentProject(doc.key, draftProjectID.value) }
    catch (exc) { error.value = `文档已保存，但项目归属未保存：${String(exc)}。可在项目设置中重新分配。` }
    showSettings.value = false
    syncSummary.value = doc.setup_pending ? `已识别「${doc.title}」表头，设置条件后点击同步文档` : `已打开「${doc.title}」`
    notify(doc.setup_pending ? '文档已添加，请在文档设置中设置拉取条件后同步' : '已打开已有文档')
  } catch (exc) { formError.value = String(exc); syncSummary.value = '文档识别失败，请检查链接后重试' }
  finally { busy.value = false; savingConditions.value = false; cancelling.value = false; progress.value = '' }
}
async function syncActive() {
  if (pendingCount.value) { error.value = '有待同步的状态或备注修改，请先回写或撤销，再同步文档。'; return }
  const doc = active.value
  if (!doc || showSettings.value) return
  await perform(() => api.SyncConfiguredDocument(doc.key, true), doc.title)
}
async function importExcel() {
  const projectID = showSettings.value ? draftProjectID.value : activeProject.value?.id ?? ''
  showSettings.value = false
  await perform(() => api.ImportExcel('', true), '本地 Excel', projectID)
}
async function syncOnStartup() {
  if (disposed || loadError.value || busy.value || statusBusy.value || authorizing.value) return
  const queue = documents.value.filter(doc => !!doc.source_url && !doc.setup_pending)
  if (!queue.length) return
  busy.value = true; cancelling.value = false; queueStopped = false
  queueTotal.value = queue.length; syncIssues.value = []; syncSummary.value = ''
  let completed = 0, failed = 0, skipped = 0
  try {
    for (const [index, doc] of queue.entries()) {
      if (queueStopped || disposed) break
      queuePosition.value = index + 1; syncingTitle.value = doc.title; progress.value = '正在读取任务…'
      if (doc.sheets.some(sheet => sheet.rows.some(row => !!row.status_edit))) {
        skipped++
        syncIssues.value.push(`${doc.title}：有待回写的状态或备注修改，已跳过`)
        continue
      }
      try {
        const updated = await (doc.pull_conditions ? api.SyncConfiguredDocument(doc.key, false) : api.SyncDocumentOnStartup(doc.source_url, doc.owner, true))
        if (disposed) break
        replaceDocument(updated)
        completed++
      } catch (exc) {
        if (queueStopped || disposed) break
        failed++
        syncIssues.value.push(`${doc.title}：${String(exc)}`)
      }
    }
    syncSummary.value = `${queueStopped ? '已取消自动同步' : '自动同步完成'} · ${completed} 份成功${failed ? ` · ${failed} 份失败` : ''}${skipped ? ` · ${skipped} 份跳过` : ''}`
  } finally {
    busy.value = false; cancelling.value = false; progress.value = ''; queueTotal.value = 0
  }
}
const schedules = ref<SyncSchedule[]>([])
const scheduleLoadError = ref(''), runningScheduleID = ref('')
const schedulesReady = ref(false)
const scheduleBlocked = computed(() => !schedulesReady.value || disposed || loading.value || !!loadError.value || !!scheduleLoadError.value || busy.value || statusBusy.value || authorizing.value || showDetails.value || showSettings.value || showProjectSettings.value || savingConditions.value || settingsEditingLocked.value || pageNavigationLocked.value)
const scheduleRunner = createScheduleRunner({
  items: () => schedules.value,
  canRun: () => !scheduleBlocked.value,
  run: runScheduledPull,
  complete: async (item, result) => { schedules.value = await api.CompleteSyncSchedule(item.id, item.revision, result) },
  onRunning: id => { runningScheduleID.value = id },
  onError: message => { syncSummary.value = message; notify(message) },
})
async function loadSchedules() {
  try { schedules.value = await api.GetSyncSchedules(); scheduleLoadError.value = '' }
  catch (exc) { scheduleLoadError.value = `定时任务加载失败：${String(exc)}` }
  scheduleRunner.wake()
}
function schedulesUpdated(items: SyncSchedule[]) { schedules.value = items; scheduleRunner.wake() }
async function runScheduleNow(id: string) {
  if (!(await scheduleRunner.runNow(id))) notify('请等待当前同步、编辑或弹窗操作结束后再拉取。')
}
async function runScheduledPull(item: SyncSchedule): Promise<string> {
  // Reserve the shared queue before reading the latest project membership.
  busy.value = true; cancelling.value = false; queueStopped = false
  queuePosition.value = 0; queueTotal.value = 0; syncIssues.value = []; syncSummary.value = ''
  syncingTitle.value = projects.value.find(project => project.id === item.project_id)?.name ?? '定时任务'
  progress.value = '正在读取项目文档…'
  let completed = 0, failed = 0, skipped = 0
  let projectName = syncingTitle.value
  try {
    const state = await api.GetLibrary()
    if (disposed) return '应用已关闭'
    const project = state.projects?.find(project => project.id === item.project_id)
    if (!project) throw new Error('项目已不存在，请修改或删除定时任务。')
    projectName = project.name
    projects.value = state.projects ?? []
    // No task editor can save while this queue holds busy. Keep current selection
    // and filters, but include documents added to the project since the last run.
    documents.value = state.documents
    const queue = state.documents.filter(doc => project.document_keys.includes(doc.key) && !!doc.source_url && !doc.setup_pending)
    queueTotal.value = queue.length
    for (const [index, doc] of queue.entries()) {
      if (queueStopped || disposed) break
      queuePosition.value = index + 1; syncingTitle.value = doc.title; progress.value = '定时拉取 · 正在读取任务…'
      if (doc.sheets.some(sheet => sheet.rows.some(row => !!row.status_edit))) {
        skipped++; syncIssues.value.push(`${doc.title}：有待回写的状态或备注修改，已跳过`); continue
      }
      try {
        const updated = await api.SyncConfiguredDocument(doc.key, false)
        if (disposed) break
        replaceDocument(updated); completed++
      } catch (exc) {
        if (queueStopped || disposed) break
        failed++; syncIssues.value.push(`${doc.title}：${String(exc)}`)
      }
    }
    syncSummary.value = `${queueStopped ? '已取消定时拉取' : '定时拉取完成'} · ${projectName} · ${completed} 份成功${failed ? ` · ${failed} 份失败` : ''}${skipped ? ` · ${skipped} 份跳过` : ''}${!queue.length ? ' · 暂无在线文档' : ''}`
    // Bound persisted diagnostics by UTF-8 bytes without splitting a character.
    let result = syncSummary.value
    for (const issue of syncIssues.value) {
      const next = `${result}\n${issue}`
      if (new TextEncoder().encode(next).length > 7600) break
      result = next
    }
    return result
  } catch (exc) {
    syncSummary.value = `定时拉取失败 · ${projectName}：${String(exc)}`
    return Array.from(syncSummary.value).slice(0, 1600).join('')
  } finally {
    busy.value = false; cancelling.value = false; progress.value = ''; queueTotal.value = 0
  }
}
function wakeSchedules() { scheduleRunner.wake() }
watch([scheduleBlocked, showAppSettings], wakeSchedules)
async function cancel() {
  queueStopped = true
  cancelling.value = true
  try { await api.CancelSync() } catch (exc) { error.value = String(exc); cancelling.value = false }
}
async function copy() {
  if (!selected.value) return
  try { await api.CopyText(copyTask(selected.value)); notify('已复制任务文字') } catch (exc) { error.value = String(exc) }
}
async function openSource() {
  if (!active.value) return
  try { await api.OpenSource(active.value.key) } catch (exc) { error.value = String(exc) }
}
function pictureCaption(column: number) { return text(selected.value?.sheet.headers[column - 1]) || '任务配图' }
function replaceDocument(doc: Document) {
  const index = documents.value.findIndex(item => item.key === doc.key)
  if (index >= 0) documents.value[index] = { ...doc, pull_conditions: doc.pull_conditions ?? documents.value[index]?.pull_conditions }
}
async function authorizeRemote() {
  if (busy.value || statusBusy.value || authorizing.value) return
  authorizing.value = true; formError.value = ''; statusError.value = ''
  try { remoteAuth.value = await api.AuthorizeRemote(); notify('腾讯文档授权成功，可以回写任务状态与备注') }
  catch (exc) { if (showSettings.value) formError.value = String(exc); else statusError.value = String(exc) }
  finally { authorizing.value = false }
}
async function changeStatus(action: 'sync' | 'revert') {
  const doc = active.value, task = selected.value, value = statusDraft.value
  const notes = noteFields.value.map(field => ({ column: field.column, value: noteDrafts.value[field.column] ?? '' }))
  if (!doc || !task || busy.value || statusBusy.value || authorizing.value) return
  statusBusy.value = true; cancelling.value = false; statusError.value = ''
  try {
    let updated: Document
    if (action === 'revert') {
      updated = await api.RevertTaskStatus(doc.key, task.sheet.name, task.row.number)
    } else {
      if (detailsChanged.value) {
        updated = await api.UpdateTaskDetails(doc.key, task.sheet.name, task.row.number, value === task.status ? task.statusValue : value, notes)
        replaceDocument(updated)
      }
      if (!remoteAuth.value.authorized) {
        authorizing.value = true
        try { remoteAuth.value = await api.AuthorizeRemote() }
        finally { authorizing.value = false }
      }
      updated = await api.PushTaskStatus(doc.key, task.sheet.name, task.row.number)
    }
    replaceDocument(updated)
    const row = updated.sheets.find(sheet => sheet.name === task.sheet.name)?.rows.find(row => row.number === task.row.number)
    if (row?.status_edit?.error) statusError.value = row.status_edit.error
    else notify(action === 'sync' ? '本地与远程的状态、备注已同步' : '已撤销本地修改')
  } catch (exc) { statusError.value = String(exc) }
  finally { statusBusy.value = false }
}
async function loadLibrary() {
  loading.value = true; loadError.value = ''
  try {
    const state = await api.GetLibrary()
    documents.value = state.documents
    projects.value = state.projects ?? []
    libraryWarnings.value = state.warnings
    selectedKey.value = state.selected_key
  } catch (exc) { loadError.value = String(exc) }
  finally { loading.value = false }
}

watch(selectedKey, () => {
  showDetails.value = false
  error.value = ''
  const rememberedSheet = localGet(`sheet:${selectedKey.value}`)
  sheetName.value = active.value?.sheets.some(sheet => sheet.name === rememberedSheet) ? rememberedSheet : active.value?.sheets[0]?.name ?? ''
  resetFilters()
  selectedTaskID.value = localGet(`task:${selectedKey.value}:${sheetName.value}`)
}, { flush: 'sync' })
watch(sheetName, () => {
  showDetails.value = false
  resetFilters()
  localSet(`sheet:${selectedKey.value}`, sheetName.value)
  const remembered = localGet(`task:${selectedKey.value}:${sheetName.value}`)
  selectedTaskID.value = tasks.value.some(task => task.id === remembered) ? remembered : tasks.value[0]?.id ?? ''
})
watch(filtered, rows => {
  if (!rows.some(task => task.id === selectedTaskID.value)) {
    showDetails.value = false
    selectedTaskID.value = rows[0]?.id ?? ''
  }
}, { immediate: true })
watch(showDetails, show => { if (!show) { imagePreviewOpen.value = false; statusMenuOpen.value = false } })
watch(() => selected.value, task => { resetDetailDrafts(task); statusError.value = '' })
watch(selectedTaskID, () => {
  imagePreviewOpen.value = false
  if (selectedTaskID.value) localSet(`task:${selectedKey.value}:${sheetName.value}`, selectedTaskID.value)
  nextTick(() => detail.value?.scrollTo({ top: 0 }))
})
onMounted(async () => {
  api.GetActiveTaskExecution().then(run => { if (run && !disposed) trackExecution(run) }).catch(() => {})
  refreshToday()
  window.addEventListener('focus', refreshToday)
  window.addEventListener('focus', wakeSchedules)
  document.addEventListener('visibilitychange', wakeSchedules)
  document.addEventListener('visibilitychange', refreshVisibleDay)
  offProgress = onProgress(message => { progress.value = message })
  window.addEventListener('keydown', globalKey, true)
  await Promise.all([loadLibrary(), loadSchedules()])
  try { remoteAuth.value = await api.GetRemoteAuth() } catch { /* Reading cached tasks does not require write authorization. */ }
  await syncOnStartup()
  if (!disposed) { schedulesReady.value = true; scheduleRunner.wake() }
})
onUnmounted(() => { scheduleRunner.stop(); window.removeEventListener('focus', wakeSchedules); document.removeEventListener('visibilitychange', wakeSchedules); disposed = true; queueStopped = true; offProgress(); clearTimeout(feedbackTimer); clearTimeout(dayTimer); clearTimeout(executionTimer); window.removeEventListener('focus', refreshToday); document.removeEventListener('visibilitychange', refreshVisibleDay); window.removeEventListener('keydown', globalKey, true) })
</script>

<template>
  <NConfigProvider :theme-overrides="theme" :locale="zhCN" :date-locale="dateZhCN">
    <div class="desktop">
      <header class="window-bar"><span>我的任务</span><span class="window-caption">本地任务库</span></header>
      <div class="app-layout">
        <aside class="sidebar">
          <div class="brand"><span class="brand-mark"><PhCheck weight="bold" :size="22" /></span><div><strong>我的任务</strong><span>让工作清晰一点</span></div></div>
          <div class="nav-heading"><span>项目与文档</span><div class="nav-heading-actions"><NTooltip><template #trigger><button class="icon-button" aria-label="新建项目" :disabled="loading || !!loadError || navigationLocked" @click="projectSettings()"><PhFolderSimple :size="17" /></button></template>新建项目</NTooltip><NTooltip><template #trigger><button class="icon-button" aria-label="添加在线文档" :disabled="busy || statusBusy || authorizing || navigationLocked" @click="settings(true)"><PhPlus :size="17" weight="bold" /></button></template>添加在线文档</NTooltip></div></div>
          <nav class="document-nav" aria-label="文档列表">
            <template v-if="loading"><NSkeleton v-for="i in 2" :key="i" height="42px" :sharp="false" /></template>
            <div v-for="group in documentGroups" :key="group.id" class="project-group">
              <div class="project-group-heading">
                <button class="project-name" :title="group.name" :disabled="navigationLocked" :aria-label="`${projectExpanded(group.id) ? '收起' : '展开'}项目 ${group.name}`" :aria-expanded="projectExpanded(group.id)" :aria-controls="`project-documents-${group.id || 'ungrouped'}`" @click="toggleProject(group.id)"><PhCaretRight class="project-chevron" :class="{ expanded: projectExpanded(group.id) }" :size="12" /><PhFolderSimple :size="15" /><span>{{ group.name }}</span></button>
                <button v-if="group.project" class="icon-button" :aria-label="`为 ${group.name} 添加文档`" title="添加文档" :disabled="busy || statusBusy || authorizing || navigationLocked" @click="settings(true, group.id)"><PhPlus :size="14" /></button>
                <NDropdown v-if="group.project && projectPaths(group.project).length > 1" :options="directoryOptions(group.project)" trigger="click" @select="openProjectDirectory(group.project!, Number($event))"><button class="icon-button" :aria-label="`打开 ${group.name} 本地目录`" :title="`${projectPaths(group.project).length} 个本地目录`" :disabled="navigationLocked"><PhArrowSquareOut :size="14" /></button></NDropdown>
                <button v-else-if="group.project && projectPaths(group.project).length" class="icon-button" :title="projectPaths(group.project)[0]" :aria-label="`打开 ${group.name} 本地目录`" :disabled="navigationLocked" @click="openProjectDirectory(group.project)"><PhArrowSquareOut :size="14" /></button>
                <button v-if="group.project" class="icon-button project-settings-button" :aria-label="`${group.name} 项目设置`" :disabled="navigationLocked" @click="projectSettings(group.project)"><PhSlidersHorizontal :size="14" /></button>
              </div>
              <div v-show="projectExpanded(group.id)" :id="`project-documents-${group.id || 'ungrouped'}`" class="project-documents">
              <button v-if="group.project && group.project.daily_report_enabled !== false" class="project-report-link daily-report-link" :class="{ active: currentPage === 'report' && reportProject?.id === group.id }" :aria-current="currentPage === 'report' && reportProject?.id === group.id ? 'page' : undefined" :aria-label="`${group.name} 工作日报`" :disabled="navigationLocked" @click="openDailyReport(group.project)"><PhNotebook class="document-icon" :size="19" weight="duotone" /><span class="document-label"><strong>工作日报</strong></span></button>
              <hr v-if="group.project && group.project.daily_report_enabled !== false && group.documents.length" class="project-report-divider" />
              <p v-if="!group.documents.length" class="project-empty">暂无文档 · 点击项目管理</p>
            <button v-for="doc in group.documents" :key="doc.key" class="document-link" :class="{ active: currentPage === 'tasks' && doc.key === selectedKey }" :aria-current="currentPage === 'tasks' && doc.key === selectedKey ? 'page' : undefined" :disabled="navigationLocked" @click="chooseDocument(doc.key)">
              <PhFileXls v-if="!doc.source_url" class="document-icon" :size="19" weight="duotone" /><PhSquaresFour v-else class="document-icon" :size="19" weight="duotone" />
              <span class="document-label"><strong>{{ shortTitle(doc) }}</strong></span>
              <span v-if="doc.cached" class="nav-count">{{ documentStats(doc).tasks }}</span>
              <span v-else class="unsynced-dot" />
            </button>
              </div>
            </div>
          </nav>
          <div class="sidebar-bottom">
            <div class="offline-note"><PhStack :size="18" weight="duotone" /><div><strong>保存在这台 Mac</strong><p>任务与图片，离线也能看</p></div></div>
            <button class="app-settings-link" :class="{ active: showAppSettings }" :aria-expanded="showAppSettings" aria-haspopup="dialog" aria-label="软件设置" title="设置" :disabled="busy || statusBusy || authorizing || navigationLocked" @click="openSettings"><PhGear :size="22" /><span><strong>设置</strong><small>AI · 定时任务</small></span><PhCaretRight :size="16" /></button>
          </div>
        </aside>

        <div class="page-area">
        <DailyReportPanel v-if="currentPage === 'report' && reportProject" ref="reportPage" :key="reportProject.id" :project="reportProject" @lock="reportNavigationLocked = $event" />
        <main v-show="currentPage === 'tasks'" class="workspace" :aria-label="active?.title || '任务库'">
          <header class="workspace-header document-toolbar" aria-label="文档操作">
            <DocumentPullBar v-if="active" :document="active" />
            <span v-else class="document-toolbar-empty">选择文档后查看拉取条件</span>
            <div class="document-actions"><span v-if="pendingCount" class="pending-total">{{ pendingCount }} 项待回写</span><div class="sync-meta"><span class="connection-dot" :class="{ syncing: busy, uncached: !active?.cached }" />{{ busy ? '同步中' : active?.cached ? `已同步 ${formatDate(active.updated_at, true)}` : '尚未同步' }}</div><div class="action-buttons"><NButton v-if="busy" size="small" type="primary" disabled><template #icon><PhArrowsClockwise class="spin" :size="16" /></template>同步中</NButton><NButton v-else size="small" type="primary" :disabled="!active || statusBusy || authorizing || showSettings" @click="syncActive"><template #icon><PhArrowsClockwise :size="16" /></template>{{ active?.source_url ? '同步文档' : '重新读取' }}</NButton><NTooltip><template #trigger><button class="icon-button document-settings-button" aria-label="当前文档设置" :disabled="!active || busy || statusBusy || authorizing || navigationLocked" @click="settings()"><PhGear :size="18" /></button></template>当前文档设置</NTooltip></div></div>
          </header>

          <div v-if="error || loadError" class="error-banner" role="alert"><span>{{ error || loadError }}<small v-if="active?.cached">本地内容已保留，可以继续浏览。</small></span><NButton v-if="loadError" size="small" @click="loadLibrary">重试</NButton><button v-else class="icon-button" aria-label="关闭错误提示" @click="error = ''"><PhX :size="17" /></button></div>

          <div class="reader-layout">
            <section class="task-column" aria-label="任务列表">
              <div class="list-toolbar">
                <div class="list-summary"><span>任务列表 <b>{{ filtered.length }}</b><span v-if="hasFilters" class="filtered-total"> / {{ tasks.length }}</span></span><NSelect v-if="sheetOptions.length > 1" v-model:value="sheetName" :options="sheetOptions" size="small" class="sheet-select" aria-label="选择工作表" /><span v-else class="list-order">{{ listOrderLabel }}</span></div>
                <div class="task-controls">
                  <div class="search-tools" :class="{ expanded: searchExpanded }">
                    <template v-if="searchExpanded"><NInput id="task-search" ref="searchInput" v-model:value="search" size="small" clearable placeholder="搜索任务内容…" aria-label="搜索任务内容" @keydown.esc.stop.prevent="closeSearch"><template #prefix><PhMagnifyingGlass :size="16" /></template></NInput><button class="icon-button" aria-label="收起搜索" title="收起搜索（Esc）" @click="closeSearch"><PhX :size="16" /></button></template>
                    <button v-else ref="searchToggle" class="icon-button search-toggle" aria-label="搜索任务" aria-expanded="false" aria-controls="task-search" aria-keyshortcuts="Meta+F Control+F" title="搜索任务（Command + F）" @click="openSearch"><PhMagnifyingGlass :size="18" /></button>
                  </div>
                  <div class="filter-line"><NSelect :value="selectedStatuses" :options="options" @update:value="updateStatusFilter" multiple clearable :render-tag="renderStatusSummary" placeholder="全部状态" size="small" aria-label="筛选处理状态" class="status-select" /></div>
                </div>
              </div>
              <div v-if="filtered.length" class="task-table-heading" :class="{ 'has-priority': priorityFields.length }" aria-hidden="true"><span>任务内容</span><span v-if="priorityFields.length">{{ priorityColumnLabel }}</span><span>处理状态</span><span>配图</span><span>日期</span><span /></div>
              <div ref="taskList" class="task-list" tabindex="0" aria-label="使用上下方向键选择任务" @keydown="listKey">
                <div v-if="loading" class="list-skeleton"><div v-for="i in 5" :key="i"><NSkeleton text width="35%" /><NSkeleton text :repeat="2" /><NSkeleton text width="55%" /></div></div>
                <template v-else-if="filtered.length">
                  <button v-for="task in filtered" :key="task.id" class="task-item" :class="{ selected: showDetails && selectedTaskID === task.id, 'due-today': isDueToday(task, today), 'has-priority': priorityFields.length }" :data-task-id="task.id" :aria-current="showDetails && selectedTaskID === task.id ? 'true' : undefined" aria-haspopup="dialog" @click="chooseTask(task)">
                    <div class="task-main"><div class="task-eyebrow"><span class="row-number">#{{ String(task.row.number).padStart(3, '0') }}</span><span class="task-context">{{ task.context || '任务' }}</span><span v-if="task.row.status_edit" class="pending-label">{{ task.row.status_edit.error ? '回写未成功' : '待回写' }}</span></div><h2>{{ task.title }}</h2></div>
                    <span v-if="priorityFields.length" class="task-priority"><PriorityBadges v-if="task.priorities.length" :values="task.priorities" /><span v-else class="priority-empty">—</span></span>
                    <span class="status-label" :class="tone(task.status)"><i />{{ task.status }}</span>
                    <span class="picture-count"><template v-if="task.row.pictures.length && active"><img :src="imageURL(active.key, task.row.pictures[0]!.file, 'thumbnail')" :alt="`第 ${task.row.number} 行配图缩略图`" class="task-thumbnail" loading="lazy" /><span>{{ task.row.pictures.length }}</span></template><span v-else>—</span></span>
                    <span class="task-date"><span>{{ task.date || '—' }}</span><span v-if="isDueToday(task, today)" class="due-today-label"><PhAlarm :size="12" />今日截止</span></span><PhCaretRight class="row-chevron" :size="16" />
                  </button>
                </template>
                <div v-else class="list-empty"><PhMagnifyingGlass v-if="hasFilters" :size="30" weight="duotone" /><PhStack v-else :size="32" weight="duotone" /><strong>{{ hasFilters ? '没有符合条件的任务' : active?.cached ? unfinishedOnly ? '暂无未完成任务' : '暂无匹配任务' : '等待同步文档' }}</strong><p>{{ hasFilters ? '试试其他关键词或筛选条件' : active?.cached ? unfinishedOnly ? '当前没有需要跟进的任务，可同步文档获取最新内容' : '可在文档设置中调整拉取条件后重新同步' : '同步后，任务会集中显示在这里' }}</p><NButton v-if="hasFilters" size="small" @click="clearAllFilters">清除筛选</NButton><NButton v-else-if="!active?.cached" size="small" :disabled="busy || statusBusy || authorizing || showSettings" @click="syncActive">开始同步</NButton></div>
              </div>

            </section>
          </div>
        </main>
              <footer class="list-footer">
                <div class="footer-status" :class="{ syncing: busy || statusBusy || authorizing, 'has-issues': !busy && !statusBusy && !authorizing && syncIssues.length }" role="status" :title="syncIssues.length && !busy ? syncIssues.join('\n') : syncStatusText">
                  <PhArrowsClockwise v-if="busy || statusBusy || authorizing" class="spin" :size="14" /><PhCheckCircle v-else-if="active?.cached" :size="14" /><PhWifiSlash v-else :size="14" />
                  <span>{{ syncStatusText || (active?.cached ? `${stats.tasks} 条任务 · ${stats.pictures} 张图片已缓存` : '同步后自动保存到本机') }}</span>
                </div>
                <button v-if="busy" class="footer-cancel" :disabled="cancelling" @click="cancel">{{ cancelling ? '取消中…' : '取消同步' }}</button>
                <button v-if="executionRun" class="footer-execution" :title="`查看 AI 执行记录 · ${executionRun.repository}`" @click="openExecution"><PhSparkle :size="14" />AI {{ executionLabel }} · #{{ executionRun.row }}</button>
                <WorkdayCountdown />
                <ResourceMonitor />
              </footer>
        </div>
      </div>
      <Transition name="toast"><div v-if="feedback" class="feedback-toast" role="status"><PhCheckCircle :size="18" weight="fill" />{{ feedback }}</div></Transition>
    </div>

    <NDrawer v-model:show="showDetails" placement="right" :close-on-esc="!imagePreviewOpen" :trap-focus="false" aria-label="任务详情" @after-enter="focusDetails" width="min(720px, calc(100vw - 64px))" class="task-drawer" @after-leave="restoreTaskFocus">
      <section class="detail-column" aria-label="任务详情内容">
        <template v-if="selected && active">
          <header class="detail-toolbar"><span>任务详情 <span class="toolbar-divider">/</span><span class="row-number">第 {{ selected.row.number }} 行</span></span><div><NTooltip><template #trigger><button class="icon-button" aria-label="复制任务文字" @click="copy"><PhCopy :size="17" /></button></template>复制任务文字</NTooltip><NTooltip v-if="active.source_url"><template #trigger><button class="icon-button" aria-label="打开原始腾讯文档" @click="openSource"><PhArrowSquareOut :size="17" /></button></template>在浏览器查看原表</NTooltip><span class="toolbar-divider">|</span><button class="icon-button" :disabled="selectedIndex <= 0" aria-label="上一条任务" @click="moveTask(-1)"><PhCaretLeft :size="16" /></button><button class="icon-button" :disabled="selectedIndex >= filtered.length - 1" aria-label="下一条任务" @click="moveTask(1)"><PhCaretRight :size="16" /></button><span class="toolbar-divider">|</span><button class="icon-button" aria-label="关闭任务详情" @click="showDetails = false"><PhX :size="18" /></button></div></header>
          <article ref="detail" :key="selected.id" class="task-detail">
            <div class="detail-title-block"><div class="detail-badges"><span class="status-badge" :class="tone(selected.status)"><i />{{ selected.status }}</span><PriorityBadges v-if="selected.priorities.length" :values="selected.priorities" /><span v-if="isDueToday(selected, today)" class="due-today-label"><PhAlarm :size="13" />今日截止</span><span class="detail-reference">#{{ String(selected.row.number).padStart(3, '0') }}</span></div><h2>{{ selected.context || '任务详情' }}</h2><div class="detail-owner"><template v-if="selectedOwner"><span class="mini-avatar">{{ selectedOwner.slice(-1) }}</span><span>{{ selectedOwner }}</span><span class="dot-separator">·</span></template><span>{{ selected.sheet.name }}</span></div></div>
            <section v-if="editableStatus" class="status-editor" aria-label="修改任务状态">
              <div class="status-editor-heading"><label>任务状态与备注</label><span>{{ selected.row.status_edit ? '本地已更新 · 等待远程确认' : selected.row.status_synced_at ? '本地与远程已同步' : '同步时更新本地与远程' }}</span></div>
              <div class="status-editor-controls"><NSelect v-model:value="statusDraft" v-model:show="statusMenuOpen" :options="statusChoices" :disabled="statusBusy || busy || authorizing" aria-label="修改任务状态" /><NButton type="primary" :loading="statusBusy" :disabled="busy || authorizing || (!detailsChanged && !selected.row.status_edit)" @click="changeStatus('sync')">同步</NButton><NButton v-if="authorizing" :disabled="cancelling" @click="cancel">取消授权</NButton></div>
              <div v-for="field in noteFields" :key="field.column" class="note-editor">
                <label :for="`task-note-${field.column}`">{{ field.label }}</label>
                <NInput v-model:value="noteDrafts[field.column]" type="textarea" :autosize="{ minRows: 3, maxRows: 8 }" :maxlength="10000" show-count
                  placeholder="填写备注，与任务状态一起同步到原文档" :disabled="statusBusy || busy || authorizing"
                  :input-props="{ id: `task-note-${field.column}`, 'aria-label': `编辑${field.label}` }" />
              </div>
              <div v-if="selected.row.status_edit" class="pending-actions"><span><template v-if="selected.row.status_edit.original !== selected.statusValue">原状态：{{ selected.row.status_edit.original || '空白' }} → {{ selected.status }}</template><template v-if="selected.row.status_edit.notes?.length">{{ selected.row.status_edit.original !== selected.statusValue ? ' · ' : '' }}{{ selected.row.status_edit.notes.length }} 项备注待回写</template></span><NButton size="small" :disabled="statusBusy || busy || authorizing" @click="changeStatus('revert')">撤销本地修改</NButton></div>
              <p v-if="statusError || selected.row.status_edit?.error" class="status-error" role="alert">{{ statusError || selected.row.status_edit?.error }}</p>
            </section>
            <TaskWorkEntry v-if="activeProject && activeProject.daily_report_enabled !== false" :key="`${active.key}:${selected.id}`" :document-key="active.key" :sheet="selected.sheet.name" :row="selected.row.number" :disabled="busy || statusBusy || authorizing" />
            <TaskExecutionPanel v-if="activeProject" :key="`execution:${active.key}:${selected.id}`" :document-key="active.key" :sheet="selected.sheet.name" :row="selected.row.number" :disabled="busy || statusBusy || authorizing" :launch="launchTaskExecution" :continue-launch="continueTaskExecution" />
            <section class="description-section"><h3><PhSquaresFour :size="16" />{{ selected.sheet.headers.some(h => text(h).includes('问题描述')) ? '问题描述' : '任务内容' }}</h3><p class="task-description">{{ selected.description || '该行未填写任务描述。' }}</p></section>
            <section v-if="selected.notes.length && !editableStatus" class="notes-section"><div v-for="(note, index) in selected.notes" :key="index"><span class="note-label">{{ note.label }}</span><p>{{ note.value }}</p></div></section>
            <section v-if="currentImages.length" class="images-section"><div class="section-heading"><h3><PhImage :size="17" />任务配图<span class="section-count">{{ currentImages.length }}</span></h3><span>点击图片放大</span></div><NImageGroup v-model:show="imagePreviewOpen" :render-toolbar="imageToolbar"><div class="image-grid"><figure v-for="(picture, index) in currentImages" :key="`${picture.file}:${index}`" class="picture-card"><NImage :src="imageURL(active.key, picture.file, 'preview')" :preview-src="imageURL(active.key, picture.file)" :alt="`${pictureCaption(picture.column)}，原表第 ${selected.row.number} 行`" object-fit="contain" :img-props="{ loading: 'lazy', draggable: false }"><template #error><div class="image-error"><PhImage :size="26" /><span>图片无法加载，请重新同步</span></div></template></NImage><figcaption><span><PhImage :size="13" />{{ pictureCaption(picture.column) }}</span><span>{{ String(index + 1).padStart(2, '0') }}</span></figcaption></figure></div></NImageGroup><p class="image-footnote"><PhCheckCircle :size="13" />原始图片已保存到本机</p></section>
            <section v-else class="no-images"><PhImage :size="17" /><span>这条任务没有配图</span></section>
            <section v-if="selected.fields.length" class="fields-section"><h3><PhSlidersHorizontal :size="16" />任务信息</h3><dl class="field-grid"><div v-for="(field, index) in selected.fields" :key="index"><dt>{{ field.label }}</dt><dd>{{ field.value }}</dd></div></dl></section>
            <NAlert v-if="warnings.length" title="读取提示" type="warning" :show-icon="false" class="read-warnings"><p v-for="(warning, index) in warnings" :key="index">{{ warning }}</p></NAlert>
            <footer class="detail-footer"><PhFileXls :size="14" /><span>{{ active.title }} · 原表第 {{ selected.row.number }} 行</span><span>{{ selectedIndex + 1 }} / {{ filtered.length }}</span></footer>
          </article>
        </template>
      </section>
    </NDrawer>

    <NModal v-model:show="showSettings" preset="card" :closable="!busy && !authorizing && !savingConditions" :close-on-esc="!busy && !authorizing && !savingConditions" :title="isNew ? '添加在线文档' : '文档设置'" :class="['settings-modal', { 'document-settings-modal': !isNew }]" :bordered="false" :mask-closable="!busy && !authorizing && !savingConditions" :auto-focus="false">
      <p class="settings-intro">{{ isNew ? '先识别工作表和表头，添加后在文档设置中配置拉取条件。' : '管理文档归属、拉取条件与远程回写授权。' }}</p>
      <NForm label-placement="top" @submit.prevent="submitSettings">
        <NFormItem label="所属项目"><NSelect v-model:value="draftProjectID" :options="projectOptions" :disabled="busy || statusBusy || authorizing || savingConditions" aria-label="所属项目" /></NFormItem>
        <NFormItem v-if="isNew || draftURL" label="在线文档链接"><NInput v-model:value="draftURL" :readonly="!isNew" placeholder="https://docs.qq.com/sheet/…" :clearable="isNew" :input-props="{ 'aria-label': '在线文档链接' }" :disabled="busy || statusBusy || authorizing || savingConditions" /></NFormItem>
        <DocumentPullSettings v-if="!isNew && editingDocument" v-model="draftPullConditions" :document="editingDocument" :disabled="busy || statusBusy || authorizing || savingConditions" :pending-edits="editingHasPendingChanges" />
        <div v-if="!isNew" class="remote-authorization"><div><strong>远程状态与备注回写</strong><p>{{ remoteAuth.authorized ? '已授权 · 可将本地状态与备注同步到原表' : '使用有编辑权限的账号完成授权后，即可回写任务状态与备注' }}</p></div><NButton size="small" :loading="authorizing" :disabled="busy || statusBusy || savingConditions" @click="authorizeRemote">{{ remoteAuth.authorized ? '重新授权' : '授权腾讯文档' }}</NButton><NButton v-if="authorizing" size="small" @click="cancel">取消授权</NButton></div>
        <NAlert v-if="formError" type="error" :show-icon="false" class="form-error">{{ formError }}</NAlert>
        <div class="settings-footer"><NButton v-if="isNew" quaternary :disabled="busy || statusBusy || authorizing || savingConditions" @click="importExcel"><template #icon><PhFileXls :size="17" /></template>导入 Excel</NButton><div><NButton :disabled="busy || statusBusy || authorizing || savingConditions" @click="showSettings = false">返回</NButton><NButton type="primary" :loading="savingConditions" :disabled="busy || statusBusy || authorizing || savingConditions" attr-type="submit">{{ isNew ? '添加并识别表头' : '保存文档设置' }}</NButton></div></div>
      </NForm>
    </NModal>
    <SettingsDialog v-if="showAppSettings" :projects="projects" :documents="documents" :schedules="schedules" :runningID="runningScheduleID" :blocked="scheduleBlocked" :load-error="scheduleLoadError" @close="leaveSettings" @lock="settingsEditingLocked = $event" @projects-updated="projects = $event" @updated="schedulesUpdated" @refresh="loadSchedules" @run="runScheduleNow" />
    <ProjectSettingsPanel v-if="showProjectSettings" :project="editingProject" @close="showProjectSettings = false" @saved="projectSaved" />
  </NConfigProvider>
</template>
