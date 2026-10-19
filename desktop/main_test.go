package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestDesktopBridgeKeepsMainAppAndAllMethods(t *testing.T) {
	bridge := reflect.TypeOf(&App{})
	if bridge.Elem().PkgPath() != "taskpicker" || bridge.Elem().Name() != "App" {
		t.Fatalf("desktop bridge type changed: %v", bridge)
	}
	expected := []string{
		"AssignDocumentProject",
		"AuthorizeRemote",
		"CancelDailyReport",
		"CancelSync",
		"CompleteSyncSchedule",
		"ContinueDailyReport",
		"ContinueProjectDailyReport",
		"ContinueTaskExecution",
		"CopyText",
		"DeleteSyncSchedule",
		"GenerateDailyReport",
		"GenerateProjectDailyReport",
		"GetActiveTaskExecution",
		"GetDailyAISettings",
		"GetDailyReport",
		"GetLibrary",
		"GetProjectDailyReport",
		"GetProjectDailyReportSettings",
		"GetRemoteAuth",
		"GetResourceUsage",
		"GetSyncSchedules",
		"GetTaskExecution",
		"GetTaskExecutionContext",
		"GetTaskExecutionProgress",
		"GetTaskExecutionReport",
		"GetTaskExecutions",
		"ImportExcel",
		"OpenProjectDirectory",
		"OpenProjectDirectoryAt",
		"OpenSource",
		"OpenTaskExecutionProgress",
		"OpenTaskExecutionReport",
		"OpenTaskExecutionWorkspace",
		"PickProjectDirectory",
		"PrepareOnlineDocument",
		"PushTaskStatus",
		"RecordTaskWork",
		"ReimportDocument",
		"RevertTaskStatus",
		"SaveDailyAISettings",
		"SaveDailyReport",
		"SaveDailyReportPrompt",
		"SaveDocumentFilters",
		"SaveDocumentPullConditions",
		"SaveProject",
		"SaveProjectDailyReport",
		"SaveProjectDailyReportPrompt",
		"SaveSyncSchedule",
		"SelectDocument",
		"SetProjectEnabled",
		"StartTaskExecution",
		"StopTaskExecution",
		"SyncConfiguredDocument",
		"SyncDocument",
		"SyncDocumentOnStartup",
		"UpdateTaskDetails",
		"UpdateTaskStatus",
	}
	actual := make([]string, 0, bridge.NumMethod())
	for i := 0; i < bridge.NumMethod(); i++ {
		actual = append(actual, bridge.Method(i).Name)
	}
	sort.Strings(actual)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Wails methods changed:\n got %v\nwant %v", actual, expected)
	}
}
