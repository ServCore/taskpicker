package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
)

func TestTaskHistoryIsAtomicWithEditsAndSurvivesPullAndRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	doc := notesFixture(t, root)
	app := &App{root: root}
	date, sheet := time.Now().Format("2006-01-02"), doc.Sheets[0].Name
	if _, err := app.UpdateTaskDetails(doc.Key, sheet, 4, "修复中", []TaskNoteUpdate{{Column: 3, Value: "联调完成\n等待验证"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateTaskDetails(doc.Key, sheet, 4, "修复中", []TaskNoteUpdate{{Column: 3, Value: "联调完成\n等待验证"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateTaskDetails(doc.Key, sheet, 4, "已修复", []TaskNoteUpdate{{Column: 1, Value: "错误列"}}); err == nil {
		t.Fatal("invalid edit accepted")
	}
	if _, err := app.PushTaskStatus(doc.Key, sheet, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := app.RevertTaskStatus(doc.Key, sheet, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := app.RecordTaskWork(doc.Key, sheet, 4, "核对接口日志，定位权限问题"); err != nil {
		t.Fatal(err)
	}
	history, err := dailyActivities(root, date)
	if err != nil || len(history) != 4 {
		t.Fatalf("no-op or rejected edit logged, or history missing: %d %v", len(history), err)
	}
	if history[0].Kind != "edit" || len(history[0].Changes) != 2 || history[0].Changes[0].Before != "未处理" || history[0].Changes[1].After != "联调完成\n等待验证" {
		t.Fatal("lost actual before/after changes", history[0])
	}
	if history[1].Kind != "sync_failed" || history[2].Kind != "revert" || history[2].Changes[1].After != "原备注" || history[3].Kind != "work" {
		t.Fatal("inaccurate activity outcomes", history)
	}
	for _, item := range history {
		if item.DocumentTitle != doc.Title || item.Task != reader.StringValue(doc.Sheets[0].Rows[0].Values[1]) || item.ID == "" {
			t.Fatal("missing task snapshot", item)
		}
	}
	// A pull can remove completed rows entirely without erasing their daily history.
	doc.Sheets[0].Rows = []model.TaskRow{}
	if _, err := saveDocument(context.Background(), root, doc); err != nil {
		t.Fatal(err)
	}
	report, err := (&App{root: root}).GetDailyReport(date)
	if err != nil || len(report.Activities) != 4 {
		t.Fatal("history lost across pull/restart", err)
	}
	library, _ := app.GetLibrary()
	raw, _ := json.Marshal(library)
	if strings.Contains(string(raw), "核对接口日志") {
		t.Fatal("history loaded into ordinary library responses")
	}
	if _, err := app.RecordTaskWork(doc.Key, sheet, 4, "不存在的任务"); err == nil {
		t.Fatal("recorded missing task")
	}
	if _, err := app.GetDailyReport("../../secret"); err == nil {
		t.Fatal("date path traversal accepted")
	}
	if _, err := app.GetDailyReport("2026-02-30"); err == nil {
		t.Fatal("invalid date accepted")
	}
}

func TestDailyAIConfigKeepsKeyPrivateAndBindsItToEndpoint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	app := &App{root: root}
	settings, err := app.SaveDailyAISettings(DailyAIUpdate{BaseURL: "https://example.com/v1/", Model: "example-model", APIKey: "private-key"})
	if err != nil || !settings.HasAPIKey {
		t.Fatal(err)
	}
	if _, err := app.SaveDailyReportPrompt("按项目整理"); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(settings)
	if strings.Contains(string(raw), "private-key") {
		t.Fatal("key returned to frontend")
	}
	if info, _ := os.Stat(dailyAIPath(root)); info.Mode().Perm() != 0600 {
		t.Fatal("config is not private")
	}
	if _, err := app.SaveDailyAISettings(DailyAIUpdate{BaseURL: settings.BaseURL, Model: "new-model"}); err != nil {
		t.Fatal(err)
	}
	config, _ := readDailyAI(root)
	if config.APIKey != "private-key" || config.Prompt != "按项目整理" {
		t.Fatal("key/prompt not retained")
	}
	if _, err := app.SaveDailyAISettings(DailyAIUpdate{BaseURL: "https://other.example/v1", Model: "new-model"}); err == nil {
		t.Fatal("old key moved to different host")
	}
	if _, err := app.SaveDailyAISettings(DailyAIUpdate{BaseURL: "https://other.example/v1", Model: "new-model", ClearAPIKey: true}); err != nil {
		t.Fatal(err)
	}
	config, _ = readDailyAI(root)
	if config.APIKey != "" || config.Prompt != "按项目整理" {
		t.Fatal("clear/reset failed")
	}
	// Updating only the report prompt preserves the latest model connection and key.
	if _, err := app.SaveDailyAISettings(DailyAIUpdate{BaseURL: "https://other.example/v1", Model: "latest-model", APIKey: "new-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveDailyReportPrompt("三条要点"); err != nil {
		t.Fatal(err)
	}
	config, _ = readDailyAI(root)
	if config.BaseURL != "https://other.example/v1" || config.Model != "latest-model" || config.APIKey != "new-key" || config.Prompt != "三条要点" {
		t.Fatal("prompt save changed model connection")
	}
	if _, err := app.SaveDailyReportPrompt(strings.Repeat("字", 20001)); err == nil {
		t.Fatal("oversized prompt accepted")
	}
	config, _ = readDailyAI(root)
	if config.Prompt != "三条要点" {
		t.Fatal("rejected prompt replaced saved prompt")
	}
	if saved, err := app.SaveDailyReportPrompt(" "); err != nil || saved != defaultDailyPrompt {
		t.Fatal("default prompt reset failed", err)
	}
	for _, bad := range []string{"http://example.com", "https://user:secret@example.com", "file:///etc/passwd", "https://example.com?key=secret"} {
		if _, err := chatEndpoint(bad); err == nil {
			t.Fatal("accepted invalid endpoint", bad)
		}
	}
	endpoint, err := chatEndpoint("https://example.com/v1")
	if err != nil || endpoint != "https://example.com/v1/chat/completions" {
		t.Fatal("invalid endpoint", endpoint, err)
	}
}

func TestDailyGenerationUsesOnlyChosenDayAndPreservesDraftOnFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	doc := notesFixture(t, root)
	app := &App{root: root}
	activity, err := app.RecordTaskWork(doc.Key, doc.Sheets[0].Name, 4, "定位权限问题，等待验证")
	if err != nil {
		t.Fatal(err)
	}
	older := activity
	older.ID = "old"
	older.Date = "2000-01-01"
	older.Note = "OTHER_DAY_PRIVATE"
	if _, err := persistActivityDocument(root, doc, older); err != nil {
		t.Fatal(err)
	}
	local, err := app.GenerateDailyReport(activity.Date, false)
	if err != nil || !strings.Contains(local.Draft.Text, activity.Note) || strings.Contains(local.Draft.Text, older.Note) {
		t.Fatal("invalid local draft", err)
	}
	if _, err := app.SaveDailyReport(activity.Date, "我编辑后的日报"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveDailyReportPrompt("只写三条"); err != nil {
		t.Fatal(err)
	}
	var requestCount int
	mode := "ok"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fake-test-key" {
			t.Error("incorrect API request")
		}
		raw, _ := reader.LimitedRead(r.Body, 1<<20)
		if !strings.Contains(string(raw), "只写三条") || !strings.Contains(string(raw), activity.Note) || strings.Contains(string(raw), older.Note) || strings.Contains(string(raw), doc.SourceURL) || strings.Contains(string(raw), "fake-test-key") {
			t.Error("incorrect or excessive AI input")
		}
		switch mode {
		case "error":
			w.WriteHeader(401)
			w.Write([]byte("fake-test-key should not be shown"))
		case "empty":
			w.Write([]byte(`{"choices":[]}`))
		case "truncated":
			w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"不完整正文"}}]}`))
		default:
			w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"今日工作：定位权限问题。"}}]}`))
		}
	}))
	defer server.Close()
	if _, err := app.SaveDailyAISettings(DailyAIUpdate{BaseURL: server.URL + "/v1", Model: "test-model", APIKey: "fake-test-key"}); err != nil {
		t.Fatal(err)
	}
	ai, err := app.GenerateDailyReport(activity.Date, true)
	if err != nil || ai.Draft.Source != "ai" || ai.Draft.Text != "今日工作：定位权限问题。" || ai.Draft.ActivityCount != 1 {
		t.Fatal("AI result not persisted", err)
	}
	for _, failure := range []string{"error", "empty", "truncated"} {
		mode = failure
		if _, err := app.GenerateDailyReport(activity.Date, true); err == nil || strings.Contains(err.Error(), "fake-test-key") {
			t.Fatal("failed generation leaked key or succeeded", err)
		}
		saved, _ := app.GetDailyReport(activity.Date)
		if saved.Draft.Text != ai.Draft.Text {
			t.Fatal("failure overwrote edited draft")
		}
	}
	if _, err := app.GenerateDailyReport("1999-01-01", true); err == nil || requestCount != 4 {
		t.Fatal("sent empty day to AI")
	}
	loaded, _ := (&App{root: root}).GetDailyReport(activity.Date)
	if loaded.Draft.Text != ai.Draft.Text {
		t.Fatal("draft not durable")
	}
}

