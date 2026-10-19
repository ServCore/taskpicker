package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"taskpicker/internal/storage"
)

type TaskExecutionProgress struct {
	RunID    string `json:"run_id"`
	RootID   string `json:"root_id"`
	Status   string `json:"status"`
	Markdown string `json:"markdown"`
	Path     string `json:"path"`
}

type executionProgressPointer struct {
	RunID     string `json:"run_id"`
	StartedAt string `json:"started_at"`
}

func executionRootID(run TaskExecution) string {
	if run.RootID != "" {
		return run.RootID
	}
	return run.ID // Pre-continuation records own their worktree.
}

func executionOriginalTask(run TaskExecution) string {
	if run.OriginalTask != "" {
		return run.OriginalTask
	}
	return run.Task
}

func executionProgressFolder(root, taskID string) string {
	return filepath.Join(root, "task-executions", "tasks", taskID)
}

// The summary is cumulative when supplied by AI. Interrupted/legacy runs fall back
// to their saved facts and the previous summary, without making another model call.
func executionCarryover(run TaskExecution) string {
	var out strings.Builder
	fmt.Fprintf(&out, "记录编号：%s\n记录状态：%s\n", run.ID, ExecutionStatusLabel(run.Status))
	if run.Error != "" {
		out.WriteString("停止或失败原因：" + executionClip(run.Error, 1500) + "\n")
	}
	if run.ProgressSummary != "" {
		out.WriteString("\n累计进展摘要（AI 自述，需以实际代码和验证为准）：\n\n" + executionClip(run.ProgressSummary, 12000) + "\n")
	} else {
		out.WriteString("\n以下按已保存记录整理；本轮未返回专门的累计摘要。\n")
		if run.Instructions != "" {
			out.WriteString("\n本轮用户要求：\n" + executionClip(run.Instructions, 1500) + "\n")
		}
		if run.Result != "" {
			out.WriteString("\n本轮结果（AI 自述）：\n" + executionClip(run.Result, 6500) + "\n")
		}
		if run.Understanding != "" {
			out.WriteString("\n本轮任务理解：\n" + executionClip(run.Understanding, 1000) + "\n")
		}
		nodes := run.Milestones
		if len(nodes) > 5 {
			nodes = nodes[len(nodes)-5:]
		}
		if len(nodes) > 0 {
			out.WriteString("\n最近关键节点：\n")
		}
		for _, node := range nodes {
			out.WriteString("- " + executionClip(node.Text, 400) + "\n")
		}
		if run.ContextSummary != "" {
			out.WriteString("\n此前已知进展：\n" + executionClip(run.ContextSummary, 3500) + "\n")
		}
	}
	if run.ChangedFiles != "" {
		out.WriteString("\n上次记录的工作区修改（累计，启动后重新检查）：\n" + executionClip(run.ChangedFiles, 1500) + "\n")
	}
	out.WriteString("\n未明确验证的内容不能当作已验证。历史摘要可能不完整，继续前核对当前工作区；需要细节时查阅本链路的历史 MD。")
	return executionClip(out.String(), 16000)
}

func executionProgressMarkdown(run TaskExecution) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# 任务当前进展\n\n- 最近执行：%s\n- 处理链路：%s\n- 更新状态：%s\n- 最近开始：%s\n", run.ID, executionRootID(run), ExecutionStatusLabel(run.Status), executionReportTime(run.StartedAt))
	if run.FinishedAt != "" {
		fmt.Fprintf(&out, "- 最近结束：%s\n", executionReportTime(run.FinishedAt))
	}
	out.WriteString("\n## 原始任务\n\n" + executionQuote(executionOriginalTask(run)))
	if run.OriginalInstructions != "" {
		out.WriteString("## 最初要求\n\n" + executionQuote(run.OriginalInstructions))
	}
	out.WriteString("## 当前进展与待办\n\n" + executionCarryover(run) + "\n\n")
	out.WriteString("## 当前工作区\n\n" + executionQuote(run.Workspace))
	out.WriteString("分支：\n\n" + executionQuote(run.Branch))
	out.WriteString("本摘要跟随最近一次执行所在的处理链路；重新执行会建立新链路，已有工作区和各轮日志保留。\n")
	return out.String()
}

