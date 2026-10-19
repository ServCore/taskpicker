package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"taskpicker/internal/execution"
)

func TestContinueExecutionReusesChangesAndSummaryAfterRestart(t *testing.T) {
	app, doc, repo := executionFixture(t)
	app.executionRunner = func(_ context.Context, workspace, _ string, log func(string, string) error) (string, error) {
		if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("first fix"), 0600); err != nil {
			return "", err
		}
		if err := log("summary", "## 已完成\n修复权限判断\n## 未解决\n补充异常测试\n## 验证\n未测试真实设备"); err != nil {
			return "", err
		}
		return "第一轮已修改，待补异常测试", nil
	}
	first, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "保留鉴权规则")
	if err != nil {
		t.Fatal(err)
	}
	firstDone := waitExecution(t, app, first.ID).Run
	firstReport, _ := os.ReadFile(filepath.Join(execution.ExecutionFolder(app.root, first.ID), "report.md"))
	// A later edit in the original repository must not block continuation of the
	// existing isolated worktree, nor be overwritten by the continuation.
	if err := os.WriteFile(filepath.Join(repo, "main.txt"), []byte("user local edit"), 0600); err != nil {
		t.Fatal(err)
	}
	doc.Sheets[0].Rows[0].Values[1] = "文档后续补充说明"
	if _, err := persistStatusDocument(app.root, doc); err != nil {
		t.Fatal(err)
	}
	restarted := &App{root: app.root}
	t.Cleanup(func() { restarted.cancelTaskExecutions(); restarted.executionService().Wait() })
	restarted.executionRunner = func(_ context.Context, workspace, prompt string, log func(string, string) error) (string, error) {
		data, _ := os.ReadFile(filepath.Join(workspace, "main.txt"))
		if workspace != first.Workspace || string(data) != "first fix" {
			return "", errors.New("previous changes were lost")
		}
		for _, required := range []string{"无法打开麦克风", "文档后续补充说明", "修复权限判断", "补充异常测试", "未测试真实设备", "本次补上测试", "保留鉴权规则", filepath.Join(execution.ExecutionFolder(app.root, first.ID), "report.md")} {
			if !strings.Contains(prompt, required) {
				return "", errors.New("missing context: " + required)
			}
		}
		if err := os.WriteFile(filepath.Join(workspace, "test.txt"), []byte("regression"), 0600); err != nil {
			return "", err
		}
		if err := log("summary", "## 已完成\n修复权限判断；新增异常测试\n## 验证\n回归通过；未测试真实设备\n## 下一步\n真实设备验收"); err != nil {
			return "", err
		}
		return "第二轮测试已补齐", nil
	}
	second, err := restarted.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, "本次补上测试")
	if err != nil {
		t.Fatal(err)
	}
	secondDone := waitExecution(t, restarted, second.ID).Run
	if secondDone.Status != "succeeded" {
		t.Fatalf("continuation failed: %+v", secondDone)
	}
	if second.ID == first.ID || second.Workspace != first.Workspace || second.Branch != first.Branch || second.RootID != first.ID || second.PreviousID != first.ID || second.ContextSummary == "" {
		t.Fatalf("incorrect continuation identity: %+v", second)
	}
	if !strings.Contains(second.OriginalTask, "无法打开麦克风") || !strings.Contains(second.Task, "文档后续补充说明") {
		t.Fatal("original and current task snapshots were mixed")
	}
	if _, err := os.Stat(filepath.Join(execution.ExecutionFolder(app.root, second.ID), "workspace")); !os.IsNotExist(err) {
		t.Fatal("continuation created a second workspace")
	}
	original, _ := os.ReadFile(filepath.Join(repo, "main.txt"))
	if string(original) != "user local edit" {
		t.Fatal("original repository was modified")
	}
	after, _ := os.ReadFile(filepath.Join(execution.ExecutionFolder(app.root, first.ID), "report.md"))
	if string(after) != string(firstReport) {
		t.Fatal("old execution document was overwritten")
	}
	secondReport, _ := os.ReadFile(filepath.Join(execution.ExecutionFolder(app.root, second.ID), "report.md"))
	if !strings.Contains(string(secondReport), "本轮接续上下文") || !strings.Contains(string(secondReport), "第二轮测试已补齐") {
		t.Fatal("new round report missing context or result")
	}
	if _, err := restarted.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, ""); err == nil || !strings.Contains(err.Error(), "后续执行") {
		t.Fatal("accepted stale round", err)
	}
	progress, err := restarted.GetTaskExecutionProgress(doc.Key, doc.Sheets[0].Name, 4)
	if err != nil || progress.RunID != second.ID || !strings.Contains(progress.Markdown, "真实设备验收") {
		t.Fatalf("progress missing: %+v %v", progress, err)
	}
	raw, _ := os.ReadFile(progress.Path)
	if string(raw) != progress.Markdown {
		t.Fatal("task progress MD was not persisted")
	}
	// Reading old run data cannot roll current progress back.
	if _, err := restarted.GetTaskExecutionReport(firstDone.ID); err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(progress.Path)
	if string(current) != progress.Markdown {
		t.Fatal("history read rewound task progress")
	}
	// A third round receives the updated cumulative summary, not just the first result.
	restarted.executionRunner = func(_ context.Context, workspace, prompt string, _ func(string, string) error) (string, error) {
		if !strings.Contains(prompt, "新增异常测试") || !strings.Contains(prompt, "真实设备验收") || !strings.Contains(prompt, "保留鉴权规则") || workspace != first.Workspace {
			return "", errors.New("summary did not advance")
		}
		if _, err := os.Stat(filepath.Join(workspace, "test.txt")); err != nil {
			return "", err
		}
		return "第三轮核对完成", nil
	}
	third, err := restarted.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, second.ID, "继续核对")
	if err != nil {
		t.Fatal(err)
	}
	if done := waitExecution(t, restarted, third.ID).Run; done.Status != "succeeded" {
		t.Fatalf("third round: %+v", done)
	}
}

