// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NButton, NDatePicker } from 'naive-ui'
import DailyReportPanel from './DailyReportPanel.vue'
import TaskWorkEntry from './TaskWorkEntry.vue'
import type { DailyAISettings, DailyReport, TaskActivity } from '../../shared/types/types'
import type { api } from '../../shared/api/api'

let wrapper: ReturnType<typeof mount> | undefined
const activity: TaskActivity = { id: 'event-1', date: '2026-10-16', at: '2026-10-16T15:00:00+08:00', document_key: 'doc-1', document_title: '项目 A', sheet: '任务', row: 88, owner: '王小明', task: '发布接口', context: '项目发布', kind: 'work', status: '进行中', changes: [], note: '完成联调，等待验证' }
const config: DailyAISettings = { base_url: '', model: '', prompt: '按项目组织日报', has_api_key: false, default_prompt: '默认提示词' }
function report(text = ''): DailyReport { return { date: activity.date, activities: [structuredClone(activity)], draft: { text, source: text ? 'local' : '', saved_at: text ? 'saved' : '', generated_at: text ? 'generated' : '', activity_count: text ? 1 : 0 } } }
function setup(text = '') {
  const apiMock = {
    GetDailyReport: vi.fn().mockImplementation(async (date: string) => ({ ...report(text), date })),
    GetDailyAISettings: vi.fn().mockResolvedValue(structuredClone(config)),
    SaveDailyReportPrompt: vi.fn().mockImplementation(async value => value),
    SaveDailyAISettings: vi.fn().mockImplementation(async value => ({ ...config, ...value, has_api_key: true })),
    SaveDailyReport: vi.fn().mockImplementation(async (_date: string, value: string) => ({ ...report().draft, text: value, saved_at: 'now' })),
    ContinueDailyReport: vi.fn().mockResolvedValue(report('原日报\n\n补充工作\n完成新工作')) ,
    GenerateDailyReport: vi.fn().mockResolvedValue(report('生成的日报正文')),
    CancelDailyReport: vi.fn().mockResolvedValue(undefined),
    CopyText: vi.fn().mockResolvedValue(undefined),
    RecordTaskWork: vi.fn().mockResolvedValue(activity),
  }
  window.go = { main: { App: apiMock as unknown as typeof api } }
  return apiMock
}
async function open(text = '') {
  const mock = setup(text)
  wrapper = mount(DailyReportPanel, { attachTo: document.body, global: { stubs: { teleport: true } } })
  await flushPromises()
  return { app: wrapper, mock }
}
function button(label: string) { return wrapper!.findAllComponents(NButton).find(item => item.text() === label)! }
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = ''; delete window.go })

