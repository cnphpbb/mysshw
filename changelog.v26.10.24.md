# Changelog v26.10.24

## Release Info
- **Tag**: v26.10.24
- **Date**: 2026-10-24
- **Previous**: v26.07.26.0
- **Commits**: 27 (含 4 个 merge)
- **Tag 指向**: `160ecd9` (Merge branch 'gh-dev' into main) — 本 changelog 文件的提交 (`docs: 添加 v26.10.24 changelog`) 按 AGENTS.md 流程排在 tag 之后

> 本次发布的两个核心变化：**配置文件的远程备份改为端到端加密**（远程服务器被入侵也拿不到明文配置），以及**本地配置关键项体检**（启动即检查关键配置是否配置完整）。

---

## ✨ 新功能

### feat: sync 端到端加密 — 上传加密存放 / 下载解密落盘 (`f1a45a3`) ⭐ 本次主要特性

防远程备份服务器被入侵后备份文件被拖走的风险：**远程永远只存密文，口令只留本地**。

- 上传默认加密（Argon2id + AES-256-GCM），远程只存密文
- 下载本地解密落盘（0600 权限），`--keep-encrypted` 可保留密文
- 口令解析优先级：env（默认 `MYSSHW_SYNC_PASSPHRASE`）> `[sync].passphrase` > 交互输入
- 上传前 **fail-fast 预检加密参数**：加密开启但无口令且非 TTY（cron/CI/MCP）→ 直接报错退出，不挂起等输入
- 上传时口令二次确认；口令不落盘 / 不进日志 / 不进命令行参数
- 幂等：本地已是密文（`MSEC` magic）不做双重加密
- 兼容旧版明文备份（原样落地 + 告警）
- 解密失败不覆盖本地已有配置（SCP 走临时文件）
- 抽出 `SyncBackend` 字节级统一接口，覆盖 WebDAV / S3
- 新增配置字段：`[sync].encrypt` (bool) + `[sync].passphrase` (string)
- `${VAR}` / `${VAR:-default}` 引用解析下沉到 `config` 包

### feat: 本地配置关键项体检 — 启动自动检查 + `config check` 子命令 (`2ae4f52`)

- 新增 `config/config_check.go`：`CheckConfigItems` 返回分级问题（error / warn / info）
  - 节点：凭据缺失 / 明文密码 / name+alias 重复 / 代理引用不存在
  - SSH：`strict_host_key_checking=false` / 端口非法 / 密钥文件不存在
  - sync：协议必填项 / `encrypt` 开关 / `passphrase` 是否可解析
  - 加密：`encryption_algorithm` 与 `secret_key` 配对 / `cipher` 合法性
  - 文件：配置目录与配置文件权限（建议 0600）
- **启动自动体检**：`LoadViperConfig` 加载后自动跑，只报 warn/error，输出走 **stderr**
  （`list -o json` 与 MCP 的 stdout 保持纯 JSON）
- 新增 `mysshw config check` 子命令：完整报告（含 info），支持 `-o json` 供 AI agent 消费
  - 只读解析（`ReadConfigOrEmpty`），检查时不修改用户配置
- 硬校验仍由 `ValidateConfig` 负责，体检只告警不阻断

## 🐛 修复

### fix(mcp): 修裸 string node 触发 mass fan-out + 节点 panic 兜底 + 错因暴露 (`4f931e6`)
- 裸 string 类型的 node 参数被误解析成批量目标，导致对**所有**节点并发下发（mass fan-out）
- 节点级 panic 兜底，错误信息带出真实原因而非空泛失败

### fix(mcp): 修复 exec 状态泄漏 + panic 兜底，exec 在 MCP 上下文跑得通 (`399ceeb`)
- exec 工具执行后状态未正确清理，导致后续调用行为异常
- handler panic 兜底，进程不再因单次调用异常被杀

### fix(mcp): 启用 WithRecovery，handler panic 不再杀进程断连 (`28c9884`)
- 开启 MCP server 的 panic recovery，单个 tool call 崩溃不再导致整个 server 断连

