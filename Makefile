.PHONY: help bindings ent i18n dev build check lint-go lint-go-fix lint-frontend test-go fuzz-go clean install deps update-deps format-i18n format-i18n-go format-i18n-frontend

# 过滤 macOS 链接器噪声（ld: warning / was built for newer / ignoring duplicate libraries），
# 让真正的编译/测试/检查错误清晰可见。用法：<命令> $(FILTER) || exit 1
FILTER := 2>&1 | grep -vE 'ld: warning|was built for newer|ignoring duplicate libraries'

# 默认目标
help: ## 显示帮助信息
	@echo "CertFlow 开发命令"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==================== 开发工具 ====================

install: ## 安装前端依赖
	cd frontend && pnpm install

bindings: ## 生成 Wails TypeScript 绑定
	wails3 generate bindings -clean=true -ts -i

ent: ## 生成 Ent ORM 代码
	go run -tags entc ./entc_generate.go

i18n: i18n-go i18n-frontend ## 合并所有 i18n 拆分文件到主文件

i18n-go: ## 合并 Go 后端 i18n 拆分文件
	./scripts/merge-go-i18n.sh

i18n-frontend: ## 合并前端 i18n 拆分文件
	./scripts/merge-frontend-i18n.sh

dev: i18n ## 运行 Wails 开发模式
	wails3 dev -port 9246

# ==================== 格式化 / 修复 ====================

format: format-go-fmt format-go-fix format-frontend-write format-frontend-fix format-i18n ## 格式化和修复（全部）

format-go-fmt: ## 格式化 Go 代码
	gofmt -w -s .
	go fmt ./...

format-go-fix: ## 修复 Go 代码
	go fix ./...

format-frontend-write: ## 格式化前端代码（Vue + TypeScript）
	cd frontend && pnpm exec prettier --write "src/**/*.{vue,ts,js,css}"

format-frontend-fix: ## 修复前端代码（Vue + TypeScript）
	cd frontend && pnpm exec eslint --fix "src/**/*.{vue,ts,js}"

# ==================== i18n JSON 格式化 ====================

format-i18n: format-i18n-go format-i18n-frontend ## 格式化 Go 与前端 i18n JSON 文件

format-i18n-go: ## 格式化 Go 后端 i18n JSON 文件（internal/i18n 下全部）
	cd frontend && pnpm exec prettier --config .prettierrc --write \
		"../internal/i18n/**/*.json"

format-i18n-frontend: ## 格式化前端 i18n JSON 文件（主文件 + 拆分文件）
	cd frontend && pnpm exec prettier --config .prettierrc --write \
		"src/locales/*.json" \
		"src/locales/split/**/*.json"

# ==================== 检查 / 测试 ====================

check: lint-frontend lint-go test-go fuzz-go vuln-go ## 检查和测试（全部）

lint-go: ## Go 代码检查
	golangci-lint run ./... $(FILTER) || exit 1

lint-go-fix: ## Go 代码检查（自动修复）
	golangci-lint run --fix ./...

lint-frontend: ## 前端 TypeScript 类型检查
	cd frontend && pnpm exec vue-tsc --noEmit

test-go: ## Go 后端测试
	go test -vet=off -v ./internal/... -count=1 $(FILTER) || exit 1

fuzz-go: ## Go 模糊测试（make fuzz-go FUZZ=FuzzXxx 时间=30s）
	go test -vet=off -fuzz=$(FUZZ) -fuzztime=$(or $(TIME),30s) ./internal/... $(FILTER) || exit 1

vuln-go: ## Go 依赖漏洞检查
	govulncheck ./...

# ==================== 构建打包 ====================

go-build: ## 快速编译（make go-build VERSION=1.0.0）
	go build -tags production -trimpath -buildvcs=false "-ldflags=-w -s" -o bin/certflow

build: ## 构建生产包（make build VERSION=1.0.0）
	wails3 task build VERSION=$(VERSION)

package: ## 打包应用（make package VERSION=1.0.0）
	wails3 task package VERSION=$(VERSION)

# ==================== 其他 ====================

clean: ## 清理构建产物
	rm -rf frontend/dist frontend/bindings
	rm -f certflow bin/*

tool-deps: ## 工具依赖
	@echo "==> 安装必要的工具依赖..."

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

setup: deps bindings ent ## 完整项目初始化
	@echo "项目初始化完成！运行 'make dev' 启动开发模式"

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