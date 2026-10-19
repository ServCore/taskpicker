package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	application "taskpicker/internal/app"
)

//go:embed all:ui
var assets embed.FS

// App keeps the existing window.go.main.App bridge namespace.
type App struct{ *application.App }

func main() {
	app := &App{App: application.NewApp()}
	appMenu := menu.NewMenu()
	if runtime.GOOS == "darwin" {
		appMenu.Append(menu.AppMenu())
		appMenu.Append(menu.EditMenu())
	}
	err := wails.Run(&options.App{
		Title: "我的任务", Width: 1380, Height: 860, MinWidth: 1080, MinHeight: 660,
		BackgroundColour: options.NewRGB(245, 246, 247),
		AssetServer: &assetserver.Options{Assets: assets, Middleware: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/local-image/") {
					application.ImageHandler(app.App).ServeHTTP(w, r)
					return
				}
				next.ServeHTTP(w, r)
			})
		}},
		OnStartup:  func(ctx context.Context) { application.Startup(app.App, ctx) },
		OnShutdown: func(ctx context.Context) { application.Shutdown(app.App, ctx) },
		Bind:       []interface{}{app}, Menu: appMenu,
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(), Appearance: mac.NSAppearanceNameAqua,
			About: &mac.AboutInfo{Title: "我的任务", Message: "腾讯文档 · 本地任务库\nWails + Vue · 2.0"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
