# TODO List

> 按优先级组织。✅ = 已完成，🔄 = 进行中，⏸️ = 搁置，❌ = 未开始。
> 最后更新：2026-08-15 (阶段 5 收尾: S3/WebDAV 集成测试 + webdav IsErrNotFound bug 修复 + s3 leading-slash 前置校验)
>
> **当前 main 最新 tag**: v26.07.26.0 (含 MCP 偶发断开修复 + 多层代理链 SSH 隧道堆叠顺序修正 + 跳板链 e2e 测试)

---

## 0. 仓库健康 (P0 — 立即处理) ✅ 全部完成

| 状态 | 任务 | 说明 | 完成日期 |
|------|------|------|---------|
| ✅ | 添加 `.gitattributes` 强制 LF 行尾 | 覆盖 `*.go` `*.md` `*.toml` `*.yml` `Dockerfile` `LICENSE` 等 | 2026-06-13 |
| ✅ | 文档统一行尾为 LF | `sed -i 's/\r$//'` + .gitattributes 自动保护 | 2026-06-13 |
| ✅ | `analysis.md` 加入 `.gitignore` + 从索引 untrack | 本地 AI 文档不入库, 历史记录保留 | 2026-06-13 |
| ✅ | `analysis.md` 解除 `.gitignore` + 入库 + 加 AI 辅助说明 | 改为正式项目文档 (opencode + MiniMax-M3) | 2026-06-13 |
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
| ✅ | (阶段 4 补充) 非交互 TOFU auto-trust (accept-new 语义) | `ssh/hostkey.go` | 2026-07-04 |

> 阶段 4 补充说明: AI / 脚本场景 stdin 非 TTY 或立即 EOF 时, 自动 trust 新主机并写入 known_hosts
> (OpenSSH StrictHostKeyChecking=accept-new 等价)。指纹不匹配仍拒绝 (防 MITM)。

---

## 2. 关键 Bug 修复 (P0/P1) ✅ 全部完成

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | 修复 `scp/client.go::CopyFromRemotePassThru` 双重发送 panic | `scp/client.go:240-242` | 2025 (P0 时) |
| ✅ | 修复 `ssh/client.go::Login` 多个错误路径未调 callback (Dial/NewSession) | `ssh/client.go:230,262,275,294,301` | 2025 (P0 时) |
| ✅ | (阶段 4) 修复 NonInteractive 模式丢弃明文密码 (bcrypt/空分支外的明文也被弃) | `ssh/client.go::resolvePasswordForSSH` | 2026-07-04 |

> 阶段 4 bug 修复影响所有 NewClientNonInteractive 调用路径 (Executor / Dial / Tunnel 全链路)。
> 之前客户端 auth_methods 为空 → 服务端握手 `attempted methods [none]` → 拒绝。

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
| ✅ | (阶段 4) 抽出共享 helpers (parseDuration / isSoftErr / isNodeNotFound / isCancelLike) | `cmd/helpers.go` | 2026-07-04 |
| ✅ | (阶段 4) 重构 connect() 抽出公共方法 | `ssh/client.go::connect` | 2026-07-04 |

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

## 5. 配置管理 (P2) ✅ 全部完成

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|---------|
| ✅ | 完善配置文件校验功能 | `config/validated_config.go` |
| ✅ | 支持自定义配置文件路径 (`-c` / `--cfg`) | `cmd/cobra_cmd.go` |
| ✅ | 跨平台路径支持 (Windows / Linux / macOS) | `config/utils.go` |
| ✅ | 配置文件路径校验 + 内容校验 | `config/config.go` |
| ✅ | sshw YAML 配置导入 | `cmd/yml_cmd.go` |
| ✅ | 远程备份与恢复 (SCP / WebDAV / S3) | `cmd/sync_cobra.go` |
| ✅ | 配置文件自动备份 | `config/backup_config.go` |
| ✅ | 写 TOML 时自动备份 + 可配置保留数量 (`backup_keep`, 默认 5) | `config/backup_config.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 密码 bcrypt 哈希存储 (`password = "$2a$10$..."`) | `config/password.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 密码支持多格式自动识别 (明文 / ${ENV} / bcrypt / 空) | `config/password.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 空密码 = 交互输入 (term.ReadPassword, 不写回) | `ssh/client.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 首次运行向导 (`mysshw init`, huh 驱动 4 步) | `cmd/init_cmd.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 首次运行显示 What's New + 升级时显示一次 | `cmd/init_cmd.go`、`cmd/cobra_cmd.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 密码 bcrypt-pepper 增强 (顶层 `secret_key` 字段) | `config/password.go`、`config/model_config.go`、`ssh/client.go` | 2026-07-08 (v26.07.08.0) |
| ✅ | 文件级加密 (Argon2id KDF + AES-256-GCM, `MSEC` magic 自动检测) | `config/encrypt.go`、`cmd/encrypt_cmd.go` | 2026-07-08 (v26.07.08.0) |