func TestContinuationLegacyFallbackAndIndependentRestart(t *testing.T) {
	app, doc, repo := executionFixture(t)
	app.executionRunner = func(_ context.Context, workspace, _ string, log func(string, string) error) (string, error) {
		if err := log("progress", "已定位旧问题"); err != nil {
			return "", err
		}
		return "旧执行结果：依赖缺失，未运行验证", errors.New("依赖缺失")
	}
	first, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "旧要求")
	if err != nil {
		t.Fatal(err)
	}
	legacy := waitExecution(t, app, first.ID).Run
	legacy.RootID = ""
	legacy.OriginalTask = ""
	legacy.ProgressSummary = ""
	legacy.ReportVersion = 0
	if err := execution.WriteExecution(app.root, legacy); err != nil {
		t.Fatal(err)
	}
	app.executionRunner = func(_ context.Context, workspace, prompt string, _ func(string, string) error) (string, error) {
		if workspace != first.Workspace || !strings.Contains(prompt, "旧执行结果") || !strings.Contains(prompt, "依赖缺失") || !strings.Contains(prompt, "未运行验证") || !strings.Contains(prompt, "已定位旧问题") {
			return "", errors.New("legacy fallback context missing")
		}
		return "已继续旧执行", nil
	}
	continued, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, "继续旧记录")
	if err != nil {
		t.Fatal(err)
	}
	if done := waitExecution(t, app, continued.ID).Run; done.Status != "succeeded" {
		t.Fatalf("legacy continuation: %+v", done)
	}
	app.executionRunner = func(_ context.Context, workspace, prompt string, _ func(string, string) error) (string, error) {
		data, _ := os.ReadFile(filepath.Join(workspace, "main.txt"))
		if workspace == first.Workspace || string(data) != "original" || strings.Contains(prompt, "<previous_progress>") || strings.Contains(prompt, "旧执行结果") {
			return "", errors.New("restart inherited stale workspace/context")
		}
		return "新的独立执行结果", nil
	}
	fresh, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "重新开始")
	if err != nil {
		t.Fatal(err)
	}
	if done := waitExecution(t, app, fresh.ID).Run; done.Status != "succeeded" {
		t.Fatalf("restart: %+v", done)
	}
	progress, err := app.GetTaskExecutionProgress(doc.Key, doc.Sheets[0].Name, 4)
	if err != nil || progress.RunID != fresh.ID || !strings.Contains(progress.Markdown, "新的独立执行结果") || strings.Contains(progress.Markdown, "旧执行结果") {
		t.Fatalf("restart mixed task context: %+v %v", progress, err)
	}
	if _, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, continued.ID, ""); err != nil {
		t.Fatal("independent old workspace should remain continuable", err)
	}
	app.cancelTaskExecutions()
	app.executionService().Wait()
}

