# Changelog v26.07.26.0

## Release Info
- **Tag**: v26.07.26.0
- **Date**: 2026-07-26
- **Commits**: 12 (含 2 个 merge + 2 个 v26.07.12.0 retroactive doc)

---

## ✨ 新功能

### feat: 修正多层代理链的 SSH 隧道堆叠顺序 (`33cfc31`)
- 修复多层 Proxy Jump 时隧道嵌套方向错误
- 跳板链按 `chain[0]->chain[1]->...->target` 顺序正确堆叠
- `ssh/proxy.go` 重构 (-51/+72)

## 🐛 修复

### fix(mcp): 修复 MCP server 偶发断开服务 (`2687ce8`) ⭐ 本次主要修复

mysshw mcp 在使用中偶发断开, AI 客户端 stdio EOF 后无响应. 三个根因:

1. **cmd/exec_cmd.go**: `ssh_exec` 远端命令非零退出 (1-255) 时直接
   `os.Exit(result.ExitCode)`, 通过 `runSubcommandCaptured` 重入 MCP 时
   会杀掉整个 server 进程 (任何 `grep`/`test`/`diff` 失败即触发, 极常见).
   改为 `return WrapExit(code, nil)`, 让 cobra 顶层 `Execute()` 处理 `os.Exit`,
   CLI 行为不变, MCP server 存活.
2. **cmd/mcp_cmd.go**: `runSubcommandCaptured` 丢弃 MCP 请求 ctx
   (`_ context.Context`), AI 客户端 cancel 时内层 SSH/cobra 不知道, 跑到自然结束.
   改为透传 ctx, 客户端 cancel 能立即中断内层 SSH.
3. **cmd/list_cmd.go**: `printNodeTable` 在 writer 为 nil 时兜底写 `os.Stdout`,
   MCP stdio 模式下 `os.Stdout` = JSON-RPC 通道, 写入污染协议流.
   改为 no-op.

附加: **cmd/mcp_cmd.go** 加 `mcpSubcommandMu sync.Mutex` 串行化并发
`runSubcommandCaptured` 调用. 函数修改全局 `rootCmd.SetArgs/SetOut/SetErr/
SetContext` 后调 `Execute()`, 并发 MCP tool call 会撕扯全局状态且 cobra
内部 (parsed flags, command tree) 非并发安全. 全局锁即可, 升级到
`exec.Command` 子进程时再重构.

## 🧪 测试

### test: 添加跳板链连接 e2e 测试和 dial 顺序回归测试 (`5ca1ad9`)
- `ssh/e2e_test.go`: 跳板链连接 e2e 测试
- `ssh/proxy_test.go`: dial 顺序回归测试
- 共 2 个新测试函数

### test(mcp): 加 MCP 偶发断开回归测试 (`9f88c7e`)
- `TestRunSubcommandCaptured_ExitErrorSurvives`: 内层 RunE 返回 ExitError
  时 `runSubcommandCaptured` 必须返回 error 而不是 `os.Exit` 杀进程
- `TestPrintNodeTable_NilWriterNoOp`: nil writer 不应兜底写 `os.Stdout`
  (重定向 os.Stdout 通过 pipe 验证)
- `TestRunSubcommandCaptured_Concurrent`: 10 goroutine 并发调 + `-race`
  验证 `mcpSubcommandMu` 正确串行化
- 共 3 个新测试函数, 全部 `-race` 兼容

## 📝 文档

### docs: 在中英文 README 中添加 ping/info/config/export/import/sync/yml/init/completions/mcp 子命令文档 (`781b378`)
- 中文 README +230 行, 英文 README +583 行

### docs: 补充密码哈希与文件级加密文档 (`c169f62`)
- `mysshw hash` 与 `mysshw encrypt` 子命令完整说明
- 中英文 README 各 +281 行

### docs: 添加跳板链中 AllowTcpForwarding 的配置说明 (`416b2ff`)
- 中英文 README 各 +14 行

### docs (retroactive): v26.07.12.0 变更日志与版本号 (`3fc9550` + `baa76f6`)
- 补充上次发布的 `changelog.v26.07.12.0.md`
- 更新 README 中的版本号引用至 v26.07.12.0

## 🏠 构建

### chore: 更新 Dockerfile 使用 alpine:latest 基础镜像 (`83c3f97`)

## 📦 元数据

### merge: 合并 gh-dev (MCP server 偶发断开修复) (`6fa144a`)
### merge: 合并 origin/gh-dev (同步远程进展) (`c78d1b7`)

---

## 📊 统计

- **12 个 commit** (含 2 个 merge, 2 个 retroactive doc)
- **+2189 / -68 行** 跨 13 文件
  - 生产代码 (cmd/ssh/config/auth/*.go): **+421 / -60**
  - 测试 (cmd/ssh/*_test.go): **+324 / -2**
  - 文档 (readme/changelog/AGENTS): **+1444 / -6**
- **5 个新测试函数** (全部 `-race` 兼容)
- **3 个生产文件修改** (MCP 修复, 共 +25/-7)
- **向后兼容**: ✅ 是
  - CLI 行为不变, 仅 stderr 多打印 `exit N` (之前是静默退出)
  - exit code 透传语义保持
  - MCP 客户端无需改动
- **推荐升级**: ✅ 强烈推荐 (修复偶发断开)