# TODO List

> 按优先级组织。✅ = 已完成，🔄 = 进行中，❌ = 未开始。
> 最后更新：2026-06-13

---

## 0. 仓库健康 (P0 — 立即处理)

| 状态 | 任务 | 说明 |
|------|------|------|
| ❌ | 添加 `.gitattributes` 强制 LF 行尾 | 工作区 `analysis.md`、`hostkey.go`、`hostkey_test.go`、`model_config_test.go` 被改成 CRLF，导致 834 行虚假 diff |
| ❌ | 文档统一行尾为 LF | `git checkout -- <file>` 还原，或 `dos2unix` 批量转换 |

建议的 `.gitattributes`：

```ini
* text=auto eol=lf
*.go   text eol=lf
*.md   text eol=lf
*.toml text eol=lf
*.yml  text eol=lf
*.yaml text eol=lf
```

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
| ✅ | 修复 `scp/client.go::CopyFromRemotePassThru` 双重发送 panic | `scp/client.go:240-242` | 已修复 |
| ✅ | 修复 `ssh/client.go::Login` 多个错误路径未调 callback | `ssh/client.go:230,262,275,294,301` | 已修复 |

---

## 3. 代码质量 (P2)

| 状态 | 任务 | 涉及文件 | 说明 |
|------|------|---------|------|
| ❌ | `auth` 包清理：`PrivateKey`/`PrivateKeyWithPassphrase`/`SshAgent` 三个函数全无调用方 | `auth/auth.go` | 要么删除 `auth` 包，要么重构 `ssh/client.go::genSSHConfig` 使用它 |
| ❌ | `SCPConfig.KeyPath` tag `keyPath` 与 `SSHNode.KeyPath` tag `keypath` 大小写不一致 | `config/model_config.go:91-96` | 统一为 `keypath` |
| ❌ | `magefile.go::tag()` 注释说返回 git tag，实际返回 branch | `magefile.go` | 修注释或改实现 |
| ❌ | `s3/client.go` 和 `webdav/client.go` 的 `SyncConfig`/`GetConfig` 未被引用 | `s3/client.go`、`webdav/client.go`、`cmd/sync_cobra.go` | 删除或整合到 sync 命令 |
| ❌ | `S3.NewClient` / `WebDAV.NewClient` 用 `filepath.IsAbs` 校验远程路径不合理 | `s3/client.go`、`webdav/client.go` | S3 key / WebDAV 路径无"绝对"概念，改为非 `/` 开头 |
| ❌ | S3 / WebDAV 客户端硬编码 `context.Background()`，无法取消/超时 | `s3/client.go`、`webdav/client.go` | 接受外部 `context.Context` |
| ❌ | 默认 TOML 模板中 `[sync.webdav]` 用 `Auth` (大写) 与 struct tag `auth` 不一致 | `config/model_config.go:8-66` | 改为 `auth`（依赖 TOML 库大小写不敏感暂未出错）|

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

| 状态 | 任务 | 涉及包 |
|------|------|-------|
| ✅ | 单元测试：`getConfigPath` 路径解析 | `config/config_test.go` |
| ✅ | 单元测试：`SSHNode.IsStrictHostKeyChecking` | `config/model_config_test.go` |
| ✅ | 单元测试：`expandHomeDir` 9 个子用例 | `ssh/client_test.go` |
| ✅ | 单元测试：TOFU 主机指纹 8 个用例 | `ssh/hostkey_test.go` |
| ❌ | 集成测试：SCP 协议 (`ParseResponse` / `CopyN`) | `scp/` |
| ❌ | 集成测试：SCP 传输 (`CopyFile` / `CopyFromRemote`) | `scp/` |
| ❌ | 集成测试：S3 / WebDAV 客户端 | `s3/`、`webdav/` |
| ❌ | 集成测试：YML 迁移 | `cmd/` |
| ❌ | 集成测试：配置校验 (`ValidateConfig`) | `config/` |
| ❌ | 端到端测试：模拟 SSH 服务器 (go-stdlib test helpers) | 新建 |

---

## 9. 发布与运维 (P3)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | Mage 构建任务 (Build / Pack / Test / Clean) | `magefile.go` |
| ✅ | 8 平台交叉编译 + 打包 | `magefile.go::Pack` |
| ✅ | GitHub Releases 发布 | 历史记录 |
| ❌ | 自动化 CI/CD (`.github/workflows/`) | 新建 |
| ❌ | Docker 镜像 (`Dockerfile` + `docker-compose.yml` 发布版) | 新建（当前 `docker-compose.yml` 仅开发环境）|
| ❌ | Homebrew tap (macOS) | 外部仓库 |
| ❌ | Scoop bucket (Windows) | 外部仓库 |
| ❌ | `go install github.com/cnphpbb/mysshw@latest` 文档化 | `readme.md` |

---

## 10. 文档 (P3)

| 状态 | 任务 |
|------|------|
| ✅ | `readme.md` (英文) |
| ✅ | `readme.zh.md` (中文) |
| ✅ | `AGENTS.md` (AI 代理开发指南) |
| ✅ | `analysis.md` (项目深度分析) |
| ❌ | `readme.md` 与 `readme.zh.md` 功能描述与代码同步（已知 `readme.md:93` 仍提 `pkg/sftp` 但未使用）|
| ❌ | 添加架构图 (mermaid) 到 readme |
| ❌ | 添加 changelog 链接到 readme |

---

## 优先级总览

| 优先级 | 类别 | 任务数 |
|--------|------|--------|
| P0 | 仓库健康 (CRLF)、关键 Bug | 2 + 1 |
| P1 | 安全 (密码/非交互) | 5 ✅ 全部完成 2026-06-13 |
| P2 | 代码质量 + RunSSH + 配置 + Sync + UI + 测试 | 15+ |
| P3 | 发布 + 文档 | 7 |

---

## 备注

- "完成" 的定义：代码已提交到 HEAD，文档已同步。
- "进行中" 仅用于有 WIP 分支或工作区修改未提交的情况。
- 安全相关任务即使排 P1，也应在下一次发布前完成。
