.PHONY: help bindings ent sqlc i18n dev build check lint-go lint-go-fix lint-frontend test-go fuzz-go clean install deps update-deps format-i18n format-i18n-go format-i18n-frontend sync pull


# 默认目标
help: ## 显示帮助信息
	@echo "CertFlow 开发命令"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==================== 开发工具 ====================

install: ## 安装前端依赖
	pnpm --dir ./frontend install

bindings: ## 生成 Wails TypeScript 绑定
	wails3 generate bindings -clean=true -ts -i

icons: ## 生成图标资源
	wails3 task common:generate:icons

build-assets: ## 同步版本号/应用名到构建资源
	wails3 task common:update:build-assets

# wails3 生成命令全集（备查）：
#   通用（全平台）：generate bindings / generate icons / update build-assets  ← 已聚合进下方 wails3-generate
#   Windows 专属：generate syso / generate webview2bootstrapper  ← 由 build/windows/Taskfile.yml 在 build/package 时自动触发
#   Linux  专属：generate appimage / generate .desktop            ← 由 build/linux/Taskfile.yml 在 build/package 时自动触发
# 平台专属命令不在本 Makefile 暴露（非目标平台跑无意义，依赖各自 Taskfile 自动跑）。
wails3-generate: bindings icons build-assets ## 生成 Wails 绑定/图标/构建资源

ent: ## 生成 Ent ORM 代码
	go run -tags entc ./entc_generate.go

sqlc: ## 生成 sqlc 代码
	cd internal/httplog && sqlc generate

i18n: i18n-go i18n-frontend ## 合并所有 i18n 拆分文件到主文件

i18n-go: ## 合并 Go 后端 i18n 拆分文件
	./scripts/merge-go-i18n.sh

i18n-frontend: ## 合并前端 i18n 拆分文件
	./scripts/merge-frontend-i18n.sh

dev: i18n ## 运行 Wails 开发模式
	wails3 dev -port 9246

# ==================== 格式化 / 修复 ====================

format: format-go-fmt format-go-fix format-frontend-write format-frontend-fix format-i18n ## 格式化和修复（Go + Vue + TypeScript）

format-go-fmt: ## 格式化 Go 代码
	gofmt -w -s .
	go fmt ./...

format-go-fix: ## 修复 Go 代码
	go fix ./...

format-frontend-write: ## 格式化前端代码（Vue + TypeScript）
	pnpm --dir ./frontend exec prettier --write "src/**/*.{vue,ts,js,css}"

format-frontend-fix: ## 修复前端代码（Vue + TypeScript）
	pnpm --dir ./frontend exec eslint --fix "src/**/*.{vue,ts,js}"

# ==================== i18n JSON 格式化 ====================

format-i18n: format-i18n-go format-i18n-frontend ## 格式化 Go 与前端 i18n JSON 文件

format-i18n-go: ## 格式化 Go 后端 i18n JSON 文件（internal/i18n 下全部）
	pnpm --dir ./frontend exec prettier --config .prettierrc --write \
		"../internal/i18n/**/*.json"

format-i18n-frontend: ## 格式化前端 i18n JSON 文件（主文件 + 拆分文件）
	pnpm --dir ./frontend exec prettier --config .prettierrc --write \
		"src/locales/*.json" \
		"src/locales/split/**/*.json"

# ==================== 检查 / 测试 ====================

check: lint-frontend lint-go test-go fuzz-go vuln-go ## 检查和测试（Go + Vue + TypeScript）

lint-go: ## Go 代码检查（有 issue 即停止）
	golangci-lint run ./...

lint-go-fix: ## Go 代码检查（自动修复）
	golangci-lint run --fix ./...

lint-frontend: ## 前端 TypeScript 类型检查（类型错误即停止）
	pnpm --dir ./frontend exec vue-tsc --noEmit

test-go: ## Go 后端测试（测试失败即停止）
	go test -vet=off -v ./internal/... -count=1

