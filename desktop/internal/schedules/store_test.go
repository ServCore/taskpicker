package schedules

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const projectA = "aaaaaaaaaaaaaaaaaaaaaaaa"
const projectB = "bbbbbbbbbbbbbbbbbbbbbbbb"

func input(item Schedule) Input {
	return Input{ID: item.ID, ProjectID: item.ProjectID, IntervalMinutes: item.IntervalMinutes, Enabled: item.Enabled, Revision: item.Revision}
}
func save(t *testing.T, s *Store, in Input) Schedule {
	t.Helper()
	items, err := s.Save(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range items {
		if v.ProjectID == in.ProjectID {
			return v
		}
	}
	t.Fatal("missing saved item")
	return Schedule{}
}

func TestSchedulePersistsAndUsesCompletionBasedDeadline(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	now := time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	item := save(t, s, Input{ProjectID: projectA, IntervalMinutes: 30, Enabled: true})
	if item.NextRunAt != now.Add(30*time.Minute).Format(time.RFC3339Nano) {
		t.Fatal(item)
	}
	items, err := New(root).List()
	if err != nil || len(items) != 1 || items[0] != item {
		t.Fatalf("restart: %v %v", items, err)
	}
	now = now.Add(36 * time.Minute)
	items, err = s.Complete(item.ID, item.Revision, "两份成功")
	if err != nil || items[0].LastRunAt != now.Format(time.RFC3339Nano) || items[0].NextRunAt != now.Add(30*time.Minute).Format(time.RFC3339Nano) || items[0].LastResult != "两份成功" {
		t.Fatalf("completion: %v %v", items, err)
	}
	// Saving unchanged settings must retain the next deadline and revision.
	same := save(t, s, input(items[0]))
	if same != items[0] {
		t.Fatal("unchanged save changed deadline")
	}
}

func TestPauseEditDeleteCannotBeResurrectedByRunningJob(t *testing.T) {
	s := New(t.TempDir())
	original := save(t, s, Input{ProjectID: projectA, IntervalMinutes: 1, Enabled: true})
	in := input(original)
	in.Enabled = false
	paused := save(t, s, in)
	items, err := s.Complete(original.ID, original.Revision, "old run")
	if err != nil || items[0] != paused || paused.NextRunAt != "" {
		t.Fatalf("pause: %v %v", items, err)
	}
	in = input(paused)
	in.Enabled = true
	resumed := save(t, s, in)
	if resumed.NextRunAt == "" || resumed.Revision == paused.Revision {
		t.Fatal("resume did not rearm")
	}
	in = input(resumed)
	in.ProjectID = projectB
	edited := save(t, s, in)
	items, err = s.Complete(resumed.ID, resumed.Revision, "old project")
	if err != nil || items[0] != edited {
		t.Fatal("old project completion changed edited schedule")
	}
	if _, err = s.Delete(edited.ID, resumed.Revision); err == nil {
		t.Fatal("stale delete allowed")
	}
	if _, err = s.Delete(edited.ID, edited.Revision); err != nil {
		t.Fatal(err)
	}
	items, err = s.Complete(edited.ID, edited.Revision, "finished after delete")
	if err != nil || len(items) != 0 {
		t.Fatal("deleted schedule resurrected")
	}
}

func TestScheduleValidationAndStaleEdits(t *testing.T) {
	s := New(t.TempDir())
	for _, in := range []Input{{ProjectID: "../bad", IntervalMinutes: 1}, {ProjectID: projectA, IntervalMinutes: 0}, {ProjectID: projectA, IntervalMinutes: MaxIntervalMinutes + 1}} {
		if _, err := s.Save(in); err == nil {
			t.Fatalf("accepted: %v", in)
		}
	}
	item := save(t, s, Input{ProjectID: projectA, IntervalMinutes: MaxIntervalMinutes})
	if item.NextRunAt != "" {
		t.Fatal("paused schedule armed")
	}
	if _, err := s.Save(Input{ProjectID: projectA, IntervalMinutes: 1}); err == nil {
		t.Fatal("duplicate project accepted")
	}
	in := input(item)
	in.IntervalMinutes = 1
	changed := save(t, s, in)
	if _, err := s.Save(in); err == nil {
		t.Fatal("stale edit allowed")
	}
	completed, err := s.Complete(changed.ID, changed.Revision, "manual paused run")
	if err != nil || completed[0].Enabled || completed[0].NextRunAt != "" || completed[0].LastRunAt == "" {
		t.Fatal("manual completion enabled paused schedule")
	}
}

func TestCorruptConfigurationIsNeverOverwritten(t *testing.T) {
	for _, raw := range []string{`broken`, `{"version":2,"items":[]}`, `{"version":1,"items":[{"id":"bad"}]}`} {
		root := t.TempDir()
		path := filepath.Join(root, "sync-schedules.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s := New(root)
		if _, err := s.List(); err == nil {
			t.Fatal("corruption ignored")
		}
		if _, err := s.Save(Input{ProjectID: projectA, IntervalMinutes: 30}); err == nil {
			t.Fatal("corruption overwritten")
		}
		after, _ := os.ReadFile(path)
		if string(after) != raw {
			t.Fatal("file changed")
		}
	}
}

func TestConcurrentCompletionsAndEdits(t *testing.T) {
	s := New(t.TempDir())
	item := save(t, s, Input{ProjectID: projectA, IntervalMinutes: 1, Enabled: true})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.Complete(item.ID, item.Revision, "complete"); _, _ = s.List() }()
	}
	in := input(item)
	in.Enabled = false
	paused := save(t, s, in)
	wg.Wait()
	items, err := s.List()
	if err != nil || len(items) != 1 || items[0].Enabled || items[0].Revision != paused.Revision || items[0].NextRunAt != "" {
		t.Fatalf("race: %v %v", items, err)
	}
}
