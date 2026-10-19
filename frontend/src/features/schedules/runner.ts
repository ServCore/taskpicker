import type { SyncSchedule } from '../../shared/types/types'

interface Options {
  items: () => SyncSchedule[]
  canRun: () => boolean
  run: (item: SyncSchedule) => Promise<string>
  complete: (item: SyncSchedule, result: string) => Promise<void>
  onRunning: (id: string) => void
  onError: (message: string) => void
}

// A single bounded timeout, rather than one interval per schedule. Long sleep,
// busy UI and overdue startup jobs are coalesced into one round per project.
export function createScheduleRunner(options: Options) {
  let timer: ReturnType<typeof setTimeout> | undefined
  let running = false, stopped = false
  const retryAfter = new Map<string, number>()
  const key = (item: SyncSchedule) => `${item.id}:${item.revision}`
  const dueAt = (item: SyncSchedule) => Math.max(Date.parse(item.next_run_at), retryAfter.get(key(item)) ?? 0)

  function arm(delay: number) { timer = setTimeout(wake, Math.max(1, Math.min(delay, 60_000))) }
  function wake() {
    clearTimeout(timer)
    if (stopped || running) return
    const live = new Set(options.items().map(key))
    for (const entry of retryAfter.keys()) if (!live.has(entry)) retryAfter.delete(entry)
    const entries = options.items().filter(item => item.enabled && Number.isFinite(Date.parse(item.next_run_at)))
      .sort((a, b) => dueAt(a) - dueAt(b) || a.id.localeCompare(b.id))
    const next = entries[0]
    if (!next) return
    const delay = dueAt(next) - Date.now()
    if (delay > 0) { arm(delay); return }
    if (!options.canRun()) { arm(15_000); return }
    void execute({ ...next })
  }

  async function execute(item: SyncSchedule) {
    running = true
    options.onRunning(item.id)
    try {
      let result: string
      try { result = await options.run(item) }
      catch (exc) { result = `拉取失败：${String(exc)}` }
      if (!stopped) {
        try {
          await options.complete(item, result)
          retryAfter.delete(key(item))
        } catch (exc) {
          // Don't hammer the source when persisting the next deadline fails.
          retryAfter.set(key(item), Date.now() + item.interval_minutes * 60_000)
          options.onError(`拉取结果未能保存，将在下一间隔重试：${String(exc)}`)
        }
      }
    } finally {
      running = false
      options.onRunning('')
      wake()
    }
  }

  return {
    wake,
    async runNow(id: string): Promise<boolean> {
      if (stopped || running || !options.canRun()) return false
      const item = options.items().find(value => value.id === id)
      if (!item) return false
      clearTimeout(timer)
      await execute({ ...item })
      return true
    },
    stop() { stopped = true; clearTimeout(timer) },
  }
}
