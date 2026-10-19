import type { DocumentPullConditions, SyncSchedule, SyncScheduleInput, Project, DailyAISettings, DailyAIUpdate, DailyDraft, DailyReport, Document, Library, RemoteAuthState, ResourceUsage, TaskActivity, TaskNoteUpdate, TaskExecutionContext, TaskExecution, TaskExecutionUpdate, TaskExecutionReport, TaskExecutionProgress } from '../types/types'

interface DesktopAPI {
  PrepareOnlineDocument(link: string): Promise<Document>
  SaveDocumentFilters(key: string, conditions: DocumentPullConditions): Promise<Document>
  GetSyncSchedules(): Promise<SyncSchedule[]>
  SaveSyncSchedule(input: SyncScheduleInput): Promise<SyncSchedule[]>
  DeleteSyncSchedule(id: string, revision: string): Promise<SyncSchedule[]>
  CompleteSyncSchedule(id: string, revision: string, result: string): Promise<SyncSchedule[]>
  GetActiveTaskExecution(): Promise<TaskExecution | null>
  GetTaskExecutionContext(key: string, sheet: string, row: number): Promise<TaskExecutionContext>
  GetTaskExecutions(key: string, sheet: string, row: number): Promise<TaskExecution[]>
  GetTaskExecution(id: string, after: number): Promise<TaskExecutionUpdate>
  GetTaskExecutionReport(id: string): Promise<TaskExecutionReport>
  OpenTaskExecutionReport(id: string): Promise<void>
  ContinueTaskExecution(key: string, sheet: string, row: number, previousID: string, instructions: string): Promise<TaskExecution>
  GetTaskExecutionProgress(key: string, sheet: string, row: number): Promise<TaskExecutionProgress>
  OpenTaskExecutionProgress(key: string, sheet: string, row: number): Promise<void>
  StartTaskExecution(key: string, sheet: string, row: number, repository: string, instructions: string): Promise<TaskExecution>
  StopTaskExecution(id: string): Promise<void>
  OpenTaskExecutionWorkspace(id: string): Promise<void>
  GetProjectDailyReport(id: string, date: string): Promise<DailyReport>
  SaveProjectDailyReport(id: string, date: string, text: string): Promise<DailyDraft>
  GenerateProjectDailyReport(id: string, date: string, useAI: boolean): Promise<DailyReport>
  ContinueProjectDailyReport(id: string, date: string, useAI: boolean): Promise<DailyReport>
  GetProjectDailyReportSettings(id: string): Promise<DailyAISettings>
  SaveProjectDailyReportPrompt(id: string, prompt: string): Promise<string>
  OpenProjectDirectoryAt(id: string, index: number): Promise<void>
  SaveDocumentPullConditions(key: string, owner: string, includeCompleted: boolean): Promise<Document>
  SyncConfiguredDocument(key: string, interactive: boolean): Promise<Document>
  SaveProject(project: Project): Promise<Project[]>
  SetProjectEnabled(id: string, enabled: boolean): Promise<Project[]>
  AssignDocumentProject(key: string, projectID: string): Promise<Project[]>
  PickProjectDirectory(current: string): Promise<string>
  OpenProjectDirectory(id: string): Promise<void>
  RecordTaskWork(key: string, sheet: string, row: number, note: string): Promise<TaskActivity>
  GetDailyReport(date: string): Promise<DailyReport>
  SaveDailyReport(date: string, text: string): Promise<DailyDraft>
  GenerateDailyReport(date: string, useAI: boolean): Promise<DailyReport>
  ContinueDailyReport(date: string, useAI: boolean): Promise<DailyReport>
  CancelDailyReport(): Promise<void>
  GetDailyAISettings(): Promise<DailyAISettings>
  SaveDailyAISettings(settings: DailyAIUpdate): Promise<DailyAISettings>
  SaveDailyReportPrompt(prompt: string): Promise<string>
  GetResourceUsage(): Promise<ResourceUsage>
  GetLibrary(): Promise<Library>
  GetRemoteAuth(): Promise<RemoteAuthState>
  AuthorizeRemote(): Promise<RemoteAuthState>
  UpdateTaskDetails(key: string, sheet: string, row: number, value: string, notes: TaskNoteUpdate[]): Promise<Document>
  UpdateTaskStatus(key: string, sheet: string, row: number, value: string): Promise<Document>
  RevertTaskStatus(key: string, sheet: string, row: number): Promise<Document>
  PushTaskStatus(key: string, sheet: string, row: number): Promise<Document>
  SelectDocument(key: string): Promise<void>
  SyncDocumentOnStartup(url: string, owner: string, includeCompleted: boolean): Promise<Document>
  SyncDocument(url: string, owner: string, includeCompleted: boolean): Promise<Document>
  ImportExcel(owner: string, includeCompleted: boolean): Promise<Document | null>
  ReimportDocument(key: string, includeCompleted: boolean): Promise<Document>
  CancelSync(): Promise<void>
  CopyText(text: string): Promise<void>
  OpenSource(key: string): Promise<void>
}
declare global {
  interface Window {
    go?: { main: { App: DesktopAPI } }
    runtime?: {
      EventsOnMultiple(name: string, callback: (message: string) => void, count: number): () => void
    }
  }
}
function desktop(): DesktopAPI {
  if (!window.go?.main?.App) throw new Error('请通过 make run 或桌面应用打开任务库。')
  return window.go.main.App
}
// Keep configuration responses in mutation order, including a completion racing
// with pause/edit/delete. An older response must not restore stale UI settings.
let scheduleRequest: Promise<unknown> = Promise.resolve()
function scheduleCall<T>(action: () => Promise<T>): Promise<T> {
  const request = scheduleRequest.then(action)
  scheduleRequest = request.then(() => undefined, () => undefined)
  return request
}
export const api: DesktopAPI = {
  PrepareOnlineDocument: link => desktop().PrepareOnlineDocument(link),
  SaveDocumentFilters: (key, conditions) => desktop().SaveDocumentFilters(key, conditions),
  GetSyncSchedules: () => scheduleCall(() => desktop().GetSyncSchedules()),
  SaveSyncSchedule: input => scheduleCall(() => desktop().SaveSyncSchedule(input)),
  DeleteSyncSchedule: (id, revision) => scheduleCall(() => desktop().DeleteSyncSchedule(id, revision)),
  CompleteSyncSchedule: (id, revision, result) => scheduleCall(() => desktop().CompleteSyncSchedule(id, revision, result)),
  GetActiveTaskExecution: () => desktop().GetActiveTaskExecution(),
  GetTaskExecutionContext: (key, sheet, row) => desktop().GetTaskExecutionContext(key, sheet, row),
  GetTaskExecutions: (key, sheet, row) => desktop().GetTaskExecutions(key, sheet, row),
  GetTaskExecution: (id, after) => desktop().GetTaskExecution(id, after),
  GetTaskExecutionReport: id => desktop().GetTaskExecutionReport(id),
  OpenTaskExecutionReport: id => desktop().OpenTaskExecutionReport(id),
  ContinueTaskExecution: (key, sheet, row, previousID, instructions) => desktop().ContinueTaskExecution(key, sheet, row, previousID, instructions),
  GetTaskExecutionProgress: (key, sheet, row) => desktop().GetTaskExecutionProgress(key, sheet, row),
  OpenTaskExecutionProgress: (key, sheet, row) => desktop().OpenTaskExecutionProgress(key, sheet, row),
  StartTaskExecution: (key, sheet, row, repository, instructions) => desktop().StartTaskExecution(key, sheet, row, repository, instructions),
  StopTaskExecution: id => desktop().StopTaskExecution(id),
  OpenTaskExecutionWorkspace: id => desktop().OpenTaskExecutionWorkspace(id),
  GetProjectDailyReport: (id, date) => desktop().GetProjectDailyReport(id, date),
  SaveProjectDailyReport: (id, date, text) => desktop().SaveProjectDailyReport(id, date, text),
  GenerateProjectDailyReport: (id, date, useAI) => desktop().GenerateProjectDailyReport(id, date, useAI),
  ContinueProjectDailyReport: (id, date, useAI) => desktop().ContinueProjectDailyReport(id, date, useAI),
  GetProjectDailyReportSettings: id => desktop().GetProjectDailyReportSettings(id),
  SaveProjectDailyReportPrompt: (id, prompt) => desktop().SaveProjectDailyReportPrompt(id, prompt),
  OpenProjectDirectoryAt: (id, index) => desktop().OpenProjectDirectoryAt(id, index),
  SaveDocumentPullConditions: (key, owner, includeCompleted) => desktop().SaveDocumentPullConditions(key, owner, includeCompleted),
  SyncConfiguredDocument: (key, interactive) => desktop().SyncConfiguredDocument(key, interactive),
  SaveProject: project => desktop().SaveProject(project),
  SetProjectEnabled: (id, enabled) => desktop().SetProjectEnabled(id, enabled),
  AssignDocumentProject: (key, projectID) => desktop().AssignDocumentProject(key, projectID),
  PickProjectDirectory: current => desktop().PickProjectDirectory(current),
  OpenProjectDirectory: id => desktop().OpenProjectDirectory(id),
  RecordTaskWork: (key, sheet, row, note) => desktop().RecordTaskWork(key, sheet, row, note),
  GetDailyReport: date => desktop().GetDailyReport(date),
  SaveDailyReport: (date, text) => desktop().SaveDailyReport(date, text),
  GenerateDailyReport: (date, useAI) => desktop().GenerateDailyReport(date, useAI),
  ContinueDailyReport: (date, useAI) => desktop().ContinueDailyReport(date, useAI),
  CancelDailyReport: () => desktop().CancelDailyReport(),
  GetDailyAISettings: () => desktop().GetDailyAISettings(),
  SaveDailyAISettings: settings => desktop().SaveDailyAISettings(settings),
  SaveDailyReportPrompt: prompt => desktop().SaveDailyReportPrompt(prompt),
  GetResourceUsage: () => desktop().GetResourceUsage(),
  GetLibrary: () => desktop().GetLibrary(),
  GetRemoteAuth: () => desktop().GetRemoteAuth(),
  AuthorizeRemote: () => desktop().AuthorizeRemote(),
  UpdateTaskDetails: (key, sheet, row, value, notes) => desktop().UpdateTaskDetails(key, sheet, row, value, notes),
  UpdateTaskStatus: (key, sheet, row, value) => desktop().UpdateTaskStatus(key, sheet, row, value),
  RevertTaskStatus: (key, sheet, row) => desktop().RevertTaskStatus(key, sheet, row),
  PushTaskStatus: (key, sheet, row) => desktop().PushTaskStatus(key, sheet, row),
  SelectDocument: key => desktop().SelectDocument(key),
  SyncDocumentOnStartup: (url, owner, includeCompleted) => desktop().SyncDocumentOnStartup(url, owner, includeCompleted),
  SyncDocument: (url, owner, includeCompleted) => desktop().SyncDocument(url, owner, includeCompleted),
  ImportExcel: (owner, includeCompleted) => desktop().ImportExcel(owner, includeCompleted),
  ReimportDocument: (key, includeCompleted) => desktop().ReimportDocument(key, includeCompleted),
  CancelSync: () => desktop().CancelSync(),
  CopyText: text => desktop().CopyText(text),
  OpenSource: key => desktop().OpenSource(key),
}
export function onProgress(callback: (message: string) => void) {
  return window.runtime?.EventsOnMultiple('reader:progress', callback, -1) ?? (() => {})
}
