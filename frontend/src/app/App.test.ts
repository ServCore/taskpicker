// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import App from './App.vue'
import SettingsDialog from '../features/settings/SettingsDialog.vue'
import { NButton, NDrawer, NImage, NImageGroup, NModal, NSelect, NSwitch } from 'naive-ui'
import type { Document, Project, SyncSchedule, TaskExecution } from '../shared/types/types'

const first: Document = {
  key: 'a'.repeat(24), version: 1, title: '示例应用 任务清单', owner: '王小明', cached: true,
  source_url: 'https://docs.qq.com/sheet/example-sheet?tab=ddczrm', source_path: '',
  updated_at: '2026-10-15T16:00:00', warnings: [],
  sheets: [{ name: '示例测试问题清单', header_row: 1, scanned: 109,
    headers: ['三级功能', '问题描述', '责任人', '处理状态'], rows: [
      { number: 4, values: ['麦克风按钮', '无法访问麦克风\n保留完整描述', '王小明', '未处理'], pictures: [
        { row: 4, column: 5, file: 'a'.repeat(64) + '.png', width: 400, height: 150 },
      ] },
      { number: 100, values: ['部门管理', '可以重复添加成员', '王小明', '已修复'], pictures: [] },
    ] }],
}
const second: Document = {
  ...first, key: 'b'.repeat(24), title: '示例项目（数智）',
  source_url: 'https://docs.qq.com/sheet/example-smart-sheet?tab=ss_mo1bbr',
  sheets: [{ name: '业务侧', header_row: 1, scanned: 134,
    headers: ['功能', '描述', '负责人员', '完成状态'],
    rows: [{ number: 60, values: ['资源市场', '优化资源市场审批逻辑', '王小明', '已完成'], pictures: [] }],
  }],
}
const defaultProject = { id: 'c'.repeat(24), name: 'yiqiban', local_path: '', document_keys: [first.key, second.key], daily_report_enabled: true }
let wrapper: ReturnType<typeof mount> | undefined
async function openApp(configure?: () => void, realTeleport = false) {
  window.localStorage.clear()
  window.go = { main: { App: {
    PrepareOnlineDocument: vi.fn(),
    SaveDocumentFilters: vi.fn().mockImplementation(async (key, conditions) => ({ ...structuredClone(key === first.key ? first : second), pull_conditions: conditions })),
    GetSyncSchedules: vi.fn().mockResolvedValue([]), SaveSyncSchedule: vi.fn(), DeleteSyncSchedule: vi.fn(), CompleteSyncSchedule: vi.fn(),
    GetActiveTaskExecution: vi.fn().mockResolvedValue(null),
    GetTaskExecutionContext: vi.fn().mockResolvedValue({ engine: 'Codex', available: true, repositories: [], notice: '请绑定代码仓库' }),
    GetTaskExecutionReport: vi.fn().mockResolvedValue({ markdown: '# 执行记录', path: '/report.md' }), OpenTaskExecutionReport: vi.fn().mockResolvedValue(undefined),
    ContinueTaskExecution: vi.fn(), GetTaskExecutionProgress: vi.fn().mockResolvedValue({ run_id: '', root_id: '', status: '', markdown: '', path: '' }), OpenTaskExecutionProgress: vi.fn(),
    GetTaskExecutions: vi.fn().mockResolvedValue([]), GetTaskExecution: vi.fn(), StartTaskExecution: vi.fn(), StopTaskExecution: vi.fn(), OpenTaskExecutionWorkspace: vi.fn(),
    OpenProjectDirectoryAt: vi.fn(),
    SaveDocumentPullConditions: vi.fn().mockImplementation(async (key, owner, include_completed) => ({ ...structuredClone(key === first.key ? first : second), pull_conditions: { owner, include_completed } })),
    SyncConfiguredDocument: vi.fn().mockRejectedValue('连接失败'),
    GetProjectDailyReportSettings: vi.fn().mockImplementation(() => window.go!.main.App.GetDailyAISettings()),
    SaveProjectDailyReport: vi.fn(), GenerateProjectDailyReport: vi.fn(), ContinueProjectDailyReport: vi.fn(), SaveProjectDailyReportPrompt: vi.fn(),
    SaveProject: vi.fn(), SetProjectEnabled: vi.fn(), AssignDocumentProject: vi.fn().mockResolvedValue([defaultProject]), PickProjectDirectory: vi.fn(), OpenProjectDirectory: vi.fn(),
    RecordTaskWork: vi.fn(), GetDailyReport: vi.fn(), GetProjectDailyReport: vi.fn(), SaveDailyReport: vi.fn(), GenerateDailyReport: vi.fn(),
    ContinueDailyReport: vi.fn(), CancelDailyReport: vi.fn(), SaveDailyReportPrompt: vi.fn(), GetDailyAISettings: vi.fn().mockResolvedValue({ base_url: '', model: '', prompt: '', has_api_key: false, default_prompt: '' }), SaveDailyAISettings: vi.fn(),
    GetResourceUsage: vi.fn().mockResolvedValue({ cpu_percent: 0.5, memory_bytes: 64000000 }),
    GetRemoteAuth: vi.fn().mockResolvedValue({ authorized: true, expires_at: 1900000000 }),
    AuthorizeRemote: vi.fn().mockResolvedValue({ authorized: true, expires_at: 1900000000 }),
    UpdateTaskDetails: vi.fn(), UpdateTaskStatus: vi.fn(), RevertTaskStatus: vi.fn(), PushTaskStatus: vi.fn(),
    GetLibrary: vi.fn().mockResolvedValue({ documents: structuredClone([first, second]), selected_key: first.key, warnings: [], projects: [defaultProject] }),
    SelectDocument: vi.fn().mockResolvedValue(undefined),
    SyncDocumentOnStartup: vi.fn().mockImplementation(async (url: string) => structuredClone(url === first.source_url ? first : second)),
    SyncDocument: vi.fn().mockRejectedValue('连接失败'),
    ImportExcel: vi.fn().mockResolvedValue(null), ReimportDocument: vi.fn(),
    CancelSync: vi.fn(), CopyText: vi.fn().mockResolvedValue(undefined), OpenSource: vi.fn(),
  } } }
  window.runtime = { EventsOnMultiple: () => () => {} }
  configure?.()
  wrapper = mount(App, { attachTo: document.body, global: { stubs: { teleport: !realTeleport } } })
  await flushPromises()
  return wrapper
}
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = ''; delete window.go; delete window.runtime; vi.useRealTimers() })