---

## 6. Sync Actions (P2) ✅ 全部完成

| 状态 | 任务 | 完成日期 |
|------|------|---------|
| ✅ | SCP 同步 | 2025-08 |
| ✅ | WebDAV 同步 (RustFS / MinIO / 云平台) | 2025-09-24 |
| ✅ | S3 同步 (RustFS / MinIO / 云平台 S3) | 2025-09-24 |
| ✅ | **同步端到端加密**：上传默认加密 (远程只存密文, 防远程被入侵后备份被拖走) + 下载本地解密落盘 | 2026-07-05 |

> **设计决定**：以下协议作者明确放弃（安全顾虑）：
> - GitHub / Gitee Gist 同步
> - API-HTTP(s) 同步接口
> - RPC 同步功能
>
> **同步加密说明**：`mysshw sync -u` 默认用 Argon2id + AES-256-GCM 加密后再上传，
> 远程（SCP / WebDAV / S3）永远只存密文；`mysshw sync -z` 下载后自动解密落盘（0600）。
> 口令来源：环境变量（默认 `MYSSHW_SYNC_PASSPHRASE`）> `[sync].passphrase`（支持 `${VAR}`）> 交互输入。
> 开关：`--encrypt=false` 关闭上传加密、`--keep-encrypted` 下载后本地也保留密文。
> 相关文件：`cmd/sync_crypto.go`、`cmd/sync_backend.go`、`config/model_config.go`、`webdav/client.go`、`s3/client.go`

---

## 7. 用户界面 (P2) 🟡 部分 (剩 huh 主题)

| 状态 | 任务 |
|------|------|
| ✅ | 实现命令自动补全 (cobra 默认) |
| ✅ | 用 `charmbracelet/huh` 替换 `manifoldco/promptui`（后者已停止维护） |
| ⏸️ | huh 主题定制（huh 自带 `huh.Theme*()`，可暴露配置）— **搁置** |
| ✅ | (阶段 4) AI 友好 CLI：list / exec / copy / tunnel 子命令 用 `-o json` 切换输出 (人类/机器 双格式) |

---

## 8. AI 友好 CLI (新增 P2 章节) ✅ 阶段 1+2+3+4 完成

### 8.1 阶段 1+2: SSH 层基础 (2026-07-04)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | `NodeSelector` + `ResolveNodes` 多维节点定位 (by-name 批量 / by-alias / by-host+user+port / 全部节点) | `ssh/resolver.go`, `ssh/resolver_test.go` (17 个用例) |
| ✅ | `Executor` 接口 + `executorImpl` (PTY 默认开启, 收集 stdout/stderr/exit_code/duration) | `ssh/exec.go`, `ssh/exec_test.go` (11 个用例) |
| ✅ | `BatchExecutor.RunFanOut` 多节点并发扇出 (可控 Parallel, semaphore 信号量, ctx 取消) | `ssh/exec.go` |
| ✅ | `ssh.NewClientNonInteractive` (bcrypt / 空密码直接 nil auth, 不触发 stdin) | `ssh/client.go` |
| ✅ | 抽出 `defaultClient.connect()` 公共方法 (原 Login 中的 Dial 段) | `ssh/client.go::connect` |
| ✅ | `ResolveEnvRef` 替换 bcrypt/空 env 引用 | `ssh/client.go::resolvePasswordForSSH` (新增 allowStdin 参数) |