fuzz-go: ## Go 模糊测试（make fuzz-go FUZZ=FuzzXxx 时间=30s；失败即停止）
	go test -vet=off -fuzz=$(FUZZ) -fuzztime=$(or $(TIME),30s) ./internal/...

vuln-go: ## Go 依赖漏洞检查（发现漏洞即停止）
	govulncheck -show verbose ./...

# check 为扁平先决目标，按顺序执行：任一目标返回非零，Make 即在此处停止
#（默认语义），不再继续后续环节（前端类型检查 → Go lint → 测试 → 模糊测试 → 漏洞检查）。

# ==================== 构建打包 ====================

go-build: ## 快速编译（make go-build VERSION=1.0.0）
	go build -tags production -trimpath -buildvcs=false "-ldflags=-w -s" -o bin/certflow

build: ## 构建生产包（make build VERSION=1.0.0）
	task build VERSION=$(VERSION)

package: ## 打包应用（make package VERSION=1.0.0）
	task package VERSION=$(VERSION)

# ==================== 其他 ====================

clean: ## 清理构建产物
	rm -rf frontend/dist frontend/bindings
	rm -f certflow bin/*

tool-deps: ## 安装必要的工具
	@echo "==> 安装必要的工具..."

	wails3 version || true
	go install github.com/wailsapp/wails/v3/cmd/wails3@latest
	-wails3 version
	@echo "==> wails3 工具安装或更新完成"

	sqlc version || true
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	-sqlc version
	@echo "==> sqlc 工具安装或更新完成"

	go install entgo.io/ent/cmd/ent@latest
	go install entgo.io/ent/cmd/entc@latest
	@echo "==> ent 工具安装或更新完成"

	golangci-lint version || true
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	-golangci-lint version
	@echo "==> golangci-lint 工具安装或更新完成"

	govulncheck version || true
	go install golang.org/x/vuln/cmd/govulncheck@latest
	-govulncheck version
	@echo "==> govulncheck 工具安装或更新完成"

deps: ## 安装所有依赖
	@echo "==> 安装所有依赖..."

	go version
	go mod download
	@echo "==> Go 安装所有依赖完成"

	pnpm --version
	pnpm --dir ./frontend install
	@echo "==> pnpm 安装所有依赖完成"

# 	pnpm --dir ./frontend update --latest
update-deps: ## 更新所有依赖
	@echo "==> 更新所有依赖..."

	go version
	go get -u ./...
	go mod tidy
	@echo "==> Go 更新所有依赖完成"

	pnpm --version
	pnpm --dir ./frontend update
	pnpm --dir ./frontend self-update
	@echo "==> pnpm 更新所有依赖完成"

setup: deps bindings ent sqlc ## 完整项目初始化
	@echo "项目初始化完成！运行 'make dev' 启动开发模式"

# ==================== 更新 / 拉取 ====================

sync: ## 拉取 GitHub 最新并以 fast-forward 合并（保留本地未提交改动）
	git fetch origin
	git merge --ff-only origin/master
	@echo "已同步 origin/master 最新代码，本地未提交改动已保留"

sync-cnb: ## 拉取 CNB 最新并以 fast-forward 合并（保留本地未提交改动）
	git fetch cnb
	git merge --ff-only cnb/master
	@echo "已同步 cnb/master 最新代码，本地未提交改动已保留"

pull: sync sync-cnb ## 拉取所有远程最新（先 GitHub，后 CNB 镜像）

# ==================== 推送 ====================

push: ## 推送到所有远程仓库
	git push origin HEAD
	git push cnb HEAD
	git push gitea HEAD
	git push gitlab HEAD
	git push gitee HEAD
	git push gitcode HEAD
	@echo "推送完成！"

push-force: ## 强制推送到所有远程仓库（忽略冲突）
	git push --force origin HEAD
	git push --force cnb HEAD
	git push --force gitea HEAD
	git push --force gitlab HEAD
	git push --force gitee HEAD
	git push --force gitcode HEAD
	@echo "强制推送完成！"