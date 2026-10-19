export interface WorkdayState {
  phase: 'morning' | 'lunch' | 'afternoon' | 'off'
  label: string
  remainingSeconds: number | null
  nextChange: number
}

export function workdayState(now = new Date()): WorkdayState {
  const at = (hour: number, minute: number) => {
    const boundary = new Date(now)
    boundary.setHours(hour, minute, 0, 0)
    return boundary.getTime()
  }
  const current = now.getTime()
  const morning = at(8, 30), lunch = at(11, 30), afternoon = at(13, 30), finish = at(17, 30)
  if (current < morning) return { phase: 'off', label: '非工作时段', remainingSeconds: null, nextChange: morning }
  if (current < lunch) return { phase: 'morning', label: '距离午饭还有', remainingSeconds: Math.ceil((lunch - current) / 1000), nextChange: lunch }
  if (current < afternoon) return { phase: 'lunch', label: '午休时间', remainingSeconds: null, nextChange: afternoon }
  if (current < finish) return { phase: 'afternoon', label: '距离下班还有', remainingSeconds: Math.ceil((finish - current) / 1000), nextChange: finish }
  const tomorrow = new Date(now)
  tomorrow.setDate(tomorrow.getDate() + 1)
  tomorrow.setHours(8, 30, 0, 0)
  return { phase: 'off', label: '非工作时段', remainingSeconds: null, nextChange: tomorrow.getTime() }
}

export function countdownText(seconds: number): string {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(Math.floor(seconds / 3600))}:${pad(Math.floor(seconds % 3600 / 60))}:${pad(seconds % 60)}`
}
