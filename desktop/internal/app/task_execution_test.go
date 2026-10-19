package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"taskpicker/internal/execution"
	"taskpicker/internal/model"
)

func executionFixture(t *testing.T) (*App, model.Document, string) {
	t.Helper()
	app := &App{root: t.TempDir()}
	doc := fixture(t, app.root)
	repo := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init")
	if err := os.WriteFile(filepath.Join(repo, "main.txt"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "main.txt")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture")
	state, err := app.GetLibrary()
	if err != nil {
		t.Fatal(err)
	}
	project := state.Projects[0]
	project.LocalPaths = []string{repo}
	if _, err = app.SaveProject(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.cancelTaskExecutions(); app.executionService().Wait() })
	return app, doc, repo
}

func waitExecution(t *testing.T, app *App, id string) execution.TaskExecutionUpdate {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		update, err := app.GetTaskExecution(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !execution.ExecutionActive(update.Run.Status) {
			return update
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("execution did not finish")
	return execution.TaskExecutionUpdate{}
}

func TestTaskExecutionIsolatedChangesAndPersistedHistory(t *testing.T) {
	app, doc, repo := executionFixture(t)
	doc.Sheets[0].Rows[0].RecordID = "record-one"
	if _, err := persistStatusDocument(app.root, doc); err != nil {
		t.Fatal(err)
	}
	app.executionRunner = func(ctx context.Context, workspace, prompt string, log func(string, string) error) (string, error) {
		if workspace == repo || !strings.Contains(prompt, "无法打开麦克风") || !strings.Contains(prompt, "先复现") {
			return "", errors.New("incorrect task context")
		}
		if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("fixed"), 0600); err != nil {
			return "", err
		}
		if err := log("command", "相关测试已通过"); err != nil {
			return "", err
		}
		return "已修复并验证", nil
	}
	run, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "先复现")
	if err != nil {
		t.Fatal(err)
	}
	update := waitExecution(t, app, run.ID)
	if update.Run.Status != "succeeded" || update.Run.Result != "已修复并验证" || !strings.Contains(update.Run.ChangedFiles, "main.txt") {
		t.Fatalf("bad result: %+v", update)
	}
	original, _ := os.ReadFile(filepath.Join(repo, "main.txt"))
	if string(original) != "original" {
		t.Fatal("original repository changed")
	}
	fixed, _ := os.ReadFile(filepath.Join(run.Workspace, "main.txt"))
	if string(fixed) != "fixed" {
		t.Fatal("workspace changes missing")
	}
	restored := &App{root: app.root}
	history, err := restored.GetTaskExecutions(doc.Key, doc.Sheets[0].Name, 4)
	if err != nil || len(history) != 1 || history[0].Result != "已修复并验证" {
		t.Fatalf("history lost: %+v %v", history, err)
	}
	incremental, err := restored.GetTaskExecution(run.ID, update.Next)
	if err != nil || len(incremental.Logs) != 0 {
		t.Fatal("incremental logs duplicated", err)
	}
	unchanged, _ := ReadDocument(app.root, doc.Key)
	if execution.ExecutionTaskText(unchanged.Sheets[0], unchanged.Sheets[0].Rows[0]) != execution.ExecutionTaskText(doc.Sheets[0], doc.Sheets[0].Rows[0]) {
		t.Fatal("task was edited by executor")
	}
	// Smart-sheet record identity survives remote row moves and project moves.
	unchanged.Sheets[0].Rows[0].Number = 19
	if _, err = persistStatusDocument(app.root, unchanged); err != nil {
		t.Fatal(err)
	}
	if _, err = app.SaveProject(model.Project{Name: "moved project", DocumentKeys: []string{doc.Key}}); err != nil {
		t.Fatal(err)
	}
	history, err = restored.GetTaskExecutions(doc.Key, doc.Sheets[0].Name, 19)
	if err != nil || len(history) != 1 || history[0].ID != run.ID {
		t.Fatalf("record history lost after move: %+v %v", history, err)
	}
}

func TestTaskExecutionCancellationAndSingleRun(t *testing.T) {
	app, doc, repo := executionFixture(t)
	started := make(chan struct{})
	app.executionRunner = func(ctx context.Context, _, _ string, log func(string, string) error) (string, error) {
		close(started)
		if err := log("system", "waiting"); err != nil {
			return "", err
		}
		<-ctx.Done()
		return "", ctx.Err()
	}
	run, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("runner not started")
	}
	if _, err = app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, ""); err == nil {
		t.Fatal("accepted concurrent run")
	}
	if err = app.StopTaskExecution(run.ID); err != nil {
		t.Fatal(err)
	}
	update := waitExecution(t, app, run.ID)
	if update.Run.Status != "stopped" || update.Run.FinishedAt == "" {
		t.Fatalf("not stopped: %+v", update.Run)
	}
	if _, err = os.Stat(run.Workspace); err != nil {
		t.Fatal("workspace lost after stop")
	}
	app.executionService().Wait()
}