describe('desktop reader flows', () => {
  it('hides disabled projects without moving their documents into ungrouped', async () => {
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: structuredClone([first, second]), selected_key: second.key, warnings: [], projects: [{ ...defaultProject, enabled: false, document_keys: [first.key] }] })
    })
    expect(app.find('[aria-label="yiqiban 项目设置"]').exists()).toBe(false)
    expect(app.find('[aria-label="yiqiban 工作日报"]').exists()).toBe(false)
    expect(app.findAll('.project-group')).toHaveLength(1)
    expect(app.find('.project-name').text()).toBe('未分组')
    expect(app.findAll('.document-link')).toHaveLength(1)
    expect(app.find('.document-link').text()).toContain('示例项目')
  })
  it('toggles projects in settings, retains failed changes, and restores sidebar entries', async () => {
    const app = await openApp(undefined, true)
    const modal = new DOMWrapper(document.body)
    await app.find('.app-settings-link').trigger('click'); await vi.dynamicImportSettled(); await flushPromises()
    await modal.findAll('.settings-sidebar nav button').find(button => button.text() === '项目管理')!.trigger('click'); await flushPromises()
    const toggle = () => app.findComponent(SettingsDialog).findComponent(NSwitch)
    expect(toggle().props('value')).toBe(true)
    vi.mocked(window.go!.main.App.SetProjectEnabled).mockRejectedValueOnce('无法保存项目设置')
    toggle().vm.$emit('update:value', false); await flushPromises()
    expect(modal.find('.projects-settings-page [role="alert"]').text()).toContain('无法保存项目设置')
    expect(toggle().props('value')).toBe(true)
    expect(app.findAll('.project-group')).toHaveLength(1)
    let finish!: (projects: Project[]) => void
    vi.mocked(window.go!.main.App.SetProjectEnabled).mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    toggle().vm.$emit('update:value', false); await flushPromises()
    expect(modal.find('[aria-label="关闭软件设置"]').attributes('disabled')).toBeDefined()
    finish([{ ...defaultProject, enabled: false }]); await flushPromises()
    expect(window.go!.main.App.SetProjectEnabled).toHaveBeenLastCalledWith(defaultProject.id, false)
    expect(app.findAll('.project-group')).toHaveLength(0)
    expect(toggle().props('value')).toBe(false)
    expect(modal.find('.projects-settings-page').text()).toContain('yiqiban')
    vi.mocked(window.go!.main.App.SetProjectEnabled).mockResolvedValueOnce([{ ...defaultProject, enabled: true }])
    toggle().vm.$emit('update:value', true); await flushPromises()
    expect(app.findAll('.project-group')).toHaveLength(1)
    expect(app.findAll('.document-link')).toHaveLength(2)
    expect(app.find('[aria-label="yiqiban 工作日报"]').exists()).toBe(true)
    expect(toggle().props('value')).toBe(true)
    expect(modal.find('[aria-label="关闭软件设置"]').attributes('disabled')).toBeUndefined()
  })
  it('keeps tracking a manual AI execution when its task drawer is left before launch returns', async () => {
    let finish!: (run: TaskExecution) => void
    const run: TaskExecution = { id: 'd'.repeat(24), task_id: 'e'.repeat(24), project_id: defaultProject.id, document_key: first.key, sheet: first.sheets[0].name, row: 4, task: '麦克风按钮', instructions: '', repository: '/project/backend', workspace: '/executions/workspace', branch: 'taskpicker/test', status: 'running', started_at: '2026-10-16T16:00:00+08:00', finished_at: '', result: '', error: '', changed_files: '', log_count: 0 }
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetTaskExecutionContext).mockResolvedValue({ engine: 'Codex', available: true, repositories: [{ path: run.repository, codegraph: true }], notice: '' })
      vi.mocked(window.go!.main.App.StartTaskExecution).mockImplementation(() => new Promise(resolve => { finish = resolve }))
      vi.mocked(window.go!.main.App.GetTaskExecution).mockResolvedValue({ run, logs: [], next: 0 })
    })
    await app.find('.task-item').trigger('click')
    await vi.waitFor(() => expect(app.find('.task-execution').exists()).toBe(true)); await flushPromises()
    await app.findAllComponents(NButton).find(button => button.text() === '启动执行')!.trigger('click'); await flushPromises()
    expect(window.go!.main.App.StartTaskExecution).toHaveBeenCalledWith(first.key, first.sheets[0].name, 4, run.repository, '')
    await app.findAll('.document-link')[1].trigger('click'); await flushPromises()
    finish(run); await flushPromises()
    expect(app.find('.footer-execution').text()).toContain('AI 执行中 · #4')
    expect(app.find('main.workspace').attributes('aria-label')).toBe(second.title)
    expect(window.go!.main.App.StopTaskExecution).not.toHaveBeenCalled()
    await app.find('.footer-execution').trigger('click'); await flushPromises()
    expect(app.find('main.workspace').attributes('aria-label')).toBe(first.title)
    expect(app.find('[aria-label="任务详情内容"]').text()).toContain('麦克风按钮')
  })
  it('keeps tracking a continuation after its task drawer is left before launch returns', async () => {
    let finish!: (run: TaskExecution) => void
    const previous: TaskExecution = { id: 'd'.repeat(24), task_id: 'e'.repeat(24), project_id: defaultProject.id, document_key: first.key, sheet: first.sheets[0].name, row: 4, task: '麦克风按钮', instructions: '', repository: '/project/backend', workspace: '/executions/workspace', branch: 'taskpicker/test', status: 'succeeded', started_at: '2026-10-16T16:00:00+08:00', finished_at: '', result: '', error: '', changed_files: '', log_count: 0 }
    const continued = { ...previous, id: 'f'.repeat(24), previous_id: previous.id, root_id: previous.id, status: 'running' as const }
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetTaskExecutionContext).mockResolvedValue({ engine: 'Codex', available: true, repositories: [{ path: previous.repository, codegraph: true }], notice: '' })
      vi.mocked(window.go!.main.App.GetTaskExecutions).mockResolvedValue([previous])
      vi.mocked(window.go!.main.App.ContinueTaskExecution).mockImplementation(() => new Promise(resolve => { finish = resolve }))
      vi.mocked(window.go!.main.App.GetTaskExecution).mockResolvedValue({ run: continued, logs: [], next: 0 })
    })
    await app.find('.task-item').trigger('click')
    await vi.waitFor(() => expect(app.find('.task-execution').exists()).toBe(true)); await flushPromises()
    await app.findAllComponents(NButton).find(button => button.text() === '继续处理')!.trigger('click'); await flushPromises()
    expect(window.go!.main.App.ContinueTaskExecution).toHaveBeenCalledWith(first.key, first.sheets[0].name, 4, previous.id, '')
    await app.findAll('.document-link')[1].trigger('click'); await flushPromises()
    finish(continued); await flushPromises()
    expect(app.find('.footer-execution').text()).toContain('AI 执行中 · #4')
    expect(window.go!.main.App.StopTaskExecution).not.toHaveBeenCalled()
  })
  it('groups documents under projects, keeps the grouping after startup sync and opens bound directories', async () => {
    const projects = [{ id: 'c'.repeat(24), name: 'yiqiban', local_path: '/local/yiqiban', document_keys: [first.key, second.key] }]
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: structuredClone([first, second]), selected_key: first.key, warnings: [], projects })
      vi.mocked(window.go!.main.App.OpenProjectDirectory).mockResolvedValue(undefined)
    })
    expect(app.findAll('.project-group')).toHaveLength(1)
    expect(app.find('.project-group').findAll('.document-link')).toHaveLength(2)
    expect(app.find('.project-name span').text()).toBe('yiqiban')
    await app.find('[aria-label="打开 yiqiban 本地目录"]').trigger('click'); await flushPromises()
    expect(window.go!.main.App.OpenProjectDirectory).toHaveBeenCalledWith(projects[0].id)
    await app.find('[aria-label="yiqiban 项目设置"]').trigger('click')
    await vi.waitFor(() => expect(app.find('[aria-label="项目设置"]').exists()).toBe(true))
    await flushPromises()
    expect(app.find<HTMLInputElement>('input[aria-label="项目名称"]').element.value).toBe('yiqiban')
    await app.find('[aria-label="返回项目列表"]').trigger('click'); await flushPromises()
    expect(window.go!.main.App.SaveProject).not.toHaveBeenCalled()
  })
  it('opens and saves reports within each project and hides reports for disabled projects', async () => {
    const secondProject = { ...defaultProject, id: 'e'.repeat(24), name: '项目二', document_keys: [second.key] }
    const disabledProject = { ...defaultProject, id: 'f'.repeat(24), name: '项目三', document_keys: [], daily_report_enabled: false }
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: structuredClone([first, second]), selected_key: first.key, warnings: [], projects: [{ ...defaultProject, document_keys: [first.key] }, secondProject, disabledProject] })
      vi.mocked(window.go!.main.App.GetProjectDailyReport).mockImplementation(async (id, date) => ({ date, activities: [], draft: { text: `${id} 的日报`, source: 'manual', generated_at: '', saved_at: 'saved', activity_count: 0 } }))
      vi.mocked(window.go!.main.App.SaveProjectDailyReport).mockImplementation(async (_id, _date, value) => ({ text: value, source: 'manual', generated_at: '', saved_at: 'saved', activity_count: 0 }))
      vi.mocked(window.go!.main.App.GetDailyAISettings).mockResolvedValue({ base_url: '', model: '', prompt: '项目提示词', has_api_key: false, default_prompt: '默认' })
    })
    expect(app.findAll('.daily-report-link')).toHaveLength(2)
    expect(app.find('[aria-label="项目三 工作日报"]').exists()).toBe(false)
    await app.find('[aria-label="yiqiban 工作日报"]').trigger('click')
    await vi.waitFor(() => expect(app.find('.daily-body textarea').exists()).toBe(true)); await flushPromises()
    expect(app.find('.daily-header').text()).toContain('yiqiban')
    await app.find('.daily-body textarea').setValue('只保存到 yiqiban')
    await app.find('[aria-label="项目二 工作日报"]').trigger('click'); await flushPromises()
    expect(window.go!.main.App.SaveProjectDailyReport).toHaveBeenCalledWith(defaultProject.id, expect.any(String), '只保存到 yiqiban')
    expect(window.go!.main.App.GetProjectDailyReport).toHaveBeenLastCalledWith(secondProject.id, expect.any(String))
    expect(app.find('.daily-header').text()).toContain('项目二')
    expect(app.find<HTMLTextAreaElement>('.daily-body textarea').element.value).toBe(`${secondProject.id} 的日报`)
    expect(window.go!.main.App.GetDailyReport).not.toHaveBeenCalled()
    expect(window.go!.main.App.SaveDailyReport).not.toHaveBeenCalled()
  })
  it('keeps report edits on failed navigation saves and saves before opening AI settings', async () => {
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetProjectDailyReport).mockImplementation(async (_id, date) => ({ date, activities: [], draft: { text: '已有日报', source: 'manual', generated_at: '', saved_at: 'saved', activity_count: 0 } }))
      vi.mocked(window.go!.main.App.SaveProjectDailyReport).mockImplementation(async (_id, _date, text) => ({ text, source: 'manual', generated_at: '', saved_at: 'saved', activity_count: 0 }))
      vi.mocked(window.go!.main.App.GetDailyAISettings).mockResolvedValue({ base_url: '', model: '', prompt: '提示词', has_api_key: false, default_prompt: '默认' })
    }, true)
    const modal = new DOMWrapper(document.body)
    await app.find('.daily-report-link').trigger('click')
    await vi.waitFor(() => expect(app.find('.daily-body textarea').exists()).toBe(true)); await flushPromises()
    await app.find('.daily-body textarea').setValue('刚完成的工作')
    vi.mocked(window.go!.main.App.SaveProjectDailyReport).mockRejectedValueOnce('磁盘写入失败')
    await app.find('.document-link').trigger('click'); await flushPromises()
    expect(app.find('.daily-error').text()).toContain('磁盘写入失败')
    expect(app.find<HTMLTextAreaElement>('.daily-body textarea').element.value).toBe('刚完成的工作')
    expect(app.find('.daily-report-link').attributes('aria-current')).toBe('page')
    const promptButton = app.findAllComponents(NButton).find(button => button.text() === '日报提示词')!
    await promptButton.trigger('click'); await flushPromises()
    await app.find('[aria-label="自定义日报提示词"] textarea').setValue('未保存提示词')
    expect(app.find('.document-link').attributes('disabled')).toBeDefined()
    await app.find('.app-settings-link').trigger('click'); await flushPromises()
    expect(modal.find('.ai-settings-page').exists()).toBe(false)
    await app.findAllComponents(NButton).find(button => button.text() === '返回')!.trigger('click'); await flushPromises()
    await app.find('.app-settings-link').trigger('click')
    await vi.waitFor(() => expect(modal.find('.ai-settings-page').exists()).toBe(true)); await flushPromises()
    expect(window.go!.main.App.SaveProjectDailyReport).toHaveBeenLastCalledWith(defaultProject.id, expect.any(String), '刚完成的工作')
    expect(app.find('.daily-panel').exists()).toBe(true)
    await modal.find('[aria-label="关闭软件设置"]').trigger('click'); await flushPromises()
    expect(modal.find('.software-settings-modal').exists()).toBe(false)
    expect(app.find<HTMLTextAreaElement>('.daily-body textarea').element.value).toBe('刚完成的工作')
  })
  it('edits conditions in document settings, retains failed drafts and only pulls on sync', async () => {
    const app = await openApp(undefined, true)
    const modal = new DOMWrapper(document.body)
    await app.findAll('.document-link')[1].trigger('click'); await flushPromises()
    expect(app.find('.pull-conditions-trigger').exists()).toBe(false)
    await app.find('[aria-label="当前文档设置"]').trigger('click'); await flushPromises()
    const input = modal.find('input[aria-label="条件 1 内容"]')
    await input.setValue('张三'); await input.trigger('blur'); await flushPromises()
    expect(window.go!.main.App.SaveDocumentFilters).not.toHaveBeenCalled()
    const save = () => app.findAllComponents(NButton).find(button => button.text() === '保存文档设置')!
    vi.mocked(window.go!.main.App.SaveDocumentFilters).mockRejectedValueOnce('磁盘写入失败')
    await save().trigger('click'); await flushPromises()
    expect(modal.find('.form-error').text()).toContain('磁盘写入失败')
    expect(modal.find<HTMLInputElement>('input[aria-label="条件 1 内容"]').element.value).toBe('张三')
    expect(window.go!.main.App.AssignDocumentProject).not.toHaveBeenCalled()
    await save().trigger('click'); await flushPromises()
    expect(window.go!.main.App.SaveDocumentFilters).toHaveBeenLastCalledWith(second.key, { owner: '', include_completed: true, filters: [{ sheet: second.sheets[0].name, column: 3, header: '负责人员', operator: 'member', value: '张三' }] })
    expect(window.go!.main.App.SyncConfiguredDocument).not.toHaveBeenCalled()
    expect(app.find('.pull-summary').text()).toContain('张三')
    expect(app.find('.pull-pending').text()).toContain('待同步')
    await app.find('.document-actions .n-button--primary-type').trigger('click'); await flushPromises()
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledWith(second.key, true)
  })
  it('validates conditions on save and discards a draft when returning from document settings', async () => {
    const app = await openApp(undefined, true)
    const modal = new DOMWrapper(document.body)
    await app.find('[aria-label="当前文档设置"]').trigger('click'); await flushPromises()
    await modal.find('input[aria-label="条件 1 内容"]').setValue('')
    await app.findAllComponents(NButton).find(button => button.text() === '保存文档设置')!.trigger('click'); await flushPromises()
    expect(modal.find('.form-error').text()).toContain('请填写匹配内容')
    expect(window.go!.main.App.SaveDocumentFilters).not.toHaveBeenCalled()
    await app.findAllComponents(NButton).find(button => button.text() === '返回')!.trigger('click'); await flushPromises()
    await app.find('[aria-label="当前文档设置"]').trigger('click'); await flushPromises()
    expect(modal.find<HTMLInputElement>('input[aria-label="条件 1 内容"]').element.value).toBe(first.owner)
    await app.findAllComponents(NButton).find(button => button.text() === '保存文档设置')!.trigger('click'); await flushPromises()
    expect(window.go!.main.App.SaveDocumentFilters).not.toHaveBeenCalled()
    expect(window.go!.main.App.AssignDocumentProject).toHaveBeenCalledWith(first.key, defaultProject.id)
  })
  it('assigns newly synced documents to the selected project and keeps them visible if assignment fails', async () => {
    const project = { id: 'c'.repeat(24), name: 'yiqiban', local_path: '', document_keys: [first.key, second.key] }
    const added = { ...structuredClone(first), key: 'd'.repeat(24), title: '新增文档', source_url: 'https://docs.qq.com/sheet/newdoc' }
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: structuredClone([first, second]), selected_key: first.key, warnings: [], projects: [project] })
      vi.mocked(window.go!.main.App.PrepareOnlineDocument).mockResolvedValue({ ...added, setup_pending: true, cached: false })
      vi.mocked(window.go!.main.App.AssignDocumentProject).mockRejectedValue('保存失败')
    })
    await app.find('[aria-label="添加在线文档"]').trigger('click'); await flushPromises()
    expect(app.findComponent(NSelect).exists()).toBe(true)
    const input = app.findAll('input').find(item => item.attributes('placeholder') === 'https://docs.qq.com/sheet/…')!
    await input.setValue(added.source_url)
    const submit = app.findAllComponents(NButton).find(item => item.text() === '添加并识别表头')!
    await submit.trigger('click'); await flushPromises()
    expect(window.go!.main.App.AssignDocumentProject).toHaveBeenCalledWith(added.key, project.id)
    expect(app.findAll('.document-link')).toHaveLength(3)
    expect(app.find('.error-banner').text()).toContain('文档已保存，但项目归属未保存')
  })
  it('shows all original urgency and priority values in list and details and adapts to sheets without these fields', async () => {
    const doc = structuredClone(first)
    doc.sheets[0].headers.push('紧急程度', '优先级')
    doc.sheets[0].rows[0].values.push('紧急', 'P1')
    doc.sheets[0].rows[1].values.push('', '')
    const app = await openApp(() => {
      window.localStorage.setItem('include-completed', 'true')
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [doc, second], selected_key: doc.key, warnings: [] })
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockImplementation(async url => structuredClone(url === doc.source_url ? doc : second))
    })
    expect(app.find('.task-table-heading.has-priority').text()).toContain('紧急 / 优先级')
    const rows = app.findAll('.task-item')
    expect(rows[0].find('.task-priority .priority-badge.urgent').text()).toBe('紧急程度：紧急')
    expect(rows[0].find('.task-priority .priority-badge.high').text()).toBe('优先级：P1')
    expect(rows[1].find('.task-priority').text()).toBe('—')
    await rows[0].trigger('click'); await flushPromises()
    expect(app.find('.detail-badges [aria-label="紧急程度：紧急"]').exists()).toBe(true)
    expect(app.find('.detail-badges [aria-label="优先级：P1"]').exists()).toBe(true)
    await app.find('[aria-label="复制任务文字"]').trigger('click'); await flushPromises()
    expect(window.go!.main.App.CopyText).toHaveBeenCalledWith(expect.stringContaining('优先级：P1'))
    await app.find('[aria-label="关闭任务详情"]').trigger('click')
    await app.findAll('.document-link')[1].trigger('click'); await flushPromises()
    expect(app.find('.task-table-heading.has-priority').exists()).toBe(false)
    expect(app.find('.task-priority').exists()).toBe(false)
  })
  it('opens the daily report from the sidebar without generating or sending task data', async () => {
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetProjectDailyReport).mockResolvedValue({ date: '2026-10-16', activities: [], draft: { text: '', source: '', generated_at: '', saved_at: '', activity_count: 0 } })
      vi.mocked(window.go!.main.App.GetDailyAISettings).mockResolvedValue({ base_url: '', model: '', prompt: '整理日报', has_api_key: false, default_prompt: '整理日报' })
    })
    expect(window.go!.main.App.GetProjectDailyReport).not.toHaveBeenCalled()
    await app.find('.daily-report-link').trigger('click')
    await vi.waitFor(() => expect(app.find('[aria-label="工作日报"]').exists()).toBe(true))
    await flushPromises()
    expect(app.find('.daily-empty').text()).toContain('这一天还没有记录')
    expect(window.go!.main.App.GenerateDailyReport).not.toHaveBeenCalled()
    expect(app.find('main[aria-label="工作日报"]').exists()).toBe(true)
    expect(app.find('[aria-label="工作日报"]').attributes('aria-modal')).toBeUndefined()
    expect(app.find('.daily-report-link').attributes('aria-current')).toBe('page')
    expect(app.find('.workspace:not(.daily-panel)').isVisible()).toBe(false)
    await app.find('.document-link').trigger('click'); await flushPromises()
    expect(app.find('[aria-label="工作日报"]').exists()).toBe(false)
  })
  it('opens independent AI settings, guards unsaved navigation, and loads refreshed configuration in reports', async () => {
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetProjectDailyReport).mockImplementation(async (_id, date) => ({ date, activities: [], draft: { text: '', source: '', generated_at: '', saved_at: '', activity_count: 0 } }))
      vi.mocked(window.go!.main.App.GetDailyAISettings).mockResolvedValue({ base_url: 'https://ai.example/v1', model: 'old-model', prompt: '整理日报', has_api_key: true, default_prompt: '默认' })
    }, true)
    const modal = new DOMWrapper(document.body)
    expect(app.find('.sidebar .ai-settings-link').exists()).toBe(false)
    expect(app.find('.sidebar .schedules-link').exists()).toBe(false)
    expect(app.find('.app-settings-link strong').text()).toBe('设置')
    await app.find('.app-settings-link').trigger('click')
    await vi.waitFor(() => expect(modal.find('.ai-settings-page').exists()).toBe(true)); await flushPromises()
    expect(modal.find('.settings-sidebar h2').text()).toBe('设置')
    expect(modal.find('.software-settings-modal').attributes('aria-modal')).toBe('true')
    expect(app.find('.workspace:not(.ai-settings-page)').isVisible()).toBe(true)
    expect(modal.find('.settings-sidebar [aria-current="page"]').text()).toBe('AI 设置')
    expect(window.go!.main.App.GetDailyAISettings).toHaveBeenCalledTimes(1)
    expect(app.find('.app-settings-link').attributes('aria-expanded')).toBe('true')
    expect(app.find('.document-link').attributes('aria-current')).toBe('page')
    await modal.find('[aria-label="AI 模型名称"] input').setValue('new-model')
    expect(modal.find('[aria-label="关闭软件设置"]').attributes('disabled')).toBeDefined()
    const settingsModal = app.findComponent(SettingsDialog).findComponent(NModal)
    expect(settingsModal.props('maskClosable')).toBe(false)
    expect(settingsModal.props('closeOnEsc')).toBe(false)
    settingsModal.vm.$emit('update:show', false); await flushPromises()
    expect(modal.find('.ai-settings-page').exists()).toBe(true)
    await modal.find('[aria-label="关闭软件设置"]').trigger('click'); await flushPromises()
    expect(modal.find('.ai-settings-page').exists()).toBe(true)
    await modal.findAll('.settings-sidebar nav button')[1].trigger('click'); await flushPromises()
    expect(modal.find('.ai-settings-page').exists()).toBe(true)
    expect(modal.find('.schedules-page').exists()).toBe(false)
    expect(app.find('.document-link').attributes('disabled')).toBeDefined()
    expect(app.find('.daily-report-link').attributes('disabled')).toBeDefined()
    const discard = app.findAllComponents(NButton).find(item => item.text() === '放弃修改')!
    await discard.trigger('click'); await flushPromises()
    await modal.findAll('.settings-sidebar nav button')[1].trigger('click'); await vi.dynamicImportSettled(); await flushPromises()
    expect(modal.find('.schedules-page').exists()).toBe(true)
    expect(modal.find('.ai-settings-page').exists()).toBe(false)
    await modal.find('[aria-label="关闭软件设置"]').trigger('click'); await flushPromises()
    expect(modal.find('.software-settings-modal').exists()).toBe(false)
    await app.find('.document-link').trigger('click'); await flushPromises()
    expect(modal.find('.ai-settings-page').exists()).toBe(false)
    vi.mocked(window.go!.main.App.GetDailyAISettings).mockResolvedValue({ base_url: 'https://ai.example/v1', model: 'new-model', prompt: '整理日报', has_api_key: true, default_prompt: '默认' })
    await app.find('.daily-report-link').trigger('click')
    await vi.waitFor(() => expect(app.find('#daily-date').exists()).toBe(true))
    await flushPromises()
    expect(app.find('.daily-editor').text()).toContain('AI：new-model')
    expect(app.findAllComponents(NButton).some(item => item.text() === 'AI 设置')).toBe(false)
    expect(window.go!.main.App.GenerateDailyReport).not.toHaveBeenCalled()
  })
  it('starts with the incomplete task list and opens full details only after a click', async () => {
    const app = await openApp()
    expect(app.findAll('.task-item')).toHaveLength(1)
    expect(app.find('.task-description').exists()).toBe(false)
    expect(app.findComponent(NDrawer).props('show')).toBe(false)
    await app.find('.task-item').trigger('click')
    await flushPromises()
    expect(app.findComponent(NDrawer).props('show')).toBe(true)
    expect(app.find('.task-description').text()).toBe('无法访问麦克风\n保留完整描述')
    expect(app.find('.picture-card img').attributes('src')).toContain('/local-image/' + first.key + '/')
    expect(app.find('.task-thumbnail').attributes('src')).toContain('?size=thumbnail')
    expect(app.find('.picture-card img').attributes('src')).toContain('?size=preview')
    expect(app.findComponent(NImage).props('previewSrc')).not.toContain('?size=')
    await app.find('[aria-label="关闭任务详情"]').trigger('click')
    expect(app.findComponent(NDrawer).props('show')).toBe(false)
  })
  it('searches and filters without opening details or reading the network', async () => {
    const app = await openApp()
    expect(app.find('[aria-label="文档路径"]').exists()).toBe(false)
    expect(app.find('input[placeholder="搜索任务内容…"]').exists()).toBe(false)
    if (!app.find('input[placeholder="搜索任务内容…"]').exists()) await app.find('[aria-label="搜索任务"]').trigger('click')
    await app.find('input[placeholder="搜索任务内容…"]').setValue('麦克风')
    await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(1)
    expect(app.findComponent(NDrawer).props('show')).toBe(false)
    if (!app.find('input[placeholder="搜索任务内容…"]').exists()) await app.find('[aria-label="搜索任务"]').trigger('click')
    await app.find('input[placeholder="搜索任务内容…"]').setValue('不存在的内容')
    await flushPromises()
    expect(app.find('.list-empty').text()).toContain('没有符合条件的任务')
    await app.find('[aria-label="收起搜索"]').trigger('click'); await flushPromises()
    expect(app.find('input[placeholder="搜索任务内容…"]').exists()).toBe(false)
    expect(app.findAll('.task-item')).toHaveLength(1)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', metaKey: true, bubbles: true, cancelable: true }))
    await flushPromises()
    const input = app.find<HTMLInputElement>('input[placeholder="搜索任务内容…"]')
    expect(document.activeElement).toBe(input.element)
    await input.setValue('麦克风')
    await input.trigger('keydown', { key: 'Escape' }); await flushPromises()
    expect(app.find('input[placeholder="搜索任务内容…"]').exists()).toBe(false)
    expect(document.activeElement).toBe(app.find('[aria-label="搜索任务"]').element)
    expect(window.go!.main.App.SyncDocument).not.toHaveBeenCalled()
  })
  it('can show completed cached tasks explicitly and closes details on document changes', async () => {
    const app = await openApp()
    app.findComponent(NSelect).vm.$emit('update:value', [])
    await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(2)
    await app.findAll('.task-item')[1].trigger('click')
    await flushPromises()
    expect(app.find('.task-description').text()).toBe('可以重复添加成员')
    await app.findAll('.document-link')[1].trigger('click')
    await flushPromises()
    expect(app.findComponent(NDrawer).props('show')).toBe(false)
    expect(app.findAll('.task-item')).toHaveLength(0)
    expect(app.find('.list-empty').text()).toContain('暂无未完成任务')
  })
  it('syncs incomplete tasks by default and keeps cached content when sync fails', async () => {
    const app = await openApp()
    await app.find('.document-actions .n-button--primary-type').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledWith(first.key, true)
    expect(app.find('.error-banner').text()).toContain('连接失败')
    expect(app.find('.error-banner').text()).toContain('本地内容已保留')
    expect(app.findAll('.task-item')).toHaveLength(1)
    expect(app.findComponent(NDrawer).props('show')).toBe(false)
    await app.find('.task-item').trigger('click')
    await flushPromises()
    expect(app.find('.task-description').text()).toContain('无法访问麦克风')
  })
  it('pulls all matching statuses despite legacy completion flags and keeps local filtering separate', async () => {
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [{ ...structuredClone(first), include_completed: true }, { ...structuredClone(second), include_completed: false }], selected_key: first.key, warnings: [], projects: [defaultProject] })
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockImplementation(async url => ({ ...structuredClone(url === first.source_url ? first : second), include_completed: url === first.source_url }))
    })
    expect(app.find('[aria-label="包含已完成任务"]').exists()).toBe(false)
    expect(app.find('.document-toolbar').text()).not.toContain('包含已完成任务')
    expect(app.find('.filter-line').findComponent(NSelect).props('value')).toEqual(['unfinished'])
    expect(app.findAll('.task-item')).toHaveLength(1)
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledWith(first.source_url, first.owner, true)
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledWith(second.source_url, second.owner, true)
    expect(window.go!.main.App.SaveDocumentFilters).not.toHaveBeenCalled()
    await app.find('.document-actions .n-button--primary-type').trigger('click'); await flushPromises()
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledWith(first.key, true)
    await app.findAll('.document-link')[1].trigger('click'); await flushPromises()
    expect(app.find('.filter-line').findComponent(NSelect).props('value')).toEqual(['unfinished'])
  })
  it('closes details if the active task disappears after filtering', async () => {
    const app = await openApp()
    await app.find('.task-item').trigger('click')
    await flushPromises()
    if (!app.find('input[placeholder="搜索任务内容…"]').exists()) await app.find('[aria-label="搜索任务"]').trigger('click')
    await app.find('input[placeholder="搜索任务内容…"]').setValue('不存在的内容')
    await flushPromises()
    expect(app.findComponent(NDrawer).props('show')).toBe(false)
  })
  it('keeps the drawer open while image preview handles Escape and focus', async () => {
    const app = await openApp()
    await app.find('.task-item').trigger('click')
    await flushPromises()
    app.findComponent(NImageGroup).vm.$emit('update:show', true)
    await flushPromises()
    expect(app.findComponent(NDrawer).props('closeOnEsc')).toBe(false)
    expect(app.findComponent(NDrawer).props('trapFocus')).toBe(false)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(app.findComponent(NDrawer).props('show')).toBe(true)
    expect(app.findComponent(NDrawer).props('closeOnEsc')).toBe(true)
  })
  it('saves locally and writes remotely with one click, keeping completed tasks visible until confirmed', async () => {
    const app = await openApp()
    const pending = structuredClone(first)
    pending.sheets[0].rows[0].values[3] = '已修复'
    pending.sheets[0].rows[0].status_edit = { original: '未处理', saved_at: '2026-10-16T10:00:00+08:00' }
    vi.mocked(window.go!.main.App.UpdateTaskDetails).mockResolvedValue(pending)
    const confirmed = structuredClone(pending)
    delete confirmed.sheets[0].rows[0].status_edit
    confirmed.sheets[0].rows[0].status_synced_at = '2026-10-16T10:01:00+08:00'
    let finishPush!: (doc: Document) => void
    vi.mocked(window.go!.main.App.PushTaskStatus).mockReturnValue(new Promise(resolve => { finishPush = resolve }))
    await app.find('.task-item').trigger('click')
    await flushPromises()
    expect(app.find('.status-editor').text()).not.toContain('保存本地')
    expect(app.find('.status-editor').text()).toContain('同步')
    app.find('.status-editor').findComponent(NSelect).vm.$emit('update:value', '已修复')
    await flushPromises()
    await app.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.UpdateTaskDetails).toHaveBeenCalledWith(first.key, first.sheets[0].name, 4, '已修复', [])
    expect(window.go!.main.App.PushTaskStatus).toHaveBeenCalledWith(first.key, first.sheets[0].name, 4)
    expect(vi.mocked(window.go!.main.App.UpdateTaskDetails).mock.invocationCallOrder[0])
      .toBeLessThan(vi.mocked(window.go!.main.App.PushTaskStatus).mock.invocationCallOrder[0])
    expect(app.findAll('.task-item')).toHaveLength(1)
    expect(app.find('.pending-label').text()).toBe('待回写')
    expect(app.findComponent(NDrawer).props('show')).toBe(true)
    finishPush(confirmed)
    await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(0)
    expect(app.findComponent(NDrawer).props('show')).toBe(false)
  })
  it('preserves a failed write and retries it with the same sync button', async () => {
    const app = await openApp()
    const pending = structuredClone(first)
    pending.sheets[0].rows[0].values[3] = '修复中'
    pending.sheets[0].rows[0].status_edit = { original: '未处理', saved_at: 'now' }
    const failed = structuredClone(pending)
    failed.sheets[0].rows[0].status_edit!.error = '远程状态发生冲突'
    vi.mocked(window.go!.main.App.UpdateTaskDetails).mockResolvedValue(pending)
    vi.mocked(window.go!.main.App.PushTaskStatus).mockResolvedValue(failed)
    await app.find('.task-item').trigger('click')
    await flushPromises()
    app.find('.status-editor').findComponent(NSelect).vm.$emit('update:value', '修复中')
    await flushPromises()
    await app.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(app.find('.status-error').text()).toContain('冲突')
    await app.find('.document-actions .n-button--primary-type').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.SyncDocument).not.toHaveBeenCalled()
    expect(app.find('.error-banner').text()).toContain('待同步的状态或备注修改')
    await app.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.UpdateTaskDetails).toHaveBeenCalledTimes(1)
    expect(window.go!.main.App.PushTaskStatus).toHaveBeenCalledTimes(2)
  })
  it('does not write remotely if the local save fails', async () => {
    const app = await openApp()
    vi.mocked(window.go!.main.App.UpdateTaskDetails).mockRejectedValue('本地保存失败')
    await app.find('.task-item').trigger('click')
    await flushPromises()
    app.find('.status-editor').findComponent(NSelect).vm.$emit('update:value', '修复中')
    await flushPromises()
    await app.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.PushTaskStatus).not.toHaveBeenCalled()
    expect(app.find('.status-error').text()).toContain('本地保存失败')
  })
  it('authorizes on the first sync and continues automatically', async () => {
    const app = await openApp()
    vi.mocked(window.go!.main.App.GetRemoteAuth).mockResolvedValue({ authorized: false, expires_at: 0 })
    app.unmount()
    wrapper = mount(App, { attachTo: document.body, global: { stubs: { teleport: true } } })
    await flushPromises()
    const pending = structuredClone(first)
    pending.sheets[0].rows[0].values[3] = '修复中'
    pending.sheets[0].rows[0].status_edit = { original: '未处理', saved_at: 'now' }
    vi.mocked(window.go!.main.App.UpdateTaskDetails).mockResolvedValue(pending)
    vi.mocked(window.go!.main.App.PushTaskStatus).mockResolvedValue(pending)
    await wrapper.find('.task-item').trigger('click')
    await flushPromises()
    wrapper.find('.status-editor').findComponent(NSelect).vm.$emit('update:value', '修复中')
    await flushPromises()
    await wrapper.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.AuthorizeRemote).toHaveBeenCalledTimes(1)
    expect(window.go!.main.App.PushTaskStatus).toHaveBeenCalledTimes(1)
  })
})


