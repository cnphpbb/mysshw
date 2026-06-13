# TODO List

> 按优先级组织。✅ = 已完成，🔄 = 进行中，⏸️ = 搁置，❌ = 未开始。
> 最后更新：2026-06-13 (v26.06.13.2 收尾 + AGENTS.md 新增「非 main 分支 tag 分类」规则)

---

## 0. 仓库健康 (P0 — 立即处理) ✅ 全部完成

| 状态 | 任务 | 说明 | 完成日期 |
|------|------|------|---------|
| ✅ | 添加 `.gitattributes` 强制 LF 行尾 | 覆盖 `*.go` `*.md` `*.toml` `*.yml` `Dockerfile` `LICENSE` 等 | 2026-06-13 |
| ✅ | 文档统一行尾为 LF | `sed -i 's/\r$//'` + .gitattributes 自动保护 | 2026-06-13 |
| ✅ | `analysis.md` 加入 `.gitignore` + 从索引 untrack | 本地 AI 文档不入库, 历史记录保留 | 2026-06-13 |
| ✅ | `git filter-repo` 清理已提交的二进制 (`mysshw` / `sshw/sshw`) | 11.9 MB + 7 MB, 178 commits 全重写 | 2026-06-13 |
| ✅ | AGENTS.md 新增「Tag 与 Changelog 流程」规则 | tag 必生成 `changelog.${tag}.md` 并先 commit 后 push | 2026-06-13 |
| ✅ | AGENTS.md 新增「分支与 Tag 分类」规则 | **只有 main 分支 tag 才是版本发布**, 其他分支 tag 自由命名 | 2026-06-13 |

---

## 1. 安全 (Security) — P1 ✅ 全部完成

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | 主机指纹校验 (Trust On First Use) | `ssh/hostkey.go` | 2025-06-13 |
| ✅ | per-node `strict_host_key_checking` 配置 | `config/model_config.go` | 2025-06-13 |
| ✅ | 自定义 `known_hosts_path` 配置 (与 OpenSSH 隔离) | `config/model_config.go` | 2025-06-13 |
| ✅ | 非交互场景 — `MYSSHW_SKIP_HOST_KEY_CHECK` / `MYSSHW_KNOWN_HOSTS` | `ssh/env.go`、`ssh/hostkey.go` | 2026-06-13 |
| ✅ | 密码安全：明文 → `${ENV_VAR}` 引用 | `ssh/env.go`、`ssh/client.go` | 2026-06-13 |
| ✅ | 默认模板删除弱密码占位 `$ZK7M@~1RY` | `config/model_config.go` | 2026-06-13 |
| ✅ | 配置文件落盘后 `chmod 0600` (含自动收紧旧 0644) | `config/backup_config.go` | 2026-06-13 |
| ✅ | `MYSSHW_SKIP_HOST_KEY_CHECK` 等文档化 | `readme.md`、`readme.zh.md` | 2026-06-13 |

---

## 2. 关键 Bug 修复 (P0/P1) ✅ 全部完成

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | 修复 `scp/client.go::CopyFromRemotePassThru` 双重发送 panic | `scp/client.go:240-242` | 2025 (P0 时) |
| ✅ | 修复 `ssh/client.go::Login` 多个错误路径未调 callback (Dial/NewSession) | `ssh/client.go:230,262,275,294,301` | 2025 (P0 时) |

---

## 3. 代码质量 (P2) ✅ 全部完成

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | `auth` 包清理：删除死包 (4 函数全部移除) | (包删除) | 2026-06-13 |
| ✅ | `SCPConfig.KeyPath` tag 统一为 `keypath` (与 `SSHNode` 一致) | `config/model_config.go` | 2026-06-13 |
| ✅ | `magefile.go::tag()` 重命名为 `branch()`, 修正误导性注释 | `magefile.go` | 2026-06-13 |
| ✅ | `s3/client.go` 和 `webdav/client.go` 的 `SyncConfig`/`GetConfig` 未用代码清理 | `s3/client.go`、`webdav/client.go` | 2026-06-13 |
| ✅ | `S3.NewClient` / `WebDAV.NewClient` 改用 `strings.HasPrefix(..., "/")` 校验远程路径 | `s3/client.go`、`webdav/client.go` | 2026-06-13 |
| ✅ | S3 / WebDAV 客户端接受外部 `context.Context` 参数 | `s3/client.go`、`webdav/client.go`、`cmd/sync_cobra.go` | 2026-06-13 |
| ✅ | 默认 TOML 模板 `[sync.webdav]` `Auth` → `auth` 大小写统一 | `config/model_config.go` | 2025-09 (commit 2168426) |

---

