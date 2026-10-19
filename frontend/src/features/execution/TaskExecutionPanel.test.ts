// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NButton, NSelect } from 'naive-ui'
import TaskExecutionPanel from './TaskExecutionPanel.vue'
import type { TaskExecution } from '../../shared/types/types'
import type { api } from '../../shared/api/api'

let wrapper: ReturnType<typeof mount> | undefined
const run: TaskExecution = { id: 'a'.repeat(24), task_id: 'b'.repeat(24), project_id: 'c'.repeat(24), document_key: 'doc', sheet: '任务', row: 4, task: '修复登录错误', instructions: '', repository: '/project/backend', workspace: '/executions/workspace', branch: 'taskpicker/fixture', status: 'running', started_at: '2026-10-16T16:00:00+08:00', finished_at: '', result: '', error: '', changed_files: '', log_count: 1 }
function setup(history: TaskExecution[] = []) {
  const mock = {
    GetTaskExecutionContext: vi.fn().mockResolvedValue({ engine: 'Codex', available: true, repositories: [{ path: run.repository, codegraph: true }], notice: '' }),
    GetTaskExecutions: vi.fn().mockResolvedValue(history),
    ContinueTaskExecution: vi.fn().mockImplementation(async (_key, _sheet, _row, previousID, instructions) => { const continued = { ...run, id: 'e'.repeat(24), previous_id: previousID, root_id: previousID, instructions }; mock.GetTaskExecutions.mockResolvedValue([continued, ...history]); return continued }),
    GetTaskExecutionProgress: vi.fn().mockResolvedValue({ run_id: run.id, root_id: run.id, status: 'succeeded', markdown: '# 任务当前进展\n\n## 待办\n\n补充异常测试', path: '/tasks/progress.md' }),
    OpenTaskExecutionProgress: vi.fn().mockResolvedValue(undefined),
    StartTaskExecution: vi.fn().mockImplementation(async () => { mock.GetTaskExecutions.mockResolvedValue([run]); return run }),
    GetTaskExecutionReport: vi.fn().mockResolvedValue({ markdown: '# 任务执行记录\n\n## 用户原文\n\n修复登录错误\n\n## AI 理解\n\n修复登录失败\n\n## 执行过程（关键节点）\n\n- 已定位问题\n\n## 最后结果\n\n执行中', path: '/executions/report.md' }),
    OpenTaskExecutionReport: vi.fn().mockResolvedValue(undefined),
    CopyText: vi.fn().mockResolvedValue(undefined),
    StopTaskExecution: vi.fn().mockImplementation(async () => { const stopped = { ...run, status: 'stopped', error: '执行已停止' }; mock.GetTaskExecutions.mockResolvedValue([stopped]); mock.GetTaskExecutionReport.mockResolvedValue({ markdown: '# 任务执行记录\n\n执行已停止，修改保留', path: '/executions/report.md' }) }),
    OpenTaskExecutionWorkspace: vi.fn().mockResolvedValue(undefined),
  }
  window.go = { main: { App: mock as unknown as typeof api } }
  return mock
}
async function open(history: TaskExecution[] = []) {
  const mock = setup(history)
  wrapper = mount(TaskExecutionPanel, { props: { documentKey: 'doc', sheet: '任务', row: 4, disabled: false }, attachTo: document.body })
  await flushPromises()
  return { app: wrapper, mock }
}
function button(text: string) { return wrapper!.findAllComponents(NButton).find(item => item.text() === text)! }
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = ''; delete window.go; vi.useRealTimers() })

