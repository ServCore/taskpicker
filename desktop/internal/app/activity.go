package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

type ActivityChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type TaskActivity struct {
	ID             string           `json:"id"`
	ProjectID      string           `json:"project_id,omitempty"`
	ReportExcluded bool             `json:"report_excluded,omitempty"`
	Date           string           `json:"date"`
	At             string           `json:"at"`
	DocumentKey    string           `json:"document_key"`
	DocumentTitle  string           `json:"document_title"`
	Sheet          string           `json:"sheet"`
	Row            int              `json:"row"`
	RecordID       string           `json:"record_id,omitempty"`
	Owner          string           `json:"owner"`
	Task           string           `json:"task"`
	Context        string           `json:"context"`
	Kind           string           `json:"kind"`
	Status         string           `json:"status"`
	Changes        []ActivityChange `json:"changes"`
	Note           string           `json:"note"`
}

// History lives in the same atomic snapshot as task edits, but is not sent with
// every library response. Pulling a fresh table preserves this independent history.
type activitySnapshot struct {
	model.Document
	Activities []TaskActivity `json:"activities,omitempty"`
}

func readActivities(root, key string) ([]TaskActivity, error) {
	var history struct {
		Activities []TaskActivity `json:"activities"`
	}
	raw, err := os.ReadFile(filepath.Join(root, key, "snapshot.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &history); err != nil {
		return nil, err
	}
	return history.Activities, nil
}

func marshalActivitySnapshot(root string, doc model.Document, added []TaskActivity) ([]byte, error) {
	history, err := readActivities(root, doc.Key)
	if err != nil {
		return nil, fmt.Errorf("无法保留任务操作记录：%w", err)
	}
	return json.Marshal(activitySnapshot{Document: doc, Activities: append(history, added...)})
}

func persistActivityDocument(root string, doc model.Document, added ...TaskActivity) (model.Document, error) {
	// Capture project ownership at the time of work. Moving a document later
	// must not move earlier work between projects' reports.
	if len(added) > 0 {
		if _, err := os.Stat(projectPath(root)); err == nil {
			if projects, err := readProjects(root, nil); err == nil {
				for _, project := range projects {
					for _, key := range project.DocumentKeys {
						if key == doc.Key {
							for i := range added {
								added[i].ProjectID = project.ID
								added[i].ReportExcluded = !*project.DailyReportEnabled
							}
						}
					}
				}
			}
		}
	}
	raw, err := marshalActivitySnapshot(root, doc, added)
	if err == nil {
		err = storage.AtomicWrite(filepath.Join(root, doc.Key, "snapshot.json"), raw)
	}
	return doc, err
}

func newTaskActivity(doc model.Document, sheet model.TaskSheet, row model.TaskRow, kind string) TaskActivity {
	var random [16]byte
	_, _ = rand.Read(random[:])
	now := time.Now()
	activity := TaskActivity{ID: hex.EncodeToString(random[:]), Date: now.Format("2006-01-02"), At: now.Format(time.RFC3339Nano),
		DocumentKey: doc.Key, DocumentTitle: doc.Title, Sheet: sheet.Name, Row: row.Number, RecordID: row.RecordID,
		Owner: doc.Owner, Kind: kind, Status: rowText(&row, reader.StatusColumn(sheet.Headers)), Changes: []ActivityChange{}}
	field := func(names ...string) string {
		for _, name := range names {
			for col, header := range sheet.Headers {
				if reader.Normalized(header) == name {
					return rowText(&row, col)
				}
			}
		}
		return ""
	}
	activity.Task = rowText(&row, reader.TitleColumn(sheet.Headers))
	activity.Context = field("三级功能", "功能", "二级功能", "模块", "项目")
	if activity.Task == "" {
		activity.Task = fmt.Sprintf("第 %d 行任务", row.Number)
	}
	return activity
}

func rowActivityChanges(sheet model.TaskSheet, before model.TaskRow, after model.TaskRow) []ActivityChange {
	changes := []ActivityChange{}
	status := reader.StatusColumn(sheet.Headers)
	for col, header := range sheet.Headers {
		if col != status && !noteColumn(sheet.Headers, col) {
			continue
		}
		old, value := rowText(&before, col), rowText(&after, col)
		if old != value {
			changes = append(changes, ActivityChange{Field: reader.StringValue(header), Before: old, After: value})
		}
	}
	return changes
}

func validReportDate(date string) error {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		return errors.New("请选择有效的日报日期。")
	}
	return nil
}

func dailyActivities(root, date string) ([]TaskActivity, error) {
	excluded, err := projectReportExclusions(root)
	if err != nil {
		return nil, err
	}
	all, err := allDailyActivities(root, date)
	if err != nil {
		return nil, err
	}
	result := []TaskActivity{}
	for _, activity := range all {
		if !excluded[activity.DocumentKey] && !activity.ReportExcluded {
			result = append(result, activity)
		}
	}
	return result, nil
}

func allDailyActivities(root, date string) ([]TaskActivity, error) {
	if err := validReportDate(date); err != nil {
		return nil, err
	}
	result := []TaskActivity{}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !storage.KeyPattern.MatchString(entry.Name()) {
			continue
		}
		history, err := readActivities(root, entry.Name())
		if err != nil {
			return nil, errors.New("有一份文档的操作记录无法读取，请检查本地缓存。")
		}
		for _, activity := range history {
			if activity.Date == date {
				result = append(result, activity)
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339Nano, result[i].At)
		right, rightErr := time.Parse(time.RFC3339Nano, result[j].At)
		if leftErr == nil && rightErr == nil {
			return left.Before(right)
		}
		return result[i].At < result[j].At
	})
	return result, nil
}

func (a *App) RecordTaskWork(key, sheetName string, rowNumber int, note string) (TaskActivity, error) {
	note = strings.TrimSpace(note)
	if note == "" || !utf8.ValidString(note) || utf8.RuneCountInString(note) > 10000 {
		return TaskActivity{}, errors.New("请填写实际工作进展，最多 10000 个字符。")
	}
	_, finish, err := a.startStatusOperation(15 * time.Second)
	if err != nil {
		return TaskActivity{}, err
	}
	defer finish()
	doc, err := ReadDocument(a.root, key)
	if err != nil {
		return TaskActivity{}, err
	}
	sheet, row, err := StatusRow(&doc, sheetName, rowNumber)
	if err != nil {
		return TaskActivity{}, err
	}
	a.projects.Lock()
	excluded, projectErr := projectReportExclusions(a.root)
	a.projects.Unlock()
	if projectErr != nil {
		return TaskActivity{}, projectErr
	}
	if excluded[key] {
		return TaskActivity{}, errors.New("此项目未开启日报，请先在项目设置中开启。")
	}
	activity := newTaskActivity(doc, *sheet, *row, "work")
	activity.Note = note
	_, err = persistActivityDocument(a.root, doc, activity)
	return activity, err
}
