import { afterEach, describe, expect, it, vi } from 'vitest'
import { createScheduleRunner } from './runner'
import type { SyncSchedule } from '../../shared/types/types'

const job = (id = 'a', enabled = true): SyncSchedule => ({ id, project_id: id, interval_minutes: 1, revision: 'r1', enabled, next_run_at: enabled ? new Date(Date.now() + 60_000).toISOString() : '', last_run_at: '', last_result: '' })
function setup(initial?: SyncSchedule[]) {
  let items = initial ?? [job()]
  let allowed = true
  const run = vi.fn(async (_item: SyncSchedule) => '拉取完成')
  const complete = vi.fn(async (item: SyncSchedule, result: string) => {
    items = items.map(value => value.id === item.id && value.revision === item.revision ? { ...value, next_run_at: value.enabled ? new Date(Date.now() + 60_000).toISOString() : '', last_result: result } : value)
  })
  const onRunning = vi.fn(), onError = vi.fn()
  const runner = createScheduleRunner({ items: () => items, canRun: () => allowed, run, complete, onRunning, onError })
  return { runner, run, complete, onError, onRunning, setItems: (value: SyncSchedule[]) => { items = value; runner.wake() }, getItems: () => items, block: (value: boolean) => { allowed = !value } }
}
afterEach(() => { vi.useRealTimers() })

describe('project recurring pulls', () => {
  it('waits the interval, persists completion and rearms from completion', async () => {
    vi.useFakeTimers(); const s = setup(); s.runner.wake()
    await vi.advanceTimersByTimeAsync(59_999); expect(s.run).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1); expect(s.run).toHaveBeenCalledTimes(1); expect(s.complete).toHaveBeenCalledWith(expect.objectContaining({ id: 'a' }), '拉取完成')
    await vi.advanceTimersByTimeAsync(60_000); expect(s.run).toHaveBeenCalledTimes(2); s.runner.stop()
  })
  it('serializes overdue projects and coalesces a long sleep', async () => {
    vi.useFakeTimers(); const s = setup([job('a'), job('b')]); let finish!: (value: string) => void
    s.run.mockImplementationOnce(() => new Promise(resolve => { finish = resolve })); s.runner.wake()
    vi.setSystemTime(Date.now() + 12 * 3600_000); s.runner.wake()
    expect(s.run).toHaveBeenCalledTimes(1)
    s.runner.wake(); expect(await s.runner.runNow('b')).toBe(false)
    finish('first'); await vi.advanceTimersByTimeAsync(1)
    expect(s.run).toHaveBeenCalledTimes(2); expect(s.run.mock.calls.map(call => (call as unknown as [SyncSchedule])[0].id)).toEqual(['a', 'b'])
    await vi.advanceTimersByTimeAsync(59_999); expect(s.run).toHaveBeenCalledTimes(4); s.runner.stop()
  })
  it('defers while editing or another sync owns the queue', async () => {
    vi.useFakeTimers(); const s = setup(); s.block(true); s.runner.wake()
    await vi.advanceTimersByTimeAsync(120_000); expect(s.run).not.toHaveBeenCalled()
    expect(await s.runner.runNow('a')).toBe(false)
    s.block(false); s.runner.wake(); await vi.advanceTimersByTimeAsync(1); expect(s.run).toHaveBeenCalledTimes(1); s.runner.stop()
  })
  it('supports manual paused runs, while stale completion preserves pause and edits', async () => {
    vi.useFakeTimers(); const s = setup([job('a', false)])
    s.runner.wake(); await vi.advanceTimersByTimeAsync(120_000); expect(s.run).not.toHaveBeenCalled()
    await s.runner.runNow('a'); expect(s.getItems()[0].enabled).toBe(false); expect(s.getItems()[0].last_result).toBe('拉取完成')
    let finish!: (value: string) => void
    s.setItems([job()]); s.run.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await vi.advanceTimersByTimeAsync(60_000)
    s.setItems([{ ...job('a', false), revision: 'r2' }]); finish('older round'); await vi.advanceTimersByTimeAsync(120_000)
    expect(s.getItems()[0].last_result).toBe(''); expect(s.run).toHaveBeenCalledTimes(2); s.runner.stop()
  })
  it('backs off persistence failures and records source errors', async () => {
    vi.useFakeTimers(); const s = setup(); s.run.mockRejectedValueOnce(new Error('需要登录')); s.complete.mockRejectedValueOnce(new Error('磁盘已满'))
    s.runner.wake(); await vi.advanceTimersByTimeAsync(60_000)
    expect(s.complete).toHaveBeenCalledWith(expect.anything(), expect.stringContaining('需要登录')); expect(s.onError).toHaveBeenCalledTimes(1)
    s.runner.wake(); await vi.advanceTimersByTimeAsync(59_999); expect(s.run).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1); expect(s.run).toHaveBeenCalledTimes(2); s.runner.stop()
  })
  it('stops timers and never persists an in-flight run after unmount', async () => {
    vi.useFakeTimers(); const s = setup(); let finish!: (value: string) => void
    s.run.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    s.runner.wake(); await vi.advanceTimersByTimeAsync(60_000); s.runner.stop(); finish('done')
    await vi.advanceTimersByTimeAsync(120_000); expect(s.complete).not.toHaveBeenCalled(); expect(s.run).toHaveBeenCalledTimes(1)
  })
})