func TestAICancelAndConcurrentEditingDoNotOverwriteDraft(t *testing.T) {
	for _, supplement := range []bool{false, true} {
		for _, cancel := range []bool{false, true} {
			t.Run(map[bool]string{false: "generate_", true: "continue_"}[supplement]+map[bool]string{false: "edit", true: "cancel"}[cancel], func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "library")
				doc := statusFixture(t, root)
				app := &App{root: root}
				activity, _ := app.RecordTaskWork(doc.Key, doc.Sheets[0].Name, 4, "真实工作进展")
				original := ""
				if supplement {
					if _, err := app.GenerateDailyReport(activity.Date, false); err != nil {
						t.Fatal(err)
					}
					original = "保留原日报"
					app.SaveDailyReport(activity.Date, original)
					app.RecordTaskWork(doc.Key, doc.Sheets[0].Name, 4, "新增工作")
				}
				started, release := make(chan struct{}), make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					close(started)
					<-release
					w.Write([]byte(`{"choices":[{"message":{"content":"AI draft"}}]}`))
				}))
				defer server.Close()
				app.SaveDailyAISettings(DailyAIUpdate{BaseURL: server.URL, Model: "test"})
				done := make(chan error, 1)
				go func() {
					var err error
					if supplement {
						_, err = app.ContinueDailyReport(activity.Date, true)
					} else {
						_, err = app.GenerateDailyReport(activity.Date, true)
					}
					done <- err
				}()
				<-started
				if cancel {
					app.CancelDailyReport()
				} else {
					if _, err := app.SaveDailyReport(activity.Date, "另行编辑的正文"); err != nil {
						t.Fatal(err)
					}
				}
				close(release)
				if err := <-done; err == nil {
					t.Fatal("stale/cancelled generation committed")
				}
				report, _ := app.GetDailyReport(activity.Date)
				if cancel && report.Draft.Text != original || !cancel && report.Draft.Text != "另行编辑的正文" {
					t.Fatal("draft overwritten")
				}
			})
		}
	}
}

