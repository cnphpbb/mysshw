# TODO List

> 按优先级组织。✅ = 已完成，🔄 = 进行中，❌ = 未开始。
> 最后更新：2026-06-13 (readme 结构同步 + 仓库健康收尾)

---

## 0. 仓库健康 (P0 — 立即处理)

| 状态 | 任务 | 说明 | 完成日期 |
|------|------|------|---------|
| ✅ | 添加 `.gitattributes` 强制 LF 行尾 | 覆盖 `*.go` `*.md` `*.toml` `*.yml` `Dockerfile` `LICENSE` 等 | 2026-06-13 |
| ✅ | 文档统一行尾为 LF | `sed -i 's/\r$//'` + .gitattributes 自动保护 | 2026-06-13 |
| ✅ | `analysis.md` 加入 `.gitignore` + 从索引 untrack | 本地 AI 文档不入库, 历史记录保留 | 2026-06-13 |
| ✅ | `git filter-repo` 清理已提交的二进制 (`mysshw` / `sshw/sshw`) | 11.9 MB + 7 MB, 178 commits 全重写 | 2026-06-13 |

---

## 1. 安全 (Security) — P1

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | 主机指纹校验 (Trust On First Use) | `ssh/hostkey.go` | 2025-06-13 |
| ✅ | per-node `strict_host_key_checking` 配置 | `config/model_config.go` | 2025-06-13 |
| ✅ | 自定义 `known_hosts_path` 配置 (与 OpenSSH 隔离) | `config/model_config.go` | 2025-06-13 |
| ✅ | 非交互场景 (CI/脚本) 支持 — `MYSSHW_SKIP_HOST_KEY_CHECK` / `MYSSHW_KNOWN_HOSTS` | `ssh/env.go`、`ssh/hostkey.go` | 2026-06-13 |
| ✅ | 密码安全：明文 → `${ENV_VAR}` 引用 | `ssh/env.go`、`ssh/client.go` | 2026-06-13 |
| ✅ | 默认模板删除弱密码占位 `$ZK7M@~1RY` | `config/model_config.go` | 2026-06-13 |
| ✅ | 配置文件落盘后 `chmod 0600` | `config/backup_config.go` | 2026-06-13 |
| ✅ | `MYSSHW_SKIP_HOST_KEY_CHECK` 等安全相关环境变量文档化 | `readme.md`、`readme.zh.md` | 2026-06-13 |

---

## 2. 关键 Bug 修复 (P0/P1)

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | 修复 `scp/client.go::CopyFromRemotePassThru` 双重发送 panic | `scp/client.go:240-242` | 已修复 (2026-06-13) |
| ✅ | 修复 `ssh/client.go::Login` 多个错误路径未调 callback | `ssh/client.go:230,262,275,294,301` | 已修复 (2026-06-13) |

---

## 3. 代码质量 (P2)

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | `auth` 包清理：删除死包，4 个函数全部移除 | (包删除) | 2026-06-13 |
| ✅ | `SCPConfig.KeyPath` tag 统一为 `keypath` (与 `SSHNode` 一致) | `config/model_config.go` | 2026-06-13 |
| ✅ | `magefile.go::tag()` 重命名为 `branch()`, 修正注释 | `magefile.go` | 2026-06-13 |
| ✅ | `s3/client.go` 和 `webdav/client.go` 的 `SyncConfig`/`GetConfig` 未用代码清理 | `s3/client.go`、`webdav/client.go` | 2026-06-13 |
| ✅ | `S3.NewClient` / `WebDAV.NewClient` 改用 `strings.HasPrefix(..., "/")` 校验远程路径 | `s3/client.go`、`webdav/client.go` | 2026-06-13 |
| ✅ | S3 / WebDAV 客户端接受外部 `context.Context` 参数 | `s3/client.go`、`webdav/client.go`、`cmd/sync_cobra.go` | 2026-06-13 |
| ✅ | 默认 TOML 模板中 `[sync.webdav]` `Auth` 改为 `auth` (大小写统一) | `config/model_config.go` | 2025-09 (commit 2168426) |

