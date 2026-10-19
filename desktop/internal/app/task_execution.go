package app

import (
	"errors"

	"taskpicker/internal/execution"
	"taskpicker/internal/model"
)

func (a *App) executionTask(key, sheetName string, rowNumber int) (model.Project, model.Document, model.TaskSheet, model.TaskRow, error) {
	doc, err := ReadDocument(a.root, key)
	if err != nil {
		return model.Project{}, doc, model.TaskSheet{}, model.TaskRow{}, err
	}
	sheet, row, err := StatusRow(&doc, sheetName, rowNumber)
	if err != nil {
		return model.Project{}, doc, model.TaskSheet{}, model.TaskRow{}, err
	}
	a.projects.Lock()
	state, err := readLibrary(a.root)
	projects := state.Projects
	a.projects.Unlock()
	if err == nil {
		for _, project := range projects {
			for _, docKey := range project.DocumentKeys {
				if docKey == key {
					return project, doc, *sheet, *row, nil
				}
			}
		}
		err = errors.New("请先将文档归入项目，并绑定本地代码目录。")
	}
	return model.Project{}, doc, *sheet, *row, err
}

func (a *App) executionService() *execution.Service {
	a.executionOnce.Do(func() {
		a.executionEngine = execution.New(a.root, a.ctx, a.executionTask, func(key, sheetName string, rowNumber int) (model.Document, model.TaskSheet, model.TaskRow, error) {
			doc, err := ReadDocument(a.root, key)
			if err != nil {
				return doc, model.TaskSheet{}, model.TaskRow{}, err
			}
			sheet, row, err := StatusRow(&doc, sheetName, rowNumber)
			if err != nil {
				return doc, model.TaskSheet{}, model.TaskRow{}, err
			}
			return doc, *sheet, *row, nil
		})
	})
	return a.executionEngine
}
func (a *App) cancelTaskExecutions() { a.executionService().Close() }

func (a *App) GetTaskExecutionProgress(key, sheet string, row int) (execution.TaskExecutionProgress, error) {
	return a.executionService().GetTaskExecutionProgress(key, sheet, row)
}

func (a *App) OpenTaskExecutionProgress(key, sheet string, row int) error {
	return a.executionService().OpenTaskExecutionProgress(key, sheet, row)
}

func (a *App) ContinueTaskExecution(key, sheet string, row int, previousID, instructions string) (execution.TaskExecution, error) {
	a.executionService().SetRunner(a.executionRunner)
	return a.executionService().ContinueTaskExecution(key, sheet, row, previousID, instructions)
}

func (a *App) GetTaskExecutionReport(id string) (execution.TaskExecutionReport, error) {
	return a.executionService().GetTaskExecutionReport(id)
}

func (a *App) OpenTaskExecutionReport(id string) error {
	return a.executionService().OpenTaskExecutionReport(id)
}

func (a *App) GetTaskExecutionContext(key, sheet string, row int) (execution.TaskExecutionContext, error) {
	return a.executionService().GetTaskExecutionContext(key, sheet, row)
}

func (a *App) GetTaskExecutions(key, sheetName string, rowNumber int) ([]execution.TaskExecution, error) {
	return a.executionService().GetTaskExecutions(key, sheetName, rowNumber)
}

func (a *App) GetTaskExecution(id string, after int) (execution.TaskExecutionUpdate, error) {
	return a.executionService().GetTaskExecution(id, after)
}

func (a *App) StartTaskExecution(key, sheetName string, rowNumber int, repository, instructions string) (execution.TaskExecution, error) {
	a.executionService().SetRunner(a.executionRunner)
	return a.executionService().StartTaskExecution(key, sheetName, rowNumber, repository, instructions)
}

func (a *App) StopTaskExecution(id string) error {
	return a.executionService().StopTaskExecution(id)
}

func (a *App) GetActiveTaskExecution() (*execution.TaskExecution, error) {
	return a.executionService().GetActiveTaskExecution()
}

func (a *App) OpenTaskExecutionWorkspace(id string) error {
	return a.executionService().OpenTaskExecutionWorkspace(id)
}