func TestAIWillNotFollowRedirectWithCredential(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	if _, err := requestDailyAI(context.Background(), dailyAIConfig{BaseURL: redirect.URL, Model: "test", APIKey: "fake"}, "2026-10-16", nil); err == nil || reached {
		t.Fatal("followed credential redirect")
	}
}

func TestDailyContinuationPreservesEditedBodyAndTracksNewIDs(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "ids", true: "existing draft"}[legacy], func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "library")
			doc := notesFixture(t, root)
			app := &App{root: root}
			old, err := app.RecordTaskWork(doc.Key, doc.Sheets[0].Name, 4, "OLD_WORK_ALREADY_INCLUDED")
			if err != nil {
				t.Fatal(err)
			}
			initial, err := app.GenerateDailyReport(old.Date, false)
			if err != nil {
				t.Fatal(err)
			}
			if legacy {
				initial.Draft.ActivityIDs = nil
				if _, err := writeDailyDraft(root, old.Date, initial.Draft); err != nil {
					t.Fatal(err)
				}
			}
			original := "我手工核对的日报\n保留这段措辞和空行。\n"
			if _, err := app.SaveDailyReport(old.Date, original); err != nil {
				t.Fatal(err)
			}
			newWork, err := app.RecordTaskWork(doc.Key, doc.Sheets[0].Name, 4, "NEW_WORK_TO_APPEND")
			if err != nil {
				t.Fatal(err)
			}
			continued, err := app.ContinueDailyReport(old.Date, false)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(continued.Draft.Text, original) || !strings.Contains(continued.Draft.Text, newWork.Note) || strings.Contains(continued.Draft.Text, old.Note) || continued.Draft.ActivityCount != 2 || len(continued.Draft.ActivityIDs) != 2 {
				t.Fatal("continuation lost edits or repeated old work", continued.Draft)
			}
			if result, err := app.ContinueDailyReport(old.Date, false); err != nil || result.Notice == "" {
				t.Fatal("expected a normal no-new-records notice", err)
			}
			loaded, _ := app.GetDailyReport(old.Date)
			if loaded.Draft.Text != continued.Draft.Text || loaded.Draft.SavedAt != continued.Draft.SavedAt {
				t.Fatal("no-op continuation modified draft")
			}
			// New work can sort before already included work after a clock adjustment.
			outOfOrder := newTaskActivity(doc, doc.Sheets[0], doc.Sheets[0].Rows[0], "work")
			instant, _ := time.Parse(time.RFC3339Nano, old.At)
			outOfOrder.At = instant.Add(-time.Hour).Format(time.RFC3339Nano)
			outOfOrder.Note = "NEW_EARLIER_TIMESTAMP"
			if _, err := persistActivityDocument(root, doc, outOfOrder); err != nil {
				t.Fatal(err)
			}
			last, err := app.ContinueDailyReport(old.Date, false)
			if err != nil || !strings.HasPrefix(last.Draft.Text, continued.Draft.Text) || strings.Count(last.Draft.Text, newWork.Note) != 1 || !strings.Contains(last.Draft.Text, outOfOrder.Note) {
				t.Fatal("ID tracking failed for out-of-order event", err)
			}
		})
	}
}