---

## 4. RunSSH 功能 (P2)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | 退出 SSH 会话并返回主界面 | `cmd/runssh_cmd.go` |
| ✅ | 修复 EOF 错误 | `ssh/client.go` |
| ✅ | 支持 `Ctrl+d` 退出 (Windows 下失效) | `cmd/runssh_cmd.go` |
| ✅ | 支持 `q` / `Q` 退出 | `cmd/runssh_cmd.go` |
| ✅ | 支持 `Ctrl+c` 退出 | `cmd/runssh_cmd.go` |
| ✅ | 主界面支持搜索 | `ssh/cmd.go` (huh Filter) |
| ❌ | 主界面支持主题 | `ssh/cmd.go` (lipgloss theme) |

---

## 5. 配置管理 (P2)

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

## 6. Sync Actions (P2)

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

## 7. 用户界面 (P2)

| 状态 | 任务 |
|------|------|
| ✅ | 实现命令自动补全 (cobra 默认) |
| ✅ | 用 `charmbracelet/huh` 替换 `manifoldco/promptui`（后者已停止维护）|
| ❌ | huh 主题定制（huh 自带 `huh.Theme*()`，可暴露配置） |

---

## 8. 测试 (P2)

| 状态 | 任务 | 涉及包 | 完成日期 |
|------|------|-------|---------|
| ✅ | 单元测试：`getConfigPath` 路径解析 | `config/config_test.go` | 已有 |
| ✅ | 单元测试：`SSHNode.IsStrictHostKeyChecking` | `config/model_config_test.go` | 已有 |
| ✅ | 单元测试：`expandHomeDir` 9 个子用例 | `ssh/client_test.go` | 已有 |
| ✅ | 单元测试：TOFU 主机指纹 8 个用例 | `ssh/hostkey_test.go` | 已有 |
| ✅ | 单元测试：`isEnvTrue` + env 路径解析 + callback 策略 7 个用例 | `ssh/hostkey_test.go` | 2026-06-13 |
| ✅ | 单元测试：`resolveEnvRef` 8 个用例 | `ssh/env_test.go` | 2026-06-13 |
| ✅ | 单元测试：`secureConfigFile` 4 个用例 | `config/backup_config_test.go` | 2026-06-13 |
| ✅ | 集成测试：SCP 协议 11 个用例 (`ParseResponse` / `Ack` / `CopyN` / `FileInfos`) | `scp/protocol_test.go` | 2026-06-13 |
| ✅ | 集成测试：SCP 传输 4 个用例 (mock SSH server 端到端) | `scp/client_test.go` | 2026-06-13 |
| ✅ | 集成测试：配置校验 27 个用例 (`ValidateConfig` 全分支) | `config/validated_config_test.go` | 2026-06-13 |
| ✅ | 集成测试：YML 迁移 12 个用例 (`escapeTomlString` + `convertYAMLToTOML`) | `cmd/yml_cmd_test.go` | 2026-06-13 |
| ✅ | 单元测试：s3/webdav `joinRemotePath` 4 子用例 + `minioFileInfo` | `s3/client_test.go`、`webdav/client_test.go` | 2026-06-13 |
| ✅ | 单元测试：cmd `createSSHConfig` 4 个用例 (env 引用) | `cmd/sync_cobra_test.go` | 2026-06-13 |
| ❌ | 集成测试：S3 / WebDAV 客户端 (需 mock S3/MinIO server) | `s3/`、`webdav/` | — |
| ❌ | 端到端测试：完整 SSH 客户端连接流程 (含键盘交互认证) | 新建 | — |

---

## 9. 发布与运维 (P3)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | Mage 构建任务 (Build / Pack / Test / Clean) | `magefile.go` |
| ✅ | 8 平台交叉编译 + 打包 | `magefile.go::Pack` |
| ✅ | GitHub Releases 发布 | 历史记录 |
| ✅ | `v26.06.13` tag 创建并推送 | annotated tag, 包含完整 changelog |
| ❌ | 自动化 CI/CD (`.github/workflows/`) | 新建 |
| ❌ | Docker 镜像 (`Dockerfile` + `docker-compose.yml` 发布版) | 新建（当前 `docker-compose.yml` 仅开发环境）|
| ❌ | Homebrew tap (macOS) | 外部仓库 |
| ❌ | Scoop bucket (Windows) | 外部仓库 |
| ❌ | `go install github.com/cnphpbb/mysshw@latest` 文档化 | `readme.md` |

