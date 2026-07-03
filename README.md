# CertFlow

[English](README_EN.md) | 中文

SSL 证书管理工具，支持证书申请、续期、过期监控等功能。

采用 Go + Vue 3 跨平台桌面应用架构，基于 Wails v3 框架。

## 功能特性

- **证书管理** — 申请、续期、撤销 SSL 证书，支持 30+ DNS 提供商自动验证
- **域名监控** — HTTPS/HTTP 健康检查，证书过期预警
- **自动续期** — 定时任务自动续期即将过期的证书
- **多平台** — 支持 macOS、Windows、Linux

## 快速开始

```bash
# 安装依赖
make deps

# 启动开发模式
make dev

# 构建生产包（带版本号）
make build VERSION=1.0.0

# 打包 macOS 应用
make package VERSION=1.0.0
```

## 开发命令

```bash
make help          # 查看所有命令
make lint          # Go 代码检查
make lint-fix      # Go 代码检查（自动修复）
make check         # 前端 TypeScript 类型检查
make test          # Go 后端测试
make bindings      # 生成 Wails TypeScript 绑定
make ent           # 生成 Ent ORM 代码
```

## 技术栈

- **后端**：Go 1.27 + Wails v3 + Ent ORM + lego v5（ACME）+ SQLite
- **前端**：Vue 3 + TypeScript + Vite 8 + TailwindCSS v4 + DaisyUI v5

## 仓库

| 平台 | 地址 |
|------|------|
| CNB | https://cnb.cool/dtapp/certflow |
| GitHub | https://github.com/dtapps/certflow |
| Gitea | https://gitea.com/dtapps/certflow |
| GitLab | https://gitlab.com/dtapps/certflow |
| Gitee | https://gitee.com/dtapps/certflow |
| GitCode | https://gitcode.com/dtapp/certflow |

## 发布

在任意平台的手动触发工作流中输入版本号即可发布，构建产物从 GitHub Actions 下载。

## 开发文档

详见 [DEVELOPMENT.md](DEVELOPMENT.md)
