package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"taskpicker/internal/model"
	"taskpicker/internal/storage"
)

func projectReportRoot(root, id string) string { return filepath.Join(root, "projects", id) }

func (a *App) reportProject(id string) (model.Project, error) {
	a.projects.Lock()
	defer a.projects.Unlock()
	state, err := readLibrary(a.root)
	if err != nil {
		return model.Project{}, err
	}
	projects, err := readProjects(a.root, state.Documents)
	if err != nil {
		return model.Project{}, err
	}
	for _, project := range projects {
		if project.ID == id {
			return project, nil
		}
	}
	return model.Project{}, errors.New("项目不存在。")
}

func readProjectReportPrompt(root, id, fallback string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(projectReportRoot(root, id), "daily-prompt.json"))
	if os.IsNotExist(err) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	var value struct {
		Prompt string `json:"prompt"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return "", errors.New("项目日报提示词无法读取，原文件已保留。")
	}
	if strings.TrimSpace(value.Prompt) == "" {
		return defaultDailyPrompt, nil
	}
	return value.Prompt, nil
}

func (a *App) GetProjectDailyReportSettings(id string) (DailyAISettings, error) {
	if _, err := a.reportProject(id); err != nil {
		return DailyAISettings{}, err
	}
	a.reports.Lock()
	defer a.reports.Unlock()
	config, err := readDailyAI(a.root)
	if err != nil {
		return DailyAISettings{}, err
	}
	config.Prompt, err = readProjectReportPrompt(a.root, id, config.Prompt)
	return publicDailyAI(config), err
}

func (a *App) SaveProjectDailyReportPrompt(id, prompt string) (string, error) {
	if _, err := a.reportProject(id); err != nil {
		return "", err
	}
	if !utf8.ValidString(prompt) || utf8.RuneCountInString(prompt) > 20000 {
		return "", errors.New("日报提示词最多支持 20000 个字符。")
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultDailyPrompt
	}
	a.reports.Lock()
	defer a.reports.Unlock()
	path := filepath.Join(projectReportRoot(a.root, id), "daily-prompt.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	raw, err := json.Marshal(map[string]string{"prompt": prompt})
	if err == nil {
		err = storage.AtomicWrite(path, raw)
	}
	return prompt, err
}

func (a *App) GetProjectDailyReport(id, date string) (DailyReport, error) {
	if err := validReportDate(date); err != nil {
		return DailyReport{}, err
	}
	project, err := a.reportProject(id)
	if err != nil {
		return DailyReport{}, err
	}
	if !*project.DailyReportEnabled {
		return DailyReport{}, errors.New("此项目未开启日报。")
	}
	all, err := allDailyActivities(a.root, date)
	if err != nil {
		return DailyReport{}, err
	}
	keys := map[string]bool{}
	for _, key := range project.DocumentKeys {
		keys[key] = true
	}
	activities := []TaskActivity{}
	for _, activity := range all {
		if !activity.ReportExcluded && (activity.ProjectID == id || (activity.ProjectID == "" && keys[activity.DocumentKey])) {
			activities = append(activities, activity)
		}
	}
	a.reports.Lock()
	defer a.reports.Unlock()
	root := projectReportRoot(a.root, id)
	draft, err := readDailyDraft(root, date)
	// The first project owns the legacy reports only when all their source records
	// belong to it. Copy on first access; never remove or overwrite the old file.
	if err == nil && id == storage.DocumentKey("project:yiqiban", "") && len(all) == len(activities) {
		if _, statErr := os.Stat(dailyDraftPath(root, date)); os.IsNotExist(statErr) {
			legacy, legacyErr := readDailyDraft(a.root, date)
			if legacyErr != nil {
				return DailyReport{}, legacyErr
			}
			if legacy.Text != "" {
				draft, err = writeDailyDraft(root, date, legacy)
			}
		}
	}
	return DailyReport{Date: date, Activities: activities, Draft: draft}, err
}

func (a *App) SaveProjectDailyReport(id, date, text string) (DailyDraft, error) {
	if _, err := a.reportProject(id); err != nil {
		return DailyDraft{}, err
	}
	// The draft writer is shared; only its storage root changes.
	return a.saveDailyReportAt(projectReportRoot(a.root, id), date, text)
}

func (a *App) GenerateProjectDailyReport(id, date string, useAI bool) (DailyReport, error) {
	return a.generateScopedDailyReport(id, date, useAI, false)
}

func (a *App) ContinueProjectDailyReport(id, date string, useAI bool) (DailyReport, error) {
	return a.generateScopedDailyReport(id, date, useAI, true)
}