describe('daily report panel', () => {
  it('renders as a page and allows focus to move to the sidebar', async () => {
    setup()
    wrapper = mount(DailyReportPanel, { attachTo: document.body, global: { stubs: { transition: false } } })
    await flushPromises()
    const input = document.querySelector<HTMLTextAreaElement>('.daily-body textarea')!
    input.focus()
    expect(document.activeElement).toBe(input)
    const outside = document.createElement('input')
    document.body.append(outside); outside.focus()
    expect(document.activeElement).toBe(outside)
    expect(wrapper.find('main[aria-label="工作日报"]').exists()).toBe(true)
    expect(document.querySelector('[aria-modal="true"]')).toBeNull()
    await button('日报提示词').trigger('click'); await flushPromises()
    const prompt = document.querySelector<HTMLTextAreaElement>('[aria-label="自定义日报提示词"] textarea')!
    prompt.focus()
    expect(document.activeElement).toBe(prompt)
    prompt.value = '只写实际完成的工作'
    prompt.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(document.activeElement).toBe(prompt)
  })
  it('loads local records without calling AI, generates a local draft and copies editable text', async () => {
    const { app, mock } = await open()
    expect(app.find('.daily-timeline').text()).toContain(activity.note)
    expect(app.find('.daily-section-heading').text()).toContain('1 次 · 1 项任务')
    expect(mock.GenerateDailyReport).not.toHaveBeenCalled()
    expect(app.findAllComponents(NButton).some(item => item.text() === 'AI 设置')).toBe(false)
    expect(button('AI 生成日报').props('disabled')).toBe(true)
    await button('生成本地草稿').trigger('click'); await flushPromises()
    const day = mock.GetDailyReport.mock.calls[0][0]
    expect(mock.GenerateDailyReport).toHaveBeenCalledWith(day, false)
    expect((app.find('.daily-body textarea').element as HTMLTextAreaElement).value).toBe('生成的日报正文')
    await app.find('.daily-body textarea').setValue('修改后可提交的日报')
    await button('复制日报').trigger('click'); await flushPromises()
    expect(mock.CopyText).toHaveBeenCalledWith('修改后可提交的日报')
    expect(await (app.vm as unknown as { prepareToLeave: () => Promise<boolean> }).prepareToLeave()).toBe(true)
    expect(mock.SaveDailyReport).toHaveBeenCalledWith(day, '修改后可提交的日报')
  })
  it('saves the old day before switching and keeps the panel open if saving fails', async () => {
    const { app, mock } = await open('原日报')
    const oldDay = mock.GetDailyReport.mock.calls[0][0]
    await app.find('.daily-body textarea').setValue('未保存的进展')
    mock.SaveDailyReport.mockRejectedValueOnce('磁盘写入失败')
    await app.find('#daily-date').trigger('click'); await flushPromises()
    app.findComponent(NDatePicker).vm.$emit('update:formattedValue', '2026-10-15'); await flushPromises()
    expect(app.find('[role="alert"]').text()).toContain('磁盘写入失败')
    expect(mock.GetDailyReport).toHaveBeenCalledTimes(1)
    expect(app.find('#daily-date').text()).toBe(oldDay)
    expect((app.find('.daily-body textarea').element as HTMLTextAreaElement).value).toBe('未保存的进展')
    await app.find('#daily-date').trigger('click'); await flushPromises()
    app.findComponent(NDatePicker).vm.$emit('update:formattedValue', '2026-10-14'); await flushPromises()
    expect(mock.SaveDailyReport).toHaveBeenLastCalledWith(oldDay, '未保存的进展')
    expect(mock.GetDailyReport).toHaveBeenLastCalledWith('2026-10-14')
  })
  it('saves edits before continuing, appends fresh work and keeps regeneration separate', async () => {
    const { app, mock } = await open('原日报')
    expect(button('补充本地草稿')).toBeDefined()
    expect(button('AI 续写日报').props('disabled')).toBe(true)
    await app.find('.daily-body textarea').setValue('手工修改后的原日报')
    mock.SaveDailyReport.mockRejectedValueOnce('保存失败')
    await button('补充本地草稿').trigger('click'); await flushPromises()
    expect(mock.ContinueDailyReport).not.toHaveBeenCalled()
    mock.ContinueDailyReport.mockResolvedValueOnce(report('手工修改后的原日报\n\n补充工作\n完成新工作'))
    await button('补充本地草稿').trigger('click'); await flushPromises()
    expect(mock.SaveDailyReport).toHaveBeenLastCalledWith(mock.GetDailyReport.mock.calls[0][0], '手工修改后的原日报')
    expect(mock.ContinueDailyReport).toHaveBeenCalledWith(mock.GetDailyReport.mock.calls[0][0], false)
    expect(mock.GenerateDailyReport).not.toHaveBeenCalled()
    expect((app.find('.daily-body textarea').element as HTMLTextAreaElement).value).toContain('手工修改后的原日报\n\n补充工作')
    mock.ContinueDailyReport.mockResolvedValueOnce({ ...report('手工修改后的原日报\n\n补充工作\n完成新工作'), notice: '没有新增操作记录，现有日报已保留' })
    await button('补充本地草稿').trigger('click'); await flushPromises()
    expect(app.find('[role="alert"]').exists()).toBe(false)
    expect(app.find('.daily-toast[role="status"]').text()).toBe('没有新增操作记录，现有日报已保留')
    expect((app.find('.daily-body textarea').element as HTMLTextAreaElement).value).toContain('完成新工作')
  })
  it('continues with AI without regenerating or replacing the existing draft on cancellation', async () => {
    const mock = setup('原日报')
    mock.GetDailyAISettings.mockResolvedValue({ ...config, base_url: 'http://127.0.0.1:1234/v1', model: 'local' })
    wrapper = mount(DailyReportPanel, { attachTo: document.body, global: { stubs: { teleport: true } } })
    await flushPromises()
    let reject!: (error: string) => void
    mock.ContinueDailyReport.mockImplementationOnce(() => new Promise((_resolve, fail) => { reject = fail }))
    await button('AI 续写日报').trigger('click'); await flushPromises()
    expect(mock.ContinueDailyReport).toHaveBeenCalledWith(mock.GetDailyReport.mock.calls[0][0], true)
    expect(mock.GenerateDailyReport).not.toHaveBeenCalled()
    await button('取消生成').trigger('click'); await flushPromises()
    reject('已取消生成，原日报已保留。'); await flushPromises()
    expect((wrapper.find('.daily-body textarea').element as HTMLTextAreaElement).value).toBe('原日报')
  })
  it('edits the report prompt independently, preserves failed edits and protects unsaved changes', async () => {
    const { app, mock } = await open()
    await button('日报提示词').trigger('click'); await flushPromises()
    const input = app.find('[aria-label="自定义日报提示词"] textarea')
    await input.setValue('只写今日完成和风险')
    expect(app.emitted('lock')?.at(-1)).toEqual([true])
    expect(await (app.vm as unknown as { prepareToLeave: () => Promise<boolean> }).prepareToLeave()).toBe(false)
    mock.SaveDailyReportPrompt.mockRejectedValueOnce('保存提示词失败')
    await app.find('form').trigger('submit'); await flushPromises()
    expect(app.find('[role="alert"]').text()).toContain('保存提示词失败')
    expect((input.element as HTMLTextAreaElement).value).toBe('只写今日完成和风险')
    await app.find('form').trigger('submit'); await flushPromises()
    expect(mock.SaveDailyReportPrompt).toHaveBeenLastCalledWith('只写今日完成和风险')
    expect(mock.SaveDailyAISettings).not.toHaveBeenCalled()
    expect(app.emitted('lock')?.at(-1)).toEqual([false])
    await button('恢复默认提示词').trigger('click')
    expect((input.element as HTMLTextAreaElement).value).toBe('默认提示词')
    await button('返回').trigger('click'); await flushPromises()
    expect(app.find('.daily-editor').exists()).toBe(true)
    await button('日报提示词').trigger('click')
    expect((app.find('[aria-label="自定义日报提示词"] textarea').element as HTMLTextAreaElement).value).toBe('只写今日完成和风险')
  })
  it('keeps AI errors visible and supports cancel without losing the existing draft', async () => {
    const mock = setup()
    mock.GetDailyAISettings.mockResolvedValue({ ...config, base_url: 'http://127.0.0.1:1234/v1', model: 'local' })
    wrapper = mount(DailyReportPanel, { attachTo: document.body, global: { stubs: { teleport: true } } })
    const app = wrapper
    await flushPromises()
    let reject!: (error: string) => void
    mock.GenerateDailyReport.mockImplementationOnce(() => new Promise((_resolve, fail) => { reject = fail }))
    await button('AI 生成日报').trigger('click'); await flushPromises()
    expect(app.find('#daily-date').attributes('disabled')).toBeDefined()
    await button('取消生成').trigger('click'); await flushPromises()
    expect(mock.CancelDailyReport).toHaveBeenCalledTimes(1)
    reject('已取消 AI 生成，原日报已保留'); await flushPromises()
    expect(app.find('[role="alert"]').text()).toContain('已取消 AI 生成')
    expect((app.find('.daily-body textarea').element as HTMLTextAreaElement).value).toBe('')
  })
})

describe('manual task work entry', () => {
  it('records actual progress without pushing task changes remotely and retains failed input', async () => {
    const mock = setup()
    wrapper = mount(TaskWorkEntry, { props: { documentKey: 'key', sheet: '业务侧', row: 88, disabled: false } })
    await wrapper.find('textarea').setValue('完成联调，等待验证')
    mock.RecordTaskWork.mockRejectedValueOnce('保存失败')
    await button('记入日报').trigger('click'); await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe('保存失败')
    expect(wrapper.find('textarea').element.value).toBe('完成联调，等待验证')
    await button('记入日报').trigger('click'); await flushPromises()
    expect(mock.RecordTaskWork).toHaveBeenLastCalledWith('key', '业务侧', 88, '完成联调，等待验证')
    expect(wrapper.find('textarea').element.value).toBe('')
    expect(wrapper.find('[role="status"]').text()).toContain('已记入今天')
    expect(mock.GenerateDailyReport).not.toHaveBeenCalled()
  })
})
