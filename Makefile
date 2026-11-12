.PHONY: help bindings ent sqlc i18n dev build check lint-go lint-go-fix lint-frontend test-go fuzz-go clean install deps update-deps format-i18n format-i18n-go format-i18n-frontend sync pull

# 环境变量优先级高于 .env：include 的赋值会覆盖环境值，先暂存一份
ifeq ($(origin GIT_REMOTE),environment)
    GIT_REMOTE_ENV := $(GIT_REMOTE)
endif

# 加载 .env 文件（如果存在）
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

ifneq ($(GIT_REMOTE_ENV),)
    GIT_REMOTE := $(GIT_REMOTE_ENV)
endif

# 同步/推送目标远程，逗号分隔：GIT_REMOTE=origin,github,gitlab
# 可通过 .env / 环境变量 / 命令行覆盖，未配置时默认 origin。
GIT_REMOTE ?= origin
export GIT_REMOTE

# 默认目标
help: ## 显示帮助信息
	@echo "CertFlow 开发命令"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==================== 开发工具 ====================

install: ## 安装前端依赖
	pnpm --dir ./frontend install

# -clean=true 生成前清理输出目录
# -ts 生成 TypeScript
# -i 使用 TS 接口
# -names 使用名称而非 ID
bindings: ## 生成 Wails 绑定
	wails3 generate bindings -clean=true -ts -i

icons: ## 生成图标资源
	wails3 task common:generate:icons

build-assets: ## 同步版本号/应用名到构建资源
	wails3 task common:update:build-assets

wails3-generate: bindings icons build-assets ## 生成 Wails 绑定/图标/webview/构建资源

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

format: format-go-fmt format-go-fix format-frontend-write format-frontend-fix format-i18n format-json format-yaml format-markdown format-shell format-docker ## 格式化和修复（Go + Vue + TypeScript）

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

# ==================== 其他 格式化 ====================

# 格式化 JSON 文件
.PHONY: format-json
format-json:
	@echo "[Format] 格式化 JSON 文件..."
	@command -v gojq >/dev/null 2>&1 || { \
		echo "⚠️  gojq 未安装，跳过 JSON 格式化。" ; \
		echo "   提示: 可通过运行 'go install github.com/itchyny/gojq/cmd/gojq@latest' 安装" ; \
		exit 0 ; \
	}
	@echo "[Format] 使用 gojq 格式化 JSON..."
	@for f in $(shell find internal/i18n/locales frontend/src/locales -type f -name '*.json'); do \
		[ -f "$$f" ] || continue; \
		if gojq . "$$f" > "$$f.tmp" 2>/dev/null; then \
			mv "$$f.tmp" "$$f"; \
		else \
			echo "❌ gojq 格式化失败: $$f"; rm -f "$$f.tmp"; exit 1; \
		fi; \
	done
	@echo "[Format] JSON 格式化完成（gojq）。"

# 格式化 YAML 文件
.PHONY: format-yaml
format-yaml:
	@echo "[Format] 格式化配置文件 (YAML)…"
	@command -v yamlfmt >/dev/null 2>&1 || { \
		echo "⚠️  yamlfmt 未安装，跳过 YAML 格式化。" ; \
		echo "   提示: 可通过运行 'go install github.com/google/yamlfmt/cmd/yamlfmt@latest' 安装" ; \
		exit 0 ; \
	}
	@echo "[Format] 使用 yamlfmt 格式化 YAML..."
	@yamlfmt -conf .yamlfmt \
		".cnb" \
		".github" \
		".gitcode" \
		".workflow" \
		".cnb.yml" \
		".nfpm.yaml" \
		".golangci.yml" \
		".gitlab-ci.yml" || { echo "❌ yamlfmt 格式化失败"; exit 1; }
	@echo "[Format] YAML 格式化完成（yamlfmt）。"

# 格式化 Markdown 文件
.PHONY: format-markdown
format-markdown:
	@echo "[Format] 格式化文档 (Markdown)…"
	@command -v markdownfmt >/dev/null 2>&1 || { \
		echo "⚠️  markdownfmt 未安装，跳过 Markdown 格式化。" ; \
		echo "   提示: 可通过运行 'go install github.com/Kunde21/markdownfmt/v3/cmd/markdownfmt@latest' 安装" ; \
		exit 0 ; \
	}
	@echo "[Format] 使用 markdownfmt 格式化 Markdown..."
	@for f in $(shell find . -type f -name '*.md' \
		-not -path '*/node_modules/*' \
		-not -path './.git/*' \
		-not -path './.vscode/*' \
		-not -path './.idea/*' \
		-not -path './.codebuddy/*' \
		-not -path './.trae/*' \
		-not -path './.mimocode/*'); do \
		[ -f "$$f" ] || continue; \
		markdownfmt -w "$$f" || { echo "❌ markdownfmt 格式化失败: $$f"; exit 1; }; \
	done
	@echo "[Format] Markdown 格式化完成（markdownfmt）。"

