package execution

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"taskpicker/internal/storage"
)

type ExecutionMilestone struct {
	At   string `json:"at"`
	Text string `json:"text"`
}

type TaskExecutionReport struct {
	Markdown string `json:"markdown"`
	Path     string `json:"path"`
}

// Keep user-facing progress, never terminal output or internal model reasoning.
func ApplyExecutionMilestone(run *TaskExecution, event ExecutionLog) {
	text := strings.TrimSpace(event.Text)
	if event.Kind == "summary" {
		run.ProgressSummary = executionClip(text, 12000)
		return
	}
	if event.Kind == "understanding" || (event.Kind == "message" && strings.HasPrefix(text, "【任务理解】")) {
		if run.Understanding == "" {
			run.Understanding = executionClip(strings.TrimSpace(strings.TrimPrefix(text, "【任务理解】")), 6000)
		}
		return
	}
	switch event.Kind {
	case "system", "progress", "validation":
	case "message":
		if !strings.HasPrefix(text, "【执行节点】") {
			return
		}
		text = strings.TrimSpace(strings.TrimPrefix(text, "【执行节点】"))
	default:
		return
	}
	// Progress is intentionally compact. The final report retains the complete result.
	text = strings.TrimSpace(strings.SplitN(text, "```", 2)[0])
	text = executionClip(text, 1200)
	if text == "" {
		return
	}
	n := len(run.Milestones)
	if n > 0 && run.Milestones[n-1].Text == text {
		return
	}
	// Bound memory even during unusually chatty runs; retain initial and latest nodes.
	if n >= 120 {
		run.Milestones = append(run.Milestones[:1], run.Milestones[2:]...)
	}
	run.Milestones = append(run.Milestones, ExecutionMilestone{At: event.At, Text: text})
}

func executionValidationCommand(command string) bool {
	// Only identify a verification step; do not copy command bodies or their output.
	fields := strings.Fields(strings.ToLower(command))
	for _, field := range fields {
		switch strings.Trim(field, "\"'();") {
		case "test", "test:unit", "test:integration", "pytest", "vitest", "jest", "check", "typecheck", "build", "cargo-nextest":
			return true
		}
	}
	return false
}

func executionReportTime(value string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.Local().Format("2006-01-02 15:04:05")
	}
	return value
}

func ExecutionStatusLabel(status string) string {
	switch status {
	case "preparing":
		return "准备工作区"
	case "running":
		return "正在执行"
	case "stopping":
		return "正在停止"
	case "succeeded":
		return "执行结束 · 待核对"
	case "failed":
		return "执行失败"
	case "stopped":
		return "已停止"
	case "interrupted":
		return "执行中断"
	}
	return status
}

func executionQuote(text string) string {
	return "> " + strings.ReplaceAll(strings.TrimSpace(text), "\n", "\n> ") + "\n\n"
}

func executionMarkdown(run TaskExecution) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# 任务执行记录\n\n- 执行编号：%s\n- 开始时间：%s\n- 执行状态：%s\n", run.ID, executionReportTime(run.StartedAt), ExecutionStatusLabel(run.Status))
	if run.FinishedAt != "" {
		fmt.Fprintf(&out, "- 结束时间：%s\n", executionReportTime(run.FinishedAt))
	}
	if run.PreviousID != "" {
		fmt.Fprintf(&out, "- 继续自：%s\n- 处理链路：%s\n", run.PreviousID, executionRootID(run))
	}
	out.WriteString("\n## 用户原文\n\n### 任务内容\n\n")
	if run.Task == "" {
		out.WriteString("未保存任务原文。\n\n")
	} else {
		out.WriteString(executionQuote(run.Task))
	}
	out.WriteString("### 补充要求\n\n")
	if run.Instructions == "" {
		out.WriteString("无。\n\n")
	} else {
		out.WriteString(executionQuote(run.Instructions))
	}
	if run.ContextSummary != "" {
		out.WriteString("## 本轮接续上下文\n\n### 最初任务\n\n" + executionQuote(executionOriginalTask(run)))
		if run.OriginalInstructions != "" {
			out.WriteString("### 最初要求\n\n" + executionQuote(run.OriginalInstructions))
		}
		out.WriteString("### 上轮进展\n\n" + run.ContextSummary + "\n\n")
	}
	out.WriteString("## AI 理解\n\n")
	if run.Understanding != "" {
		out.WriteString(run.Understanding + "\n\n")
	} else if ExecutionActive(run.Status) {
		out.WriteString("等待 AI 说明任务理解。\n\n")
	} else {
		out.WriteString("本次执行未单独记录 AI 的任务理解。\n\n")
	}
	out.WriteString("## 执行过程（关键节点）\n\n")
	if len(run.Milestones) == 0 {
		out.WriteString("未记录关键执行节点。\n\n")
	}
	for _, node := range run.Milestones {
		fmt.Fprintf(&out, "- **%s** %s\n", executionReportTime(node.At), strings.ReplaceAll(node.Text, "\n", "\n  "))
	}
	if len(run.Milestones) >= 120 {
		out.WriteString("\n关键节点较多，保留开始节点与最近进展。\n")
	}
	out.WriteString("\n## 最后结果\n\n")
	if run.Result != "" {
		out.WriteString(run.Result + "\n\n")
	} else if ExecutionActive(run.Status) {
		out.WriteString("执行中，尚未生成最终结果。\n\n")
	} else {
		out.WriteString("本次执行未生成最终总结。\n\n")
	}
	if run.Error != "" {
		out.WriteString("### 停止或失败原因\n\n" + run.Error + "\n\n")
	}
	if run.Status == "succeeded" {
		out.WriteString("执行结束表示 AI 已结束本轮处理，修改与测试结论仍需核对。\n\n")
	}
	out.WriteString("### 修改文件\n\n")
	if run.ChangedFiles != "" {
		out.WriteString(executionQuote(run.ChangedFiles))
	} else if ExecutionActive(run.Status) {
		out.WriteString("执行结束后汇总。\n\n")
	} else {
		out.WriteString("未记录到文件修改。\n\n")
	}
	if run.ProgressSummary != "" {
		out.WriteString("## 累计进展摘要\n\n" + run.ProgressSummary + "\n\n")
	}
	out.WriteString("## 执行位置\n\n")
	out.WriteString("仓库：\n\n" + executionQuote(run.Repository))
	out.WriteString("独立工作区：\n\n" + executionQuote(run.Workspace))
	out.WriteString("分支：\n\n" + executionQuote(run.Branch))
	return out.String()
}

