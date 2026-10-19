package execution

import (
	"context"
	"sync"

	"taskpicker/internal/model"
)

// Service owns execution state, workspaces, reports and continuation summaries.
// Lookups keep application storage and project locking outside this package.
type Service struct {
	ctx              context.Context
	root             string
	resolveTask      func(string, string, int) (model.Project, model.Document, model.TaskSheet, model.TaskRow, error)
	readTask         func(string, string, int) (model.Document, model.TaskSheet, model.TaskRow, error)
	executions       sync.Mutex
	executionID      string
	executionClosing bool
	executionCancel  context.CancelFunc
	executionWG      sync.WaitGroup
	runnerMu         sync.RWMutex
	executionRunner  ExecutionRunner
}

func New(root string, ctx context.Context,
	resolve func(string, string, int) (model.Project, model.Document, model.TaskSheet, model.TaskRow, error),
	read func(string, string, int) (model.Document, model.TaskSheet, model.TaskRow, error),
) *Service {
	return &Service{root: root, ctx: ctx, resolveTask: resolve, readTask: read}
}

// SetRunner allows the application to supply an executor; nil uses the local CLI.
func (s *Service) SetRunner(runner ExecutionRunner) {
	s.runnerMu.Lock()
	defer s.runnerMu.Unlock()
	s.executionRunner = runner
}
func (s *Service) runner() ExecutionRunner {
	s.runnerMu.RLock()
	defer s.runnerMu.RUnlock()
	return s.executionRunner
}
func (s *Service) Wait() { s.executionWG.Wait() }
