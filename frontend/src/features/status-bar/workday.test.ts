import { describe, expect, it } from 'vitest'
import { countdownText, workdayState } from './workday'

const at = (hour: number, minute: number, second = 0, ms = 0) => new Date(2026, 9, 16, hour, minute, second, ms)
describe('workday schedule', () => {
  it.each([
    [8, 29, 59, 'off', null],
    [8, 30, 0, 'morning', 10800],
    [11, 29, 59, 'morning', 1],
    [11, 30, 0, 'lunch', null],
    [13, 29, 59, 'lunch', null],
    [13, 30, 0, 'afternoon', 14400],
    [17, 29, 59, 'afternoon', 1],
    [17, 30, 0, 'off', null],
  ] as const)('uses the correct phase at %i:%i:%i', (hour, minute, second, phase, remainingSeconds) => {
    expect(workdayState(at(hour, minute, second))).toMatchObject({ phase, remainingSeconds })
  })
  it('schedules the next local boundary, including the following morning', () => {
    expect(workdayState(at(8, 0)).nextChange).toBe(at(8, 30).getTime())
    expect(workdayState(at(12, 0)).nextChange).toBe(at(13, 30).getTime())
    expect(workdayState(at(18, 0)).nextChange).toBe(new Date(2026, 9, 17, 8, 30).getTime())
    expect(workdayState(at(11, 29, 59, 999)).remainingSeconds).toBe(1)
  })
  it('formats the remaining hours, minutes and seconds', () => {
    expect(countdownText(10800)).toBe('03:00:00')
    expect(countdownText(3661)).toBe('01:01:01')
    expect(countdownText(1)).toBe('00:00:01')
    expect(countdownText(0)).toBe('00:00:00')
  })
})
