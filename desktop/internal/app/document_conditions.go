package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

type documentConditionsFile struct {
	Version   int                                     `json:"version"`
	Documents map[string]model.DocumentPullConditions `json:"documents"`
}

func documentConditionsPath(root string) string { return filepath.Join(root, "document-pulls.json") }

func readDocumentPullConditions(root string) (map[string]model.DocumentPullConditions, error) {
	raw, err := os.ReadFile(documentConditionsPath(root))
	if os.IsNotExist(err) {
		return map[string]model.DocumentPullConditions{}, nil
	}
	if err != nil {
		return nil, errors.New("无法读取文档拉取条件。")
	}
	var state documentConditionsFile
	if json.Unmarshal(raw, &state) != nil || state.Version != 1 {
		return nil, errors.New("文档拉取条件无法解析，原文件已保留。")
	}
	if state.Documents == nil {
		state.Documents = map[string]model.DocumentPullConditions{}
	}
	for key, condition := range state.Documents {
		if !storage.KeyPattern.MatchString(key) || reader.ValidatePullConditions(condition) != nil {
			return nil, errors.New("文档拉取条件格式无效，原文件已保留。")
		}
		// Completion is no longer an implicit pull condition, including for old files.
		condition.IncludeCompleted = true
		state.Documents[key] = condition
	}
	return state.Documents, nil
}

func validatePullOwner(owner string) error {
	if reader.Normalized(owner) == "" || !utf8.ValidString(owner) || utf8.RuneCountInString(owner) > 80 || strings.ContainsAny(owner, "\r\n") {
		return errors.New("请填写负责人姓名，最多 80 个字符。")
	}
	return nil
}

func (a *App) SaveDocumentPullConditions(key, owner string, includeCompleted bool) (model.Document, error) {
	owner = strings.TrimSpace(owner)
	if err := validatePullOwner(owner); err != nil {
		return model.Document{}, err
	}
	return a.SaveDocumentFilters(key, model.DocumentPullConditions{Owner: owner, IncludeCompleted: includeCompleted})
}

func (a *App) SaveDocumentFilters(key string, condition model.DocumentPullConditions) (model.Document, error) {
	condition.IncludeCompleted = true
	condition.Owner = strings.TrimSpace(condition.Owner)
	if err := reader.ValidatePullConditions(condition); err != nil {
		return model.Document{}, err
	}

	_, finish, err := a.startStatusOperation(15 * time.Second)
	if err != nil {
		return model.Document{}, err
	}
	defer finish()
	a.projects.Lock()
	defer a.projects.Unlock()
	state, err := readLibrary(a.root)
	if err != nil {
		return model.Document{}, err
	}
	var doc *model.Document
	for i := range state.Documents {
		if state.Documents[i].Key == key {
			doc = &state.Documents[i]
			break
		}
	}
	if doc == nil {
		return model.Document{}, errors.New("文档不存在。")
	}
	previous := doc.PullConditions
	if previous == nil {
		previous = &model.DocumentPullConditions{Owner: doc.Owner, IncludeCompleted: doc.IncludeCompleted}
	}
	if !reflect.DeepEqual(*previous, condition) && pendingStatusCount(*doc) > 0 {
		return model.Document{}, errors.New("请先回写或撤销待同步修改，再修改拉取条件。")
	}
	conditions, err := readDocumentPullConditions(a.root)
	if err != nil {
		return model.Document{}, err
	}
	// Validate field references against known headers before saving. The reader
	// validates them again against the remote schema on every pull.
	for _, f := range condition.Filters {
		found := false
		for _, sheet := range doc.Sheets {
			if sheet.Name == f.Sheet {
				found = true
				if f.Column > len(sheet.Headers) || reader.StringValue(sheet.Headers[f.Column-1]) != f.Header {
					return model.Document{}, errors.New("筛选字段已变化，请重新选择。")
				}
			}
		}
		if !found {
			return model.Document{}, errors.New("筛选工作表不存在。")
		}
	}
	conditions[key] = condition
	raw, err := json.Marshal(documentConditionsFile{Version: 1, Documents: conditions})
	if err == nil {
		err = storage.AtomicWrite(documentConditionsPath(a.root), raw)
	}
	if err != nil {
		return model.Document{}, err
	}
	doc.PullConditions = &condition
	return *doc, nil
}

func (a *App) SyncConfiguredDocument(key string, interactive bool) (model.Document, error) {
	a.projects.Lock()
	state, err := readLibrary(a.root)
	conditions, conditionsErr := readDocumentPullConditions(a.root)
	a.projects.Unlock()
	if err != nil {
		return model.Document{}, err
	}
	if conditionsErr != nil {
		return model.Document{}, conditionsErr
	}
	var original *model.Document
	for i := range state.Documents {
		if state.Documents[i].Key == key {
			original = &state.Documents[i]
			break
		}
	}
	if original == nil {
		return model.Document{}, errors.New("文档不存在。")
	}
	condition, ok := conditions[key]
	if !ok {
		if original.PullConditions != nil {
			condition = *original.PullConditions
		} else {
			condition = model.DocumentPullConditions{Owner: original.Owner, IncludeCompleted: original.IncludeCompleted}
		}
	}
	condition.IncludeCompleted = true
	doc, err := a.runDocumentJob(condition.Owner, key, func(ctx context.Context, notify reader.ProgressFunc) (result model.Document, resultErr error) {
		defer func() {
			if resultErr == nil {
				result.AppliedPullConditions = &condition
				result.PullConditions = &condition
			}
		}()
		if existing, e := ReadDocument(a.root, key); e == nil && pendingStatusCount(existing) > 0 {
			return model.Document{}, errors.New("这份文档有待同步的状态或备注修改，请先回写或撤销，再重新读取。")
		}
		if original.SourceURL == "" {
			notify("正在读取 Excel 的任务和图片…")
			return reader.FilterWorkbook(ctx, original.SourcePath, condition.Owner, nil, condition.IncludeCompleted, reader.ReadOptions{Conditions: &condition})
		}
		link, e := storage.ValidateURL(original.SourceURL)
		if e != nil {
			return model.Document{}, e
		}
		result, e = reader.ReadPublicTasks(ctx, link, condition.Owner, notify, nil, condition.IncludeCompleted, reader.ReadOptions{Conditions: &condition})
		if errors.Is(e, reader.ErrNeedsLogin) {
			if !interactive {
				return model.Document{}, errors.New("文档需要登录，请手动同步完成登录。")
			}
			return reader.ReadAuthenticatedTasks(ctx, link, condition.Owner, notify, condition.IncludeCompleted, reader.ReadOptions{Conditions: &condition})
		}
		return result, e
	})
	if err == nil {
		doc.PullConditions = &condition
	}
	return doc, err
}