# 格式化 Shell 脚本
.PHONY: format-shell
format-shell:
	@echo "[Format] 格式化脚本 (Shell)…"
	@command -v shfmt >/dev/null 2>&1 || { \
		echo "⚠️  shfmt 未安装，跳过 Shell 格式化。" ; \
		echo "   提示: 可通过运行 'go install mvdan.cc/sh/v3/cmd/shfmt@latest' 安装" ; \
		exit 0 ; \
	}
	@echo "[Format] 使用 shfmt 格式化 Shell..."
	@shfmt -w -i 2 -ci -bn -s -ln bash scripts/ || { echo "❌ shfmt 格式化失败"; exit 1; }
	@echo "[Format] Shell 格式化完成（shfmt）。"

# 格式化 Dockerfile 文件
.PHONY: format-docker
format-docker:
	@echo "[Format] 格式化 Dockerfile..."
	@command -v dockerfmt >/dev/null 2>&1 || { \
		echo "⚠️  dockerfmt 未安装，跳过 Dockerfile 格式化。" ; \
		echo "   提示: 可通过运行 'go install github.com/reteps/dockerfmt@latest' 安装" ; \
		exit 0 ; \
	}
	@echo "[Format] 使用 dockerfmt 格式化 Dockerfile..."
	@find .cnb -type f -name "Dockerfile*" -exec dockerfmt -w {} + || { echo "❌ dockerfmt 格式化失败"; exit 1; }
	@find .github -type f -name "Dockerfile*" -exec dockerfmt -w {} + || { echo "❌ dockerfmt 格式化失败"; exit 1; }
	@echo "[Format] Dockerfile 格式化完成（dockerfmt）。"

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

	gojq --version || true
	go install github.com/itchyny/gojq/cmd/gojq@latest
	-gojq --version
	@echo "==> gojq 工具安装或更新完成"

	yamlfmt -version || true
	go install github.com/google/yamlfmt/cmd/yamlfmt@latest
	-yamlfmt -version
	@echo "==> yamlfmt 工具安装或更新完成"

	shfmt -version || true
	go install mvdan.cc/sh/v3/cmd/shfmt@latest
	-shfmt -version
	@echo "==> shfmt 工具安装或更新完成"

	dockerfmt version || true
	go install github.com/reteps/dockerfmt@latest
	-dockerfmt version
	@echo "==> dockerfmt 工具安装或更新完成"

	markdownfmt -version || true
	go install github.com/Kunde21/markdownfmt/v3/cmd/markdownfmt@latest
	-markdownfmt -version
	@echo "==> markdownfmt 工具安装或更新完成"

deps: ## 安装所有依赖
	@echo "==> 安装所有依赖..."

	go version
	go mod download
	@echo "==> Go 安装所有依赖完成"

	pnpm --version
	pnpm --dir ./frontend install
	@echo "==> pnpm 安装所有依赖完成"

update-deps: ## 更新所有依赖
	@echo "==> 更新所有依赖..."

	go version
	go get -u ./...
	go mod tidy
	@echo "==> Go 更新所有依赖完成"

	pnpm --version
	pnpm --dir ./frontend self-update
# 	pnpm --dir ./frontend update
	pnpm --dir ./frontend update --latest
	@echo "==> pnpm 更新所有依赖完成"

setup: deps bindings ent sqlc ## 完整项目初始化
	@echo "项目初始化完成！运行 'make dev' 启动开发模式"

# ==================== 更新 / 拉取 ====================

# 按 GIT_REMOTE 顺序先全部拉取合并，再统一推送（保留本地未提交改动）
.PHONY: sync
sync:
	@IFS=,; for r in $$GIT_REMOTE; do \
		git fetch "$$r" && git merge --ff-only "$$r/master" || exit 1; \
	done
	@IFS=,; for r in $$GIT_REMOTE; do git push "$$r" HEAD || exit 1; done
	@echo "同步完成！"

# ==================== 推送 ====================

# 推送到所有远程仓库
.PHONY: push
push:
	@IFS=,; for r in $$GIT_REMOTE; do git push "$$r" HEAD || exit 1; done
	@echo "推送完成！"

# 强制推送到所有远程仓库（忽略冲突）
.PHONY: push-force
push-force:
	@IFS=,; for r in $$GIT_REMOTE; do git push --force "$$r" HEAD || exit 1; done
	@echo "强制推送完成！"