### 8.2 阶段 1+2: CLI 子命令 (2026-07-04)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | BSD sysexits 风格退出码常量 + `ExitError` 类型 + `HandleExit`/`WrapExit` 工具 | `cmd/errors.go`, `cmd/errors_test.go` (6 用例) |
| ✅ | `WriteOut` 统一输出器 + `-o plain\|json` 全局 flag (`RegisterOutputFlag` / `GetOutputFormat`) | `cmd/output.go`, `cmd/output_test.go` (9 用例) |
| ✅ | `list` 子命令: `--group` / `--search` 过滤; JSON schema 永不含 password 字段; `auth` 字段分类 (key/password/env/bcrypt/empty) | `cmd/list_cmd.go`, `cmd/list_cmd_test.go` (10 用例) |
| ✅ | `exec` 子命令: `--node`/`--alias`/`--host`/`--user`/`--port` 定位; 默认 PTY; `--timeout`/`--parallel`; 单节点透传命令退出码; 批量任一失败 → exit 1; 全 BSD sysexits | `cmd/exec_cmd.go`, 测试覆盖 |

### 8.3 阶段 3: 文件传输 + 端口转发 + dry-run (2026-07-04)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | `copy` 子命令: `--upload`(`-U`) / `--download`(`-d`), `--local`/`--remote`/`--mode`/`--timeout`/`--parallel`; SCP via 复用 SSH dial | `cmd/copy_cmd.go`, `cmd/copy_cmd_test.go` (8 用例) |
| ✅ | `tunnel local` 子命令: `-L` 等价, `--local`/`--remote`/`--idle-timeout`, Ctrl+C 优雅退出 | `cmd/tunnel_cmd.go`(重构后), `ssh/tunnel.go` |
| ✅ | `mysshw exec --dry-run` 安全预演 (不连任何节点, JSON 输出完整 plan 含 selector/nodes/auth/pty/timeout/command) | `cmd/exec_cmd.go` |

### 8.4 阶段 4: Proxy + 反向 + SOCKS5 (2026-07-04)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | SSHNode 增加 `Proxy string` 字段 (TOML key `proxy`); 默认 client.connect() 检测 Proxy 走 `resolveProxyChain + dialViaProxyChain`; 多跳链 + 6 节点上限 + 环引用检测 | `ssh/proxy.go`, `config/model_config.go`, `ssh/proxy_test.go` (9 用例), `cmd/list_cmd.go::nodeSummary` |
| ✅ | `ReverseTunnel` 接口 (`-R` 等价): SSH server 端 listen `client.Listen("tcp", bind)`; accept 后 net.Dial TargetAddr; 双向 pipe | `ssh/reverse.go`, `ssh/reverse_test.go` |
| ✅ | `DynamicTunnel` SOCKS5 代理 (`-D` 等价): 仅支持 NO AUTH method; SOCKS5 CONNECT 命令完整握手; 支持 IPv4/Domain/IPv6 | `ssh/socks5.go`, `socks5 单元测试` (5 用例) |
| ✅ | `tunnel` 重构为 3 子命令 (`local`/`reverse`/`dynamic`); 共用信号处理 / node 解析 | `cmd/tunnel_cmd.go`(重构), `cmd/helpers.go`(抽出共享 helpers) |

