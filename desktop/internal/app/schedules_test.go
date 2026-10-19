package app

import (
	"taskpicker/internal/schedules"
	"testing"
)

func TestSyncScheduleRequiresExistingProjectAndSurvivesRestart(t *testing.T) {
	a := &App{root: t.TempDir()}
	state, err := a.GetLibrary()
	if err != nil {
		t.Fatal(err)
	}
	in := schedules.Input{ProjectID: "aaaaaaaaaaaaaaaaaaaaaaaa", IntervalMinutes: 15, Enabled: true}
	if _, err = a.SaveSyncSchedule(in); err == nil {
		t.Fatal("missing project accepted")
	}
	in.ProjectID = state.Projects[0].ID
	items, err := a.SaveSyncSchedule(in)
	if err != nil || len(items) != 1 {
		t.Fatalf("save: %v %v", items, err)
	}
	b := &App{root: a.root}
	loaded, err := b.GetSyncSchedules()
	if err != nil || len(loaded) != 1 || loaded[0] != items[0] {
		t.Fatalf("load: %v %v", loaded, err)
	}
	completed, err := b.CompleteSyncSchedule(items[0].ID, items[0].Revision, "完成")
	if err != nil || completed[0].LastResult != "完成" {
		t.Fatalf("complete: %v %v", completed, err)
	}
	removed, err := b.DeleteSyncSchedule(items[0].ID, items[0].Revision)
	if err != nil || len(removed) != 0 {
		t.Fatalf("delete: %v %v", removed, err)
	}
	library, err := b.GetLibrary()
	if err != nil || len(library.Projects) != len(state.Projects) || len(library.Documents) != len(state.Documents) {
		t.Fatal("schedule deletion changed project/library")
	}
}