describe('startup sync queue', () => {
  it('syncs one document at a time and keeps the current document and search unchanged', async () => {
    const finish: ((doc: Document) => void)[] = []
    let emitProgress!: (message: string) => void
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockImplementation(() => new Promise(resolve => { finish.push(resolve) }))
      window.runtime = { EventsOnMultiple: (_name, callback) => { emitProgress = callback; return () => {} } }
    })
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledTimes(1)
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledWith(first.source_url, first.owner, true)
    expect(app.find('.footer-status').text()).toContain('正在同步 1/2「示例应用 任务清单」')
    emitProgress('正在保存图片 1/3…')
    await flushPromises()
    expect(app.find('.footer-status').text()).toContain('正在保存图片 1/3')
    await app.findAll('.document-link')[1].trigger('click')
    if (!app.find('input[placeholder="搜索任务内容…"]').exists()) await app.find('[aria-label="搜索任务"]').trigger('click')
    await app.find('input[placeholder="搜索任务内容…"]').setValue('审批')
    finish[0](structuredClone(first))
    await flushPromises()
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledTimes(2)
    expect(app.find('.footer-status').text()).toContain('正在同步 2/2「示例项目（数智）」')
    expect(app.find('main.workspace').attributes('aria-label')).toBe(second.title)
    expect((app.find('input[placeholder="搜索任务内容…"]').element as HTMLInputElement).value).toBe('审批')
    finish[1](structuredClone(second))
    await flushPromises()
    expect(app.find('.footer-status').text()).toContain('自动同步完成 · 2 份成功')
    expect(app.find('.footer-cancel').exists()).toBe(false)
    expect(window.go!.main.App.PushTaskStatus).not.toHaveBeenCalled()
  })

  it('continues after a failure and keeps the failed document cached', async () => {
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockRejectedValueOnce('连接失败')
        .mockResolvedValueOnce(structuredClone(second))
    })
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledTimes(2)
    expect(app.find('.footer-status').text()).toContain('1 份成功 · 1 份失败')
    expect(app.find('.footer-status').attributes('title')).toContain('示例应用 任务清单：连接失败')
    expect(app.findAll('.task-item')).toHaveLength(1)
    expect(app.find('.task-item').text()).toContain('麦克风')
  })

  it('cancels the remaining queue as well as the current request', async () => {
    let reject!: (reason: string) => void
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockReturnValue(new Promise((_resolve, fail) => { reject = fail }))
    })
    await app.find('.footer-cancel').trigger('click')
    expect(window.go!.main.App.CancelSync).toHaveBeenCalledTimes(1)
    reject('已取消同步，本地内容已保留')
    await flushPromises()
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledTimes(1)
    expect(app.find('.footer-status').text()).toContain('已取消自动同步')
    expect(app.findAll('.task-item')).toHaveLength(1)
  })

  it('skips pending edits and local Excel documents and honors the saved scope', async () => {
    const pending = structuredClone(first)
    pending.sheets[0].rows[0].status_edit = { original: '未处理', saved_at: 'now' }
    const local = { ...structuredClone(first), key: 'c'.repeat(24), source_url: '', source_path: '/tasks.xlsx' }
    const app = await openApp(() => {
      window.localStorage.setItem('include-completed', 'true')
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [pending, second, local], selected_key: first.key, warnings: [] })
    })
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledTimes(1)
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledWith(second.source_url, second.owner, true)
    expect(app.find('.footer-status').text()).toContain('1 份成功 · 1 份跳过')
    expect(app.find('.footer-status').attributes('title')).toContain('有待回写的状态或备注修改')
    expect(window.go!.main.App.ReimportDocument).not.toHaveBeenCalled()
    expect(app.find('.pending-label').text()).toBe('待回写')
  })
})