## 4. RunSSH 功能 (P2) 🟡 部分 (剩主题切换)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | 退出 SSH 会话并返回主界面 | `cmd/runssh_cmd.go` |
| ✅ | 修复 EOF 错误 | `ssh/client.go` |
| ✅ | 支持 `Ctrl+d` 退出 (Windows 下失效) | `cmd/runssh_cmd.go` |
| ✅ | 支持 `q` / `Q` 退出 | `cmd/runssh_cmd.go` |
| ✅ | 支持 `Ctrl+c` 退出 | `cmd/runssh_cmd.go` |
| ✅ | 主界面支持搜索 | `ssh/cmd.go` (huh Filter) |
| ⏸️ | 主界面支持主题 | `ssh/cmd.go` (lipgloss theme) — **搁置** |

---

## 5. 配置管理 (P2) ✅ 主要完成 (剩加密)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | 完善配置文件校验功能 | `config/validated_config.go` |
| ✅ | 支持自定义配置文件路径 (`-c` / `--cfg`) | `cmd/cobra_cmd.go` |
| ✅ | 跨平台路径支持 (Windows / Linux / macOS) | `config/utils.go` |
| ✅ | 配置文件路径校验 + 内容校验 | `config/config.go` |
| ✅ | sshw YAML 配置导入 | `cmd/yml_cmd.go` |
| ✅ | 远程备份与恢复 (SCP / WebDAV / S3) | `cmd/sync_cobra.go` |
| ✅ | 配置文件自动备份 | `config/backup_config.go` |
| ❌ | 配置文件加密选项 | `config/` (新功能) |

---

## 6. Sync Actions (P2) ✅ 全部完成

| 状态 | 任务 | 完成日期 |
|------|------|---------|
| ✅ | SCP 同步 | 2025-08 |
| ✅ | WebDAV 同步 (RustFS / MinIO / 云平台) | 2025-09-24 |
| ✅ | S3 同步 (RustFS / MinIO / 云平台 S3) | 2025-09-24 |

> **设计决定**：以下协议作者明确放弃（安全顾虑）：
> - GitHub / Gitee Gist 同步
> - API-HTTP(s) 同步接口
> - RPC 同步功能

---

## 7. 用户界面 (P2) 🟡 部分 (剩 huh 主题)

| 状态 | 任务 |
|------|------|
| ✅ | 实现命令自动补全 (cobra 默认) |
| ✅ | 用 `charmbracelet/huh` 替换 `manifoldco/promptui`（后者已停止维护）|
| ⏸️ | huh 主题定制（huh 自带 `huh.Theme*()`，可暴露配置）— **搁置** |

---

## 8. 测试 (P2) 🟡 主要完成 (剩 2 集成)

| 状态 | 任务 | 涉及包 | 完成日期 |
|------|------|-------|---------|
| ✅ | 单元测试：`getConfigPath` 路径解析 | `config/config_test.go` | 已有 |
| ✅ | 单元测试：`SSHNode.IsStrictHostKeyChecking` | `config/model_config_test.go` | 已有 |
| ✅ | 单元测试：`expandHomeDir` 9 个子用例 | `ssh/client_test.go` | 已有 |
| ✅ | 单元测试：TOFU 主机指纹 8 个用例 | `ssh/hostkey_test.go` | 已有 |
| ✅ | 单元测试：`isEnvTrue` + env 路径解析 + callback 策略 7 个用例 | `ssh/hostkey_test.go` | 2026-06-13 |
| ✅ | 单元测试：`ResolveEnvRef` 8 个用例 | `ssh/env_test.go` | 2026-06-13 |
| ✅ | 单元测试：`secureConfigFile` 4 个用例 | `config/backup_config_test.go` | 2026-06-13 |
| ✅ | 集成测试：SCP 协议 11 个用例 (`ParseResponse` / `Ack` / `CopyN` / `FileInfos`) | `scp/protocol_test.go` | 2026-06-13 |
| ✅ | 集成测试：SCP 传输 4 个用例 (mock SSH server 端到端) | `scp/client_test.go` | 2026-06-13 |
| ✅ | 集成测试：配置校验 27 个用例 (`ValidateConfig` 全分支) | `config/validated_config_test.go` | 2026-06-13 |
| ✅ | 集成测试：YML 迁移 12 个用例 (`escapeTomlString` + `convertYAMLToTOML`) | `cmd/yml_cmd_test.go` | 2026-06-13 |
| ✅ | 单元测试：s3/webdav `joinRemotePath` 8 子用例 + `minioFileInfo` | `s3/client_test.go`、`webdav/client_test.go` | 2026-06-13 |
| ✅ | 单元测试：cmd `createSSHConfig` 4 个用例 (env 引用) | `cmd/sync_cobra_test.go` | 2026-06-13 |
| ❌ | 集成测试：S3 / WebDAV 客户端 (需 mock S3/MinIO server) | `s3/`、`webdav/` | — |
| ❌ | 端到端 SSH 客户端测试 (含键盘交互认证) | 新建 | — |

