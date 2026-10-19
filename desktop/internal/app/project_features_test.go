package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"taskpicker/internal/model"
)

func TestProjectMultipleDirectoriesAndLegacyMigration(t *testing.T) {
	app := &App{root: t.TempDir()}
	state, _ := app.GetLibrary()
	path1, path2 := t.TempDir(), t.TempDir()
	legacy := model.Project{ID: state.Projects[0].ID, Name: "yiqiban", LocalPath: path1, DocumentKeys: state.Projects[0].DocumentKeys}
	if err := writeProjects(app.root, []model.Project{legacy}); err != nil {
		t.Fatal(err)
	}
	state, err := app.GetLibrary()
	if err != nil || !reflect.DeepEqual(state.Projects[0].LocalPaths, []string{path1}) || !*state.Projects[0].DailyReportEnabled {
		t.Fatalf("legacy migration: %+v %v", state.Projects, err)
	}
	project := state.Projects[0]
	project.LocalPaths = []string{path1, path2, path1}
	projects, err := app.SaveProject(project)
	if err != nil || !reflect.DeepEqual(projects[0].LocalPaths, []string{path1, path2}) {
		t.Fatalf("multi-directory save: %+v %v", projects, err)
	}
	if err := os.Remove(path1); err != nil {
		t.Fatal(err)
	}
	project = projects[0]
	project.Name = "offline directory retained"
	if _, err := app.SaveProject(project); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(projectPath(app.root))
	project.LocalPaths = append(project.LocalPaths, filepath.Join(t.TempDir(), "missing"))
	if _, err := app.SaveProject(project); err == nil {
		t.Fatal("invalid extra directory saved")
	}
	after, _ := os.ReadFile(projectPath(app.root))
	if string(before) != string(after) {
		t.Fatal("partial directory changes saved")
	}
	if err := app.OpenProjectDirectoryAt(project.ID, 99); err == nil {
		t.Fatal("invalid directory index accepted")
	}
}