describe('task notes writeback', () => {
  async function openNotes(initial: string, configure?: (doc: Document) => void) {
    const doc = structuredClone(first)
    doc.sheets[0].headers.push('备注 ')
    doc.sheets[0].rows[0].values.push(initial)
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [doc], selected_key: doc.key, warnings: [] })
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockResolvedValue(doc)
      configure?.(doc)
    })
    await app.find('.task-item').trigger('click')
    await flushPromises()
    return { app, doc }
  }

  it('allows a multiline notes-only change with the same sync button', async () => {
    const { app, doc } = await openNotes('')
    const button = () => app.find('.status-editor-controls .n-button')
    expect(button().attributes('disabled')).toBeDefined()
    expect((app.find('.note-editor textarea').element as HTMLTextAreaElement).value).toBe('')
    await app.find('.note-editor textarea').setValue('修复说明\n需要复测')
    expect(button().attributes('disabled')).toBeUndefined()
    const pending = structuredClone(doc)
    pending.sheets[0].rows[0].values[4] = '修复说明\n需要复测'
    pending.sheets[0].rows[0].status_edit = { original: '未处理', saved_at: 'now', notes: [{ column: 4, header: '备注 ', original: '' }] }
    vi.mocked(window.go!.main.App.UpdateTaskDetails).mockResolvedValue(pending)
    let finish!: (value: Document) => void
    vi.mocked(window.go!.main.App.PushTaskStatus).mockReturnValue(new Promise(resolve => { finish = resolve }))
    await button().trigger('click')
    await flushPromises()
    expect(window.go!.main.App.UpdateTaskDetails).toHaveBeenCalledWith(doc.key, doc.sheets[0].name, 4, '未处理', [{ column: 4, value: '修复说明\n需要复测' }])
    expect(app.find('.pending-actions').text()).toContain('1 项备注待回写')
    expect((app.find('.note-editor textarea').element as HTMLTextAreaElement).value).toBe('修复说明\n需要复测')
    const confirmed = structuredClone(pending)
    delete confirmed.sheets[0].rows[0].status_edit
    confirmed.sheets[0].rows[0].status_synced_at = 'now'
    finish(confirmed)
    await flushPromises()
    expect(app.find('.pending-actions').exists()).toBe(false)
    expect(button().attributes('disabled')).toBeDefined()
    expect(app.find('.status-editor-heading').text()).toContain('本地与远程已同步')
  })

  it('submits status and cleared notes together and retries without another local save', async () => {
    const { app, doc } = await openNotes('原有说明')
    app.find('.status-editor').findComponent(NSelect).vm.$emit('update:value', '修复中')
    await app.find('.note-editor textarea').setValue('')
    const pending = structuredClone(doc)
    pending.sheets[0].rows[0].values[3] = '修复中'
    pending.sheets[0].rows[0].values[4] = ''
    pending.sheets[0].rows[0].status_edit = { original: '未处理', saved_at: 'now', notes: [{ column: 4, header: '备注 ', original: '原有说明' }], error: '备注回读失败' }
    vi.mocked(window.go!.main.App.UpdateTaskDetails).mockResolvedValue(pending)
    vi.mocked(window.go!.main.App.PushTaskStatus).mockResolvedValue(pending)
    await app.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.UpdateTaskDetails).toHaveBeenCalledWith(doc.key, doc.sheets[0].name, 4, '修复中', [{ column: 4, value: '' }])
    expect(app.find('.status-error').text()).toContain('备注回读失败')
    await app.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.UpdateTaskDetails).toHaveBeenCalledTimes(1)
    expect(window.go!.main.App.PushTaskStatus).toHaveBeenCalledTimes(2)
    expect((app.find('.note-editor textarea').element as HTMLTextAreaElement).value).toBe('')
  })

  it('keeps unsaved notes if the local write fails and does not send them remotely', async () => {
    const { app } = await openNotes('原有说明')
    await app.find('.note-editor textarea').setValue('还未保存的说明')
    vi.mocked(window.go!.main.App.UpdateTaskDetails).mockRejectedValue('无法保存')
    await app.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.PushTaskStatus).not.toHaveBeenCalled()
    expect((app.find('.note-editor textarea').element as HTMLTextAreaElement).value).toBe('还未保存的说明')
    expect(app.find('.status-error').text()).toContain('无法保存')
  })

  it('keeps an empty status empty when only notes change', async () => {
    const { app, doc } = await openNotes('', doc => { doc.sheets[0].rows[0].values[3] = '' })
    await app.find('.note-editor textarea').setValue('补充说明')
    const pending = structuredClone(doc)
    pending.sheets[0].rows[0].values[4] = '补充说明'
    pending.sheets[0].rows[0].status_edit = { original: '', saved_at: 'now', notes: [{ column: 4, header: '备注 ', original: '' }] }
    vi.mocked(window.go!.main.App.UpdateTaskDetails).mockResolvedValue(pending)
    vi.mocked(window.go!.main.App.PushTaskStatus).mockResolvedValue(pending)
    await app.find('.status-editor-controls .n-button').trigger('click')
    await flushPromises()
    expect(window.go!.main.App.UpdateTaskDetails).toHaveBeenCalledWith(doc.key, doc.sheets[0].name, 4, '', [{ column: 4, value: '补充说明' }])
    expect(app.find('.pending-actions').text()).not.toContain('原状态')
  })

  it('loads each task’s own notes when moving between tasks', async () => {
    const { app } = await openNotes('第一条备注', doc => {
      doc.sheets[0].rows[1].values[3] = '未处理'
      doc.sheets[0].rows[1].values.push('第二条备注')
    })
    await app.find('.note-editor textarea').setValue('草稿')
    await app.find('[aria-label="下一条任务"]').trigger('click')
    await flushPromises()
    expect((app.find('.note-editor textarea').element as HTMLTextAreaElement).value).toBe('第二条备注')
    expect(app.find('.status-editor-controls .n-button').attributes('disabled')).toBeDefined()
  })
})