func TestContinuationRejectsMissingChangedAndForeignWorkspaces(t *testing.T) {
	app, doc, repo := executionFixture(t)
	app.executionRunner = func(context.Context, string, string, func(string, string) error) (string, error) {
		return "完成", nil
	}
	first, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	done := waitExecution(t, app, first.ID).Run
	otherTask := done
	otherTask.TaskID = strings.Repeat("f", 24)
	if err := execution.WriteExecution(app.root, otherTask); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, ""); err == nil || !strings.Contains(err.Error(), "当前任务") {
		t.Fatal("accepted foreign task", err)
	}
	if err := execution.WriteExecution(app.root, done); err != nil {
		t.Fatal(err)
	}
	if _, err := execution.ExecutionGit(context.Background(), first.Workspace, "checkout", "-b", "user-branch"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, ""); err == nil || !strings.Contains(err.Error(), "分支") {
		t.Fatal("accepted changed branch", err)
	}
	if _, err := execution.ExecutionGit(context.Background(), first.Workspace, "checkout", first.Branch); err != nil {
		t.Fatal(err)
	}
	moved := first.Workspace + "-moved"
	if err := os.Rename(first.Workspace, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, ""); err == nil || !strings.Contains(err.Error(), "不存在或已移动") {
		t.Fatal("accepted missing workspace", err)
	}
	if err := os.Rename(moved, first.Workspace); err != nil {
		t.Fatal(err)
	}
	tampered := done
	tampered.Workspace = repo
	if err := execution.WriteExecution(app.root, tampered); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, ""); err == nil {
		t.Fatal("accepted original directory as continuation workspace")
	}
	if err := execution.WriteExecution(app.root, done); err != nil {
		t.Fatal(err)
	}
	state, err := app.GetLibrary()
	if err != nil {
		t.Fatal(err)
	}
	state.Projects[0].LocalPaths = nil
	state.Projects[0].LocalPath = ""
	if _, err := app.SaveProject(state.Projects[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, ""); err == nil || !strings.Contains(err.Error(), "绑定目录") {
		t.Fatal("continued unbound repository", err)
	}
	if _, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, "../../escape", ""); err == nil {
		t.Fatal("accepted invalid ID")
	}
}

func TestStoppedContinuationCarriesPartialProgressAndRetainsSingleRunLimit(t *testing.T) {
	app, doc, repo := executionFixture(t)
	app.executionRunner = func(context.Context, string, string, func(string, string) error) (string, error) {
		return "初次结果", nil
	}
	first, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	waitExecution(t, app, first.ID)
	started := make(chan struct{})
	app.executionRunner = func(ctx context.Context, workspace, _ string, log func(string, string) error) (string, error) {
		if err := os.WriteFile(filepath.Join(workspace, "partial.txt"), []byte("partial"), 0600); err != nil {
			return "", err
		}
		if err := log("progress", "已改权限处理，测试尚未执行"); err != nil {
			return "", err
		}
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}
	second, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, "补充测试")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("not started")
	}
	if _, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, first.ID, ""); err == nil {
		t.Fatal("accepted concurrent continuation")
	}
	if _, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, ""); err == nil {
		t.Fatal("accepted concurrent fresh run")
	}
	if err := app.StopTaskExecution(second.ID); err != nil {
		t.Fatal(err)
	}
	stopped := waitExecution(t, app, second.ID).Run
	if stopped.Status != "stopped" || !strings.Contains(stopped.ChangedFiles, "partial.txt") {
		t.Fatalf("partial work lost: %+v", stopped)
	}
	app.executionRunner = func(_ context.Context, workspace, prompt string, _ func(string, string) error) (string, error) {
		if !strings.Contains(prompt, "已停止") || !strings.Contains(prompt, "测试尚未执行") || !strings.Contains(prompt, "初次结果") {
			return "", errors.New("partial context lost")
		}
		if _, err := os.Stat(filepath.Join(workspace, "partial.txt")); err != nil {
			return "", err
		}
		return "已恢复", nil
	}
	third, err := app.ContinueTaskExecution(doc.Key, doc.Sheets[0].Name, 4, second.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if done := waitExecution(t, app, third.ID).Run; done.Status != "succeeded" {
		t.Fatalf("resume partial: %+v", done)
	}
}

func TestSummaryMarkerExtractsFinalResultWithoutLosingCumulativeSummary(t *testing.T) {
	executable := fakeExecutionBinary(t, `printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"## 本轮结果\n回归通过\n\n【进展摘要】\n## 累计完成\n修复登录和权限\n## 待办\n真实设备验证"}}' '{"type":"turn.completed"}'`)
	run := execution.TaskExecution{}
	result, err := execution.RunCodexExecutable(context.Background(), executable, t.TempDir(), "task", func(kind, text string) error {
		execution.ApplyExecutionMilestone(&run, execution.ExecutionLog{Kind: kind, Text: text})
		return nil
	})
	if err != nil || !strings.Contains(result, "回归通过") || strings.Contains(result, "【进展摘要】") || !strings.Contains(run.ProgressSummary, "真实设备验证") {
		t.Fatalf("summary parsing: result=%s summary=%s err=%v", result, run.ProgressSummary, err)
	}
	body, summary := execution.SplitExecutionSummary("【进展摘要】\n仅摘要")
	if body != "" || summary != "仅摘要" {
		t.Fatalf("standalone summary: %q %q", body, summary)
	}
}
