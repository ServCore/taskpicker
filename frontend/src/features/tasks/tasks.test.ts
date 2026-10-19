import { describe, expect, it } from 'vitest'
import { copyTask, documentStats, isCompleted, isDueToday, localDay, priorityTone, tasksFor, tone } from './tasks'
import type { Document, TaskSheet } from '../../shared/types/types'

const sheet: TaskSheet = {
  name: '示例测试问题清单', header_row: 1, scanned: 109,
  headers: ['三级功能', '问题描述', '处理状态', '责任人', '问题截图1', '备注 ', '预计修复时间'],
  rows: [{ number: 4, values: ['麦克风按钮', '无法访问麦克风\n第二行描述', '暂不处理', '王小明', ' ', '需要 HTTPS', '2026-10-15'],
    pictures: [{ row: 4, column: 5, file: 'a'.repeat(64) + '.png', width: 400, height: 150 }] }],
}
describe('task presentation', () => {
  it('keeps all urgency and priority fields with their original labels and values', () => {
    const [task] = tasksFor({ ...sheet, headers: [...sheet.headers, ' 紧急 状态 ', '优先级', '严重程度'],
      rows: [{ ...sheet.rows[0], values: [...sheet.rows[0].values, '紧急', 'P1', '自定义级别'] }] })
    expect(task.priorities).toEqual([{ label: '紧急 状态', value: '紧急' }, { label: '优先级', value: 'P1' }, { label: '严重程度', value: '自定义级别' }])
    expect(task.fields.some(field => field.label === '优先级')).toBe(false)
    expect(copyTask(task)).toContain('优先级：P1')
    expect(copyTask(task)).toContain('严重程度：自定义级别')
    expect(tasksFor(sheet)[0].priorities).toEqual([])
  })
  it('highlights explicit priority levels without misclassifying non-urgent or unknown text', () => {
    for (const value of ['紧急', 'P0', 'critical']) expect(priorityTone(value)).toBe('urgent')
    for (const value of ['高', 'p1', '严重']) expect(priorityTone(value)).toBe('high')
    expect(priorityTone('P2')).toBe('medium')
    expect(priorityTone('低')).toBe('low')
    for (const value of ['不紧急', '非紧急', '不高', '普通', '自定义', '']) expect(priorityTone(value)).toBe('neutral')
  })
  it('keeps multiline content and source row while presenting an abbreviated list title', () => {
    const [task] = tasksFor(sheet)
    expect(task.description).toBe('无法访问麦克风\n第二行描述')
    expect(task.title).toBe('无法访问麦克风 第二行描述')
    expect(task.context).toBe('麦克风按钮')
    expect(task.row.number).toBe(4)
    expect(task.row.pictures).toHaveLength(1)
  })
  it('keeps original metadata and notes without rendering empty image placeholders as text', () => {
    const [task] = tasksFor(sheet)
    expect(task.fields).toContainEqual({ label: '责任人', value: '王小明' })
    expect(task.fields.some(field => field.label === '问题截图1')).toBe(false)
    expect(task.notes).toEqual([{ label: '备注', value: '需要 HTTPS' }])
    expect(task.search).toContain('需要 https')
    expect(copyTask(task)).toContain('来源：示例测试问题清单 · 第 4 行')
  })
  it('supports smart sheet descriptions, multiple owners and missing statuses', () => {
    const [task] = tasksFor({ name: '业务侧', header_row: 1, scanned: 134,
      headers: ['功能', '描述', '负责人员', '完成状态'],
      rows: [{ number: 88, values: ['山大', '对接山大现有系统', '王小明、赵小金', null], pictures: [] }],
    })
    expect(task.title).toBe('对接山大现有系统')
    expect(task.status).toBe('未标记')
    expect(task.fields).toContainEqual({ label: '负责人员', value: '王小明、赵小金' })
  })
  it('counts all sheets and their actual images', () => {
    expect(documentStats({ sheets: [sheet, { ...sheet, rows: [] }] } as Document)).toEqual({ tasks: 1, pictures: 1 })
  })
  it('maps known states without treating an unknown state as complete', () => {
    expect(tone('已修复')).toBe('green')
    expect(tone('未处理')).toBe('amber')
    expect(tone('进行中')).toBe('blue')
    expect(tone('暂不处理')).toBe('gray')
  })
})