describe('multiple status filter', () => {
  async function openStatuses(legacyExclusions = false) {
    const doc = structuredClone(first)
    doc.sheets[0].rows[0].values[3] = '已修复待发布'
    doc.sheets[0].rows[1].values[3] = '已修复'
    doc.sheets[0].status_options = ['未处理', '已修复', '已修复待发布', '自定义状态']
    return openApp(() => {
      window.localStorage.setItem('include-completed', 'true')
      if (legacyExclusions) window.localStorage.setItem('hidden-task-statuses', '["已修复","已修复待发布"]')
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [doc, second], selected_key: doc.key, warnings: [] })
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockImplementation(async url => structuredClone(url === doc.source_url ? doc : second))
    })
  }
  const select = (app: ReturnType<typeof mount>) => app.find('.filter-line').findComponent(NSelect)

  it('includes any selected status, combines with search and never writes or pulls remotely', async () => {
    const app = await openStatuses()
    const filter = select(app)
    expect(filter.props('multiple')).toBe(true)
    expect(filter.props('clearable')).toBe(true)
    expect(filter.props('options')).toContainEqual({label:'自定义状态',value:'自定义状态'})
    filter.vm.$emit('update:value', ['已修复']); await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(1)
    expect(app.find('.task-item .status-label').text()).toBe('已修复')
    filter.vm.$emit('update:value', ['已修复', '已修复待发布']); await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(2)
    expect(filter.find('.status-selection-summary').text()).toBe('已修复、已修复待发布')
    expect(filter.find('.n-tag').exists()).toBe(false)
    expect(app.find('.filtered-total').text()).toContain('/ 2')
    await app.find('[aria-label="搜索任务"]').trigger('click')
    await app.find('input[placeholder="搜索任务内容…"]').setValue('麦克风'); await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(1)
    expect(app.find('.task-item .status-label').text()).toBe('已修复待发布')
    expect(window.go!.main.App.PushTaskStatus).not.toHaveBeenCalled()
    expect(window.go!.main.App.UpdateTaskDetails).not.toHaveBeenCalled()
    expect(window.go!.main.App.SyncConfiguredDocument).not.toHaveBeenCalled()
  })

  it('removes the exclusion control and ignores legacy hidden statuses', async () => {
    const app = await openStatuses(true)
    expect(app.find('[aria-label="设置不查看的状态"]').exists()).toBe(false)
    expect(app.find('.filter-line').text()).not.toContain('不查看')
    expect(app.findAll('.task-item')).toHaveLength(2)
    select(app).vm.$emit('update:value', ['已修复']); await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(1)
    await app.findAll('.document-link')[1].trigger('click'); await flushPromises()
    expect(select(app).props('value')).toEqual([])
    expect(app.findAll('.task-item')).toHaveLength(1)
  })

  it('restores separate document and sheet selections after switching, syncing and restarting', async () => {
    const doc = structuredClone(first)
    doc.sheets.push({ ...structuredClone(doc.sheets[0]), name: '第二张表' })
    const other = { ...structuredClone(second), sheets: [{ ...structuredClone(second.sheets[0]), name: doc.sheets[0].name }] }
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [doc, other], selected_key: doc.key, warnings: [], projects: [defaultProject] })
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockImplementation(async url => structuredClone(url === doc.source_url ? doc : other))
      vi.mocked(window.go!.main.App.SyncConfiguredDocument).mockResolvedValue(doc)
    })
    select(app).vm.$emit('update:value', ['已修复', '历史状态']); await flushPromises()
    await app.findAll('.document-link')[1].trigger('click'); await flushPromises()
    expect(select(app).props('value')).toEqual(['unfinished'])
    select(app).vm.$emit('update:value', ['已完成']); await flushPromises()
    await app.findAll('.document-link')[0].trigger('click'); await flushPromises()
    expect(select(app).props('value')).toEqual(['已修复', '历史状态'])
    expect(select(app).props('options')).toContainEqual({label:'历史状态',value:'历史状态'})
    app.findAllComponents(NSelect).find(item => item.classes().includes('sheet-select'))!.vm.$emit('update:value', '第二张表'); await flushPromises()
    expect(select(app).props('value')).toEqual(['unfinished'])
    select(app).vm.$emit('update:value', []); await flushPromises()
    app.findAllComponents(NSelect).find(item => item.classes().includes('sheet-select'))!.vm.$emit('update:value', doc.sheets[0].name); await flushPromises()
    expect(select(app).props('value')).toEqual(['已修复', '历史状态'])
    await app.find('.document-actions .n-button--primary-type').trigger('click'); await flushPromises()
    expect(select(app).props('value')).toEqual(['已修复', '历史状态'])
    expect(app.findAll('.task-item')).toHaveLength(1)
    app.unmount()
    wrapper = mount(App, { attachTo: document.body, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(select(wrapper).props('value')).toEqual(['已修复', '历史状态'])
    wrapper.findAllComponents(NSelect).find(item => item.classes().includes('sheet-select'))!.vm.$emit('update:value', '第二张表'); await flushPromises()
    expect(select(wrapper).props('value')).toEqual([])
    expect(wrapper.findAll('.task-item')).toHaveLength(2)
    await wrapper.findAll('.document-link')[1].trigger('click'); await flushPromises()
    expect(select(wrapper).props('value')).toEqual(['已完成'])
    expect(wrapper.findAll('.task-item')).toHaveLength(1)
  })

  it('falls back safely from invalid saved values and persists clear filters as all statuses', async () => {
    const key = `task-statuses:${first.key}:${first.sheets[0].name}`
    const app = await openApp(() => {
      window.localStorage.setItem(key, '{invalid')
      window.localStorage.setItem(`task-statuses:${second.key}:${second.sheets[0].name}`, '[42]')
    })
    expect(select(app).props('value')).toEqual(['unfinished'])
    expect((select(app).props('options') as {label:string;value:string}[]).filter(option => option.value === 'unfinished')).toEqual([{label:'未完成任务',value:'unfinished'}])
    expect(select(app).find('.status-selection-summary').text()).toBe('未完成任务')
    select(app).vm.$emit('update:value', ['不存在的状态']); await flushPromises()
    await app.find('.list-empty .n-button').trigger('click'); await flushPromises()
    expect(window.localStorage.getItem(key)).toBe('[]')
    await app.findAll('.document-link')[1].trigger('click'); await flushPromises()
    expect(select(app).props('value')).toEqual(['unfinished'])
    await app.findAll('.document-link')[0].trigger('click'); await flushPromises()
    expect(select(app).props('value')).toEqual([])
    expect(app.findAll('.task-item')).toHaveLength(2)
  })

  it('shows all cached statuses when cleared and restores tasks from an empty result', async () => {
    const app = await openStatuses()
    select(app).vm.$emit('update:value', ['自定义状态']); await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(0)
    expect(app.find('.list-empty').text()).toContain('没有符合条件的任务')
    await app.find('.list-empty .n-button').trigger('click'); await flushPromises()
    expect(select(app).props('value')).toEqual([])
    expect(app.findAll('.task-item')).toHaveLength(2)
    select(app).vm.$emit('update:value', ['已修复']); await flushPromises()
    select(app).vm.$emit('update:value', []); await flushPromises()
    expect(app.findAll('.task-item')).toHaveLength(2)
  })
})


