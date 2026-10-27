# CertFlow 文档索引

本目录放两类东西：**规范文档**（架构、数据、使用、开发、运维、ADR）和**过程记录**（按日期命名的开发日志、CI 专题、交接文档）。

面向用户的功能介绍在根目录 [README.md](../README.md)，面向开发者的环境搭建与命令在 [DEVELOPMENT.md](../DEVELOPMENT.md)；协作流程见 [CONTRIBUTING.md](../CONTRIBUTING.md)，安全报告见 [SECURITY.md](../SECURITY.md)，版本变更见 [CHANGELOG.md](../CHANGELOG.md)。本索引不重复它们的内容，只负责指路。

## 1. 规范文档

### 1.1 总览与架构

| 文档                                               | 内容                                                                      |
|----------------------------------------------------|---------------------------------------------------------------------------|
| [architecture.md](architecture.md)                 | Wails v3 分层、服务包装层清单、`internal/` 包职责、启动装配、事件、数据流 |
| [database.md](database.md)                         | 15 个 ent 实体、迁移、SQLite 双驱动、导入导出、`httplog.db`、数据目录     |
| [handoff.md](handoff.md)                           | 阶段性交接（进度、遗留项、测试基建约定），随任务更新，不是长期规范        |
| [前端主题与国际化方案.md](前端主题与国际化方案.md) | 配色、主题 store、i18n store 的实现说明与设计取舍                         |

### 1.2 使用指南（`guides/`）

| 文档                                                       | 内容                                                           |
|------------------------------------------------------------|----------------------------------------------------------------|
| [guides/getting-started.md](guides/getting-started.md)     | 安装、首次启动、数据目录、界面导航、第一张证书                 |
| [guides/certificates.md](guides/certificates.md)           | CA 配置、申请、DNS-01、手动 DNS、续期、吊销、删除、密钥类型    |
| [guides/dns-providers.md](guides/dns-providers.md)         | 32 家 DNS 提供商的表单字段与后端解析支持                       |
| [guides/deployment.md](guides/deployment.md)               | 云厂商 / 面板 / WAF 部署矩阵、凭证来源、部署流程、当前证书对比 |
| [guides/monitoring.md](guides/monitoring.md)               | 域名监控、证书扫描、检查历史、状态判定、自动续期               |
| [guides/security-and-data.md](guides/security-and-data.md) | 口令 / TOTP / Passkey、数据导入导出、代理、更新校验            |

### 1.3 开发（`development/`）

| 文档                                             | 内容                                                             |
|--------------------------------------------------|------------------------------------------------------------------|
| [development/testing.md](development/testing.md) | `make test-go` / `make check`、`-vet=off` 原因、覆盖率口径、红线 |
| [development/i18n.md](development/i18n.md)       | Go 与前端 i18n 的 split → merge 工作流、key 归属、错误信封       |
| [development/release.md](development/release.md) | 六平台构建、预发布、发布平台与同步、签名 / 公证、`SHA256SUMS`    |

### 1.4 运维与排障

| 文档                                             | 内容                                                                       |
|--------------------------------------------------|----------------------------------------------------------------------------|
| [operations/security.md](operations/security.md) | 认证与密钥存储事实、备份风险、攻击面、漏洞报告                             |
| [troubleshooting.md](troubleshooting.md)         | 症状 → 原因 → 处置：申请失败、DNS 传播、部署 401/403、更新 / 代理、DB 驱动 |

### 1.5 决策记录（`adr/`）

| ADR                                                              | 决定                                    |
|------------------------------------------------------------------|-----------------------------------------|
| [adr/0001-wails-v3.md](adr/0001-wails-v3.md)                     | 用 Wails v3 做桌面外壳                  |
| [adr/0002-ent-and-sqlc.md](adr/0002-ent-and-sqlc.md)             | ent 管业务库、sqlc 管请求日志库         |
| [adr/0003-sqlite-dual-driver.md](adr/0003-sqlite-dual-driver.md) | SQLite 双驱动：默认 modernc，可选 mattn |
| [adr/0004-no-db-file-copy.md](adr/0004-no-db-file-copy.md)       | 导入导出禁止复制 `.db` 文件             |
| [adr/0005-uuid-v7.md](adr/0005-uuid-v7.md)                       | 主键之外的 ID 统一用 UUID v7            |

## 2. CI / 覆盖率专题（既有）

这几篇比规范文档更细，专讲流水线本身：

| 文档                                 | 内容                                                     |
|--------------------------------------|----------------------------------------------------------|
| [ci-image.md](ci-image.md)           | CI 基础镜像：两份 Dockerfile、工具版本、重建流程         |
| [coverage-go.md](coverage-go.md)     | Go 覆盖率统计口径、CNB 流水线、门禁阈值状态              |
| [coverage-node.md](coverage-node.md) | 前端覆盖率：Vitest 配置、CNB 流水线、门禁阈值状态        |
| [codecov.md](codecov.md)             | Codecov 三支柱（覆盖率 / bundle / 测试结果）与归一化逻辑 |
| [coveralls.md](coveralls.md)         | Coveralls 双 flag 上报与排错                             |
| [gitlab-ci.md](gitlab-ci.md)         | GitLab 侧只做发布同步，及其历史原因                      |

## 3. 开发日志（32 篇）

`docs/2026-*.md` 是按日期命名的开发日志，从 `2026-07-26.md` 到 `2026-10-24.md`，共 32 篇：

- 内容结构不固定，常见小节是「任务目标 / 执行细节 / 验证 / 遗留」。
- 它们是**过程记录**：记录当时的判断、踩过的坑、临时结论，可能已被后续提交推翻。
- 冲突时以代码和规范文档为准；需要当前状态看 [handoff.md](handoff.md)。
- 日志只增不改，不参与 markdownfmt 之外的整理，不要为了「统一格式」重写历史结论。

## 4. 写作约定

1. **中文为主**。代码路径、标识符、命令、字段名保留原文，不加书名号。
2. **事实必须可追溯**。写某个行为时给出代码位置（`internal/xxx/yyy.go` 的符号名）或配置文件路径；代码里没有的功能不写。文档与代码冲突时，先按代码写，并在文中标注「文档/README 与代码现状不一致」。
3. **能验证才写**。覆盖率、测试数量、版本号这类会漂移的数字，注明取数时间与口径；不确定的写「未实现 / 未接线 / 待确认」，而不是猜。
4. **相对链接**。同目录用 `xxx.md`，跨目录用 `../`；新增文档要回填本索引。
5. **格式统一**。全部 Markdown 过 `markdownfmt -w docs`；表格用于枚举，编号列表用于有先后的步骤。
6. **不写敏感数据**。示例域名用 `example.com`，不抄真实 Token、AccessKey、私有域名、内网地址。
7. **一篇一个主题**。架构讲结构，指南讲操作，排障讲症状，ADR 讲取舍；同一个事实只在归属文档展开，其它文档链接过去。
8. **ADR 只追加**。已生效的决定不回头改写理由；要推翻就新写一篇并标注取代关系。