### fix(mcp): genSSHConfig 诊断输出走 stderr + keyboard-interactive 守卫 (`4b9b6fe`) ⭐
- 历史 bug：`${ENV}` 密码节点每次连接都往 **stdout** 写一行诊断信息
  （`password resolved from env var`），MCP stdio 模式下 stdout = JSON-RPC 通道
  → 协议流被污染 → AI 客户端解析失败断开。表现为「偶发断开」
- 修复：所有诊断输出改走 `os.Stderr`；补 keyboard-interactive 守卫
- 回归测试 `TestGenSSHConfig_NoStdoutPollution` 锁死该行为

### fix(config): 所有 config 加载期的 fmt.Printf 改走 stderr，防 JSON-RPC 污染 (`e95e82d`)
- 配置加载过程中的调试输出统一改走 stderr，避免污染 MCP 的 stdout 协议流

### fix(copy): 暴露首条失败节点 + error 信息，跟 exec 错误格式一致 (`381a532`)
- copy 批量传输失败时不返回任何错误信息，改为暴露首个失败节点及原因

### fix: import 节点不写入文件 + export TOML 顶层数组支持 (`ef870c7`)
- `cmd/import_cmd.go::applyImportNode` 丢弃了 `configWithNodeAdded` 返回的**新** `*Configs`
  → `mysshw import --confirm` 报告 "imported: +1 added" 但节点**根本没写入文件**
- `cmd/export_cmd.go` 直接编码 `[]portableNode` 触发
  `toml: top-level values must be Go maps or structs` (exit 74)
  → 用 `exportTOMLWrapper{Nodes: nodes}` 包装，输出 `nodes = [...]` 格式

### fix(webdav): 修复远程路径不存在判断逻辑并补充集成测试 (`def17b2`)
- `os.IsNotExist` 对 WebDAV 库的 `*os.PathError{StatusError{404}}` **恒为 false**
  → 「远程文件不存在」被误报成「检查远程文件失败」
- 改用 `gowebdav.IsErrNotFound`，并补 `golang.org/x/net/webdav` 内存 server 集成测试
  （覆盖 NewClient / UploadBytes / DownloadBytes / ListFiles / DeleteFile 全流程）

## 🔧 重构

### refactor: 抽取 SyncBackend / PrintTable / ClassifyAuth 共享组件 (`b0ff1b0`)
- 消除 3 处典型复制粘贴，三处共用逻辑统一收敛到共享组件

### refactor: 修 runssh stdin 死代码 + getConfigPath 去副作用 (`17d853e`)
- `cmd/runssh_cmd.go` 删除 `select { time.After + default: ReadRune }` 死代码
  （`time.After` 的 channel 永远不会被接收，`default` 总是立即触发；
  `bufio.NewReader(os.Stdin)` 与 huh 抢 stdin 导致 huh 读到脏数据）
- `config/utils.go` 的 `getConfigPath` 改为**纯函数**，去除 `CFG_PATH` 全局副作用
- `cmd/init_cmd.go::promptPwdFromStdin` 改用 `term.ReadPassword` 关闭回显
  （之前用 `fmt.Scanln`，密码逐字回显到终端）

### refactor: 删除历史死代码 + Nodes.Groups 加 mapstructure tag (`d5e8265`)

### refactor(s3): 将远程路径校验前置到网络调用之前 (`106515f`)

### style: gofmt 清理历史格式问题 (`690c270`)
- `scp/client_test.go` 删除文件末尾多余空行
- `ssh/client_test.go` 修复 struct 测试用例字段对齐
- `ssh/env.go` doc 注释代码示例改为 gofmt 标准缩进

## 🧪 测试

### test: 修 19 个 import/export 测试路由 bug + 清理 2 个 vet 警告 (`d61011c`)
- `runExportCmd` / `runImportCmd` 误走 `runConfigCmd`（后者会前置 `config` 子命令）
  → 实际执行 `mysshw -c '' config -c <path> export` 而非期望的 `mysshw -c <path> export`
  → 新增 `runMySshwCmd` 通用 helper，**17 个测试从失败变通过**