describe('today deadline highlighting', () => {
  it('puts today’s unfinished tasks first while preserving source order within both groups and applying filters', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 9, 16, 12))
    try {
      const doc = structuredClone(first)
      doc.sheets[0].headers.push('截止日期')
      doc.sheets[0].rows = [
        { number: 2, values: ['普通 A', '普通 A', '王小明', '未处理', '2026-10-17'], pictures: [] },
        { number: 3, values: ['今日 A', '今日 A', '王小明', '未处理', '2026-10-16'], pictures: [] },
        { number: 4, values: ['已完成', '已完成', '王小明', '已修复', '2026-10-16'], pictures: [] },
        { number: 5, values: ['今日 B', '今日 B', '王小明', '进行中', '2026-10-16'], pictures: [] },
        { number: 6, values: ['普通 B', '普通 B', '王小明', '未处理', '2026-10-15'], pictures: [] },
      ]
      const app = await openApp(() => {
        window.localStorage.setItem('include-completed', 'true')
        vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [doc], selected_key: doc.key, warnings: [] })
        vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockResolvedValue(doc)
      })
      const rowOrder = () => app.findAll('.row-number').map(row => row.text())
      expect(rowOrder()).toEqual(['#003', '#005', '#002', '#004', '#006'])
      expect(app.find('.list-summary').text()).toContain('今日截止优先')
      await app.findAll('.task-item')[1].trigger('click')
      await flushPromises()
      expect(app.find('.task-description').text()).toBe('今日 B')
      await app.find('[aria-label="关闭任务详情"]').trigger('click')
      app.find('.filter-line').findComponent(NSelect).vm.$emit('update:value', ['未处理', '已修复'])
      await flushPromises()
      expect(rowOrder()).toEqual(['#003', '#002', '#004', '#006'])
      if (!app.find('input[placeholder="搜索任务内容…"]').exists()) await app.find('[aria-label="搜索任务"]').trigger('click')
      await app.find('input[placeholder="搜索任务内容…"]').setValue('普通')
      expect(rowOrder()).toEqual(['#002', '#006'])
      expect(app.find('.list-summary').text()).toContain('按原表顺序')
      if (!app.find('input[placeholder="搜索任务内容…"]').exists()) await app.find('[aria-label="搜索任务"]').trigger('click')
      await app.find('input[placeholder="搜索任务内容…"]').setValue('')
      vi.setSystemTime(new Date(2026, 9, 17, 8))
      window.dispatchEvent(new Event('focus'))
      await flushPromises()
      expect(rowOrder()).toEqual(['#002', '#003', '#004', '#006'])
      expect(doc.sheets[0].rows.map(row => row.number)).toEqual([2, 3, 4, 5, 6])
    } finally { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() }
  })
  async function openDueTasks() {
    const doc = structuredClone(first)
    doc.sheets[0].headers.push('截止日期')
    doc.sheets[0].rows[0].values.push('2026-10-16')
    doc.sheets[0].rows[1].values.push('2026-10-16')
    return openApp(() => {
      window.localStorage.setItem('include-completed', 'true')
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [doc], selected_key: doc.key, warnings: [] })
      vi.mocked(window.go!.main.App.SyncDocumentOnStartup).mockResolvedValue(doc)
    })
  }
  it('highlights unfinished tasks due today in both list and drawer', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 9, 16, 12))
    try {
      const app = await openDueTasks()
      const rows = app.findAll('.task-item')
      expect(rows).toHaveLength(2)
      expect(rows[0].classes()).toContain('due-today')
      expect(rows[0].find('.due-today-label').text()).toBe('今日截止')
      expect(rows[1].classes()).not.toContain('due-today')
      await rows[0].trigger('click')
      await flushPromises()
      expect(app.find('.detail-badges .due-today-label').text()).toBe('今日截止')
    } finally { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() }
  })
  it('updates at local midnight and when returning to the window on another day', async () => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] })
    vi.setSystemTime(new Date(2026, 9, 15, 23, 59, 59, 500))
    try {
      const app = await openDueTasks()
      expect(app.findAll('.task-item.due-today')).toHaveLength(0)
      await vi.advanceTimersByTimeAsync(700)
      expect(app.findAll('.task-item.due-today')).toHaveLength(1)
      vi.setSystemTime(new Date(2026, 9, 17, 8))
      window.dispatchEvent(new Event('focus'))
      await flushPromises()
      expect(app.findAll('.task-item.due-today')).toHaveLength(0)
    } finally { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() }
  })
})