func TestAIContinuationOnlySendsNewWorkAndPreservesDraftOnFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	doc := notesFixture(t, root)
	app := &App{root: root}
	old, _ := app.RecordTaskWork(doc.Key, doc.Sheets[0].Name, 4, "OLD_WORK_ALREADY_INCLUDED")
	if _, err := app.GenerateDailyReport(old.Date, false); err != nil {
		t.Fatal(err)
	}
	original := "手工编辑的原正文，必须完整保留。"
	app.SaveDailyReport(old.Date, original)
	var requests int
	mode := "ok"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		raw, _ := reader.LimitedRead(r.Body, 1<<20)
		if strings.Contains(string(raw), old.Note) || strings.Contains(string(raw), original) || !strings.Contains(string(raw), "NEW_WORK_TO_APPEND") || !strings.Contains(string(raw), "只输出这些记录对应的补充工作内容") {
			t.Error("AI continuation received wrong records")
		}
		if mode == "error" {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"完成新增联调工作。"}}]}`))
	}))
	defer server.Close()
	if _, err := app.SaveDailyAISettings(DailyAIUpdate{BaseURL: server.URL, Model: "test"}); err != nil {
		t.Fatal(err)
	}
	if result, err := app.ContinueDailyReport(old.Date, true); err != nil || result.Notice == "" || requests != 0 {
		t.Fatal("called AI without new records")
	}
	app.RecordTaskWork(doc.Key, doc.Sheets[0].Name, 4, "NEW_WORK_TO_APPEND")
	mode = "error"
	if _, err := app.ContinueDailyReport(old.Date, true); err == nil {
		t.Fatal("AI failure accepted")
	}
	failed, _ := app.GetDailyReport(old.Date)
	if failed.Draft.Text != original || failed.Draft.ActivityCount != 1 {
		t.Fatal("failed continuation consumed records or overwrote draft")
	}
	mode = "ok"
	continued, err := app.ContinueDailyReport(old.Date, true)
	if err != nil || continued.Draft.Text != original+"\n\n补充工作\n完成新增联调工作。\n" || continued.Draft.ActivityCount != 2 {
		t.Fatal("AI continuation failed", err)
	}
	if result, err := app.ContinueDailyReport(old.Date, true); err != nil || result.Notice == "" || requests != 2 {
		t.Fatal("AI called twice for already included work")
	}
}