describe('single task AI execution', () => {
  it('loads context without starting AI and passes manual instructions and the chosen repository on start', async () => {
    const { app, mock } = await open()
    expect(mock.StartTaskExecution).not.toHaveBeenCalled()
    expect(app.text()).toContain('这条任务还没有执行记录')
    expect(app.findComponent(NSelect).props('options')![0].label).toContain('CodeGraph')
    await app.find('textarea').setValue('先复现，再修复')
    await button('启动执行').trigger('click'); await flushPromises()
    expect(mock.StartTaskExecution).toHaveBeenCalledWith('doc', '任务', 4, run.repository, '先复现，再修复')
    expect(app.emitted('started')?.[0]).toEqual([run])
    expect(app.find('.execution-report').text()).toContain('已定位问题')
    expect(app.find('.execution-status').text()).toBe('正在执行')
    expect(app.find('textarea').attributes('disabled')).toBeDefined()
  })
  it('stops an existing execution and keeps its history and logs', async () => {
    const { app, mock } = await open([run])
    await button('停止执行').trigger('click'); await flushPromises()
    expect(mock.StopTaskExecution).toHaveBeenCalledWith(run.id)
    expect(app.find('.execution-status').text()).toBe('已停止')
    expect(app.find('.execution-report').text()).toContain('执行已停止')
    expect(button('重新执行')).toBeDefined()
  })
  it('shows saved results and file changes without new AI requests', async () => {
    const completed = { ...run, status: 'succeeded' as const, result: '已修复，测试通过', changed_files: 'M src/login.ts', finished_at: run.started_at }
    const mock = setup([completed])
    mock.GetTaskExecutionReport.mockResolvedValue({ markdown: '# 任务执行记录\n\n已修复，测试通过', path: '/executions/report.md' })
    wrapper = mount(TaskExecutionPanel, { props: { documentKey: 'doc', sheet: '任务', row: 4, disabled: false }, attachTo: document.body })
    await flushPromises()
    expect(wrapper.find('.execution-status').text()).toBe('执行结束 · 待核对')
    expect(wrapper.text()).toContain('已修复，测试通过')
    await wrapper.findAll('.n-tabs-tab').find(tab => tab.text() === '修改文件')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('M src/login.ts')
    await button('打开执行工作区').trigger('click'); await flushPromises()
    expect(mock.OpenTaskExecutionWorkspace).toHaveBeenCalledWith(run.id)
    expect(mock.StartTaskExecution).not.toHaveBeenCalled()
  })
  it('retains instructions on launch errors and stops polling when unmounted without cancelling the task', async () => {
    const { app, mock } = await open()
    mock.StartTaskExecution.mockRejectedValueOnce('已有 AI 任务正在执行')
    await app.find('textarea').setValue('保留这些要求')
    await button('启动执行').trigger('click'); await flushPromises()
    expect(app.find('[role="alert"]').text()).toContain('已有 AI 任务正在执行')
    expect(app.find('textarea').element.value).toBe('保留这些要求')
    await button('启动执行').trigger('click'); await flushPromises()
    vi.useFakeTimers()
    app.unmount(); wrapper = undefined
    const calls = mock.GetTaskExecutionReport.mock.calls.length
    await vi.advanceTimersByTimeAsync(5000)
    expect(mock.GetTaskExecutionReport).toHaveBeenCalledTimes(calls)
    expect(mock.StopTaskExecution).not.toHaveBeenCalled()
  })
  it('opens independent Markdown documents for each historical execution and copies or opens the selected file', async () => {
    const older = { ...run, id: 'd'.repeat(24), status: 'failed' as const, started_at: '2026-10-15T16:00:00+08:00' }
    const latest = { ...run, status: 'succeeded' as const }
    const mock = setup([latest, older])
    mock.GetTaskExecutionReport.mockImplementation(async (id: string) => ({ markdown: `# ${id === older.id ? '第一次：验证失败' : '第二次：修复完成'}`, path: `/executions/${id}/report.md` }))
    wrapper = mount(TaskExecutionPanel, { props: { documentKey: 'doc', sheet: '任务', row: 4, disabled: false }, attachTo: document.body })
    await flushPromises()
    expect(wrapper.find('.execution-report').text()).toContain('第二次：修复完成')
    const rows = wrapper.findAll('.execution-history button')
    expect(rows).toHaveLength(2)
    await rows[1]!.trigger('click'); await flushPromises()
    expect(wrapper.find('.execution-report').text()).toContain('第一次：验证失败')
    expect(wrapper.find('.execution-report').text()).not.toContain('第二次：修复完成')
    await button('复制 Markdown').trigger('click'); await flushPromises()
    expect(mock.CopyText).toHaveBeenCalledWith('# 第一次：验证失败')
    await button('打开 MD 文件').trigger('click'); await flushPromises()
    expect(mock.OpenTaskExecutionReport).toHaveBeenCalledWith(older.id)
    expect(mock.StartTaskExecution).not.toHaveBeenCalled()
  })
  it('ignores a late document response after selecting another execution', async () => {
    const older = { ...run, id: 'd'.repeat(24), status: 'failed' as const, started_at: '2026-10-15T16:00:00+08:00' }
    const mock = setup([{ ...run, status: 'succeeded' as const }, older])
    let resolveFirst!: (value: { markdown: string; path: string }) => void
    mock.GetTaskExecutionReport.mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve })).mockResolvedValue({ markdown: '# 旧执行文档', path: '/old/report.md' })
    wrapper = mount(TaskExecutionPanel, { props: { documentKey: 'doc', sheet: '任务', row: 4, disabled: false }, attachTo: document.body })
    await flushPromises()
    await wrapper.findAll('.execution-history button')[1]!.trigger('click'); await flushPromises()
    expect(wrapper.find('.execution-report').text()).toBe('旧执行文档')
    resolveFirst({ markdown: '# 迟到的另一份文档', path: '/late/report.md' }); await flushPromises()
    expect(wrapper.find('.execution-report').text()).toBe('旧执行文档')
    expect(wrapper.find('.execution-report-path').text()).toBe('/old/report.md')
  })

  it('continues the selected round using its existing workspace and clears only successfully submitted instructions', async () => {
    const previous = { ...run, status: 'succeeded' as const }
    const { app, mock } = await open([previous])
    await app.find('textarea').setValue('继续补异常测试')
    await button('继续处理').trigger('click'); await flushPromises()
    expect(mock.ContinueTaskExecution).toHaveBeenCalledWith('doc', '任务', 4, previous.id, '继续补异常测试')
    expect(mock.StartTaskExecution).not.toHaveBeenCalled()
    expect(app.emitted('started')?.[0]?.[0]).toMatchObject({ previous_id: previous.id, workspace: previous.workspace })
    expect(app.find('textarea').element.value).toBe('')
    expect(app.text()).toContain('已承接上一轮摘要')
  })
  it('restarts independently instead of continuing when the user chooses restart', async () => {
    const { app, mock } = await open([{ ...run, status: 'succeeded' as const }])
    await app.find('textarea').setValue('重新检查')
    await button('重新执行').trigger('click'); await flushPromises()
    expect(mock.StartTaskExecution).toHaveBeenCalledWith('doc', '任务', 4, run.repository, '重新检查')
    expect(mock.ContinueTaskExecution).not.toHaveBeenCalled()
  })
  it('prevents continuing a stale round in a shared workspace and shows the task progress document', async () => {
    const older = { ...run, id: 'd'.repeat(24), status: 'succeeded' as const, started_at: '2026-10-15T16:00:00+08:00' }
    const latest = { ...run, previous_id: older.id, status: 'succeeded' as const }
    const { app, mock } = await open([latest, older])
    await app.findAll('.execution-history button')[1]!.trigger('click'); await flushPromises()
    expect(button('继续处理').props('disabled')).toBe(true)
    expect(app.text()).toContain('所选工作区已有后续执行')
    await app.findAll('.n-tabs-tab').find(tab => tab.text() === '任务进展')!.trigger('click'); await flushPromises()
    expect(app.text()).toContain('补充异常测试')
    await button('打开进展 MD').trigger('click'); await flushPromises()
    expect(mock.OpenTaskExecutionProgress).toHaveBeenCalledWith('doc', '任务', 4)
    expect(mock.ContinueTaskExecution).not.toHaveBeenCalled()
  })
  it('keeps continuation instructions if the previous workspace is missing', async () => {
    const { app, mock } = await open([{ ...run, status: 'succeeded' as const }])
    mock.ContinueTaskExecution.mockRejectedValueOnce('上次工作区不存在或已移动')
    await app.find('textarea').setValue('别丢失这次要求')
    await button('继续处理').trigger('click'); await flushPromises()
    expect(app.find('[role="alert"]').text()).toContain('上次工作区不存在')
    expect(app.find('textarea').element.value).toBe('别丢失这次要求')
    expect(mock.StartTaskExecution).not.toHaveBeenCalled()
  })

})
