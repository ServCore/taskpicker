package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"taskpicker/internal/model"
)

func TestProjectEnabledDefaultsAndPersistence(t *testing.T) {
	app := &App{root: t.TempDir()}
	state, err := app.GetLibrary()
	if err != nil {
		t.Fatal(err)
	}
	project := state.Projects[0]
	if project.Enabled == nil || !*project.Enabled {
		t.Fatal("initial project should be enabled")
	}
	// Older configurations have no enabled field.
	project.Enabled = nil
	if err := writeProjects(app.root, []model.Project{project}); err != nil {
		t.Fatal(err)
	}
	state, err = app.GetLibrary()
	if err != nil || state.Projects[0].Enabled == nil || !*state.Projects[0].Enabled {
		t.Fatalf("legacy project should be enabled: %+v %v", state.Projects, err)
	}
	before := state.Projects[0]
	projects, err := app.SetProjectEnabled(project.ID, false)
	if err != nil || projects[0].Enabled == nil || *projects[0].Enabled {
		t.Fatalf("disable failed: %+v %v", projects, err)
	}
	before.Enabled = projects[0].Enabled
	if !reflect.DeepEqual(before, projects[0]) {
		t.Fatal("toggle modified other project settings")
	}
	restarted := &App{root: app.root}
	state, err = restarted.GetLibrary()
	if err != nil || *state.Projects[0].Enabled || len(state.Documents) == 0 {
		t.Fatalf("disabled state or documents lost: %+v %v", state.Projects, err)
	}
	// An older editor omitting enabled must preserve a disabled project.
	update := state.Projects[0]
	update.Enabled = nil
	update.Name = "重命名"
	projects, err = restarted.SaveProject(update)
	if err != nil || *projects[0].Enabled {
		t.Fatalf("editing re-enabled project: %+v %v", projects, err)
	}
	projects, err = restarted.SetProjectEnabled(project.ID, true)
	if err != nil || !*projects[0].Enabled {
		t.Fatalf("re-enable failed: %+v %v", projects, err)
	}
	raw, err := os.ReadFile(projectPath(app.root))
	if err != nil {
		t.Fatal(err)
	}
	var saved projectFile
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Projects[0].Enabled == nil || !*saved.Projects[0].Enabled {
		t.Fatalf("enabled state not written: %s %v", raw, err)
	}
	if _, err := app.SetProjectEnabled(strings.Repeat("f", 24), false); err == nil {
		t.Fatal("unknown project accepted")
	}
	after, _ := os.ReadFile(projectPath(app.root))
	if string(raw) != string(after) {
		t.Fatal("invalid toggle modified saved projects")
	}
	projects, err = app.SaveProject(model.Project{Name: "新项目"})
	if err != nil || projects[1].Enabled == nil || !*projects[1].Enabled {
		t.Fatalf("new project should be enabled: %+v %v", projects, err)
	}
}

func TestProjectsInitializeAndSurviveDocumentRefresh(t *testing.T) {
	root := t.TempDir()
	doc := fixture(t, root)
	app := &App{root: root}
	state, err := app.GetLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Projects) != 1 || state.Projects[0].Name != "yiqiban" || len(state.Projects[0].DocumentKeys) != 2 {
		t.Fatalf("default grouping: %+v", state.Projects)
	}
	project := state.Projects[0]
	if project.DailyReportEnabled == nil || *project.DailyReportEnabled {
		t.Fatal("initial project should leave reports disabled")
	}
	project.LocalPath = t.TempDir()
	project.DocumentKeys = []string{doc.Key}
	want, err := app.SaveProject(project)
	if err != nil {
		t.Fatal(err)
	}
	// A new snapshot replaces the document, never its project relationship.
	fixture(t, root)
	restarted := &App{root: root}
	state, err = restarted.GetLibrary()
	if err != nil || !reflect.DeepEqual(state.Projects, want) {
		t.Fatalf("relationship lost after refresh/restart: %+v %v", state.Projects, err)
	}
	if _, err = restarted.AssignDocumentProject(doc.Key, ""); err != nil {
		t.Fatal(err)
	}
	state, _ = restarted.GetLibrary()
	if len(state.Projects[0].DocumentKeys) != 0 {
		t.Fatal("initial migration ran again after unassigning")
	}
}

