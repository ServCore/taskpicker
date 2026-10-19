package app

import (
	"errors"

	"taskpicker/internal/schedules"
)

func (a *App) scheduleStore() *schedules.Store {
	a.scheduleOnce.Do(func() { a.scheduleConfig = schedules.New(a.root) })
	return a.scheduleConfig
}

func (a *App) GetSyncSchedules() ([]schedules.Schedule, error) { return a.scheduleStore().List() }

func (a *App) SaveSyncSchedule(input schedules.Input) ([]schedules.Schedule, error) {
	a.projects.Lock()
	defer a.projects.Unlock()
	state, err := readLibrary(a.root)
	if err != nil {
		return nil, err
	}
	for _, project := range state.Projects {
		if project.ID == input.ProjectID {
			return a.scheduleStore().Save(input)
		}
	}
	return nil, errors.New("项目不存在，请刷新项目列表。")
}

func (a *App) DeleteSyncSchedule(id, revision string) ([]schedules.Schedule, error) {
	return a.scheduleStore().Delete(id, revision)
}

func (a *App) CompleteSyncSchedule(id, revision, result string) ([]schedules.Schedule, error) {
	return a.scheduleStore().Complete(id, revision, result)
}
