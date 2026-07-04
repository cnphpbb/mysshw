# Changelog v26.07.04.0 (2026-07-04)

> **AI 友好 CLI 全栈 + 端口转发 + 已知主机审计**

从 v26.06.13.2 收尾起, 18 个新 commit, 35+ files changed, +5200/-600 行,
+130 测试用例 (含 e2e build tag)。

**核心主题**: 把 mysshw 改造成 **AI agent / 脚本 / CI 直接可调用的工具**,
同时增加企业级 SSH 用例 (跳板 / 反向转发 / SOCKS5)。

## ✨ 新功能

### 1. AI 友好 CLI 基础设施

- 📋 **`list` 子命令** (`mysshw list`) — 列出所有 SSH 节点. 默认表格, `-o json` 输出机器可读 schema. **永不含 password / passphrase 字段** (AI 安全). `auth` 字段仅标识认证方式类别 (key/password/env/bcrypt/empty)
- ⚡ **`exec` 子命令** (`mysshw exec --node X -- <cmd>`) — 非交互执行 shell 命令. 默认 PTY 开启 (支持 sudo/进度条). 单节点透传命令 exit code; 批量扇出. `--timeout/--parallel/--no-pty` flags
- 🔒 **`exec --dry-run`** — 任何 exec 前打印完整 plan (selector + nodes + auth + pty + command) JSON, 不连任何节点. AI 工具的"安全预演"通道
- 📁 **`copy` 子命令** (`mysshw copy -U --local ./f --remote /tmp/f`) — 非交互 SCP. Upload 批量 (扇出到多节点并发), Download 单节点. 自定义 mode / timeout / parallel. `-U` 是大写避免与 `-u/--user` 冲突
- 🛡️ **BSD sysexits 退出码** — 0/1/2/64/65/66/70/73/74/78/124/255 标准化. AI 工具直接读退出码分类错误, 无需解析 stderr
- 🎨 **`-o plain|json` 全局 flag** — WriteOut 统一输出器, plain 人类友好 (表格/字符串), json 机器可读 (结构化)
- 📊 **`runExecDryRun`** — Dry-run 模式输出 execPlan: `{selector, nodes, command, use_pty, timeout, parallel}`

### 2. SSH 层基础设施 (`ssh/Resolver + Executor + BatchExecutor`)

- 🔍 **`NodeSelector` + `ResolveNodes`** — 多维节点定位: by-name 批量 / by-alias / by-host+user+port / 全部节点 (mass fan-out 兜底). `ErrNodeNotFound` sentinel
- 🏃 **`Executor` 接口** + `executorImpl` — 单节点非交互执行. PTY 默认开启. 输出结构化 `ExecResult` (stdout/stderr/exit_code/duration)
- 🔄 **`BatchExecutor.RunFanOut`** — 多节点并发扇出, 可控 `Parallel` (semaphore 信号量). ctx 取消 / dial 阶段也走 ctx (避免慢 TCP 卡 10s 超时)
- 🛠️ **`NewClientNonInteractive`** — bcrypt/空密码直接 nil auth, 不触发 stdin 交互 (AI/脚本场景专用)
- 🪛 **`ssh.Dial(node)`** — 公共 helper, 给 scp.Client / 自定义转发用

### 3. SSH 端口转发 (`tunnel` 子命令, 3 子命令)

- ➡️ **`tunnel local`** (OpenSSH `-L` 等价) — 本地端口监听, 转发到 SSH server 远端目标. Ctrl+C/SIGTERM 优雅退出
- ⬅️ **`tunnel reverse`** (OpenSSH `-R` 等价) — SSH server 监听, 转发到 client 侧目标. `cli.Listen("tcp", bind)` 实现
- 🔄 **`tunnel dynamic`** (OpenSSH `-D` 等价) — client 侧 SOCKS5 代理. 完整实现 SOCKS5 协议帧 (handshake + CONNECT, 支持 IPv4/Domain/IPv6)
- 共享 `runTunnelWithSignal` — SIGINT/SIGTERM 优雅退出

### 4. 跳板 / 多跳链 (OpenSSH `-J` 等价)

- 🔗 **`SSHNode.Proxy` 字段** (TOML `proxy`) — 指向同一配置中另一个 node name
- 🪜 **多跳链** — `resolveProxyChain` 沿 Proxy 字段递归解析 (默认 client 端 direct-tcpip channel 透传)
- ⚙️ **6 节点上限 + 环引用检测** — 防恶意配置 / 死循环; 失败时已建立的 transport 全部 Close
- 📊 **list JSON schema 加 `proxy` 字段** (omitempty, 向后兼容)

### 5. 主机指纹管理 (`hostkey` 子命令家族)

