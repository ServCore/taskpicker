// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import WorkdayCountdown from './WorkdayCountdown.vue'

let wrapper: ReturnType<typeof mount> | undefined
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.restoreAllMocks(); vi.useRealTimers() })
function openAt(hour: number, minute: number, second = 0) {
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] })
  vi.setSystemTime(new Date(2026, 9, 16, hour, minute, second))
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  wrapper = mount(WorkdayCountdown)
  return wrapper
}
describe('status bar countdown', () => {
  it('counts down to lunch and switches automatically through the lunch boundary', async () => {
    const app = openAt(11, 29, 58)
    expect(app.text()).toContain('距离午饭还有00:00:02')
    await vi.advanceTimersByTimeAsync(1000)
    expect(app.find('time').text()).toBe('00:00:01')
    await vi.advanceTimersByTimeAsync(1000)
    expect(app.text()).toBe('午休时间')
    expect(app.find('time').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(2 * 60 * 60 * 1000)
    expect(app.text()).toContain('距离下班还有04:00:00')
  })
  it('switches to off-hours and starts again the next morning', async () => {
    const app = openAt(17, 29, 59)
    expect(app.find('time').text()).toBe('00:00:01')
    await vi.advanceTimersByTimeAsync(1000)
    expect(app.text()).toBe('非工作时段')
    await vi.advanceTimersByTimeAsync(15 * 60 * 60 * 1000)
    expect(app.text()).toContain('距离午饭还有03:00:00')
  })
  it('pauses while hidden, catches up on return and clears its timer on unmount', async () => {
    const app = openAt(10, 0)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(3 * 60 * 60 * 1000)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await app.vm.$nextTick()
    expect(app.text()).toBe('午休时间')
    vi.setSystemTime(new Date(2026, 9, 16, 16, 30))
    window.dispatchEvent(new Event('focus'))
    await app.vm.$nextTick()
    expect(app.find('time').text()).toBe('01:00:00')
    app.unmount(); wrapper = undefined
    expect(vi.getTimerCount()).toBe(0)
    window.dispatchEvent(new Event('focus'))
    expect(vi.getTimerCount()).toBe(0)
  })
})
