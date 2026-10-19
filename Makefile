.DEFAULT_GOAL := run

WAILS_VERSION := v2.16.0
WAILS := $(CURDIR)/.tools/$(WAILS_VERSION)/wails
DEV_SERVER ?= 127.0.0.1:34871
FRONTEND_STAMP := frontend/node_modules/.package-lock.json
FRONTEND_SOURCES := $(shell rg --files frontend/src frontend/public) frontend/index.html frontend/vite.config.ts frontend/tsconfig.json

.PHONY: install run dev test test-live build open help frontend-build assets

$(WAILS):
	@mkdir -p ".tools/$(WAILS_VERSION)"
	GOBIN="$(CURDIR)/.tools/$(WAILS_VERSION)" go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

frontend/package-lock.json: frontend/package.json
	cd frontend && npm install

$(FRONTEND_STAMP): frontend/package-lock.json
	cd frontend && npm ci

desktop/ui/index.html: $(FRONTEND_STAMP) $(FRONTEND_SOURCES)
	cd frontend && npm run build

install: $(WAILS) desktop/ui/index.html
	cd desktop && go mod download

assets:
	node scripts/prepare-assets.mjs

run: install assets
	cd desktop && "$(WAILS)" dev -devserver "$(DEV_SERVER)"

dev: run

test: install
	cd desktop && go test ./...
	cd frontend && npm test

test-live: install
	cd desktop && TASKPICKER_LIVE_TESTS=1 go test ./internal/app -run TestLiveDocuments -v

frontend-build: $(FRONTEND_STAMP)
	cd frontend && npm run build

build: install assets
	cd desktop && "$(WAILS)" build -clean
	codesign --force --deep --sign - "build/bin/我的任务.app"
	node scripts/package-app.mjs

open:
	open "dist/我的任务.app"

help:
	@printf '%s\n' 'make / make run  启动 Go + Wails + Vue 桌面应用' 'make install     安装 Go 与前端依赖' 'make test        Go、前端测试和 TypeScript 检查' 'make test-live   用自行配置的腾讯文档验证读取' 'make build       打包 dist/我的任务.app' 'make open        打开已打包的应用' '源码需要 Go 1.26+、Node.js 22.12+ 和 Xcode Command Line Tools'