---

## 9. 发布与运维 (P3) 🟡 主要完成

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | Mage 构建任务 (Build / Pack / Test / Clean) | `magefile.go` | 已有 |
| ✅ | 8 平台交叉编译 + 打包 | `magefile.go::Pack` | 已有 |
| ✅ | GitHub Releases 发布 | 历史记录 | 已有 |
| ✅ | `v26.06.13` / `v26.06.13.1` / `v26.06.13.2` tag 推送 | annotated tag + changelog | 2026-06-13 |
| ✅ | `Dockerfile` (多阶段: golang → alpine) | `Dockerfile`、`.dockerignore` (~21 MB) | 2026-06-13 |
| ⏸️ | 自动化 CI/CD (`.github/workflows/`) | — | **搁置** (非生产环境工具) |
| ⏸️ | Homebrew tap (macOS) | — | **搁置** (作者不想推) |
| ⏸️ | Scoop bucket (Windows) | — | **搁置** (作者不想推) |
| ❌ | `go install github.com/cnphpbb/mysshw@latest` 文档化 | `readme.md` | — |

---

## 10. 文档 (P3) 🟡 主要完成 (剩 changelog 链接)

| 状态 | 任务 | 完成日期 |
|------|------|---------|
| ✅ | `readme.md` (英文) | 2026-06-13 |
| ✅ | `readme.zh.md` (中文) | 2026-06-13 |
| ✅ | `AGENTS.md` (AI 代理开发指南) | 2026-06-13 (含 tag + changelog + 分支分类 规范) |
| ✅ | `analysis.md` (项目深度分析) | 2026-06-13 (已加入 .gitignore, 仅本地) |
| ✅ | `readme.md` 与 `readme.zh.md` 大纲结构统一 (15 主章节对应) | 2026-06-13 |
| ✅ | 移除文档中过时的 `pkg/sftp` 引用 + 弱密码示例 | 2026-06-13 |
| ✅ | 新增「环境变量」章节到中英文 readme | 2026-06-13 |
| ✅ | 新增「架构」章节 (mermaid 图 + 设计决策) 到中英文 readme | 2026-06-13 |
| ❌ | 添加 changelog 链接到 readme | — |

---

## 11. AGENTS.md 规范 (流程类)

| 状态 | 规则 | 涉及章节 |
|------|------|---------|
| ✅ | commit 频率 + 中文 commit 格式 | §9.7 |
| ✅ | Tag 与 Changelog 流程 (tag 前必须生成 `changelog.${tag}.md`) | §9.7 |
| ✅ | 分支与 Tag 分类 (**只有 main 分支 tag 视为版本发布**) | §9.7 |

---

## 优先级总览

| 优先级 | 类别 | 任务数 | 状态 |
|--------|------|--------|------|
| P0 | 仓库健康 (CRLF) + 关键 Bug | 6 | ✅ 全部完成 2026-06-13 |
| P1 | 安全 (密码/非交互) | 8 | ✅ 全部完成 2026-06-13 |
| P2 | 代码质量 + RunSSH + 配置 + Sync + UI + 测试 | 7 + 1 + 1 + 0 + 1 + 5 | 🟡 主要完成 (UI 主题 + 2 集成测试未做) |
| P3 | 发布 + 文档 | 5 + 1 | 🟡 主要完成 (Docker ✅, 架构图 ✅, 4 项搁置/未做) |

---

## v26.06.13.x 三阶段发布总览

```
v26.06.13   (P1)  6 个 commit   +312/-127  安全完整集
v26.06.13.1 (P2) 12 个 commit   +975/-323  代码质量 + 测试收尾
v26.06.13.2 (P3)  5 个 commit   +318/-17   Docker + 架构图 + 流程规范
─────────────────────────────────────────────────────────
累计 23 个 commit, +1605/-467 行, +110 个测试用例
```

---

## 显式搁置项 (P2/P3)

| 项 | 原因 | 决定日期 |
|---|---|---|
| Huh UI 主题切换 | 现有 huh 默认主题已够用 | 2026-06-13 |
| Homebrew tap (macOS) | 作者不想推 | 2026-06-13 |
| Scoop bucket (Windows) | 作者不想推 | 2026-06-13 |
| GitHub Actions CI/CD | 非生产环境工具 | 2026-06-13 |

---

## 备注

- "完成" 的定义：代码已提交到 HEAD，文档已同步。
- "进行中" 仅用于有 WIP 分支或工作区修改未提交的情况。
- 安全相关任务即使排 P1，也应在下一次发布前完成。
- AGENTS.md 是项目所有流程规范的唯一真相源。