### 8.5 测试 + 文档 (2026-07-04)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | 单元测试: `cmd/errors.go` (BSD sysexits 常量恒定) + `cmd/output.go` (plain/JSON 双模式) | `cmd/errors_test.go`, `cmd/output_test.go` |
| ✅ | 单元测试: `cmd/list_cmd.go` (classifyAuth 全分支 + JSON 不含 password) | `cmd/list_cmd_test.go` |
| ✅ | 单元测试: `cmd/copy_cmd.go` (CopyResult JSON + semaphore + parseDuration + flags 不冲突) | `cmd/copy_cmd_test.go` |
| ✅ | 单元测试: `ssh/proxy.go` (多跳 / 环 / 深度上限) | `ssh/proxy_test.go` |
| ✅ | 单元测试: `ssh/exec.go` (RunCommand dial-fail / ctx-cancel) | `ssh/exec_test.go` |
| ✅ | 单元测试: `ssh/reverse.go` + `ssh/socks5.go` (参数校验 + SOCKS5 帧) | `ssh/reverse_test.go` |
| ✅ | 集成测试 (e2e, build tag): list JSON schema 永不含 password; exec 单/批量; tunnel; resolver | `ssh/e2e_test.go`, `ssh/reverse_e2e_test.go` |
| ✅ | 文档同步: readme.zh.md「AI / 脚本友好模式」章节 (list/exec/copy/tunnel/dry-run/proxy) | `readme.zh.md` |

### 8.6 用户文档与集成示例 (2026-07-04)

| 状态 | 任务 | 涉及文件 |
|------|------|---------|
| ✅ | Claude Tool Use schema (4 个工具: ssh_list_nodes / ssh_exec / ssh_copy / ssh_tunnel) | `examples/anthropic-tool-use.json` |
| ✅ | AI 集成指南 + Python SDK 完整示例 + 退码约定 + 安全规约 + 集成检查表 | `examples/ai-agent.md` |
| ✅ | Examples 索引 (OpenAI / Cursor MCP / LangChain 适配指引) | `examples/README.md` |

### 8.7 Bug 修复 (阶段 4 期间)

| 状态 | 任务 | 修复 | 完成日期 |
|------|------|------|---------|
| ✅ | NonInteractive 模式丢弃明文密码 (auth_methods 空 → 服务端 `attempted methods [none]` 拒绝) | 仅让 bcrypt/空密码在 NonInteractive 路径返回 nil; 明文/${ENV} 解析后值 fall through | 2026-07-04 |
| ✅ | `listCmd` & `execCmd` 被 `cobra_cmd.go::init()` 重复 `AddCommand` (help 显示 2 次) | 删 `cobra_cmd.go` 中的重复注册, 仅保留各 cmd 文件 init() 独享注册 | 2026-07-04 |
| ✅ | `printNodeTable` header 用 `%d` 格式化端口列 → 显示 "0" (误把 int 当成 header 占位) | header 用 `%s` 显示字符串 "PORT"; 每行用 `fmt.Sprintf("%d", port)` 转字符串 | 2026-07-04 |

### 8.8 用户体验约束 / 安全规约

- [x] `list` 子命令 JSON schema 永不含 `password` / `passphrase` / `${ENV}` 变量名 (避免 AI 工具把节点列表复制到上下文时泄露)
- [x] `copy --upload` 用 `-U` 大写, 避免与 `-u/--user` 冲突
- [x] `exec --dry-run` 强制安全预演, 任何连接前都可验证计划
- [x] Proxy 跳板链: 6 节点上限, 环引用检测, 失败时已建立的 transport 全部 Close
- [x] SOCKS5 仅 NO AUTH (其他方法返回 0xFF; 密码 / GSSAPI 后续可加)

---

## 9. 测试 (P2) ✅ 全部完成