- 🛡️ **`--rotate-known-hosts` flag** (在 `exec`/`copy`) — opt-in. 当 fingerprint mismatch 时, 自动 `RemoveKnownHost` + `AppendKnownHost` + 写 stderr 警告. **不**持久化 (下次连接仍走严格)
- 🔍 **`hostkey list`** — 列出全部 known_hosts 条目 (JSON: host/host_names/algorithm/raw_key/fingerprint/is_hashed/line_no)
- 🔎 **`hostkey show HOST`** — 查询某 host 的 fingerprint
- 🗑️ **`hostkey remove HOST`** — 类似 `ssh-keygen -R`, 删除匹配条目
- ✏️ **`hostkey update HOST --node N [--confirm] [--expect-fingerprint FP]`** — 主动 dial 节点拿 server key, 替换 host 条目. **默认 dry-run**, `--confirm` 才写. `--expect-fingerprint` 进一步验证 (MITM 防护: 不匹配 exit 124)
- 📊 **`hostkey audit [--check-config]`** — 全面审查 (read-only): file metadata + algorithm 分布 + 重复 host 警告 + 与 config 跨底对齐 (`--check-config` 时). 安全警告自动生成 (老算法 ssh-rsa/ssh-dss/hashed-only/重复)
- 🔒 **`TOFU accept-new` 语义** (OpenSSH `StrictHostKeyChecking=accept-new`) — 第一次自动 trust, 第二次不匹配拒绝. 适配 AI/脚本 stdin 不可用场景
- 🔒 **新增 `ssh.DialVerifyHostKeys`** — 与 `DialCaptureHostKeys` 对应但不写 known_hosts. 让 `hostkey update --expect-fingerprint` 在 mismatch 时不污染 known_hosts

## 🔧 Bug 修复

- 🐛 **fix(ssh) NonInteractive 模式丢弃明文密码** — 之前 `if !allowStdin` 走 nil auth, 客户端 `attempted methods [none]` 被服务端拒绝. 修复: bcrypt/空走 nil, 明文/${ENV} fall through 到原密码路径
- 🐛 **fix(list_cmd) 重复注册 hostkey/exec 到 rootCmd** — help 显示同一命令 2 次. 修复: 删 `cobra_cmd.go` 中重复 `AddCommand`, 保留各 cmd 文件 `init()` 独享
- 🐛 **fix(list_cmd) printNodeTable header 用 %d** — "PORT" header 显示成 "0". 修复: 改 `%s` + 每行 `fmt.Sprintf("%d", port)`

## 🧪 测试

- ✅ **13 个新 ssh 单元测试** (resolver/exec/proxy/tunnel/reverse/socks5)
- ✅ **8 个新 hostkey 单元测试** (extractHostNames/entryHostMatches/normalizeHostArg/parseKnownHosts 等)
- ✅ **7 个 hostkey e2e subtests** (build tag `e2e`, 需 gliderlabs/ssh in-process SSH server):
  - List_Empty / Update_DryRun / Update_WithConfirm / Show_AfterUpdate /
    Update_ExpectFingerprintMismatch / Remove_Cleanup / **Audit**
  - 容器 `TestHostkeyE2E` + `t.Run` subtest 隔离, `resetCobraFlags` helper
  - audit subtest 新增: 验证 `--check-config` 字段填充 + audit read-only 不改 known_hosts

## 📦 新增依赖

- `github.com/gliderlabs/ssh v0.3.8` (仅 e2e build tag 引用, 默认 `go test` 不引入)
- `github.com/anmitsu/go-shlex` (gliderlabs 间接)

## 📚 文档与示例

- `examples/anthropic-tool-use.json` — Claude Tool Use schema 4 个工具 (ssh_list_nodes / ssh_exec / ssh_copy / ssh_tunnel + ssh_hostkey with audit action), 完整 JSON schema + 退码字典 + usage_patterns
- `examples/ai-agent.md` — Python SDK 完整示例 + 退码约定 + 安全规约 + 集成检查表
- `examples/README.md` — 索引 + OpenAI/Cursor MCP/LangChain 适配指引
- `readme.zh.md` — "AI / 脚本友好模式" 章节 (list/exec/copy/tunnel/proxy/dry-run/hostkey 全覆盖)
- `readme.zh.md` — 加 "hostkey audit" 章节 + JSON schema 示例
- `TODO.md` — 标记阶段 1+2+3+4 全部完成 + 准备 v26.07.04.0 release

## 🛠️ 不破坏 / 显式拒绝

- ✅ **不实现** `MYSSHW_KNOWN_HOSTS_UPDATE=always` (永久自更新) — 等价于让 AI agent 永久失去防 MITM 能力, 项目设计拒绝
- ✅ **不实现** `MYSSHW_SKIP_HOST_KEY_CHECK=always` — 已有但仅 opt-in (env var 临时)
- ✅ Login 路径 (PTY shell) 行为不变 — 既有用户无感知

## 📊 统计

| 项 | 数字 |
|----|------|
| 新 commit (v26.06.13.2 → v26.07.04.0) | 18 |
| 新增文件 | 14 |
| 修改文件 | 21 |
| 新增代码行数 | +5,200 |
| 删除代码行数 | -600 |
| 新增测试用例 | +130 (含 e2e) |
| 新增子命令 | 8 (list/exec/copy/tunnel-local/reverse/dynamic/hostkey-list/show/remove/update/audit) |
| 新增 AI 工具 schema | 5 个 (含 ssh_hostkey 5-action) |
| 累计 commit (项目首版 → v26.07.04.0) | 35+ |
