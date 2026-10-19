package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"taskpicker/internal/media"
	"taskpicker/internal/model"
	"taskpicker/internal/storage"
)

type Library struct {
	Projects    []model.Project  `json:"projects"`
	Documents   []model.Document `json:"documents"`
	SelectedKey string           `json:"selected_key"`
	Warnings    []string         `json:"warnings"`
}

// Personal document URLs belong in the local library, never in distributed source.
var presets []model.Document

func ReadDocument(root, key string) (model.Document, error) {
	var doc model.Document
	if !storage.KeyPattern.MatchString(key) {
		return doc, errors.New("本地文档编号无效。")
	}
	raw, err := os.ReadFile(filepath.Join(root, key, "snapshot.json"))
	if err != nil {
		return doc, err
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		return doc, fmt.Errorf("本地文档无法读取：%w", err)
	}
	if doc.Version != 1 {
		return doc, errors.New("本地任务缓存版本不支持，请重新同步。")
	}
	doc.Key, doc.Cached = key, !doc.SetupPending
	if doc.Title == "" {
		doc.Title = strings.TrimSuffix(filepath.Base(doc.SourcePath), filepath.Ext(doc.SourcePath))
	}
	if doc.Warnings == nil {
		doc.Warnings = []string{}
	}
	if doc.Sheets == nil {
		doc.Sheets = []model.TaskSheet{}
	}
	for si := range doc.Sheets {
		if doc.Sheets[si].Rows == nil {
			doc.Sheets[si].Rows = []model.TaskRow{}
		}
		for ri := range doc.Sheets[si].Rows {
			row := &doc.Sheets[si].Rows[ri]
			if row.Pictures == nil {
				row.Pictures = []model.Picture{}
			}
			for _, pic := range row.Pictures {
				if !storage.ImagePattern.MatchString(pic.File) {
					return model.Document{}, errors.New("本地图片文件名异常，请重新同步。")
				}
				if _, err := os.Stat(filepath.Join(root, key, "images", pic.File)); err != nil {
					doc.Warnings = append(doc.Warnings, fmt.Sprintf("原第 %d 行有图片缺失，请重新同步。", row.Number))
				}
			}
		}
	}
	return doc, nil
}

func readLibrary(root string) (Library, error) {
	state := Library{Documents: []model.Document{}, Warnings: []string{}}
	if err := os.MkdirAll(root, 0700); err != nil {
		return state, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return state, err
	}
	byKey := map[string]model.Document{}
	for _, entry := range entries {
		if !entry.IsDir() || !storage.KeyPattern.MatchString(entry.Name()) {
			continue
		}
		doc, err := ReadDocument(root, entry.Name())
		if err != nil {
			state.Warnings = append(state.Warnings, "有一份本地缓存无法读取，可重新添加原链接同步："+err.Error())
			continue
		}
		byKey[entry.Name()] = doc
	}
	for _, preset := range presets {
		preset.Key = storage.DocumentKey(preset.SourceURL, preset.Owner)
		if cached, ok := byKey[preset.Key]; ok {
			state.Documents = append(state.Documents, cached)
			delete(byKey, preset.Key)
		} else {
			preset.Sheets, preset.Warnings = []model.TaskSheet{}, []string{}
			state.Documents = append(state.Documents, preset)
		}
	}
	var rest []model.Document
	for _, doc := range byKey {
		rest = append(rest, doc)
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].UpdatedAt > rest[j].UpdatedAt })
	state.Documents = append(state.Documents, rest...)
	var last struct {
		Key string `json:"key"`
	}
	if data, err := os.ReadFile(filepath.Join(root, "last.json")); err == nil {
		_ = json.Unmarshal(data, &last)
	}
	for _, doc := range state.Documents {
		if doc.Key == last.Key {
			state.SelectedKey = last.Key
		}
	}
	if state.SelectedKey == "" && len(state.Documents) > 0 {
		state.SelectedKey = state.Documents[0].Key
	}
	conditions, conditionsErr := readDocumentPullConditions(root)
	if conditionsErr != nil {
		state.Warnings = append(state.Warnings, conditionsErr.Error())
	} else {
		for i := range state.Documents {
			if condition, ok := conditions[state.Documents[i].Key]; ok {
				state.Documents[i].PullConditions = &condition
			}
		}
	}
	state.Projects, err = readProjects(root, state.Documents)
	if err != nil {
		state.Projects = []model.Project{}
		state.Warnings = append(state.Warnings, err.Error())
	}

	return state, nil
}

func rememberDocument(root, key string) error {
	if !storage.KeyPattern.MatchString(key) {
		return errors.New("文档编号无效。")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]string{"key": key})
	f, err := os.CreateTemp(root, "last-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(root, "last.json"))
}

func ImageHandler(a *App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if len(parts) != 3 || parts[0] != "local-image" || !storage.KeyPattern.MatchString(parts[1]) || !storage.ImagePattern.MatchString(parts[2]) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		path, err := media.LibraryFile(a.root, filepath.Join(a.root, parts[1], "images", parts[2]))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		variant := r.URL.Query().Get("size")
		var data []byte
		if variant != "" {
			if _, ok := media.ImageVariantSizes[variant]; !ok {
				http.NotFound(w, r)
				return
			}
			path, data, err = media.LoadImageVariant(r.Context(), a.root, parts[1], parts[2], variant, path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if data != nil {
			if r.Method != http.MethodHead {
				_, _ = w.Write(data)
			}
			return
		}
		http.ServeFile(w, r, path)
	})
}
