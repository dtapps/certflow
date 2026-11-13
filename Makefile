# 设置默认目标
.DEFAULT_GOAL := setup

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

# 前端项目目录
FRONTEND_DIR = frontend

# 编译输出目录
BUILD_DIR=bin

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

code-generate: ent sqlc i18n ##  代码生成 ent sqlc i18n

dev: i18n ## 运行 Wails 开发模式
	wails3 dev -port 9246

# ==================== 构建打包 ====================

go-build: ## 快速编译（make go-build VERSION=1.0.0）
	go build -tags production -trimpath -buildvcs=false "-ldflags=-w -s" -o bin/certflow

build: ## 构建生产包（make build VERSION=1.0.0）
	task build VERSION=$(VERSION)

package: ## 打包应用（make package VERSION=1.0.0）
	task package VERSION=$(VERSION)

build-frontend: ## 构建前端
	pnpm --dir $(FRONTEND_DIR) build

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

setup: deps ## 一键设置项目（默认执行）
	@echo "🎉 Project setup completed!"

.PHONY: deps
deps: deps-go deps-node ## 安装全部依赖
	@echo "[Deps] 完成。"

.PHONY: deps-go
deps-go: ## 安装全部Go依赖
	@echo "[Deps] 安装 Go 模块依赖..."
	go mod tidy
	go mod download

# --frozen-lockfile (冻结锁文件)
# --prefer-offline (优先使用离线缓存)
.PHONY: deps-node
deps-node: ## 安装全部前端依赖
	@echo "[Deps] 安装前端依赖..."
	pnpm --dir $(FRONTEND_DIR) install --frozen-lockfile --prefer-offline

# --latest (无视版本限制，强制升最新)
.PHONY: update-deps
update-deps: update-deps-go update-deps-node ## 更新全部依赖
	@echo "[Deps] 完成。"

.PHONY: update-deps-go
update-deps-go: ## 更新全部Go依赖
	@echo "[Deps] 更新 Go 模块依赖..."
	go get -u ./...
	go mod tidy

# --latest (无视版本范围，强制升最新，并改写 package.json 的版本范围)
.PHONY: update-deps-node
update-deps-node: ## 更新全部前端依赖
	@echo "[Deps] 更新前端依赖..."
	pnpm --dir $(FRONTEND_DIR) update --latest
	pnpm --dir $(FRONTEND_DIR) prune

.PHONY: update-deps-pnpm
update-deps-pnpm: ## 更新 pnpm 自身的版本锁定（package.json 的 packageManager 字段）
	@echo "[Deps] 更新 pnpm 版本锁定..."
	@latest="$$(npm view pnpm version)"; \
	echo "[Deps] packageManager → pnpm@$${latest}"; \
	pnpm --dir $(FRONTEND_DIR) pkg set packageManager=pnpm@$${latest}

.PHONY: deps-clean
deps-clean: ## 删除未使用的依赖：knip --fix 把它们从 package.json 摘掉，
	@echo "[Deps] 删除未使用的依赖..."
	pnpm --dir $(FRONTEND_DIR) run deps:clean
	pnpm --dir $(FRONTEND_DIR) install --lockfile-only
	pnpm --dir $(FRONTEND_DIR) prune

#####################
## 格式化 相关
#####################

.PHONY: format-go ## 格式化后端文件
format-go:
	@echo "[Format] 格式化后端文件 (GO)..."
	gofmt -w -s $(shell find . -type f -name '*.go' -not -path './internal/ent/*')
	go fmt ./...
	go fix ./...

.PHONY: format-node ## 格式前端文件
format-node:
	@echo "[Format] 格式化前端文件 (PNPM)..."
	pnpm --dir $(FRONTEND_DIR) format

.PHONY: format-yaml ## 格式化 YAML 文件
format-yaml:
	@bash scripts/format.sh yaml

.PHONY: format-json ## 格式化 JSON 文件
format-json:
	@bash scripts/format.sh json

.PHONY: format-sql ## 格式化 SQL 文件
format-sql:
	@bash scripts/format.sh sql

.PHONY: format-markdown ## 格式化 Markdown 文件
format-markdown:
	@bash scripts/format.sh markdown

.PHONY: format-docker ## 格式化 Dockerfile 文件
format-docker:
	@bash scripts/format.sh docker

.PHONY: format-shell ## 格式化 Shell 脚本
format-shell:
	@bash scripts/format.sh shell

