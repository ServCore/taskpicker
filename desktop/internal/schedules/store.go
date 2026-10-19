// Package schedules persists recurring document pulls. The desktop's existing
// sync queue executes them, so automatic and manual work share one coordinator.
package schedules

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"taskpicker/internal/storage"
)

const MaxIntervalMinutes = 30 * 24 * 60

type Schedule struct {
	ID              string `json:"id"`
	ProjectID       string `json:"project_id"`
	IntervalMinutes int    `json:"interval_minutes"`
	Enabled         bool   `json:"enabled"`
	Revision        string `json:"revision"`
	NextRunAt       string `json:"next_run_at"`
	LastRunAt       string `json:"last_run_at"`
	LastResult      string `json:"last_result"`
}

type Input struct {
	ID              string `json:"id"`
	ProjectID       string `json:"project_id"`
	IntervalMinutes int    `json:"interval_minutes"`
	Enabled         bool   `json:"enabled"`
	Revision        string `json:"revision"`
}

type file struct {
	Version int        `json:"version"`
	Items   []Schedule `json:"items"`
}

type Store struct {
	mu   sync.Mutex
	root string
	now  func() time.Time
}

func New(root string) *Store { return &Store{root: root, now: time.Now} }

func validTime(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func validInput(input Input) bool {
	return storage.KeyPattern.MatchString(input.ProjectID) && input.IntervalMinutes >= 1 && input.IntervalMinutes <= MaxIntervalMinutes
}

func (s *Store) read() ([]Schedule, error) {
	raw, err := os.ReadFile(filepath.Join(s.root, "sync-schedules.json"))
	if os.IsNotExist(err) {
		return []Schedule{}, nil
	}
	if err != nil {
		return nil, errors.New("定时任务无法读取，原文件已保留。")
	}
	var state file
	if json.Unmarshal(raw, &state) != nil || state.Version != 1 {
		return nil, errors.New("定时任务配置无法解析，原文件已保留。")
	}
	ids, projects := map[string]bool{}, map[string]bool{}
	for _, item := range state.Items {
		if !storage.KeyPattern.MatchString(item.ID) || !storage.KeyPattern.MatchString(item.Revision) || ids[item.ID] || projects[item.ProjectID] ||
			!validInput(Input{ProjectID: item.ProjectID, IntervalMinutes: item.IntervalMinutes}) ||
			(item.Enabled && !validTime(item.NextRunAt)) || (!item.Enabled && item.NextRunAt != "") ||
			(item.LastRunAt != "" && !validTime(item.LastRunAt)) {
			return nil, errors.New("定时任务配置无效，原文件已保留。")
		}
		ids[item.ID], projects[item.ProjectID] = true, true
	}
	if state.Items == nil {
		state.Items = []Schedule{}
	}
	return state.Items, nil
}

func (s *Store) write(items []Schedule) ([]Schedule, error) {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(file{Version: 1, Items: items}, "", "  ")
	if err == nil {
		err = storage.AtomicWrite(filepath.Join(s.root, "sync-schedules.json"), raw)
	}
	if err != nil {
		return nil, err
	}
	return items, nil
}

func identifier() (string, error) {
	var data [12]byte
	_, err := rand.Read(data[:])
	return hex.EncodeToString(data[:]), err
}

func (s *Store) List() ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *Store) Save(input Input) ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validInput(input) {
		return nil, errors.New("请选择项目，循环间隔应为 1 分钟至 30 天的整数分钟。")
	}
	items, err := s.read()
	if err != nil {
		return nil, err
	}
	index := -1
	for i, item := range items {
		if item.ProjectID == input.ProjectID && item.ID != input.ID {
			return nil, errors.New("该项目已有定时拉取任务，请直接修改。")
		}
		if item.ID == input.ID {
			index = i
		}
	}
	if input.ID != "" && (index < 0 || items[index].Revision != input.Revision) {
		return nil, errors.New("定时任务已变化，请刷新后重试。")
	}
	item := Schedule{}
	if index >= 0 {
		item = items[index]
		if item.ProjectID == input.ProjectID && item.IntervalMinutes == input.IntervalMinutes && item.Enabled == input.Enabled {
			return items, nil
		}
	} else {
		item.ID, err = identifier()
		if err != nil {
			return nil, err
		}
	}
	item.Revision, err = identifier()
	if err != nil {
		return nil, err
	}
	if item.ProjectID != input.ProjectID {
		item.LastRunAt, item.LastResult = "", ""
	}
	item.ProjectID, item.IntervalMinutes, item.Enabled = input.ProjectID, input.IntervalMinutes, input.Enabled
	item.NextRunAt = ""
	if item.Enabled {
		item.NextRunAt = s.now().Add(time.Duration(item.IntervalMinutes) * time.Minute).Format(time.RFC3339Nano)
	}
	if index >= 0 {
		items[index] = item
	} else {
		items = append(items, item)
	}
	return s.write(items)
}

func (s *Store) Delete(id, revision string) ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read()
	if err != nil {
		return nil, err
	}
	for i, item := range items {
		if item.ID == id {
			if item.Revision != revision {
				return nil, errors.New("定时任务已变化，请刷新后重试。")
			}
			return s.write(append(items[:i:i], items[i+1:]...))
		}
	}
	return items, nil
}

// Complete only updates the configuration that actually ran. A paused, edited
// or deleted schedule must never be resurrected by an older in-flight request.
func (s *Store) Complete(id, revision, result string) ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !utf8.ValidString(result) || len(result) > 8000 {
		return nil, errors.New("定时任务结果过长或无效。")
	}
	items, err := s.read()
	if err != nil {
		return nil, err
	}
	for i := range items {
		item := &items[i]
		if item.ID != id || item.Revision != revision {
			continue
		}
		now := s.now()
		item.LastRunAt, item.LastResult = now.Format(time.RFC3339Nano), strings.TrimSpace(result)
		item.NextRunAt = ""
		if item.Enabled {
			item.NextRunAt = now.Add(time.Duration(item.IntervalMinutes) * time.Minute).Format(time.RFC3339Nano)
		}
		return s.write(items)
	}
	return items, nil
}