func TestTaskExecutionShutdownCancelsAndRejectsNewRuns(t *testing.T) {
	app, doc, repo := executionFixture(t)
	app.executionRunner = func(ctx context.Context, _, _ string, _ func(string, string) error) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	run, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	Shutdown(app, context.Background())
	update, err := app.GetTaskExecution(run.ID, 0)
	if err != nil || update.Run.Status != "stopped" {
		t.Fatalf("shutdown lost final state: %+v %v", update, err)
	}
	if _, err = app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, ""); err == nil {
		t.Fatal("started execution during shutdown")
	}
}

func TestTaskExecutionRejectsDirtyAndUnboundRepositories(t *testing.T) {
	app, doc, repo := executionFixture(t)
	called := false
	app.executionRunner = func(context.Context, string, string, func(string, string) error) (string, error) {
		called = true
		return "", nil
	}
	if _, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, t.TempDir(), ""); err == nil {
		t.Fatal("accepted unbound repository")
	}
	if err := os.WriteFile(filepath.Join(repo, "main.txt"), []byte("user edits"), 0600); err != nil {
		t.Fatal(err)
	}
	run, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	update := waitExecution(t, app, run.ID)
	if called || update.Run.Status != "failed" || !strings.Contains(update.Run.Error, "未提交") {
		t.Fatalf("dirty repository executed: %+v", update.Run)
	}
	raw, _ := os.ReadFile(filepath.Join(repo, "main.txt"))
	if string(raw) != "user edits" {
		t.Fatal("user changes overwritten")
	}
}

func TestTaskExecutionRecoversInterruptedRunAndValidatesIDs(t *testing.T) {
	app, doc, repo := executionFixture(t)
	run := execution.TaskExecution{ID: strings.Repeat("a", 24), TaskID: execution.ExecutionTaskID(doc, doc.Sheets[0], doc.Sheets[0].Rows[0]), Status: "running", Repository: repo}
	if err := execution.WriteExecution(app.root, run); err != nil {
		t.Fatal(err)
	}
	update, err := app.GetTaskExecution(run.ID, 0)
	if err != nil || update.Run.Status != "interrupted" {
		t.Fatalf("not recovered: %+v %v", update, err)
	}
	if _, err = app.GetTaskExecution("../../escape", 0); err == nil {
		t.Fatal("invalid id accepted")
	}
	if err = app.OpenTaskExecutionWorkspace(run.ID); err == nil {
		t.Fatal("invalid workspace opened")
	}
}

func TestTaskExecutionLogsPaginationAndFailure(t *testing.T) {
	app, doc, repo := executionFixture(t)
	app.executionRunner = func(_ context.Context, _, _ string, log func(string, string) error) (string, error) {
		for i := 0; i < 110; i++ {
			if err := log("message", "progress"); err != nil {
				return "", err
			}
		}
		return "已定位问题", errors.New("验证失败")
	}
	run, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	update := waitExecution(t, app, run.ID)
	if update.Run.Status != "failed" || update.Run.Result != "已定位问题" || update.Run.Error != "验证失败" {
		t.Fatalf("failure lost: %+v", update.Run)
	}
	if len(update.Logs) != 100 {
		t.Fatalf("pagination missing: %d", len(update.Logs))
	}
	rest, err := app.GetTaskExecution(run.ID, update.Next)
	if err != nil || len(rest.Logs) != 12 || rest.Logs[0].Sequence != 101 {
		t.Fatalf("remaining logs: %+v %v", rest, err)
	}
}

func fakeExecutionBinary(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake executable")
	}
	path := filepath.Join(t.TempDir(), "fake-codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat >/dev/null\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCodexEventAdapterAndCancellation(t *testing.T) {
	executable := fakeExecutionBinary(t, `printf '%s\n' '{"type":"item.completed","item":{"type":"command_execution","command":"go test ./...","status":"completed","aggregated_output":"PASS"}}' '{"type":"item.completed","item":{"type":"file_change","changes":[{"path":"main.go","kind":"update"}]}}' '{"type":"item.completed","item":{"type":"agent_message","text":"已修复并通过测试"}}' '{"type":"turn.completed"}'`)
	logs := []string{}
	result, err := execution.RunCodexExecutable(context.Background(), executable, t.TempDir(), "task", func(kind, text string) error { logs = append(logs, kind+":"+text); return nil })
	if err != nil || result != "已修复并通过测试" || !strings.Contains(strings.Join(logs, "\n"), "1 个文件") || !strings.Contains(strings.Join(logs, "\n"), "验证命令执行结束") || strings.Contains(strings.Join(logs, "\n"), "PASS") {
		t.Fatalf("adapter result: %q %v %v", result, logs, err)
	}
	failed := fakeExecutionBinary(t, `printf '%s\n' '{"type":"turn.failed","error":{"message":"模型服务不可用"}}'`)
	_, err = execution.RunCodexExecutable(context.Background(), failed, t.TempDir(), "task", func(string, string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "模型服务不可用") {
		t.Fatalf("missing model error: %v", err)
	}
	hanging := fakeExecutionBinary(t, "sleep 30 &\nwait\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = execution.RunCodexExecutable(ctx, hanging, t.TempDir(), "task", func(string, string) error { return nil })
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("executor descendants did not stop: %v %s", err, time.Since(started))
	}
}
