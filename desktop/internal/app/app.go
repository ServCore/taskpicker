package app

import (
	"context"
	"errors"
	"runtime/debug"
	"strings"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"taskpicker/internal/execution"
	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/resources"
	"taskpicker/internal/schedules"
	"taskpicker/internal/storage"
)

type App struct {
	ctx             context.Context
	root            string
	mu              sync.Mutex
	busy            bool
	cancel          context.CancelFunc
	resources       resources.ResourceSampler
	projects        sync.Mutex
	reports         sync.Mutex
	reportCancel    context.CancelFunc
	executionOnce   sync.Once
	executionEngine *execution.Service
	executionRunner execution.ExecutionRunner
	scheduleOnce    sync.Once
	scheduleConfig  *schedules.Store
}

func NewApp() *App { return &App{root: storage.LibraryRoot()} }

func Startup(a *App, ctx context.Context) { a.ctx = ctx }

func Shutdown(a *App, ctx context.Context) {
	a.CancelSync()
	a.CancelDailyReport()
	a.cancelTaskExecutions()
	a.executionService().Wait()
}

func (a *App) GetLibrary() (Library, error) {
	a.projects.Lock()
	defer a.projects.Unlock()
	return readLibrary(a.root)
}

func (a *App) SelectDocument(key string) error { return rememberDocument(a.root, key) }

func (a *App) CopyText(text string) error { return wruntime.ClipboardSetText(a.ctx, text) }

func (a *App) OpenSource(key string) error {
	doc, err := ReadDocument(a.root, key)
	if err != nil {
		return err
	}
	link, err := storage.ValidateURL(doc.SourceURL)
	if err != nil {
		return err
	}
	wruntime.BrowserOpenURL(a.ctx, link)
	return nil
}

func (a *App) SyncDocument(link, owner string, includeCompleted bool) (model.Document, error) {
	return a.syncDocument(link, owner, includeCompleted, true)
}

// Startup sync must not pause the entire queue waiting for an interactive login.
func (a *App) SyncDocumentOnStartup(link, owner string, includeCompleted bool) (model.Document, error) {
	return a.syncDocument(link, owner, includeCompleted, false)
}

func (a *App) syncDocument(link, owner string, includeCompleted, interactive bool) (model.Document, error) {
	link, err := storage.ValidateURL(link)
	if err != nil {
		return model.Document{}, err
	}
	return a.runJob(owner, func(ctx context.Context, notify reader.ProgressFunc) (model.Document, error) {
		if existing, err := ReadDocument(a.root, storage.DocumentKey(link, owner)); err == nil && pendingStatusCount(existing) > 0 {
			return model.Document{}, errors.New("这份文档有待同步的状态或备注修改，请先回写或撤销，再重新读取。")
		}
		doc, err := reader.ReadPublicTasks(ctx, link, owner, notify, nil, includeCompleted)
		if errors.Is(err, reader.ErrNeedsLogin) {
			if !interactive {
				return model.Document{}, errors.New("文档需要登录，请手动同步完成登录。")
			}
			return reader.ReadAuthenticatedTasks(ctx, link, owner, notify, includeCompleted)
		}
		return doc, err
	})
}

func (a *App) ImportExcel(owner string, includeCompleted bool) (*model.Document, error) {

	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{Title: "导入包含图片的 Excel", Filters: []wruntime.FileFilter{{DisplayName: "Excel 工作簿", Pattern: "*.xlsx"}}})
	if err != nil || path == "" {
		return nil, err
	}
	doc, err := a.importPath(path, owner, includeCompleted)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

func (a *App) importPath(path, owner string, includeCompleted bool) (model.Document, error) {
	return a.runDocumentJob(owner, "", func(ctx context.Context, notify reader.ProgressFunc) (model.Document, error) {
		notify("正在读取 Excel 的任务和图片…")
		return reader.FilterWorkbook(ctx, path, owner, nil, includeCompleted)
	})
}

func (a *App) ReimportDocument(key string, includeCompleted bool) (model.Document, error) {
	doc, err := ReadDocument(a.root, key)
	if err != nil {
		return model.Document{}, err
	}
	if doc.SourceURL != "" {
		return a.SyncDocument(doc.SourceURL, doc.Owner, includeCompleted)
	}
	return a.importPath(doc.SourcePath, doc.Owner, includeCompleted)
}

func (a *App) runJob(owner string, read func(context.Context, reader.ProgressFunc) (model.Document, error)) (model.Document, error) {
	if strings.TrimSpace(owner) == "" {
		return model.Document{}, errors.New("请填写负责人姓名。")
	}
	return a.runDocumentJob(owner, "", read)
}

func (a *App) runDocumentJob(owner, key string, read func(context.Context, reader.ProgressFunc) (model.Document, error)) (model.Document, error) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return model.Document{}, errors.New("已有同步任务正在执行。")
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.cancel = cancel
	a.busy = true
	a.mu.Unlock()
	defer func() {
		cancel()
		// Sync/import has bursty decoding buffers. Release them between documents,
		// instead of keeping the peak heap resident throughout an idle session.
		debug.FreeOSMemory()
		a.mu.Lock()
		a.busy = false
		a.cancel = nil
		a.mu.Unlock()
	}()
	notify := func(message string) {
		if a.ctx != nil {
			wruntime.EventsEmit(a.ctx, "reader:progress", message)
		}
	}
	doc, err := read(ctx, notify)
	if err == nil {
		notify("正在保存文字和图片到本机…")
		doc, err = saveDocumentAtKey(ctx, a.root, doc, key)
	}
	if err != nil && ctx.Err() != nil {
		return model.Document{}, errors.New("已取消同步，本地内容已保留。")
	}
	return doc, err
}

func (a *App) CancelSync() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}
