import type { Document, DocumentFilter, DocumentPullConditions } from '../../shared/types/types'
import { text } from '../tasks/tasks'
export const operatorOptions = [
  { label: '等于', value: 'equals' }, { label: '不等于', value: 'not_equals' },
  { label: '包含文字', value: 'contains' }, { label: '不包含文字', value: 'not_contains' },
  { label: '包含成员', value: 'member' }, { label: '为空', value: 'is_empty' }, { label: '不为空', value: 'not_empty' },
]
export const needsValue = (rule: DocumentFilter) => !['is_empty', 'not_empty'].includes(rule.operator)
export function currentConditions(doc: Document): DocumentPullConditions {
  const c = doc.pull_conditions ?? { owner: doc.owner, include_completed: doc.include_completed ?? false }
  return { owner: c.owner, include_completed: true, filters: (c.filters ?? []).map(rule => ({...rule})) }
}
export function editableConditions(doc: Document): DocumentPullConditions {
  const c = currentConditions(doc)
  c.filters = c.filters?.map(rule => {
    const headers = doc.sheets.find(sheet => sheet.name === rule.sheet)?.headers ?? []
    if (String(headers[rule.column - 1] ?? '') === rule.header) return rule
    const matches = headers.flatMap((header, index) => rule.header && String(header ?? '') === rule.header ? [index] : [])
    return matches.length === 1 ? { ...rule, column: matches[0]! + 1 } : rule
  })
  if (c.owner) {
    const legacy: DocumentFilter[] = doc.sheets.flatMap(sheet => {
      const column = sheet.headers.findIndex(header => ['负责人', '负责人员', '责任人'].includes(text(header).replace(/\s/g, '')))
      return column < 0 ? [] : [{ sheet: sheet.name, column: column + 1, header: String(sheet.headers[column] ?? ''), operator: 'member', value: c.owner }]
    })
    if (legacy.length === doc.sheets.length && legacy.length) { c.filters = [...legacy, ...(c.filters ?? [])]; c.owner = '' }
  }
  return c
}
function signature(c: DocumentPullConditions) {
  return JSON.stringify([c.owner, c.include_completed, (c.filters ?? []).map(f => [f.sheet, f.column, f.header, f.operator, f.value])])
}
export function conditionsPending(doc: Document) {
  if (doc.setup_pending) return true
  const applied = doc.applied_pull_conditions ?? { owner: doc.owner, include_completed: doc.include_completed ?? false }
  return signature(currentConditions(doc)) !== signature(applied)
}
export function conditionSummary(doc: Document) {
  const c = currentConditions(doc), count = (c.filters?.length ?? 0) + (c.owner ? 1 : 0)
  if (!count) return '未设置'
  if (c.owner && !c.filters?.length) return `负责人 包含成员 ${c.owner}`
  if (c.filters?.length === 1 && !c.owner) { const f = c.filters[0]!; return `${f.header || `第 ${f.column} 列`} ${operatorOptions.find(o => o.value === f.operator)?.label ?? f.operator} ${needsValue(f) ? f.value : ''}` }
  return `${count} 个条件 · 全部满足`
}
