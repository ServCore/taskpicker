package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

type projectFile struct {
	Version  int             `json:"version"`
	Projects []model.Project `json:"projects"`
}

func projectPath(root string) string { return filepath.Join(root, "projects.json") }

// Relationships live outside document snapshots, so pulls never replace them.
func readProjects(root string, documents []model.Document) ([]model.Project, error) {
	raw, err := os.ReadFile(projectPath(root))
	if os.IsNotExist(err) {
		enabled := false
		projectEnabled := true
		initial := model.Project{ID: storage.DocumentKey("project:yiqiban", ""), Name: "yiqiban", Enabled: &projectEnabled, DailyReportEnabled: &enabled, DocumentKeys: []string{}}
		for _, doc := range documents {
			u, _ := url.Parse(doc.SourceURL)
			for _, preset := range presets {
				p, _ := url.Parse(preset.SourceURL)
				if u != nil && u.Host == p.Host && u.Path == p.Path {
					initial.DocumentKeys = append(initial.DocumentKeys, doc.Key)
					break
				}
			}
		}
		projects := []model.Project{initial}
		return projects, writeProjects(root, projects)
	}
	if err != nil {
		return nil, errors.New("无法读取项目设置。")
	}
	var state projectFile
	if json.Unmarshal(raw, &state) != nil || state.Version != 1 {
		return nil, errors.New("项目设置无法解析，原文件已保留。")
	}
	if state.Projects == nil {
		state.Projects = []model.Project{}
	}
	ids, assigned := map[string]bool{}, map[string]bool{}
	for i := range state.Projects {
		p := &state.Projects[i]
		if !storage.KeyPattern.MatchString(p.ID) || ids[p.ID] || strings.TrimSpace(p.Name) == "" {
			return nil, errors.New("项目设置格式无效，原文件已保留。")
		}
		ids[p.ID] = true
		if p.Enabled == nil {
			enabled := true
			p.Enabled = &enabled
		}
		if p.LocalPaths == nil {
			p.LocalPaths = []string{}
			if p.LocalPath != "" {
				p.LocalPaths = append(p.LocalPaths, p.LocalPath)
			}
		}
		if p.DailyReportEnabled == nil {
			enabled := true
			p.DailyReportEnabled = &enabled
		}

		if p.DocumentKeys == nil {
			p.DocumentKeys = []string{}
		}
		for _, key := range p.DocumentKeys {
			if !storage.KeyPattern.MatchString(key) || assigned[key] {
				return nil, errors.New("项目的文档归属无效，原文件已保留。")
			}
			assigned[key] = true
		}
	}
	return state.Projects, nil
}

func writeProjects(root string, projects []model.Project) error {
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(projectFile{Version: 1, Projects: projects})
	if err != nil {
		return err
	}
	return storage.AtomicWrite(projectPath(root), raw)
}

func localProjectDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("请选择本地项目的完整目录路径。")
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", errors.New("本地项目目录不存在或无法访问，请重新选择。")
	}
	return path, nil
}