describe('scheduled project document pulls', () => {
  function entry(): SyncSchedule { return { id: 'd'.repeat(24), project_id: defaultProject.id, interval_minutes: 1, enabled: true, revision: 'e'.repeat(24), next_run_at: new Date(Date.now() + 60_000).toISOString(), last_run_at: '', last_result: '' } }
  it('pulls only current project online documents sequentially, skipping pending edits and preserving selection', async () => {
    vi.useFakeTimers()
    let finish!: (doc: Document) => void
    const pending = structuredClone(second)
    pending.sheets[0].rows[0].status_edit = { original: '未处理', saved_at: '2026-10-18T01:00:00Z', notes: [{ column: 2, header: '备注', original: '' }] }
    const local = { ...structuredClone(first), key: 'f'.repeat(24), source_url: '', title: '本地Excel' }
    const other = { ...structuredClone(first), key: '0'.repeat(24), title: '其他项目' }
    const newDoc = { ...structuredClone(first), key: '1'.repeat(24), title: '本轮新加入的文档' }
    const project = { ...defaultProject, document_keys: [first.key, pending.key, local.key, newDoc.key] }
    const item = entry()
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetSyncSchedules).mockResolvedValue([item])
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [structuredClone(first), pending, local, other, newDoc], projects: [project], selected_key: first.key, warnings: [] })
      vi.mocked(window.go!.main.App.SyncConfiguredDocument).mockImplementationOnce(() => new Promise(resolve => { finish = resolve })).mockResolvedValueOnce(newDoc)
      vi.mocked(window.go!.main.App.CompleteSyncSchedule).mockResolvedValue([])
    })
    if (!app.find('input[placeholder="搜索任务内容…"]').exists()) await app.find('[aria-label="搜索任务"]').trigger('click')
    await app.find('input[placeholder="搜索任务内容…"]').setValue('麦克风')
    await vi.advanceTimersByTimeAsync(60_000); await flushPromises()
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledTimes(1)
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledWith(first.key, false)
    expect(app.find('.list-footer').text()).toContain('1/3')
    finish(structuredClone(first)); await flushPromises()
    expect(vi.mocked(window.go!.main.App.SyncConfiguredDocument).mock.calls).toEqual([[first.key, false], [newDoc.key, false]])
    expect(window.go!.main.App.CompleteSyncSchedule).toHaveBeenCalledWith(item.id, item.revision, expect.stringContaining('2 份成功 · 1 份跳过'))
    expect(app.find('main.workspace').attributes('aria-label')).toBe(first.title)
    expect(app.find<HTMLInputElement>('input[placeholder="搜索任务内容…"]').element.value).toBe('麦克风')
    expect(window.go!.main.App.PushTaskStatus).not.toHaveBeenCalled()
    expect(window.go!.main.App.AuthorizeRemote).not.toHaveBeenCalled()
  })
  it('waits for task details to close before pulling and cancels the remaining documents', async () => {
    vi.useFakeTimers(); let finish!: (doc: Document) => void
    const item = entry()
    const app = await openApp(() => {
      vi.mocked(window.go!.main.App.GetSyncSchedules).mockResolvedValue([item])
      vi.mocked(window.go!.main.App.SyncConfiguredDocument).mockImplementation(() => new Promise(resolve => { finish = resolve }))
      vi.mocked(window.go!.main.App.CompleteSyncSchedule).mockResolvedValue([])
      vi.mocked(window.go!.main.App.CancelSync).mockResolvedValue(undefined)
    })
    await app.find('.task-item').trigger('click'); await vi.advanceTimersByTimeAsync(120_000)
    expect(window.go!.main.App.SyncConfiguredDocument).not.toHaveBeenCalled()
    app.findComponent(NDrawer).vm.$emit('update:show', false); await flushPromises()
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledTimes(1)
    await app.find('.footer-cancel').trigger('click')
    finish(structuredClone(first)); await flushPromises()
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledTimes(1)
    expect(window.go!.main.App.CompleteSyncSchedule).toHaveBeenCalledWith(item.id, item.revision, expect.stringContaining('已取消定时拉取'))
  })
  it('opens scheduling inside settings and locks closing only for changed drafts', async () => {
    const app = await openApp(undefined, true)
    const modal = new DOMWrapper(document.body)
    await app.find('.app-settings-link').trigger('click'); await vi.dynamicImportSettled(); await flushPromises()
    await modal.findAll('.settings-sidebar nav button')[1].trigger('click'); await vi.dynamicImportSettled(); await flushPromises()
    expect(modal.find('[aria-label="定时任务"]').exists()).toBe(true)
    await app.findAllComponents(NButton).find(button => button.text() === '新建定时任务')!.trigger('click')
    const input = modal.find('input[aria-label="循环间隔"]')
    await input.setValue('15'); await input.trigger('blur'); await flushPromises()
    expect(app.find<HTMLButtonElement>('.document-link').element.disabled).toBe(true)
    expect(modal.find('[aria-label="关闭软件设置"]').attributes('disabled')).toBeDefined()
    await app.findAllComponents(NButton).find(button => button.text() === '返回' && button.attributes('disabled') === undefined)!.trigger('click'); await flushPromises()
    expect(modal.find('[aria-label="关闭软件设置"]').attributes('disabled')).toBeUndefined()
    expect(window.go!.main.App.SaveSyncSchedule).not.toHaveBeenCalled()
    await modal.findAll('.settings-sidebar nav button')[0].trigger('click'); await vi.dynamicImportSettled(); await flushPromises()
    expect(modal.find('.ai-settings-page').exists()).toBe(true)
    await modal.find('[aria-label="关闭软件设置"]').trigger('click'); await flushPromises()
    expect(modal.find('.software-settings-modal').exists()).toBe(false)
    expect(app.find('main.workspace').attributes('aria-label')).toBe(first.title)
  })
})

