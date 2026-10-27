# 更新日志

本文件记录 CertFlow 的用户可见变更与重要工程变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号以 Git tag 为准。

- 安装包与完整发布说明：[GitHub Releases](https://github.com/dtapps/certflow/releases)
- 开发过程记录：[docs/](docs/README.md)
- 更早版本的逐条记录未在此展开，可用 `git log <tag>..<tag>` 查看

## [Unreleased]

### 新增

- 文档体系补齐：架构、数据模型、用户指南、排障、测试 / 发布 / 安全说明、ADR，见 [docs/README.md](docs/README.md)。
- 新增根目录 [CONTRIBUTING.md](CONTRIBUTING.md) 与 [SECURITY.md](SECURITY.md)。

### 变更

- 升级 Go 与前端依赖（Vue `3.6.0-rc.13`、腾讯云 SDK 等）。

## [0.2.20] - 2026-10-25

### 新增

- Codecov 覆盖率 / Bundle / 测试结果三路上报，按业务域拆为 9 个组件。
- Coveralls 双 flag（`go` / `node`）覆盖率上报。
- CNB 代码评审、依赖漏洞、许可证、SBOM 流水线。
- CI 基础镜像：GitHub 侧推 GHCR，CNB 侧流水线内构建，预装 Go / Node / pnpm / GTK / WebKit 与固定版本工具链。

### 变更

- 发布流水线改用文件传递 Release 正文，并自动判定预发布标记。
- 发布同步新增 GitLab / GitCode / Gitee 三个目标（CNB 沿用）；GitCode 当前不作为发布平台。
- 覆盖率红线阈值暂停（只上报、不阻断），待基线稳定后恢复。
- Markdown 统一由 `markdownfmt` 格式化。

### 修复

- Codecov 测试报告使用绝对路径导致记录分裂。
- Coveralls 在容器内因 git `safe.directory` 报 422。

### 测试

- 补充 auth / ca 的 Passkey、TOTP 测试；覆盖率统计排除 sqlc 生成代码。

## [0.2.19] - 2026-10-22

### 新增

- Go 与 Node 覆盖率流水线，支持 push 全量、PR 增量（`diffLines`）门禁。
- Codecov 配置与 GitHub Actions 上报。
- Code Wiki 生成迁移到 tag 推送触发。

### 变更

- 覆盖率报告按 `key`（`go` / `node`）隔离，避免徽章互相覆盖。

## [0.2.18] - 2026-10-22

### 修复

- 数据备份导出表清单收窄为 `sqlite_master` 实际清单，避免混入目录里的其它 CSV。
- 导入备份时校验表名白名单并转义 SQL 标识符。
- CI 代码扫描路径规则修正，恢复全量扫描。

## [0.2.17] - 2026-10-19

### 新增

- 监控检查完成实时推送到前端，告警按状态变化去重并支持恢复通知。

### 性能

- 新增 HTTP 客户端缓存，按设置指纹复用连接池。

### 变更

- 升级 Wails v3、Vue 3.6 RC、Vite 等依赖。

## [0.2.15] - 2026-10-10

### 变更

- 升级 Vite、vue-echarts 与 Go 依赖版本。

## [0.2.14] - 2026-10-10

### 新增

- 更新下载接入中转，失败时自动回退源站。

### 变更

- 监控与托盘初始化迁移到 Wails 服务生命周期，并修复主题事件判断。
