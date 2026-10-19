export interface Picture {
  row: number
  column: number
  width: number
  height: number
  file: string
}
export interface TaskRow {
  number: number
  record_id?: string
  status_edit?: { original: string; saved_at: string; error?: string; notes?: { column: number; header: string; original: string }[] }
  status_synced_at?: string
  values: unknown[]
  pictures: Picture[]
}
export interface TaskSheet {
  name: string
  status_options?: string[]
  header_row: number
  headers: unknown[]
  scanned: number
  rows: TaskRow[]
}
export interface Document {
  key: string
  version: number
  source_url: string
  source_path: string
  owner: string
  title: string
  updated_at: string
  warnings: string[]
  sheets: TaskSheet[]
  cached: boolean
  include_completed?: boolean
  pull_conditions?: DocumentPullConditions
  applied_pull_conditions?: DocumentPullConditions
  setup_pending?: boolean
}
export interface DocumentFilter { sheet: string; column: number; header: string; operator: string; value: string }
export interface DocumentPullConditions { owner: string; include_completed: boolean; filters?: DocumentFilter[] }
export interface Project { id: string; name: string; enabled?: boolean; local_path: string; local_paths?: string[]; daily_report_enabled?: boolean; document_keys: string[] }
export interface SyncSchedule {
  id: string; project_id: string; interval_minutes: number; enabled: boolean; revision: string
  next_run_at: string; last_run_at: string; last_result: string
}
export interface SyncScheduleInput {
  id: string; project_id: string; interval_minutes: number; enabled: boolean; revision: string
}
export interface Library {
  projects?: Project[]
  documents: Document[]
  selected_key: string
  warnings: string[]
}
export interface Task {
  id: string
  row: TaskRow
  sheet: TaskSheet
  title: string
  description: string
  context: string
  status: string
  statusValue: string
  priorities: { label: string; value: string }[]
  date: string
  dueDate: string
  fields: { label: string; value: string }[]
  notes: { label: string; value: string }[]
  search: string
}

export interface RemoteAuthState { authorized: boolean; expires_at: number }

export interface ResourceUsage { cpu_percent: number | null; memory_bytes: number }

export interface TaskNoteUpdate { column: number; value: string }

export interface TaskActivity {
  project_id?: string; report_excluded?: boolean
  id: string; date: string; at: string; document_key: string; document_title: string
  sheet: string; row: number; record_id?: string; owner: string; task: string; context: string
  kind: 'edit' | 'revert' | 'synced' | 'sync_failed' | 'work'; status: string
  changes: { field: string; before: string; after: string }[]; note: string
}
export interface DailyDraft { text: string; source: string; generated_at: string; saved_at: string; activity_count: number; activity_ids?: string[] }
export interface DailyReport { date: string; activities: TaskActivity[]; draft: DailyDraft; notice?: string }
export interface DailyAISettings { base_url: string; model: string; prompt: string; has_api_key: boolean; default_prompt: string }
export interface DailyAIUpdate { base_url: string; model: string; api_key: string; clear_api_key: boolean }

export interface TaskExecutionContext {
  engine: string; available: boolean; repositories: { path: string; codegraph: boolean }[]; notice: string
}
export interface TaskExecution {
  id: string; task_id: string; project_id: string; document_key: string; sheet: string; row: number
  record_id?: string
  previous_id?: string; root_id?: string; original_task?: string; context_summary?: string; progress_summary?: string
  task: string; instructions: string; repository: string; workspace: string; branch: string
  status: 'preparing' | 'running' | 'stopping' | 'succeeded' | 'failed' | 'stopped' | 'interrupted'
  started_at: string; finished_at: string; result: string; error: string; changed_files: string; log_count: number
}
export interface ExecutionLog { sequence: number; at: string; kind: string; text: string }
export interface TaskExecutionUpdate { run: TaskExecution; logs: ExecutionLog[]; next: number }

export interface TaskExecutionReport { markdown: string; path: string }

export interface TaskExecutionProgress { run_id: string; root_id: string; status: string; markdown: string; path: string }
