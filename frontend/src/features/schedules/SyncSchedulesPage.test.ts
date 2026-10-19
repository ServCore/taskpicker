// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NButton, NInputNumber, NPopconfirm, NSelect, NSwitch } from 'naive-ui'
import SyncSchedulesPage from './SyncSchedulesPage.vue'
import { api } from '../../shared/api/api'
import type { Document, Project, SyncSchedule } from '../../shared/types/types'
const project: Project = { id: 'a'.repeat(24), name: 'yiqiban', local_path: '', document_keys: ['doc1', 'doc2'] }
const documents = [{ key: 'doc1', source_url: 'https://docs.qq.com/sheet/test' }, { key: 'doc2', source_url: '' }] as Document[]
const item: SyncSchedule = { id: 'b'.repeat(24), project_id: project.id, interval_minutes: 60, enabled: true, revision: 'c'.repeat(24), next_run_at: '2026-10-18T08:00:00Z', last_run_at: '', last_result: '' }
let wrapper: ReturnType<typeof mount> | undefined
function setup(schedules: SyncSchedule[] = []) {
 const mock = { SaveSyncSchedule: vi.fn(), DeleteSyncSchedule: vi.fn() }
 window.go = { main: { App: mock as unknown as typeof api } }
 wrapper = mount(SyncSchedulesPage, { props: { projects: [project], documents, schedules, runningID: '', blocked: false, loadError: '' }, attachTo: document.body, global: { stubs: { teleport: true, transition: false } } })
 return { panel: wrapper, mock }
}
function button(label: string) { return wrapper!.findAllComponents(NButton).find(item => item.text() === label)! }
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = ''; delete window.go })

it('saves the chosen project and time unit and retains a rejected draft', async () => {
 const { panel, mock } = setup()
 await button('新建定时任务').trigger('click')
 expect(panel.text()).toContain('1 份在线文档')
 panel.findComponent(NInputNumber).vm.$emit('update:value', 2)
 panel.findAllComponents(NSelect)[1].vm.$emit('update:value', 60)
 panel.findComponent(NSwitch).vm.$emit('update:value', false)
 await flushPromises()
 expect(panel.emitted('lock')?.at(-1)).toEqual([true])
 mock.SaveSyncSchedule.mockRejectedValueOnce('写入失败')
 await panel.find('form').trigger('submit'); await flushPromises()
 expect(panel.find('[role="alert"]').text()).toContain('写入失败')
 expect(panel.find('form').exists()).toBe(true)
 mock.SaveSyncSchedule.mockResolvedValue([item])
 await panel.find('form').trigger('submit'); await flushPromises()
 expect(mock.SaveSyncSchedule).toHaveBeenLastCalledWith({ id: '', revision: '', project_id: project.id, interval_minutes: 120, enabled: false })
 expect(panel.emitted('updated')?.[0]).toEqual([[item]])
 expect(panel.find('form').exists()).toBe(false)
 expect(panel.emitted('lock')?.at(-1)).toEqual([false])
})
it('can pause an active round, run a paused job and delete without deleting project data', async () => {
 const { panel, mock } = setup([item]); mock.SaveSyncSchedule.mockResolvedValue([{...item,enabled:false}])
 await panel.setProps({ runningID: item.id, blocked: true })
 panel.findComponent(NSwitch).vm.$emit('update:value', false); await flushPromises()
 expect(mock.SaveSyncSchedule).toHaveBeenCalledWith(expect.objectContaining({ id: item.id, revision: item.revision, enabled: false }))
 expect(button('立即拉取').attributes('disabled')).toBeDefined()
 await panel.setProps({ runningID: '', blocked: false, schedules: [{...item, enabled:false, next_run_at:''}] })
 await button('立即拉取').trigger('click'); expect(panel.emitted('run')).toEqual([[item.id]])
 mock.DeleteSyncSchedule.mockResolvedValue([])
 panel.findComponent(NPopconfirm).vm.$emit('positive-click'); await flushPromises()
 expect(mock.DeleteSyncSchedule).toHaveBeenCalledWith(item.id, item.revision)
 expect(panel.emitted('updated')?.at(-1)).toEqual([[]])
})
it('validates intervals, discards changes and prevents duplicates', async () => {
 const { panel, mock } = setup([item])
 await button('编辑').trigger('click')
 panel.findComponent(NInputNumber).vm.$emit('update:value', 43201)
 await panel.find('form').trigger('submit'); await flushPromises()
 expect(panel.find('[role="alert"]').text()).toContain('1 分钟至 30 天')
 expect(mock.SaveSyncSchedule).not.toHaveBeenCalled()
 await panel.findAllComponents(NButton).find(button => button.text() === '返回' && button.attributes('disabled') === undefined)!.trigger('click')
 expect(panel.find('form').exists()).toBe(false)
 await button('新建定时任务').trigger('click')
 expect(panel.findComponent(NSelect).props('options')).toEqual([{label:project.name,value:project.id,disabled:true}])
})
it('shows load failures with an explicit reload action', async () => {
 const { panel } = setup()
 await panel.setProps({loadError:'定时配置无法解析，原文件已保留。'})
 expect(button('新建定时任务').attributes('disabled')).toBeDefined()
 await button('重新加载').trigger('click'); expect(panel.emitted('refresh')).toHaveLength(1)
})