| 状态 | 任务 | 涉及包 | 完成日期 |
|------|------|-------|---------|
| ✅ | 单元测试：`getConfigPath` 路径解析 | `config/config_test.go` | 已有 |
| ✅ | 单元测试：`SSHNode.IsStrictHostKeyChecking` | `config/model_config_test.go` | 已有 |
| ✅ | 单元测试：`expandHomeDir` 9 个子用例 | `ssh/client_test.go` | 已有 |
| ✅ | 单元测试：TOFU 主机指纹 8 个用例 | `ssh/hostkey_test.go` | 已有 |
| ✅ | 单元测试：`isEnvTrue` + env 路径解析 + callback 策略 7 个用例 | `ssh/hostkey_test.go` | 2026-06-13 |
| ✅ | 单元测试：`ResolveEnvRef` 8 个用例 | `ssh/env_test.go` | 2026-06-13 |
| ✅ | 单元测试：`secureConfigFile` 4 个用例 | `config/backup_config_test.go` | 2026-06-13 |
| ✅ | 单元测试：WriteTomlWithBackup + PruneBackups 7 个用例 | `config/backup_config_test.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 单元测试：bcrypt 自动检测 / 哈希 / 验证 14 个用例 | `config/password_test.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 单元测试：ssh 密码三分支 7 个用例 | `ssh/client_password_test.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 单元测试：cmd init wizard 检测 8 个用例 | `cmd/init_cmd_test.go` | 2026-06-13 (v26.06.13.3) |
| ✅ | 集成测试：SCP 协议 11 个用例 (`ParseResponse` / `Ack` / `CopyN` / `FileInfos`) | `scp/protocol_test.go` | 2026-06-13 |
| ✅ | 集成测试：SCP 传输 4 个用例 (mock SSH server 端到端) | `scp/client_test.go` | 2026-06-13 |
| ✅ | 集成测试：配置校验 27 个用例 (`ValidateConfig` 全分支) | `config/validated_config_test.go` | 2026-06-13 |
| ✅ | 集成测试：YML 迁移 12 个用例 (`escapeTomlString` + `convertYAMLToTOML`) | `cmd/yml_cmd_test.go` | 2026-06-13 |
| ✅ | 单元测试：s3/webdav `joinRemotePath` 8 子用例 + `minioFileInfo` | `s3/client_test.go`、`webdav/client_test.go` | 2026-06-13 |
| ✅ | 单元测试：cmd `createSSHConfig` 4 个用例 (env 引用) | `cmd/sync_cobra_test.go` | 2026-06-13 |
| ✅ | 单元测试：AI 友好 CLI (BSD sysexits / WriteOut / list JSON / copy / exec) | `cmd/errors_test.go`、`cmd/output_test.go`、`cmd/list_cmd_test.go`、`cmd/copy_cmd_test.go` | **2026-07-04** |
| ✅ | 单元测试：ssh Executor / BatchExecutor / semaphore / ctx-cancel | `ssh/exec_test.go` (11 用例) | **2026-07-04** |
| ✅ | 单元测试：ssh Resolver (单跳 / 多跳 / alias / host / 模糊 / 环 / 深度) | `ssh/resolver_test.go` (17 用例) | **2026-07-04** |
| ✅ | 单元测试：ssh ProxyJump (resolveProxyChain / FindNodeByName / cycle / depth) | `ssh/proxy_test.go` (9 用例) | **2026-07-04** |
| ✅ | 单元测试：ssh ReverseTunnel + SOCKS5 DynamicTunnel 参数校验 / Close / 协议帧 | `ssh/reverse_test.go` (15 用例) | **2026-07-04** |
| ✅ | 端到端测试 (build tag e2e, gliderlabs/ssh in-process): list JSON schema / exec 单节点 / 批量 / tunnel / resolver | `ssh/e2e_test.go`、`ssh/reverse_e2e_test.go` | **2026-07-04** |
| ✅ | 集成测试：WebDAV 客户端 7 个用例 (mock server `golang.org/x/net/webdav`, 端到端 upload/download/list/delete + overwrite + not-found + connect-failure) | `webdav/client_integration_test.go` | **2026-08-15** |
| ✅ | 单元测试：S3 客户端 6 个用例 (无 server 依赖: leading-slash 校验 + minioFileInfo 全部 7 个 os.FileInfo 方法 + joinRemotePath 边缘场景 + endpoint/region 解析) | `s3/client_integration_test.go` | **2026-08-15** |

> **新增测试依赖**: `github.com/gliderlabs/ssh v0.3.8` (仅 e2e build tag 用, 默认 `go test` 不引入)
>
> **S3 测试说明**: 因 minio-go 内部使用 AWS Signature V4 签名, 手写一个完整 S3 mock server 工作量很大.
> S3 单元测试覆盖了所有不依赖网络的部分 (参数校验 / 数据结构 / 路径拼接).
> 完整 S3 集成测试建议用 docker 起 minio/garage server 跑手动 e2e (当前实现成本过高, 暂略).
> 同理 WebDAV 单元 / 集成测试都已就位.
>
> 测试运行:
> - 默认: `go test -timeout 60s ./...`
> - e2e:   `go test -tags e2e -timeout 90s ./ssh/...`
> - Windows 上 SSH server 启动有端口回收延迟, e2e 略慢

---

## 10. 发布与运维 (P3) 🟡 主要完成

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

> 当前 `go.mod` 中 module path 是 `mysshw`，不是 `github.com/cnphpbb/mysshw`，所以 `go install github.com/cnphpbb/mysshw@latest` 不能直接工作。
> 要支持这种安装方式，必须把 `module mysshw` 改为 `module github.com/cnphpbb/mysshw`，并修改所有 99 个 .go 文件中的 import 路径。
> 这是个大型重构（涉及 main.go + 全包 import），暂未执行；当前文档化的"从源码安装"用的是 `mage build`，是更可靠的安装路径。

---

## 11. 文档 (P3) ✅ 全部完成

| 状态 | 任务 | 完成日期 |
|------|------|---------|
| ✅ | `readme.md` (英文) | 2026-06-13 |
| ✅ | `readme.zh.md` (中文) | 2026-06-13 |
| ✅ | `AGENTS.md` (AI 代理开发指南) | 2026-06-13 (含 tag + changelog + 分支分类 规范) |
| ✅ | `analysis.md` (项目深度分析) | 2026-06-13 (v26.06.13.4 起已入库, 含 AI 辅助说明) |
| ✅ | `readme.md` 与 `readme.zh.md` 大纲结构统一 (15 主章节对应) | 2026-06-13 |
| ✅ | 移除文档中过时的 `pkg/sftp` 引用 + 弱密码示例 | 2026-06-13 |
| ✅ | 新增「环境变量」章节到中英文 readme | 2026-06-13 |
| ✅ | 新增「架构」章节 (mermaid 图 + 设计决策) 到中英文 readme | 2026-06-13 |
| ✅ | (阶段 4) 增补 AI / 脚本友好模式章节 (list / exec --dry-run / copy / tunnel / ProxyJump) | **2026-07-04** |
| ✅ | 更新 readme.md / readme.zh.md 的 changelog 链接指向 v26.07.26.0 (跟随最新 tag) | **2026-08-15** |

---

## 12. AGENTS.md 规范 (流程类)

| 状态 | 规则 | 涉及章节 |
|------|------|---------|
| ✅ | commit 频率 + 中文 commit 格式 | §9.7 |
| ✅ | Tag 与 Changelog 流程 (tag 前必须生成 `changelog.${tag}.md`) | §9.7 |
| ✅ | 分支与 Tag 分类 (**只有 main 分支 tag 视为版本发布**) | §9.7 |

---

## 优先级总览

### ✅ 已完成核心功能

| 优先级 | 类别 | 状态 |
|--------|------|------|
| P0 | 仓库健康 (CRLF) + 关键 Bug | ✅ 全部完成 |
| P1 | 安全 (密码/非交互) | ✅ 全部完成 |
| P2 | 代码质量 + RunSSH + 配置 + Sync + UI | ✅ 全部完成 |
| P2 | AI 友好 CLI (list/exec/copy/tunnel/hostkey/completions) | ✅ 全部完成 |
| P2 | SSH 代理链 + 端口转发 (local/reverse/SOCKS5) | ✅ 全部完成 |
| **P0** | **MCP Server (Phase 1+2)** | **✅ 完成** (ssh_list_nodes + ssh_exec) |
| P3 | 发布 + 文档 + 测试 | 🟡 主要完成 |

### 🆕 待实现新功能

> 全部完成 — 已无待实现功能 ✅

### ✅ 已完成 (v26.07.04.0+)

| 优先级 | 功能 | 涉及文件 | 完成日期 |
|--------|------|---------|----------|
| P0 | MCP Server (Phase 1+2): stdio transport + `ssh_list_nodes` + `ssh_exec` | `cmd/mcp_cmd.go` | 2026-07-06 |
| P1 | MCP Server (Phase 3): + `ssh_copy` + 5 个 `ssh_hostkey_*` + `ssh_completions` | `cmd/mcp_cmd.go` | 2026-07-06 |
| P1 | `ping` 子命令 (TCP 连通性检测, 不需 ICMP/root) | `cmd/ping_cmd.go` | 2026-07-06 |
| P2 | `info` 子命令 (hostname/OS/kernel/CPU/memory/disk/uptime) | `cmd/info_cmd.go` | 2026-07-06 |
| P1 | **bcrypt-pepper 字段级加密** (Phase B.1) | `config/password.go` + `ssh/client.go` | 2026-07-08 |
| P2 | **`mysshw hash` 工具** (gen/verify/pepper) | `cmd/hash_cmd.go` | 2026-07-08 |
| P2 | **文件级加密 Phase C** (Argon2id + AES-256-GCM) | `config/encrypt.go` + `cmd/encrypt_cmd.go` | 2026-07-08 |
| **P1** | **config 子命令 (CRUD)** (list/get/add/remove/update/diff) | `cmd/config_cmd.go` + `cmd/config_cmd_test.go` | 2026-07-09 |
| **P2** | **exec --follow/--stream** (逐行 JSON 事件流, 单节点+多节点) | `cmd/exec_cmd.go` + `ssh/exec.go` + `ssh/e2e_test.go` | 2026-07-09 |
| **P2** | **export/import** (配置可移植性, JSON/TOML 双格式) | `cmd/export_cmd.go` + `cmd/import_cmd.go` + `cmd/import_export_test.go` | 2026-07-09 |

---

## v26.07.04.0 发布总览 (待合并到 main)

```
v26.06.13   (P1)  6 个 commit   +312/-127  安全完整集
v26.06.13.1 (P2) 12 个 commit   +975/-323  代码质量 + 测试收尾
v26.06.13.2 (P3)  5 个 commit   +318/-17   Docker + 架构图 + 流程规范
v26.06.13.3 (后台) P2 收尾       配置 bcrypt + 向导 + 备份保留
─────────────────────────────────────────────────────────
v26.07.04.0 (新增 AI 友好 CLI) 13 个 commit, 29 files, +4342/-9  list/exec/copy/tunnel/proxy/reverse/socks5/dry-run + e2e + examples
─────────────────────────────────────────────────────────
累计 41+ 个 commit, +3585+ 行, +185+ 个测试用例
```

### v26.07.04.0 changelog (合并到 main 后用作正式发布)

- ✨ **AI 友好 CLI**: 新增 `list`/`exec`/`copy`/`tunnel` 子命令, 专为 AI agent / 脚本 / CI 设计
- ✨ **BSD sysexits 退出码**: 0/1/2/64/65/66/70/73/74/78/124/255 标准化
- ✨ **统一输出格式**: `-o plain|json` 全局 flag, AI 直接解析结构化结果
- ✨ **list JSON schema**: 永不含 password/passphrase/`${ENV}` 变量名, `auth` 字段仅标识类别
- ✨ **exec 单节点透传命令退出码**, 批量场景任一失败 → exit 1 + JSON `exit_code`
- ✨ **copy `-U`/`-d` 非交互 SCP**, upload batch / download single-node
- ✨ **tunnel 三子命令**: local (`-L`) / reverse (`-R`) / dynamic (`-D` SOCKS5)
- ✨ **ProxyJump 多跳链**: `SSHNode.Proxy` 字段, 6 节点上限 + 环引用检测
- ✨ **`--dry-run` 安全预演**: 任何 exec 前可打印完整计划
- ✨ **TOFU accept-new**: AI/exec 场景 stdin EOF 时自动 trust 新主机
- 🐛 **fix(ssh)**: NonInteractive 模式不再丢弃明文密码 (避免 `attempted methods [none]`)
- 📦 **新依赖**: `github.com/gliderlabs/ssh v0.3.8` (仅 e2e 测试用, build tag 隔离)
- 📦 **新依赖**: `github.com/anmitsu/go-shlex` (gliderlabs 间接)
- 📚 **AI 集成示例**: Claude Tool Use schema + Python SDK 完整示例 + 集成检查表

---

## 13. AI 动态 CLI Schema (新增 P3) ✅ 完成

| 状态 | 任务 | 涉及文件 | 完成日期 |
|------|------|---------|----------|
| ✅ | `completions` 子命令: `--flat` 展平模式 + 子命令筛选 (`completions tunnel`) | `cmd/completions_cmd.go` | 2026-07-06 |
| ✅ | 动态输出完整 CLI JSON schema (从 cobra 运行时反射) | `cmd/completions_cmd.go::buildSchema` | 2026-07-06 |
| ✅ | 包含字段: application / commands (递归) / flags (含 persistent/type/default) / exit_codes (BSD sysexits 全量) | `cmd/completions_cmd.go` | 2026-07-06 |
| ✅ | Args policy 推断 (反射 PositionalArgs 函数名 → noargs/arbitrary/exact(N)/custom) | `cmd/completions_cmd.go::inferArgsPolicy` | 2026-07-06 |
| ✅ | 敏感默认值 redact (password/passphrase/secret/token 类 flag 默认值不输出明文) | `cmd/completions_cmd.go::flagToSchema` | 2026-07-06 |
| ✅ | 单元测试 14 个用例 (schema 字段/命令发现/嵌套展开/filter/扁平化/JSON 往返/hidden flag 过滤) | `cmd/completions_cmd_test.go` | 2026-07-06 |

### 使用场景

- AI agent 运行时发现当前 mysshw 版本支持的完整 CLI 结构，无需硬编码
- 动态 tool 注册 (Tool Use schema 从二进制 query 生成)
- 版本差异检测 (比对 schema_version = "1.0" 兼容性)
- CLI 自动补全/文档生成

### JSON schema 设计要点

- **schema_version** = "1.0", 后续变更需递增
- **commands** 树递归 (rootCmd.Commands → completionsCmd.Commands → ...), 按 path 字典序排序
- **flags** 含 name/shorthand/type/default/usage/persistent, hidden flag 过滤, 敏感字段 redact
- **args_policy** 反射推断: well-known 函数 (NoArgs/ArbitraryArgs) → 字面量; 带参函数 (ExactArgs/MinimumNArgs) → 抽象名 (exact(N))
- **exit_codes** 14 个 BSD sysexits 全量, 与 `cmd/errors.go` 常量一致
- **filter** `completions <subcmd>` 路径筛选, **flat** `--flat` 单层数组

---

| 项 | 原因 | 决定日期 |
|---|---|---|
| Huh UI 主题切换 | 现有 huh 默认主题已够用 | 2026-06-13 |
| Homebrew tap (macOS) | 作者不想推 | 2026-06-13 |
| Scoop bucket (Windows) | 作者不想推 | 2026-06-13 |
| GitHub Actions CI/CD | 非生产环境工具 | 2026-06-13 |
| 配置文件加密 | bcrypt 已能防明文落盘, 加密是后续工作 | 2026-06-13 |
| SOCKS5 密码 / GSSAPI 认证 | 默认 NO AUTH 已满足 AI 场景, 加密代理后续可加 | 2026-07-04 |
| SSHNode.Jumps []string 多跳 TOML 字段 | 当前 Proxy 是单一字符串, 多跳通过链式 Proxy 实现, 公开字段后续可加 | 2026-07-04 |

---

## 备注

- "完成" 的定义：代码已提交到 HEAD，文档已同步。
- "进行中" 仅用于有 WIP 分支或工作区修改未提交的情况。
- 安全相关任务即使排 P1，也应在下一次发布前完成。
- AI 友好 CLI 阶段 1+2+3+4 总计 13 个 commit, 合并到 main 后建议打 **v26.07.04.0** tag 并生成 `changelog.v26.07.04.0.md`。
- AGENTS.md 是项目所有流程规范的唯一真相源。