func (a *App) SaveProject(update model.Project) ([]model.Project, error) {
	a.projects.Lock()
	defer a.projects.Unlock()
	state, err := readLibrary(a.root)
	if err != nil {
		return nil, err
	}
	projects, err := readProjects(a.root, state.Documents)
	if err != nil {
		return nil, err
	}
	update.Name = strings.TrimSpace(update.Name)
	if update.Name == "" || !utf8.ValidString(update.Name) || utf8.RuneCountInString(update.Name) > 80 || strings.ContainsAny(update.Name, "\r\n") {
		return nil, errors.New("请填写项目名称，最多 80 个字符。")
	}
	index := -1
	for i, p := range projects {
		if p.ID == update.ID {
			index = i
		}
		if p.ID != update.ID && strings.EqualFold(p.Name, update.Name) {
			return nil, errors.New("已有同名项目，请换一个名称。")
		}
	}
	if update.ID == "" {
		var id [12]byte
		if _, err := rand.Read(id[:]); err != nil {
			return nil, err
		}
		update.ID = hex.EncodeToString(id[:])
	} else if index < 0 {
		return nil, errors.New("项目不存在，请刷新后重试。")
	}
	// Existing bindings may be temporarily offline; only newly added paths need validation.
	if update.LocalPaths == nil {
		update.LocalPaths = []string{}
		if update.LocalPath != "" {
			update.LocalPaths = append(update.LocalPaths, update.LocalPath)
		}
	}
	existingPaths := map[string]bool{}
	if index >= 0 {
		for _, path := range projects[index].LocalPaths {
			existingPaths[path] = true
		}
	}
	paths := []string{}
	for _, path := range update.LocalPaths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		path = filepath.Clean(path)
		if !existingPaths[path] {
			path, err = localProjectDirectory(path)
			if err != nil {
				return nil, err
			}
		}
		paths = append(paths, path)
	}
	update.LocalPaths = reader.UniqueStrings(paths)
	update.LocalPath = ""
	if len(update.LocalPaths) > 0 {
		update.LocalPath = update.LocalPaths[0]
	}
	if update.Enabled == nil {
		enabled := true
		if index >= 0 {
			enabled = *projects[index].Enabled
		}
		update.Enabled = &enabled
	}
	if update.DailyReportEnabled == nil {
		enabled := false
		if index >= 0 {
			enabled = *projects[index].DailyReportEnabled
		}
		update.DailyReportEnabled = &enabled
	}
	known, assigned := map[string]bool{}, map[string]bool{}
	for _, doc := range state.Documents {
		known[doc.Key] = true
	}
	keys := []string{}
	for _, key := range update.DocumentKeys {
		if !known[key] {
			return nil, errors.New("所选文档不存在，请刷新后重试。")
		}
		if !assigned[key] {
			keys = append(keys, key)
			assigned[key] = true
		}
	}
	update.DocumentKeys = keys
	for i := range projects {
		if i == index {
			continue
		}
		remaining := []string{}
		for _, key := range projects[i].DocumentKeys {
			if !assigned[key] {
				remaining = append(remaining, key)
			}
		}
		projects[i].DocumentKeys = remaining
	}
	if index >= 0 {
		projects[index] = update
	} else {
		projects = append(projects, update)
	}
	if err := writeProjects(a.root, projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (a *App) SetProjectEnabled(id string, enabled bool) ([]model.Project, error) {
	a.projects.Lock()
	defer a.projects.Unlock()
	state, err := readLibrary(a.root)
	if err != nil {
		return nil, err
	}
	projects, err := readProjects(a.root, state.Documents)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].ID == id {
			projects[i].Enabled = &enabled
			if err := writeProjects(a.root, projects); err != nil {
				return nil, err
			}
			return projects, nil
		}
	}
	return nil, errors.New("项目不存在，请刷新后重试。")
}

func (a *App) AssignDocumentProject(key, projectID string) ([]model.Project, error) {
	a.projects.Lock()
	defer a.projects.Unlock()
	state, err := readLibrary(a.root)
	if err != nil {
		return nil, err
	}
	exists := false
	for _, doc := range state.Documents {
		if doc.Key == key {
			exists = true
		}
	}
	if !exists {
		return nil, errors.New("文档不存在。")
	}
	projects, err := readProjects(a.root, state.Documents)
	if err != nil {
		return nil, err
	}
	found := projectID == ""
	for _, p := range projects {
		if p.ID == projectID {
			found = true
		}
	}
	if !found {
		return nil, errors.New("项目不存在。")
	}
	for i := range projects {
		remaining := []string{}
		for _, current := range projects[i].DocumentKeys {
			if current != key {
				remaining = append(remaining, current)
			}
		}
		if projects[i].ID == projectID {
			remaining = append(remaining, key)
		}
		projects[i].DocumentKeys = remaining
	}
	if err := writeProjects(a.root, projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (a *App) PickProjectDirectory(current string) (string, error) {
	directory, _ := localProjectDirectory(current)
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "选择本地项目目录", DefaultDirectory: directory, CanCreateDirectories: false})
}

func (a *App) OpenProjectDirectory(id string) error { return a.OpenProjectDirectoryAt(id, 0) }

func (a *App) OpenProjectDirectoryAt(id string, directoryIndex int) error {
	a.projects.Lock()
	state, err := readLibrary(a.root)
	var projects []model.Project
	if err == nil {
		projects, err = readProjects(a.root, state.Documents)
	}
	a.projects.Unlock()
	if err != nil {
		return err
	}
	for _, p := range projects {
		if p.ID != id {
			continue
		}
		if len(p.LocalPaths) == 0 {
			return errors.New("请先绑定本地项目目录。")
		}
		if directoryIndex < 0 || directoryIndex >= len(p.LocalPaths) {
			return errors.New("项目目录不存在，请重新选择。")
		}
		path, err := localProjectDirectory(p.LocalPaths[directoryIndex])
		if err != nil {
			return err
		}
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", path)
		case "windows":
			cmd = exec.Command("explorer.exe", path)
		default:
			cmd = exec.Command("xdg-open", path)
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("无法打开项目目录：%w", err)
		}
		return nil
	}
	return errors.New("项目不存在。")
}

func projectReportExclusions(root string) (map[string]bool, error) {
	excluded := map[string]bool{}
	if _, err := os.Stat(projectPath(root)); os.IsNotExist(err) {
		return excluded, nil
	}
	projects, err := readProjects(root, nil)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if !*project.DailyReportEnabled {
			for _, key := range project.DocumentKeys {
				excluded[key] = true
			}
		}
	}
	return excluded, nil
}
