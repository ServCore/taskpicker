import type { Document, Task, TaskSheet } from '../../shared/types/types'

export function text(value: unknown): string {
  if (value == null) return ''
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value).trim()
}
const normalize = (value: unknown) => text(value).replace(/\s|\u200b/g, '')

export function priorityColumns(sheet?: TaskSheet): { column: number; label: string }[] {
  const names = new Set(['紧急程度', '紧急状态', '紧急级别', '紧急性', '优先级', '优先等级', '任务优先级', '问题优先级', '严重程度', '严重级别'])
  return sheet?.headers.flatMap((header, column) => names.has(normalize(header)) ? [{ column, label: text(header) }] : []) ?? []
}

export function priorityTone(value: string): 'urgent' | 'high' | 'medium' | 'low' | 'neutral' {
  const level = normalize(value).toUpperCase()
  if (['紧急', '非常紧急', '特急', '最高', '最高优先级', '重要且紧急', 'P0', 'S0', 'URGENT', 'CRITICAL', 'HIGHEST', '致命', '阻塞'].includes(level)) return 'urgent'
  if (['高', '高优先级', '高优', 'P1', 'S1', 'HIGH', '严重', '重要不紧急'].includes(level)) return 'high'
  if (['中', '中等', '中优先级', 'P2', 'S2', 'MEDIUM'].includes(level)) return 'medium'
  if (['低', '低优先级', '低优', 'P3', 'P4', 'S3', 'LOW', 'LOWEST'].includes(level)) return 'low'
  return 'neutral'
}

export function taskStatusChoices(sheet?: TaskSheet): string[] {
  if (!sheet) return []
  const presets = sheet.headers.some(header => normalize(header) === '完成状态')
    ? ['未开始', '进行中', '已完成'] : ['未处理', '修复中', '已修复', '已修复待发布', '验证未通过', '暂不处理']
  return sheet.status_options?.length ? sheet.status_options : presets
}

export function tasksFor(sheet: TaskSheet): Task[] {
  const headers = sheet.headers.map(normalize)
  const index = (names: string[]) => {
    for (const name of names) {
      const column = headers.indexOf(name)
      if (column >= 0) return column
    }
    return -1
  }
  const recognizedTitle = index(['问题描述', '任务', '任务名称', '任务内容', '事项', '事项名称', '标题', '名称', '描述', '功能'])
  const titleIndex = recognizedTitle >= 0 ? recognizedTitle : headers.findIndex(h => h && !['负责人', '负责人员', '责任人'].includes(h) && !/状态|备注|日期|时间|优先|紧急/.test(h))
  const contextIndex = index(['三级功能', '功能', '二级功能', '模块', '项目'])
  const preferredStatus = index(['处理状态', '完成状态', '任务状态', '状态'])
  const statusIndex = preferredStatus >= 0 ? preferredStatus : headers.findIndex(h => h.includes('状态'))
  const priorities = priorityColumns(sheet)
  const priorityIndexes = new Set(priorities.map(field => field.column))
  const dueDateIndex = index(['截止日期', '截止时间', '到期日期', '到期时间', '截止日', '计划结束时间', '计划完成时间', '预计修复时间', '预计完成时间', '最晚完成时间', '交付日期'])
  const dateIndex = dueDateIndex >= 0 ? dueDateIndex : index(['测试时间', '计划开始时间'])
  return sheet.rows.map(row => {
    const get = (column: number) => column >= 0 ? text(row.values[column]) : ''
    const description = get(titleIndex)
    const context = get(contextIndex)
    const fields: Task['fields'] = []
    const notes: Task['notes'] = []
    sheet.headers.forEach((header, i) => {
      const value = get(i)
      const label = text(header) || `第 ${i + 1} 列`
      if (!value || i === titleIndex || i === statusIndex || priorityIndexes.has(i)) return
      if (normalize(header).includes('备注')) notes.push({ label, value })
      else fields.push({ label, value })
    })
    return {
      id: `${sheet.name}:${row.number}`, row, sheet,
      title: description.replace(/\s+/g, ' ') || context || `第 ${row.number} 行任务`,
      description, context,
      status: get(statusIndex) || '未标记', statusValue: get(statusIndex),
      priorities: priorities.flatMap(field => get(field.column) ? [{ label: field.label, value: get(field.column) }] : []),
      date: get(dateIndex), dueDate: get(dueDateIndex),
      fields, notes, search: row.values.map(text).join('\n').toLocaleLowerCase(),
    }
  })
}
export function isCompleted(status: string): boolean {
  return ['已完成', '完成', '已修复', '已关闭', '关闭', '已解决', '已结束', '已取消', '已验收',
    'done', 'completed', 'closed', 'resolved', 'cancelled', 'canceled'].includes(normalize(status).toLowerCase())
}
export function localDay(date = new Date()): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

function calendarDay(value: string): string {
  const normalized = value.trim().replace(/年|月/g, '-').replace(/日/g, '').replace(/^(\d{4})[/.](\d{1,2})[/.](\d{1,2})/, '$1-$2-$3')
  const match = normalized.match(/^(\d{4})-(\d{1,2})-(\d{1,2})(?:[T ](\d{1,2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?)?$/i)
  if (!match) return ''
  const [, year, month, day, hour, minute, second, zone] = match
  const date = new Date(0)
  date.setFullYear(Number(year), Number(month) - 1, Number(day))
  date.setHours(12, 0, 0, 0)
  if (date.getFullYear() !== Number(year) || date.getMonth() + 1 !== Number(month) || date.getDate() !== Number(day)
    || Number(hour ?? 0) > 23 || Number(minute ?? 0) > 59 || Number(second ?? 0) > 59) return ''
  if (zone) {
    const instant = new Date(value)
    return Number.isNaN(instant.getTime()) ? '' : localDay(instant)
  }
  return localDay(date)
}

export function isDueToday(task: Pick<Task, 'dueDate' | 'status'>, today = localDay()): boolean {
  return !isCompleted(task.status) && !!task.dueDate && calendarDay(task.dueDate) === today
}

export function tone(status: string): 'green' | 'amber' | 'blue' | 'gray' {
  if (isCompleted(status)) return 'green'
  if (['未处理', '未开始', '验证未通过', '待处理'].includes(status)) return 'amber'
  if (['进行中', '修复中', '开发中', '已修复待发布'].includes(status)) return 'blue'
  return 'gray'
}
export function documentStats(doc: Document) {
  const rows = doc.sheets.flatMap(sheet => sheet.rows)
  return { tasks: rows.length, pictures: rows.reduce((count, row) => count + row.pictures.length, 0) }
}
export function imageURL(key: string, filename: string, size?: 'thumbnail' | 'preview') {
  return `/local-image/${encodeURIComponent(key)}/${encodeURIComponent(filename)}${size ? `?size=${size}` : ''}`
}
export function formatDate(value: string, includeTime = false) {
  if (!value) return '尚未同步'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit',
    ...(includeTime ? { hour: '2-digit', minute: '2-digit', hour12: false } : {}),
  }).format(date)
}
export function copyTask(task: Task) {
  return [task.context, task.description, `状态：${task.status}`, ...task.priorities.map(field => `${field.label}：${field.value}`),
    ...task.fields.map(field => `${field.label}：${field.value}`),
    ...task.notes.map(field => `${field.label}：${field.value}`),
    `来源：${task.sheet.name} · 第 ${task.row.number} 行`,
  ].filter(Boolean).join('\n\n')
}