func TestProjectReportsKeepRecordsDraftsAndPromptsSeparate(t *testing.T) {
	app := &App{root: t.TempDir()}
	doc1 := fixture(t, app.root)
	doc2 := doc1
	doc2.SourceURL = presets[1].SourceURL
	doc2.Title = "第二个项目文档"
	doc2.Sheets = []model.TaskSheet{{Name: "任务", Headers: []any{"责任人", "问题描述"}, Rows: []model.TaskRow{{Number: 4, Values: []any{"王小明", "第二项工作"}, Pictures: []model.Picture{}}}}}
	var err error
	doc2, err = saveDocument(context.Background(), app.root, doc2)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := app.GetLibrary()
	project1 := state.Projects[0]
	reportsEnabled := true
	project1.DailyReportEnabled = &reportsEnabled
	if _, err = app.SaveProject(project1); err != nil {
		t.Fatal(err)
	}
	projects, err := app.SaveProject(model.Project{Name: "项目二", DailyReportEnabled: &reportsEnabled, DocumentKeys: []string{doc2.Key}})
	if err != nil {
		t.Fatal(err)
	}
	project2 := projects[1]
	if _, err = app.RecordTaskWork(doc1.Key, doc1.Sheets[0].Name, 4, "第一项成果"); err != nil {
		t.Fatal(err)
	}
	if _, err = app.RecordTaskWork(doc2.Key, "任务", 4, "第二项成果"); err != nil {
		t.Fatal(err)
	}
	date := time.Now().Format("2006-01-02")
	first, err := app.GenerateProjectDailyReport(project1.ID, date, false)
	if err != nil || !strings.Contains(first.Draft.Text, "第一项成果") || strings.Contains(first.Draft.Text, "第二项成果") {
		t.Fatalf("mixed first report: %+v %v", first, err)
	}
	second, err := app.GenerateProjectDailyReport(project2.ID, date, false)
	if err != nil || !strings.Contains(second.Draft.Text, "第二项成果") || strings.Contains(second.Draft.Text, "第一项成果") {
		t.Fatalf("mixed second report: %+v %v", second, err)
	}
	if _, err = app.SaveProjectDailyReport(project1.ID, date, "手动整理的正文\n"); err != nil {
		t.Fatal(err)
	}
	if _, err = app.RecordTaskWork(doc1.Key, doc1.Sheets[0].Name, 4, "第一项补充"); err != nil {
		t.Fatal(err)
	}
	continued, err := app.ContinueProjectDailyReport(project1.ID, date, false)
	if err != nil || !strings.HasPrefix(continued.Draft.Text, "手动整理的正文\n") || !strings.Contains(continued.Draft.Text, "第一项补充") || strings.Contains(continued.Draft.Text, "第二项成果") {
		t.Fatalf("bad continuation: %+v %v", continued, err)
	}
	unchanged, _ := app.GetProjectDailyReport(project2.ID, date)
	if unchanged.Draft.Text != second.Draft.Text {
		t.Fatal("continuation overwrote another project's draft")
	}
	for id, prompt := range map[string]string{project1.ID: "第一项目提示词", project2.ID: "第二项目提示词"} {
		if _, err := app.SaveProjectDailyReportPrompt(id, prompt); err != nil {
			t.Fatal(err)
		}
		settings, err := app.GetProjectDailyReportSettings(id)
		if err != nil || settings.Prompt != prompt {
			t.Fatal("prompt mixed across projects", err)
		}
	}
	// Moving the document affects future work; earlier records stay with their project.
	if _, err = app.AssignDocumentProject(doc1.Key, project2.ID); err != nil {
		t.Fatal(err)
	}
	firstAfterMove, _ := app.GetProjectDailyReport(project1.ID, date)
	if len(firstAfterMove.Activities) != 2 {
		t.Fatal("moving document moved past work")
	}
	if _, err = app.RecordTaskWork(doc1.Key, doc1.Sheets[0].Name, 4, "移组后的工作"); err != nil {
		t.Fatal(err)
	}
	other, _ := app.GetProjectDailyReport(project2.ID, date)
	if len(other.Activities) != 2 {
		t.Fatalf("new work not assigned to new project: %+v", other.Activities)
	}
	// Disable reports without deleting earlier drafts or recording work during disablement.
	state, _ = app.GetLibrary()
	project2 = state.Projects[1]
	disabled := false
	project2.DailyReportEnabled = &disabled
	if _, err = app.SaveProject(project2); err != nil {
		t.Fatal(err)
	}
	if _, err = app.GetProjectDailyReport(project2.ID, date); err == nil {
		t.Fatal("disabled report accessible")
	}
	if _, err = app.RecordTaskWork(doc2.Key, "任务", 4, "should reject"); err == nil {
		t.Fatal("disabled manual work recorded")
	}
	doc2, _ = ReadDocument(app.root, doc2.Key)
	ignored := newTaskActivity(doc2, doc2.Sheets[0], doc2.Sheets[0].Rows[0], "edit")
	ignored.Note = "关闭期间的操作"
	if _, err = persistActivityDocument(app.root, doc2, ignored); err != nil {
		t.Fatal(err)
	}
	enabled := true
	project2.DailyReportEnabled = &enabled
	if _, err = app.SaveProject(project2); err != nil {
		t.Fatal(err)
	}
	reenabled, err := app.GetProjectDailyReport(project2.ID, date)
	if err != nil || reenabled.Draft.Text != second.Draft.Text || len(reenabled.Activities) != 2 {
		t.Fatalf("disabled history or draft changed: %+v %v", reenabled, err)
	}
}

