.PHONY: help bindings ent dev build check lint lint-fix test clean install deps update-deps

# 默认目标
help: ## 显示帮助信息
	@echo "CertFlow 开发命令"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==================== 开发工具 ====================

install: ## 安装前端依赖
	cd frontend && pnpm install

bindings: ## 生成 Wails TypeScript 绑定
	wails3 generate bindings -clean=true -ts -i

ent: ## 生成 Ent ORM 代码
	go run -tags entc ./entc_generate.go

dev: ## 运行 Wails 开发模式
	wails3 dev

# ==================== 格式化 / 修复 ====================

format: format-go-fmt format-go-fix format-frontend-write format-frontend-fix ## 格式化和修复所有代码

format-go-fmt: ## 格式化 Go 代码
	gofmt -w -s .
	go fmt ./...

format-go-fix: ## 修复 Go 代码
	go fix ./...	

format-frontend-write: ## 格式化前端代码（Vue + TypeScript）
	cd frontend && pnpm exec prettier --write "src/**/*.{vue,ts,js,css}"

format-frontend-fix: ## 修复前端代码（Vue + TypeScript）
	cd frontend && pnpm exec eslint --fix "src/**/*.{vue,ts,js}"

# ==================== 检查 ====================

lint: ## Go 代码检查
	golangci-lint run ./...

lint-fix: ## Go 代码检查（自动修复）
	golangci-lint run --fix ./...

check: ## 前端 TypeScript 类型检查
	cd frontend && pnpm exec vue-tsc --noEmit

test: ## Go 后端测试
	go test -vet=off ./internal/... -count=1

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

deps: ## 安装所有依赖
	go mod download
	cd frontend && pnpm install

update-deps: ## 更新所有依赖
	go get -u ./...
	go mod tidy
	cd frontend && pnpm update --latest

setup: deps bindings ent ## 完整项目初始化
	@echo "项目初始化完成！运行 'make dev' 启动开发模式"

# ==================== 推送 ====================

push: ## 推送到所有远程仓库
	git push origin HEAD
	git push github HEAD
	git push gitea HEAD
	git push gitlab HEAD
	git push gitee HEAD
	git push gitcode HEAD
	@echo "推送完成！"

push-force: ## 强制推送到所有远程仓库（忽略冲突）
	git push --force origin HEAD
	git push --force github HEAD
	git push --force gitea HEAD
	git push --force gitlab HEAD
	git push --force gitee HEAD
	git push --force gitcode HEAD
	@echo "强制推送完成！"