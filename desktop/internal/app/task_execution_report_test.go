package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"taskpicker/internal/execution"
)

func TestExecutionReportsAreIndependentAndSurviveRestart(t *testing.T) {
	app, doc, repo := executionFixture(t)
	app.executionRunner = func(_ context.Context, _, prompt string, log func(string, string) error) (string, error) {
		for _, event := range [][2]string{{"understanding", "修复麦克风无法打开的问题，限定在当前仓库。"}, {"progress", "已定位权限判断错误。"}, {"command", "RAW_SOURCE_AND_COMMAND_OUTPUT"}, {"diagnostic", "VERBOSE_DIAGNOSTICS"}, {"validation", "相关回归测试通过。"}} {
			if err := log(event[0], event[1]); err != nil {
				return "", err
			}
		}
		if !strings.Contains(prompt, "【任务理解】") || !strings.Contains(prompt, "【执行节点】") {
			t.Error("missing reporting instructions")
		}
		if strings.Contains(prompt, "第二轮要求") {
			return "第二轮修复结果", nil
		}
		return "第一轮修复结果\n\n- 已运行相关测试", nil
	}
	first, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "第一轮要求\n保留原文")
	if err != nil {
		t.Fatal(err)
	}
	waitExecution(t, app, first.ID)
	firstPath := filepath.Join(execution.ExecutionFolder(app.root, first.ID), "report.md")
	before, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.StartTaskExecution(doc.Key, doc.Sheets[0].Name, 4, repo, "第二轮要求")
	if err != nil {
		t.Fatal(err)
	}
	waitExecution(t, app, second.ID)
	restarted := &App{root: app.root}
	report, err := restarted.GetTaskExecutionReport(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Path != firstPath || report.Markdown != string(before) {
		t.Fatal("first report changed or was lost")
	}
	for _, expected := range []string{"## 用户原文", "无法打开麦克风", "第一轮要求\n> 保留原文", "## AI 理解", "修复麦克风无法打开", "## 执行过程（关键节点）", "已定位权限判断错误", "相关回归测试通过", "## 最后结果", "第一轮修复结果"} {
		if !strings.Contains(report.Markdown, expected) {
			t.Errorf("missing %q", expected)
		}
	}
	for _, excluded := range []string{"RAW_SOURCE_AND_COMMAND_OUTPUT", "VERBOSE_DIAGNOSTICS", "第二轮修复结果"} {
		if strings.Contains(report.Markdown, excluded) {
			t.Errorf("unexpected %q", excluded)
		}
	}
	other, err := restarted.GetTaskExecutionReport(second.ID)
	if err != nil || other.Path == report.Path || !strings.Contains(other.Markdown, "第二轮修复结果") || strings.Contains(other.Markdown, "第一轮修复结果") {
		t.Fatalf("reports mixed: %+v %v", other, err)
	}
}

func TestExecutionLegacyReportMigrationAndTerminalStates(t *testing.T) {
	app, doc, _ := executionFixture(t)
	run := execution.TaskExecution{ID: strings.Repeat("a", 24), TaskID: execution.ExecutionTaskID(doc, doc.Sheets[0], doc.Sheets[0].Rows[0]), Task: "历史任务原文", Instructions: "原始要求", Status: "failed", Result: "已定位，未完成验证", Error: "依赖不可用"}
	if err := execution.WriteExecution(app.root, run); err != nil {
		t.Fatal(err)
	}
	events := []execution.ExecutionLog{{Sequence: 1, Kind: "system", Text: "工作区已建立"}, {Sequence: 2, Kind: "command", Text: "for each source line..."}, {Sequence: 3, Kind: "message", Text: "已定位，未完成验证"}}
	var lines []byte
	for _, event := range events {
		raw, _ := json.Marshal(event)
		lines = append(lines, append(raw, '\n')...)
	}
	folder := execution.ExecutionFolder(app.root, run.ID)
	if err := os.WriteFile(filepath.Join(folder, "events.jsonl"), lines, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(folder, "report.md")); err != nil {
		t.Fatal(err)
	}
	history, err := app.GetTaskExecutions(doc.Key, doc.Sheets[0].Name, 4)
	if err != nil || len(history) != 1 || history[0].ReportVersion != 1 {
		t.Fatalf("migration failed: %+v %v", history, err)
	}
	report, err := app.GetTaskExecutionReport(run.ID)
	if err != nil || !strings.Contains(report.Markdown, "未单独记录 AI") || !strings.Contains(report.Markdown, "工作区已建立") || strings.Contains(report.Markdown, "for each source") {
		t.Fatalf("bad migrated report %+v %v", report, err)
	}
	raw, err := os.ReadFile(filepath.Join(folder, "report.md"))
	if err != nil || string(raw) != report.Markdown {
		t.Fatal("report not saved", err)
	}
	originalEvents, _ := os.ReadFile(filepath.Join(folder, "events.jsonl"))
	if string(originalEvents) != string(lines) {
		t.Fatal("legacy events were modified")
	}
	for _, status := range []string{"failed", "stopped", "interrupted"} {
		run.Status = status
		if err := execution.WriteExecution(app.root, run); err != nil {
			t.Fatal(err)
		}
		report, err = app.GetTaskExecutionReport(run.ID)
		if err != nil || !strings.Contains(report.Markdown, execution.ExecutionStatusLabel(status)) || !strings.Contains(report.Markdown, "依赖不可用") || !strings.Contains(report.Markdown, "已定位，未完成验证") {
			t.Errorf("lost terminal result for %s: %v", status, err)
		}
	}
	if _, err := app.GetTaskExecutionReport("../../escape"); err == nil {
		t.Fatal("invalid ID accepted")
	}
}

func TestCodexReportsUnderstandingAndKeyNodesWithoutCommandBodies(t *testing.T) {
	executable := fakeExecutionBinary(t, `printf '%s\n' '{"type":"item.started","item":{"type":"agent_message","text":"partial"}}' '{"type":"item.completed","item":{"type":"agent_message","text":"【任务理解】修复登录，保持鉴权规则"}}' '{"type":"item.completed","item":{"type":"agent_message","text":"【执行节点】已定位空值判断"}}' '{"type":"item.completed","item":{"type":"command_execution","command":"npm test","status":"completed","exit_code":1,"aggregated_output":"HUGE_RAW_OUTPUT"}}' '{"type":"item.completed","item":{"type":"agent_message","text":"## 结果\n修复完成，测试未通过"}}' '{"type":"turn.completed"}'`)
	run := execution.TaskExecution{}
	logs := []string{}
	result, err := execution.RunCodexExecutable(context.Background(), executable, t.TempDir(), "task", func(kind, text string) error {
		logs = append(logs, kind+":"+text)
		execution.ApplyExecutionMilestone(&run, execution.ExecutionLog{Kind: kind, Text: text})
		return nil
	})
	if err != nil || run.Understanding != "修复登录，保持鉴权规则" || !strings.Contains(result, "测试未通过") {
		t.Fatalf("summary data lost: %+v %s %v", run, result, err)
	}
	all := strings.Join(logs, "\n")
	if strings.Contains(all, "partial") || strings.Contains(all, "HUGE_RAW_OUTPUT") || strings.Contains(all, "npm test") || !strings.Contains(all, "退出码 1") {
		t.Fatalf("unexpected detailed or incorrect logs: %s", all)
	}
}
