// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NButton } from 'naive-ui'
import AISettingsPage from './AISettingsPage.vue'
import type { DailyAISettings } from '../../shared/types/types'
import type { api } from '../../shared/api/api'

let wrapper: ReturnType<typeof mount> | undefined
const config: DailyAISettings = { base_url: 'https://ai.example/v1', model: 'existing-model', prompt: '已有提示词', has_api_key: true, default_prompt: '默认提示词' }
async function open(fail = false) {
  const mock = {
    GetDailyAISettings: fail ? vi.fn().mockRejectedValueOnce('读取失败').mockResolvedValue(config) : vi.fn().mockResolvedValue(config),
    SaveDailyAISettings: vi.fn().mockImplementation(async value => ({ ...config, ...value })),
  }
  window.go = { main: { App: mock as unknown as typeof api } }
  wrapper = mount(AISettingsPage, { attachTo: document.body })
  await flushPromises()
  return { app: wrapper, mock }
}
function button(label: string) { return wrapper!.findAllComponents(NButton).find(item => item.text() === label)! }
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = ''; delete window.go })

describe('independent AI settings page', () => {
  it('loads existing configuration, accepts focused typing and saves without returning the key', async () => {
    const { app, mock } = await open()
    expect((app.find('[aria-label="AI 模型名称"] input').element as HTMLInputElement).value).toBe('existing-model')
    for (const [label, value] of [['AI API 基础地址', 'https://new.example/v1'], ['AI 模型名称', 'my-model'], ['AI API Key', 'fake-test-key']]) {
      const field = app.find(`[aria-label="${label}"] input`)
      ;(field.element as HTMLInputElement).focus()
      await field.setValue(value)
      expect(document.activeElement).toBe(field.element)
    }
    expect(app.find('[aria-label="自定义日报提示词"]').exists()).toBe(false)
    expect(button('返回').props('disabled')).toBe(true)
    expect(app.emitted('lock')!.at(-1)).toEqual([true])
    await app.find('form').trigger('submit'); await flushPromises()
    expect(mock.SaveDailyAISettings).toHaveBeenCalledWith({ base_url: 'https://new.example/v1', model: 'my-model', api_key: 'fake-test-key', clear_api_key: false })
    expect((app.find('[aria-label="AI API Key"] input').element as HTMLInputElement).value).toBe('')
    expect(app.emitted('lock')!.at(-1)).toEqual([false])
    await button('返回').trigger('click')
    expect(app.emitted('close')).toHaveLength(1)
  })
  it('keeps saved credentials when left blank, preserves failed edits, and supports discard', async () => {
    const { app, mock } = await open()
    await app.find('[aria-label="AI 模型名称"] input').setValue('new-model')
    mock.SaveDailyAISettings.mockRejectedValueOnce('保存失败')
    await app.find('form').trigger('submit'); await flushPromises()
    expect(app.find('[role="alert"]').text()).toContain('保存失败')
    expect((app.find('[aria-label="AI 模型名称"] input').element as HTMLInputElement).value).toBe('new-model')
    expect(mock.SaveDailyAISettings).toHaveBeenLastCalledWith(expect.objectContaining({ api_key: '', clear_api_key: false }))
    expect(button('返回').props('disabled')).toBe(true)
    await button('放弃修改').trigger('click'); await flushPromises()
    expect((app.find('[aria-label="AI 模型名称"] input').element as HTMLInputElement).value).toBe('existing-model')
    expect(button('返回').props('disabled')).toBe(false)
  })
  it('can retry a settings load without replacing saved settings with empty fields', async () => {
    const { app, mock } = await open(true)
    expect(app.find('form').exists()).toBe(false)
    expect(mock.SaveDailyAISettings).not.toHaveBeenCalled()
    await button('重试').trigger('click'); await flushPromises()
    expect(app.find('form').exists()).toBe(true)
  })
})