describe('completion status', () => {
  it('excludes terminal states while keeping pending and unknown statuses', () => {
    for (const status of ['已完成', '已修复', '已关闭', '已解决', '已验收', ' 已 完 成 ', 'DONE', 'closed']) expect(isCompleted(status)).toBe(true)
    for (const status of ['未完成', '进行中', '验证未通过', '已修复待发布', '已完成待验收', '待验证', '暂不处理', '自定义', '']) expect(isCompleted(status)).toBe(false)
  })
  it('prefers task status over unrelated status columns', () => {
    const [task] = tasksFor({ ...sheet, headers: ['发布状态', '处理状态'], rows: [{ number: 1, values: ['已完成', '进行中'], pictures: [] }] })
    expect(task.status).toBe('进行中')
  })
})

describe('status choices', () => {
  it('uses remote status choices and the appropriate fallback for each sheet type', async () => {
    const { taskStatusChoices } = await import('./tasks')
    expect(taskStatusChoices(sheet)).toContain('暂不处理')
    expect(taskStatusChoices({ ...sheet, headers: ['完成状态'] })).toEqual(['未开始', '进行中', '已完成'])
    expect(taskStatusChoices({ ...sheet, status_options: ['待处理', '暂不处理'] })).toEqual(['待处理', '暂不处理'])
  })
})


describe('today deadline reminder', () => {
  it('uses deadline columns and ignores testing/start dates', () => {
    const task = (headers: string[], values: string[]) => tasksFor({ name: '任务', header_row: 1, scanned: 1, headers,
      rows: [{ number: 2, values, pictures: [] }] })[0]
    for (const label of ['截止日期', '截止时间', '计划结束时间', '预计修复时间']) {
      const row = task(['处理状态', '测试时间', label], ['修复中', '2026-10-15', '2026-10-16'])
      expect(row.date).toBe('2026-10-16')
      expect(isDueToday(row, '2026-10-16')).toBe(true)
    }
    const testOnly = task(['处理状态', '测试时间'], ['修复中', '2026-10-16'])
    expect(testOnly.date).toBe('2026-10-16')
    expect(testOnly.dueDate).toBe('')
    expect(isDueToday(testOnly, '2026-10-16')).toBe(false)
    const startsToday = task(['处理状态', '计划开始时间', '计划结束时间'], ['修复中', '2026-10-16', '2026-10-17'])
    expect(isDueToday(startsToday, '2026-10-16')).toBe(false)
  })

  it('compares calendar dates, accepts common formats and rejects invalid dates', () => {
    for (const dueDate of ['2026-10-16', '2026/10/16', '2026.10.16', '2026年10月16日', '2026-10-16 18:00', '2026-10-16T00:00:00.123']) {
      expect(isDueToday({ dueDate, status: '进行中' }, '2026-10-16')).toBe(true)
    }
    for (const dueDate of ['', '待定', '2026-10-15', '2026-10-17', '2026-02-30', '2026-10-16 25:00', '2026-10-16 备注']) {
      expect(isDueToday({ dueDate, status: '进行中' }, '2026-10-16')).toBe(false)
    }
    expect(isDueToday({ dueDate: '2026-02-29', status: '进行中' }, '2026-03-01')).toBe(false)
    expect(isDueToday({ dueDate: '2028-02-29', status: '进行中' }, '2028-02-29')).toBe(true)
    const timestamp = '2026-10-16T23:30:00Z'
    expect(isDueToday({ dueDate: timestamp, status: '进行中' }, localDay(new Date(timestamp)))).toBe(true)
    expect(localDay(new Date(2026, 9, 16))).toBe('2026-10-16')
  })

  it('does not warn about completed tasks', () => {
    for (const status of ['已修复', '已完成', '已关闭', 'done']) {
      expect(isDueToday({ dueDate: '2026-10-16', status }, '2026-10-16')).toBe(false)
    }
  })
})