func TestProjectMoveBindAndUnbind(t *testing.T) {
	app := &App{root: t.TempDir()}
	state, _ := app.GetLibrary()
	key := state.Documents[0].Key
	path := t.TempDir()
	projects, err := app.SaveProject(model.Project{Name: "  另一个项目  ", LocalPath: path, DocumentKeys: []string{key, key}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 || projects[1].Name != "另一个项目" || len(projects[1].DocumentKeys) != 1 || len(projects[0].DocumentKeys) != 1 {
		t.Fatalf("move failed: %+v", projects)
	}
	project := projects[1]
	if project.DailyReportEnabled == nil || *project.DailyReportEnabled {
		t.Fatal("new project should leave reports disabled")
	}
	// Preserve an offline binding while updating other metadata.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	project.Name = "重命名"
	if _, err = app.SaveProject(project); err != nil {
		t.Fatal("offline binding should remain editable:", err)
	}
	if err = app.OpenProjectDirectory(project.ID); err == nil {
		t.Fatal("unavailable path opened")
	}
	project.LocalPath = ""
	project.LocalPaths = []string{}
	projects, err = app.SaveProject(project)
	if err != nil || projects[1].LocalPath != "" {
		t.Fatal("unbind failed:", err)
	}
	projects, err = app.AssignDocumentProject(key, projects[0].ID)
	if err != nil || len(projects[0].DocumentKeys) != 2 || len(projects[1].DocumentKeys) != 0 {
		t.Fatalf("assignment failed: %+v %v", projects, err)
	}
}

func TestInvalidProjectChangesPreserveSavedState(t *testing.T) {
	app := &App{root: t.TempDir()}
	state, _ := app.GetLibrary()
	before, _ := os.ReadFile(projectPath(app.root))
	invalid := []model.Project{
		{Name: " "}, {Name: "yiqiban"}, {Name: strings.Repeat("名", 81)}, {Name: "a\nb"},
		{ID: strings.Repeat("f", 24), Name: "不存在"}, {Name: "无效路径", LocalPath: "relative"},
		{Name: "找不到目录", LocalPath: filepath.Join(t.TempDir(), "missing")},
		{Name: "无效文档", DocumentKeys: []string{strings.Repeat("f", 24)}},
	}
	for _, update := range invalid {
		if _, err := app.SaveProject(update); err == nil {
			t.Fatalf("invalid update saved: %+v", update)
		}
	}
	if _, err := app.AssignDocumentProject(state.Documents[0].Key, strings.Repeat("f", 24)); err == nil {
		t.Fatal("unknown project assigned")
	}
	if _, err := app.AssignDocumentProject(strings.Repeat("f", 24), state.Projects[0].ID); err == nil {
		t.Fatal("unknown document assigned")
	}
	after, _ := os.ReadFile(projectPath(app.root))
	if string(before) != string(after) {
		t.Fatal("failed update modified projects")
	}
}

func TestCorruptProjectsPreserveDocumentsAndConfig(t *testing.T) {
	app := &App{root: t.TempDir()}
	doc := fixture(t, app.root)
	raw := []byte(`{"version":1,"projects":[broken`)
	if err := os.WriteFile(projectPath(app.root), raw, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := app.GetLibrary()
	if err != nil || len(state.Documents) != 2 || state.Documents[0].Key != doc.Key || len(state.Warnings) == 0 {
		t.Fatalf("corrupt projects hid documents: %+v %v", state, err)
	}
	if _, err := app.SaveProject(model.Project{Name: "test"}); err == nil {
		t.Fatal("corrupt config overwritten")
	}
	after, _ := os.ReadFile(projectPath(app.root))
	if string(after) != string(raw) {
		t.Fatal("corrupt original not preserved")
	}
}

func TestConcurrentProjectSavesKeepBothProjects(t *testing.T) {
	app := &App{root: t.TempDir()}
	if _, err := app.GetLibrary(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, name := range []string{"alpha", "beta"} {
		wg.Go(func() {
			if _, err := app.SaveProject(model.Project{Name: name}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	state, err := app.GetLibrary()
	if err != nil || len(state.Projects) != 3 {
		t.Fatalf("lost project: %+v %v", state.Projects, err)
	}
}