func TestLegacyReportMovesIntoInitialProjectWithoutLosingOriginal(t *testing.T) {
	app := &App{root: t.TempDir()}
	doc := fixture(t, app.root)
	state, _ := app.GetLibrary()
	// Simulate a legacy project created before the report switch existed.
	state.Projects[0].DailyReportEnabled = nil
	if err := writeProjects(app.root, state.Projects); err != nil {
		t.Fatal(err)
	}
	date := time.Now().Format("2006-01-02")
	if _, err := app.RecordTaskWork(doc.Key, doc.Sheets[0].Name, 4, "旧记录"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveDailyReport(date, "原有的日报和手动修改\n"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(dailyDraftPath(app.root, date))
	state, _ = app.GetLibrary()
	report, err := app.GetProjectDailyReport(state.Projects[0].ID, date)
	if err != nil || report.Draft.Text != "原有的日报和手动修改\n" || len(report.Activities) != 1 {
		t.Fatalf("legacy report lost: %+v %v", report, err)
	}
	after, _ := os.ReadFile(dailyDraftPath(app.root, date))
	if string(before) != string(after) {
		t.Fatal("legacy draft overwritten")
	}
	if _, err := app.GetProjectDailyReport("../../outside", date); err == nil {
		t.Fatal("invalid project accepted")
	}
}

func TestDocumentConditionsPersistAndRefreshAtStableKey(t *testing.T) {
	app := &App{root: t.TempDir()}
	path := excelFixture(t)
	doc, err := app.importPath(path, "王小明", false)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := app.GetLibrary()
	project := state.Projects[0]
	project.DocumentKeys = append(project.DocumentKeys, doc.Key)
	reportsEnabled := true
	project.DailyReportEnabled = &reportsEnabled
	if _, err = app.SaveProject(project); err != nil {
		t.Fatal(err)
	}
	_, err = app.RecordTaskWork(doc.Key, "问题清单", 3, "修改拉取条件之前的工作")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := app.SaveDocumentPullConditions(doc.Key, "李小华", true)
	if err != nil || updated.Key != doc.Key || updated.Owner != "王小明" || updated.PullConditions.Owner != "李小华" {
		t.Fatalf("conditions overwrote cache: %+v %v", updated, err)
	}
	restarted := &App{root: app.root}
	state, _ = restarted.GetLibrary()
	var condition *model.DocumentPullConditions
	for _, item := range state.Documents {
		if item.Key == doc.Key {
			condition = item.PullConditions
		}
	}
	if condition == nil || condition.Owner != "李小华" || !condition.IncludeCompleted {
		t.Fatal("conditions lost after restart")
	}
	pulled, err := restarted.SyncConfiguredDocument(doc.Key, true)
	if err != nil || pulled.Key != doc.Key || pulled.Owner != "李小华" || !pulled.IncludeCompleted || len(pulled.Sheets[0].Rows) != 1 {
		t.Fatalf("configured refresh failed: %+v %v", pulled, err)
	}
	state, _ = restarted.GetLibrary()
	if len(state.Documents) != 3 || !reflect.DeepEqual(state.Projects[0].DocumentKeys, project.DocumentKeys) {
		t.Fatal("owner change duplicated or detached document")
	}
	history, _ := readActivities(app.root, doc.Key)
	if len(history) != 1 {
		t.Fatal("owner change lost history")
	}
	before, _ := os.ReadFile(documentConditionsPath(app.root))
	if _, err = app.SaveDocumentPullConditions(doc.Key, "  ", false); err == nil {
		t.Fatal("empty owner saved")
	}
	after, _ := os.ReadFile(documentConditionsPath(app.root))
	if string(before) != string(after) {
		t.Fatal("invalid conditions overwrote file")
	}
	if err := os.WriteFile(documentConditionsPath(app.root), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = app.SaveDocumentPullConditions(doc.Key, "x", false); err == nil {
		t.Fatal("corrupt conditions overwritten")
	}
	state, _ = app.GetLibrary()
	if len(state.Documents) != 3 || len(state.Warnings) == 0 {
		t.Fatal("corrupt conditions hid task library")
	}
}