- `TestExportImport_Lifecycle` 多重前缀 → 改用 helper 的 `cfgPath` 参数
- `TestGetCfgPath` 错误断言（原以为 `~` 不展开）→ 重写为 4 个正确用例
- 清理 2 个 vet 警告（`assert.NotNil` 按值传参复制 `sync.Once`；`fp = fp` 自赋值死代码）
- 累计：**pre-existing 测试失败 20 → 0，vet 警告 2 → 0**

### test(mcp): 加 parallel 端到端回归测试，锁住 BatchExecutor 并发限制 (`2d346ec`)

### test(mcp): 强化 TestRunExecBatch_RecoversPanic，验证 goroutine 不死锁不 panic (`29a684f`)

### 新增 21 个测试文件 / 127 个测试函数
- `cmd/sync_crypto_test.go` / `cmd/sync_check_test.go` / `cmd/sync_backend_test.go` / `cmd/sync_webdav_e2e_test.go`
- `cmd/config_check_cmd_test.go` / `cmd/mcp_parallel_e2e_test.go` / `cmd/table_test.go`
- `config/config_check_test.go` / `config/sync_crypto_test.go` / `config/auth_classify_test.go`
- `s3/client_integration_test.go` / `webdav/client_integration_test.go`

### 全量测试
- `go build ./...` ✅ · `go vet ./...` ✅ 无警告 · `go test ./...` **700 passed**（8 个包）
- `gofmt -l .` 干净

## 📝 文档

### docs: AGENTS.md 明确双分支工作流（gh-dev 开发 / main 发布） (`7d0559d`)
- 原 AGENTS.md 只在「Tag 与 Changelog 流程」「分支与 Tag 分类」里提到 `gh-dev`，
  且只描述打 tag 的规则，**没有说明日常开发在哪个分支进行**
- 新增 §9.7「分支工作流」：`gh-dev` = 开发分支，`main` = 发布分支
- 明确日常开发 / 发布到 main / 反向同步 main→gh-dev 三套命令
- 列出禁止行为（main 上直接开发、gh-dev 上打 `v*` tag、未合入就推 main）
- 提示 main↔gh-dev 双向合并的常见冲突点与验证要求

### docs: AGENTS.md 修正 gh-dev 拉取方式（禁用 --rebase，避免虚假冲突） (`417be6b`)
- `gh-dev` 历史含 merge commit，`--rebase` 会线性化并重放已合并过的提交
  → 产生**虚假冲突**（明明已是最新，却报 `ssh/client_test.go` 冲突并进入 detached HEAD）
- 一律用 `git pull --no-rebase`

### docs: 同步 analysis.md / 更新 README 版本号引用 (`643744c` / `bdbd2a1` / `8fba912`)

## 📦 元数据

### merge: 合并 gh-dev（sync 端到端加密 + 配置关键项体检）到 main (`160ecd9`)
### merge: 合并 main 到 gh-dev (`03ed907`)
### merge: 同步远程进展 (`f19fe11` / `d61011c` 之前的合并)

---

## 📊 统计

- **27 个 commit**（含 4 个 merge）
- **+5805 / -954 行** 跨 61 文件
  - 生产代码：**+1994 / -676**（34 文件）
  - 测试：**+2752 / -78**（21 文件）
  - 文档：**+1059 / -200**（6 文件）
- **127 个新测试函数** / 21 个新测试文件
- **全量测试 700 passed**，vet 0 警告，gofmt 干净
- **向后兼容**：✅ 是
  - CLI 既有参数与退出码语义不变
  - sync 上传默认行为变化（默认加密），可用 `--encrypt=false` 或 `[sync].encrypt=false` 关闭
  - MCP 客户端无需改动（stdout 协议流反而更干净）
- **推荐升级**：✅ 强烈推荐（远程备份安全 + 配置体检 + MCP 断连根治）