.PHONY: format ## 格式化
format: format-node format-go format-yaml format-json format-sql format-markdown format-docker format-shell

#####################
## 检查 / 测试 相关
#####################

# 统一定义构建命令
# CGO_ENABLED=0: 禁用 CGO，实现完全静态链接
# CGO_ENABLED=1: 启用 CGO，实现动态链接
LINT_ENV = CGO_ENABLED=0

# 定义 Lint 过程中需要激活的 Go Build Tags
LINT_TAGS = goolm,stdjson,purego

# Lint 扫描的目标平台，默认当前平台；CI 通过 LINT_PLATFORMS=... 覆盖为全平台
LINT_PLATFORMS ?= $(shell go env GOOS)/$(shell go env GOARCH)

# -printf=false 关闭 printf 分析，避免 i18n.T() 作日志 format 时触发错误
.PHONY: check-go
check-go: ## 检查后端代码
	@echo "[Check] 运行 go vet 静态检查..."
	go vet -printf=false ./...

	@echo "[Check] 运行 golangci-lint 扫描 (平台: $(LINT_PLATFORMS), Tags: $(LINT_TAGS))..."
	@for os_arch in $(LINT_PLATFORMS); do \
		os=$$(echo $$os_arch | cut -d'/' -f1); \
		arch=$$(echo $$os_arch | cut -d'/' -f2); \
		echo "  -> $$os 平台 ($$arch)"; \
		$(LINT_ENV) GOOS=$$os GOARCH=$$arch golangci-lint fmt; \
		$(LINT_ENV) GOOS=$$os GOARCH=$$arch golangci-lint run --build-tags $(LINT_TAGS); \
	done

	@echo "[Check] 自动修复代码风格问题..."
	$(LINT_ENV) golangci-lint run --fix --build-tags $(LINT_TAGS)

	@echo "[Check] 运行 Go 单元测试..."
	go test $(GO_COVER_TEST_FLAGS) $(GO_COVER_SCAN)

	@echo "[Check] 运行 Go 赖漏洞检查..."
	govulncheck -show verbose ./...

	@echo "[Check] 完成。"

.PHONY: check-node
check-node: ## 检查前端代码
	@echo "[Check] 检查前端依赖与质量..."
	@echo "  -> 同步依赖 (确保环境一致)..."
	pnpm --dir $(FRONTEND_DIR) install --frozen-lockfile --prefer-offline

	@echo "  -> 检查安全漏洞 (仅高危阻断)..."
	pnpm --dir $(FRONTEND_DIR) audit --production --audit-level high --registry=https://registry.npmjs.org

	@echo "  -> 检查过时依赖..."
	pnpm --dir $(FRONTEND_DIR) outdated

	@echo "  -> 检查代码风格 (Lint)..."
	pnpm --dir $(FRONTEND_DIR) run lint

	@echo "  -> 检测未使用的依赖 (knip)..."
	pnpm --dir $(FRONTEND_DIR) run lint:unused

	@echo "  -> 运行前端单元测试..."
	pnpm --dir $(FRONTEND_DIR) exec vitest run

	@echo "[Check] 前端检查完成。"

.PHONY: check
check: check-node check-go ## 检查代码

#####################
## 更新 / 拉取 / 推送 相关
#####################

.PHONY: sync
sync: ## 按 GIT_REMOTE 顺序先全部拉取合并，再统一推送（保留本地未提交改动）
	@IFS=,; for r in $$GIT_REMOTE; do \
		git fetch "$$r" && git merge --ff-only "$$r/master" || exit 1; \
	done
	@IFS=,; for r in $$GIT_REMOTE; do git push "$$r" HEAD || exit 1; done
	@echo "同步完成！"

.PHONY: push
push: ## 推送到所有远程仓库
	@IFS=,; for r in $$GIT_REMOTE; do git push "$$r" HEAD || exit 1; done
	@echo "推送完成！"

.PHONY: push-force
push-force: ## 强制推送到所有远程仓库（忽略冲突）
	@IFS=,; for r in $$GIT_REMOTE; do git push --force "$$r" HEAD || exit 1; done
	@echo "强制推送完成！"

#####################
## 测试覆盖率 相关
#####################

# ── 清单配置：新增目录 / 生成代码只改这里，下面的命令不散落路径 ──────────────
# Go 扫描范围：业务代码全在 internal/ 下，cmd/ 只是可执行入口，不计入分母。
GO_COVER_SCAN = ./internal/...
# Go 排除目录（包路径前缀，空格分隔）：ent 生成代码体量极大，计入会把指标压到接近 0。
GO_COVER_EXCLUDE = /internal/ent
# 关闭 vet 检查，加快测试与覆盖率统计速度
GO_COVER_TEST_FLAGS = -vet=off