// Called under the execution lock. Reading or upgrading older history must never
// roll the task's current progress pointer back to an earlier round.
func writeTaskExecutionProgress(root string, run TaskExecution) error {
	if !storage.KeyPattern.MatchString(run.TaskID) {
		return nil
	}
	folder := executionProgressFolder(root, run.TaskID)
	if err := os.MkdirAll(folder, 0700); err != nil {
		return err
	}
	pointerPath := filepath.Join(folder, "progress.json")
	var current executionProgressPointer
	if raw, err := os.ReadFile(pointerPath); err == nil {
		if json.Unmarshal(raw, &current) != nil {
			return errors.New("任务进展索引无法解析，原文件已保留。")
		}
		if current.RunID != run.ID && current.StartedAt > run.StartedAt {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := storage.AtomicWrite(filepath.Join(folder, "progress.md"), []byte(executionProgressMarkdown(run))); err != nil {
		return err
	}
	raw, err := json.Marshal(executionProgressPointer{RunID: run.ID, StartedAt: run.StartedAt})
	if err != nil {
		return err
	}
	return storage.AtomicWrite(pointerPath, raw)
}

func (a *Service) GetTaskExecutionProgress(key, sheet string, row int) (TaskExecutionProgress, error) {
	// GetTaskExecutions does recovery/migration, and returns newest first.
	runs, err := a.GetTaskExecutions(key, sheet, row)
	if err != nil {
		return TaskExecutionProgress{}, err
	}
	if len(runs) == 0 {
		return TaskExecutionProgress{}, nil
	}
	a.executions.Lock()
	defer a.executions.Unlock()
	// Refresh after taking the lock in case the active execution advanced meanwhile.
	run, err := readExecution(a.root, runs[0].ID)
	if err != nil {
		return TaskExecutionProgress{}, err
	}
	if err := writeTaskExecutionProgress(a.root, run); err != nil {
		return TaskExecutionProgress{}, err
	}
	return TaskExecutionProgress{RunID: run.ID, RootID: executionRootID(run), Status: run.Status, Markdown: executionProgressMarkdown(run), Path: filepath.Join(executionProgressFolder(a.root, run.TaskID), "progress.md")}, nil
}

func (a *Service) OpenTaskExecutionProgress(key, sheet string, row int) error {
	progress, err := a.GetTaskExecutionProgress(key, sheet, row)
	if err != nil {
		return err
	}
	if progress.Path == "" {
		return errors.New("任务尚无执行进展。")
	}
	opener := "open"
	if runtime.GOOS == "windows" {
		opener = "explorer.exe"
	} else if runtime.GOOS != "darwin" {
		opener = "xdg-open"
	}
	return exec.Command(opener, progress.Path).Run()
}

func (a *Service) ContinueTaskExecution(key, sheet string, row int, previousID, instructions string) (TaskExecution, error) {
	if !storage.KeyPattern.MatchString(previousID) {
		return TaskExecution{}, errors.New("请选择要继续的执行记录。")
	}
	return a.startTaskExecution(key, sheet, row, "", instructions, previousID)
}

// Validation is also used before opening a continued worktree. Never silently
// substitute a clean/new workspace when the user's previous changes are missing.
func (a *Service) validateExecutionWorkspace(ctx context.Context, run TaskExecution) error {
	rootID := executionRootID(run)
	if !storage.KeyPattern.MatchString(rootID) {
		return errors.New("执行工作区编号无效。")
	}
	owner, err := readExecution(a.root, rootID)
	if err != nil {
		return err
	}
	path := filepath.Join(ExecutionFolder(a.root, rootID), "workspace")
	if run.Workspace != path || owner.Workspace != path || owner.TaskID != run.TaskID || owner.Repository != run.Repository || run.Branch != "taskpicker/"+rootID {
		return errors.New("执行工作区与历史记录不匹配，已保留原记录。")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("上次工作区不存在或已移动；请恢复目录，或选择重新执行。")
	}
	top, err := ExecutionGit(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return errors.New("上次工作区已不可用；请恢复 Git 工作区，或选择重新执行。")
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil || top != actual {
		return errors.New("上次 Git 工作区路径不匹配。")
	}
	canonicalDir := func(base, value string) (string, error) {
		if !filepath.IsAbs(value) {
			value = filepath.Join(base, value)
		}
		return filepath.EvalSymlinks(value)
	}
	sourceDir, err := ExecutionGit(ctx, run.Repository, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	targetDir, err := ExecutionGit(ctx, path, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	sourceDir, err = canonicalDir(run.Repository, sourceDir)
	if err != nil {
		return err
	}
	targetDir, err = canonicalDir(path, targetDir)
	if err != nil || sourceDir != targetDir {
		return errors.New("工作区已不属于原代码仓库。")
	}
	branch, err := ExecutionGit(ctx, path, "symbolic-ref", "--short", "HEAD")
	if err != nil || branch != run.Branch {
		return errors.New("工作区分支已变化；请恢复原执行分支后继续。")
	}
	return nil
}

func (a *Service) continuationContextLocked(previous TaskExecution) (TaskExecution, []string, error) {
	// A worktree is mutable. Only its newest round can be continued, so the supplied
	// summary always describes the most recent work in that worktree.
	entries, err := os.ReadDir(filepath.Join(a.root, "task-executions"))
	if err != nil {
		return previous, nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !storage.KeyPattern.MatchString(entry.Name()) {
			continue
		}
		other, err := readExecution(a.root, entry.Name())
		if err != nil {
			return previous, nil, err
		}
		if other.TaskID == previous.TaskID && other.Workspace == previous.Workspace && other.StartedAt > previous.StartedAt {
			return previous, nil, errors.New("这个工作区已有后续执行，请选择同一工作区的最新记录继续。")
		}
	}
	previous, err = a.ensureExecutionReportLocked(previous)
	if err != nil {
		return previous, nil, err
	}
	history := []string{}
	node := previous
	seen := map[string]bool{}
	for i := 0; i < 12; i++ {
		if seen[node.ID] {
			return previous, nil, errors.New("执行记录的接续关系异常，原文件已保留。")
		}
		seen[node.ID] = true
		if node.TaskID != previous.TaskID || executionRootID(node) != executionRootID(previous) {
			return previous, nil, errors.New("历史记录不属于同一个任务处理链路。")
		}
		history = append(history, filepath.Join(ExecutionFolder(a.root, node.ID), "report.md"))
		if node.PreviousID == "" {
			break
		}
		node, err = readExecution(a.root, node.PreviousID)
		if err != nil {
			return previous, nil, err
		}
	}
	rootID := executionRootID(previous)
	if !seen[rootID] {
		history = append(history, filepath.Join(ExecutionFolder(a.root, rootID), "report.md"))
	}
	return previous, history, nil
}