describe('online document setup', () => {
  it('keeps the add dialog to project and URL, discovers schema and does not pull tasks yet', async () => {
    const added: Document = { ...structuredClone(first), owner: '', key: 'f'.repeat(24), source_url: 'https://docs.qq.com/sheet/new-personal', title: '个人事项', setup_pending: true, cached: false, updated_at: '', sheets: [{ name: '个人事项', header_row: 1, headers: ['事项', '状态'], rows: [], scanned: 0 }] }
    const app = await openApp(() => { vi.mocked(window.go!.main.App.PrepareOnlineDocument).mockResolvedValue(added) })
    await app.find('[aria-label="添加在线文档"]').trigger('click'); await flushPromises()
    const form = app.find('form')
    expect(form.findAll('input')).toHaveLength(1)
    expect(form.text()).not.toContain('负责人'); expect(form.text()).not.toContain('包含已完成任务')
    await form.find('input[aria-label="在线文档链接"]').setValue(added.source_url)
    await form.trigger('submit'); await flushPromises()
    expect(window.go!.main.App.PrepareOnlineDocument).toHaveBeenCalledWith(added.source_url)
    expect(window.go!.main.App.SyncConfiguredDocument).not.toHaveBeenCalled()
    expect(window.go!.main.App.SyncDocument).not.toHaveBeenCalled()
    expect(app.find('main.workspace').attributes('aria-label')).toBe(added.title)
    expect(app.find('.pull-pending').text()).toContain('已识别表头')
    expect(app.findAll('.task-item')).toHaveLength(0)
  })
  it('keeps the submitted link editable when schema discovery fails', async () => {
    const app = await openApp(() => { vi.mocked(window.go!.main.App.PrepareOnlineDocument).mockRejectedValue('无法识别表头') })
    await app.find('[aria-label="添加在线文档"]').trigger('click'); await flushPromises()
    await app.find('form input').setValue('https://docs.qq.com/sheet/bad-schema')
    await app.find('form').trigger('submit'); await flushPromises()
    expect(app.find('form').exists()).toBe(true)
    expect(app.find<HTMLInputElement>('form input').element.value).toBe('https://docs.qq.com/sheet/bad-schema')
    expect(app.find('.form-error').text()).toContain('无法识别表头')
    expect(app.findAll('.document-link')).toHaveLength(2)
  })
  it('keeps newly discovered documents out of startup and recurring pulls until their first manual sync', async () => {
    vi.useFakeTimers()
    const pending = { ...structuredClone(second), setup_pending: true, cached: false }
    const item: SyncSchedule = { id: 'd'.repeat(24), project_id: defaultProject.id, interval_minutes: 1, enabled: true, revision: 'e'.repeat(24), next_run_at: new Date(Date.now() + 60_000).toISOString(), last_run_at: '', last_result: '' }
    await openApp(() => {
      vi.mocked(window.go!.main.App.GetSyncSchedules).mockResolvedValue([item])
      vi.mocked(window.go!.main.App.GetLibrary).mockResolvedValue({ documents: [structuredClone(first), pending], projects: [defaultProject], selected_key: first.key, warnings: [] })
      vi.mocked(window.go!.main.App.SyncConfiguredDocument).mockResolvedValue(first)
      vi.mocked(window.go!.main.App.CompleteSyncSchedule).mockResolvedValue([])
    })
    expect(window.go!.main.App.SyncDocumentOnStartup).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(60_000); await flushPromises()
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledTimes(1)
    expect(window.go!.main.App.SyncConfiguredDocument).toHaveBeenCalledWith(first.key, false)
  })
})