// Upgrade old runs from persisted observable events without calling AI or inventing a summary.
// Caller holds a.executions. New runs already carry structured key milestones.
func (a *Service) ensureExecutionReportLocked(run TaskExecution) (TaskExecution, error) {
	if run.ReportVersion == 0 {
		file, err := os.Open(filepath.Join(ExecutionFolder(a.root, run.ID), "events.jsonl"))
		if err != nil && !os.IsNotExist(err) {
			return run, err
		}
		if file != nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 4096), 256*1024)
			for scanner.Scan() {
				var event ExecutionLog
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					return run, fmt.Errorf("执行日志无法解析，原文件已保留：%w", err)
				}
				// Old executors sent plain progress messages; retain their short first
				// paragraph when it is not the final answer or a code block.
				text := strings.TrimSpace(event.Text)
				if event.Kind == "message" && text != strings.TrimSpace(run.Result) && !strings.HasPrefix(text, "【任务理解】") && !strings.HasPrefix(text, "【执行节点】") && !strings.Contains(text, "```") {
					event.Kind = "progress"
					event.Text = strings.SplitN(text, "\n\n", 2)[0]
				}
				ApplyExecutionMilestone(&run, event)
			}
			if err := scanner.Err(); err != nil {
				return run, err
			}
		}
		run.ReportVersion = 1
		return run, WriteExecution(a.root, run)
	}
	// Rebuild a missing report file from metadata (for example after moving an old library).
	path := filepath.Join(ExecutionFolder(a.root, run.ID), "report.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return run, WriteExecution(a.root, run)
	} else if err != nil {
		return run, err
	}
	return run, nil
}

func (a *Service) GetTaskExecutionReport(id string) (TaskExecutionReport, error) {
	a.executions.Lock()
	defer a.executions.Unlock()
	result := TaskExecutionReport{}
	run, err := readExecution(a.root, id)
	if err != nil {
		return result, err
	}
	run, err = a.recoverExecutionLocked(run)
	if err != nil {
		return result, err
	}
	run, err = a.ensureExecutionReportLocked(run)
	if err != nil {
		return result, err
	}
	result.Path = filepath.Join(ExecutionFolder(a.root, id), "report.md")
	result.Markdown = executionMarkdown(run)
	if err := storage.AtomicWrite(result.Path, []byte(result.Markdown)); err != nil {
		return TaskExecutionReport{}, err
	}
	return result, nil
}

func (a *Service) OpenTaskExecutionReport(id string) error {
	report, err := a.GetTaskExecutionReport(id)
	if err != nil {
		return err
	}
	opener := "open"
	if runtime.GOOS == "windows" {
		opener = "explorer.exe"
	} else if runtime.GOOS != "darwin" {
		opener = "xdg-open"
	}
	return exec.Command(opener, report.Path).Run()
}

// The contract also supports a standalone summary message, so commentary and
// final output can arrive separately without replacing the actual final result.
func SplitExecutionSummary(text string) (string, string) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "【进展摘要】") {
		return "", strings.TrimSpace(strings.TrimPrefix(text, "【进展摘要】"))
	}
	if index := strings.Index(text, "\n【进展摘要】"); index >= 0 {
		return strings.TrimSpace(text[:index]), strings.TrimSpace(text[index+len("\n【进展摘要】"):])
	}
	return text, ""
}
