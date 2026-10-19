// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NButton, NSwitch } from 'naive-ui'
import ProjectSettingsPanel from './ProjectSettingsPanel.vue'
import { api } from '../../shared/api/api'
import type { Project } from '../../shared/types/types'
const project: Project = { id: 'a'.repeat(24), name: 'yiqiban', local_path: '/existing/project', document_keys: ['doc1'] }
let wrapper: ReturnType<typeof mount> | undefined
function setup(current: Project | null = project) {
  const mock = { SaveProject: vi.fn(), PickProjectDirectory: vi.fn() }
  window.go = { main: { App: mock as unknown as typeof api } }
  wrapper = mount(ProjectSettingsPanel, { props: { project: current }, attachTo: document.body, global: { stubs: { teleport: true, transition: false } } })
  return { panel: wrapper, mock }
}
function button(label: string) { return wrapper!.findAllComponents(NButton).find(item => item.text() === label)! }
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = ''; delete window.go })

it('keeps fields focusable and retains current path when the native picker is canceled', async () => {
 const { panel, mock } = setup()
 const input = panel.find<HTMLInputElement>('input[aria-label="项目名称"]')
 input.element.focus(); expect(document.activeElement).toBe(input.element)
 mock.PickProjectDirectory.mockResolvedValue('')
 await button('选择目录').trigger('click'); await flushPromises()
 expect(mock.PickProjectDirectory).toHaveBeenCalledWith('/existing/project')
 expect(panel.find<HTMLInputElement>('input[aria-label="本地项目目录"]').element.value).toBe('/existing/project')
 await button('返回').trigger('click')
 expect(panel.emitted('close')).toHaveLength(1); expect(mock.SaveProject).not.toHaveBeenCalled()
})
it('saves project settings without changing document membership, preserving failed drafts', async () => {
 const { panel, mock } = setup()
 await panel.find('input[aria-label="项目名称"]').setValue('新项目名')
 mock.PickProjectDirectory.mockResolvedValue('/new/project')
 await button('选择目录').trigger('click'); await flushPromises()
 expect(panel.text()).not.toContain('所属文档')
 mock.SaveProject.mockRejectedValueOnce('目录不可访问')
 await button('保存项目').trigger('click'); await flushPromises()
 expect(panel.find('[role="alert"]').text()).toContain('目录不可访问')
 expect(panel.emitted('saved')).toBeUndefined()
 const updated = { ...project, name: '新项目名', local_path: '/new/project', local_paths: ['/new/project'], daily_report_enabled: true }
 mock.SaveProject.mockResolvedValue([updated])
 await button('保存项目').trigger('click'); await flushPromises()
 expect(mock.SaveProject).toHaveBeenLastCalledWith(updated)
 expect(panel.emitted('saved')?.[0]).toEqual([[updated]])
})
it('creates projects and supports clearing an existing binding', async () => {
 const { panel, mock } = setup(null)
 await button('创建项目').trigger('click'); await flushPromises()
 expect(mock.SaveProject).not.toHaveBeenCalled()
 await panel.find('input[aria-label="项目名称"]').setValue('新项目')
 mock.SaveProject.mockResolvedValue([])
 await button('创建项目').trigger('click'); await flushPromises()
 expect(mock.SaveProject).toHaveBeenCalledWith({ id: '', name: '新项目', local_path: '', local_paths: [], daily_report_enabled: false, document_keys: [] })
 panel.unmount()
 const edited = setup()
 await edited.panel.find('input[aria-label="本地项目目录"]').setValue('')
 edited.mock.SaveProject.mockResolvedValue([{...project, local_path:''}])
 await button('保存项目').trigger('click'); await flushPromises()
 expect(edited.mock.SaveProject).toHaveBeenCalledWith({...project,local_path:'', local_paths: [], daily_report_enabled: true})
})

it('saves multiple directories with the project report switch independently', async () => {
 const { panel, mock } = setup()
 await button('添加目录').trigger('click')
 await panel.find('input[aria-label="本地项目目录 2"]').setValue('/backend/project')
 panel.findComponent(NSwitch).vm.$emit('update:value', false); await flushPromises()
 mock.SaveProject.mockResolvedValue([])
 await button('保存项目').trigger('click'); await flushPromises()
 expect(mock.SaveProject).toHaveBeenCalledWith({ ...project, local_paths: ['/existing/project', '/backend/project'], daily_report_enabled: false })
})