---

## 10. 文档 (P3)

| 状态 | 任务 | 完成日期 |
|------|------|---------|
| ✅ | `readme.md` (英文) | 2026-06-13 |
| ✅ | `readme.zh.md` (中文) | 2026-06-13 |
| ✅ | `AGENTS.md` (AI 代理开发指南) | 已有 |
| ✅ | `analysis.md` (项目深度分析) | 2026-06-13 (已加入 .gitignore, 仅本地) |
| ✅ | `readme.md` 与 `readme.zh.md` 大纲结构统一 (14 主章节对应) | 2026-06-13 |
| ✅ | 移除文档中过时的 `pkg/sftp` 引用 + 弱密码示例 | 2026-06-13 |
| ✅ | 新增「环境变量」章节到中英文 readme | 2026-06-13 |
| ❌ | 添加架构图 (mermaid) 到 readme | — |
| ❌ | 添加 changelog 链接到 readme | — |

---

## 优先级总览

| 优先级 | 类别 | 任务数 | 状态 |
|--------|------|--------|------|
| P0 | 仓库健康 (CRLF) + 关键 Bug | 4 + 2 | ✅ 全部完成 2026-06-13 |
| P1 | 安全 (密码/非交互) | 8 | ✅ 全部完成 2026-06-13 |
| P2 | 代码质量 + RunSSH + 配置 + Sync + UI + 测试 | 7 + 1 + 1 + 0 + 1 + 6 | 🟡 主要完成 (剩 UI 主题 + S3/WebDAV 集成测试 + 端到端 SSH) |
| P3 | 发布 + 文档 | 4 + 2 | 🔄 部分 (Docker/CI/Homebrew/Scoop/架构图 未做) |

---

## v26.06.13.1 发布总结 (P2 收尾)

✅ **P0/P1 全部完成** (4 项 P0 + 8 项 P1 + 2 项关键 Bug)
✅ **P2 代码质量全部完成** (7/7 项: auth 删除、tag 统一、magefile 重命名、SyncConfig 删除、path 校验修正、context 传播、webdav tag)
✅ **P2 测试大幅推进** (5/7 项: SCP 协议 11 + SCP 传输 4 + 配置校验 27 + YML 12 + s3/webdav/cmd 杂项)
🟡 **P2 未完成**: UI 主题、s3/webdav 集成测试 (需 mock server)、端到端 SSH 客户端测试
🔴 **P3 多数待做** (6/9 项)

代码质量改进统计:
- 删除死代码: ~150 行 (auth 包 + s3/webdav SyncConfig/GetConfig)
- API 增强: s3/webdav 接受 context.Context
- API 一致性: SCPConfig.KeyPath tag 统一
- 测试覆盖: +78 个测试用例

测试覆盖率 (截至本次):
- scp/protocol.go: 100% (11/11 公开函数)
- scp/client.go: ~60% (4 集成测试覆盖主要路径)
- config/validated_config.go: 100% (27 用例)
- cmd/yml_cmd.go: ~80% (12 用例)
- cmd/sync_cobra.go: ~70% (4 用例)

下次发布可考虑:
- 完成 P3 任务: CI/CD、Docker 镜像、Homebrew/Scoop
- 添加 s3/webdav 集成测试 (需 mock S3/MinIO server, 类似 scp 的 mock SSH server)
- 端到端 SSH 客户端测试 (含键盘交互认证流程)

---

## 备注

- "完成" 的定义：代码已提交到 HEAD，文档已同步。
- "进行中" 仅用于有 WIP 分支或工作区修改未提交的情况。
- 安全相关任务即使排 P1，也应在下一次发布前完成。