# 前端目录与产物（相对仓库根）
NODE_DIR = $(FRONTEND_DIR)
NODE_COVER_DIR = $(NODE_DIR)/coverage
NODE_COVER_LCOV = $(NODE_COVER_DIR)/lcov.info
NODE_JUNIT_XML = $(NODE_COVER_DIR)/junit.xml
# 前端扫描 / 排除清单（glob 相对 vitest root=frontend/，由 frontend/vitest.config.ts 读取）：
#   NODE_TEST_SCAN      测试用例
#   NODE_COVER_SCAN     计入覆盖率的源码
#   NODE_COVER_EXCLUDE  不计入的测试自身 / 入口 / 声明文件
# 多个 glob 用空格或逗号分隔；留空则回退到配置文件里的默认值。
# 注意：不要用 {a,b} 花括号写法（逗号会被当作分隔符），请写成多个空格分隔的 glob。
NODE_TEST_SCAN = src/test/**/*.test.ts
NODE_COVER_SCAN = src/**/*.ts src/**/*.vue
NODE_COVER_EXCLUDE = src/test/** src/main.ts src/logviewer/main.ts src/**/*.d.ts
NODE_VITEST_ENV = VITEST_TEST_INCLUDE="$(NODE_TEST_SCAN)" VITEST_COVER_INCLUDE="$(NODE_COVER_SCAN)" VITEST_COVER_EXCLUDE="$(NODE_COVER_EXCLUDE)"

# -coverpkg 只收「扫描范围内且不在排除列表」的包。用 $$(...) 让 go list 在配方执行时才跑，
# 避免任何 make 目标都触发一次包扫描；grep 参数由排除列表生成，加目录不用改命令。
GO_COVER_EXCLUDE_ARGS = $(foreach d,$(GO_COVER_EXCLUDE),-e '$(d)')
GO_COVER_PKGS = $$(go list $(GO_COVER_SCAN) | grep -v $(GO_COVER_EXCLUDE_ARGS) | paste -sd, -)
GO_COVER_ARGS = $(GO_COVER_TEST_FLAGS) -coverpkg=$(GO_COVER_PKGS) -coverprofile=coverage.out $(GO_COVER_SCAN)

.PHONY: cover-go
cover-go: ## Go 覆盖率（golang 格式）→ coverage.out；CNB 的 testing:coverage 与 Codecov 都读这份
	@echo "[Coverage] Go 测试覆盖率 → coverage.out"
	go test $(GO_COVER_ARGS)
	go tool cover -func=coverage.out

.PHONY: junit-go
junit-go: ## Go 覆盖率 + JUnit（Codecov Test Analytics 用），依赖 CI 镜像预装的 gotestsum
	@echo "[Coverage] Go 测试 → coverage.out + go-test-report.xml"
	gotestsum --junitfile go-test-report.xml -- $(GO_COVER_ARGS)
	go tool cover -func=coverage.out

# gocover-cobertura 由统一 CI 镜像预装；-strict 让「包/模块信息缺失被静默跳过」直接失败。
.PHONY: cover-go-cobertura
cover-go-cobertura: cover-go ## Coveralls 的 golang parser 会把包前缀压成 basename，改用 Cobertura 保留完整 filename 路径
	@echo "[Coverage] coverage.out → coverage.xml（Cobertura）"
	gocover-cobertura -by-files -strict < coverage.out > coverage.xml

.PHONY: cover-node
cover-node: ## 前端覆盖率（Vitest v8）→ frontend/coverage/lcov.info
	@echo "[Coverage] 前端覆盖率 → $(NODE_COVER_LCOV)"
	cd $(NODE_DIR) && $(NODE_VITEST_ENV) pnpm exec vitest run --coverage

.PHONY: junit-node
junit-node: ## 前端 JUnit（Codecov Test Analytics 用）；outputFile 用绝对路径避开 vitest root 的相对路径歧义
	@echo "[Coverage] 前端 JUnit → $(NODE_JUNIT_XML)"
	@mkdir -p $(NODE_COVER_DIR)
	cd $(NODE_DIR) && $(NODE_VITEST_ENV) pnpm exec vitest run --reporter=junit --outputFile=$(CURDIR)/$(NODE_JUNIT_XML)