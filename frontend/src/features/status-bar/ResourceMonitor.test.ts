// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ResourceMonitor from './ResourceMonitor.vue'
import { api } from '../../shared/api/api'

vi.mock('../../shared/api/api', () => ({ api: { GetResourceUsage: vi.fn() } }))
let wrapper: ReturnType<typeof mount> | undefined
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers(); vi.resetAllMocks() })

describe('resource monitor', () => {
  it('refreshes readings, handles unavailable samples and stops when unmounted', async () => {
    vi.useFakeTimers()
    const getUsage = vi.mocked(api.GetResourceUsage)
    getUsage.mockResolvedValueOnce({ cpu_percent: null, memory_bytes: 64000000 })
      .mockResolvedValueOnce({ cpu_percent: 12.34, memory_bytes: 80500000 })
      .mockRejectedValueOnce(new Error('unavailable'))
      .mockResolvedValue({ cpu_percent: 0, memory_bytes: 64000000 })
    wrapper = mount(ResourceMonitor)
    await flushPromises()
    expect(wrapper.text()).toContain('CPU —')
    expect(wrapper.text()).toContain('64.0 MB')
    await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.text()).toContain('12.3%')
    expect(wrapper.text()).toContain('80.5 MB')
    await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.text()).toContain('内存 —')
    await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.text()).toContain('0.0%')
    wrapper.unmount(); wrapper = undefined
    await vi.advanceTimersByTimeAsync(10000)
    expect(getUsage).toHaveBeenCalledTimes(4)
  })

  it('does not overlap requests or restart polling after a late result', async () => {
    vi.useFakeTimers()
    let finish!: (value: { cpu_percent: number; memory_bytes: number }) => void
    const getUsage = vi.mocked(api.GetResourceUsage).mockImplementation(() => new Promise(resolve => { finish = resolve }))
    wrapper = mount(ResourceMonitor)
    await vi.advanceTimersByTimeAsync(10000)
    expect(getUsage).toHaveBeenCalledTimes(1)
    wrapper.unmount(); wrapper = undefined
    finish({ cpu_percent: 1, memory_bytes: 1 })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getUsage).toHaveBeenCalledTimes(1)
  })
})
