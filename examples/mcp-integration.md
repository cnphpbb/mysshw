# mysshw MCP 集成指南

mysshw 通过 [Model Context Protocol](https://modelcontextprotocol.io/) (stdio transport) 暴露 SSH 操作，
让 Claude Desktop / Cursor / Cline / Zed / VS Code 等 AI 工具**原生**集成而无需任何胶水代码。

> **推荐**：所有 AI Agent 接入 mysshw 首选 MCP 模式。CLI subprocess 模式（参见 `ai-agent.md`）
> 仍可用，但 MCP 提供了更优的体验（动态发现、错误透传、流式事件）。

---

## 1. 快速开始

### 1.1 Claude Desktop

编辑 `~/.config/Claude/claude_desktop_config.json`（macOS: `~/Library/Application Support/Claude/`）：

```json
{
  "mcpServers": {
    "mysshw": {
      "command": "/usr/local/bin/mysshw",
      "args": ["mcp"]
    }
  }
}
```

重启 Claude Desktop 即可在工具栏看到 **mysshw** 服务器及其 9 个工具。

### 1.2 Cursor

编辑项目根目录的 `.cursor/mcp.json`（或全局 `~/.cursor/mcp.json`）：

```json
{
  "mcpServers": {
    "mysshw": {
      "command": "mysshw",
      "args": ["mcp"]
    }
  }
}
```

### 1.3 Cline (VS Code)

在 Cline 设置中添加：

```json
{
  "mcpServers": {
    "mysshw": {
      "command": "mysshw",
      "args": ["mcp"],
      "disabled": false
    }
  }
}
```

### 1.4 Zed

编辑 `~/.config/zed/settings.json`：

```json
{
  "context_servers": {
    "mysshw": {
      "command": "mysshw",
      "args": ["mcp"]
    }
  }
}
```

### 1.5 验证连接

```bash
# 启 MCP server 手动探测 (stdio 协议)
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | mysshw mcp
```

成功时返回 9 个 tool 的 schema 列表。

---

## 2. 暴露的 9 个工具

| Tool | Required | 用途 |
|------|----------|------|
| `ssh_list_nodes` | - | 列出已配置的 SSH 节点 |
| `ssh_exec` | `command` | 远程执行命令（支持 `--follow` 流式） |
| `ssh_copy` | `direction`, `local`, `remote` | SCP 上传/下载文件 |
| `ssh_hostkey_list` | - | 列出 `~/.ssh/known_hosts` 条目 |
| `ssh_hostkey_show` | `host` | 查看指定 host 的指纹 |
| `ssh_hostkey_remove` | `host` | 删除指定 host 的条目 |
| `ssh_hostkey_update` | `host`, `node` | 主动重取并更新 known_hosts |
| `ssh_hostkey_audit` | - | known_hosts 健康度审计 |
| `ssh_completions` | - | 动态查询 CLI 结构（供 LLM 自发现） |

### 2.1 ssh_list_nodes

```json
{
  "group": "Production",
  "search": "web",
  "cfg": "/path/to/mysshw.toml"
}
```

**返回**（数组，不含 password/passphrase 字段）：

```json
[
  {
    "group": "Production",
    "name": "prod-1",
    "alias": "Web01",
    "user": "app",
    "host": "10.0.0.1",
    "port": 22,
    "auth": "key",
    "keypath": "/root/.ssh/id_rsa",
    "proxy": "bastion"
  }
]
```

### 2.2 ssh_exec

```json
{
  "node": ["prod-1", "prod-2"],
  "command": "systemctl status nginx",
  "timeout": "30s",
  "no_pty": false,
  "dry_run": false,
  "parallel": 10,
  "rotate_known_hosts": false
}
```

**参数对照**：

| MCP 参数 | CLI flag | 备注 |
|----------|----------|------|
| `node` | `--node` (可重复) | 数组，传多个即为批量 |
| `alias` | `--alias` | 单节点精确匹配 |
| `host` | `--host` | 与 `user` / `port` 组合 |
| `user` | `-u / --user` | |
| `port` | `-p / --port` | |
| `command` | `--` 后参数 | **必填** |
| `timeout` | `--timeout` | 默认 `30s` |
| `parallel` | `--parallel` | 默认 10 |
| `no_pty` | `--no-pty` | 默认 PTY 开启 |
| `dry_run` | `--dry-run` | 危险命令前的安全预演 |
| `rotate_known_hosts` | `--rotate-known-hosts` | ⚠️ 慎用 |

**返回**（数组，每节点一条）：

```json
[
  {
    "node": "prod-1",
    "command": "uptime",
    "stdout": " 14:32:01 up 30 days, ...",
    "stderr": "",
    "exit_code": 0,
    "duration": "0.123s"
  }
]
```

### 2.3 ssh_copy

```json
{
  "node": ["prod-1", "prod-2"],
  "direction": "upload",
  "local": "/tmp/deploy.sh",
  "remote": "/opt/deploy.sh",
  "mode": "0755",
  "timeout": "5m",
  "parallel": 5
}
```

| `direction` | 含义 | 限制 |
|------------|------|------|
| `upload` | 本地 → 远端（每个节点） | 支持批量 |
| `download` | 远端 → 本地 | **仅支持单节点** |

### 2.4 ssh_hostkey_list

```json
{}
```

**返回**：
```json
[
  {
    "host": "192.168.1.211",
    "host_names": ["192.168.1.211"],
    "algorithm": "ecdsa-sha2-nistp256",
    "fingerprint": "SHA256:YVxLIeG8BeEkJ0N0r4Optb665qQyCyi3iCKer6nETiI",
    "is_hashed": false
  }
]
```

### 2.5 ssh_hostkey_show

```json
{ "host": "prod-1.example.com" }
```

### 2.6 ssh_hostkey_remove

```json
{ "host": "prod-1.example.com" }
```

### 2.7 ssh_hostkey_update

```json
{
  "host": "prod-1.example.com",
  "node": "prod-1",
  "confirm": true,
  "expect_fingerprint": "SHA256:abc...",
  "timeout": "30s"
}
```

⚠️ **默认 `confirm: false` 是 dry-run**。`expect_fingerprint` 提供 MITM 防护。

### 2.8 ssh_hostkey_audit

```json
{ "check_config": true }
```

返回 known_hosts 文件健康度、algorithm 分布、重复 host、与 config 节点覆盖度。

### 2.9 ssh_completions

```json
{ "subcommand": "hostkey", "flat": true }
```

动态查询 CLI 结构，供 LLM 在 conversation 开始时**自发现**可用工具与 flag。

---

## 3. 错误处理协议

### 3.1 MCP `isError` 标记

业务错误（如节点未找到、超时、SSH 失败）通过 MCP `ToolResult` 的 `isError: true` 字段返回，
**tool call 本身不会产生 protocol error**。

### 3.2 BSD sysexits 退出码

退出码保留并写入 stderr（虽 MCP 协议不直接透传，但 AI 客户端通常会 log）。建议的语义映射：

| 退出码 | 含义 | AI 应对 |
|--------|------|---------|
| 0 | 成功 | 继续 |
| 1 | 批量部分失败 / 命令非 0 退出 | 读 JSON `exit_code` 字段 |
| 2 | 认证失败 | 检查密钥/密码/`${ENV}` 变量 |
| 64 | 参数错误 | 检查 flag 拼写 |
| 66 | 本地文件缺失 | 检查 `local` 路径 |
| 73 | 无法创建本地文件 | 检查权限 |
| 74 | IO 错误 | 重试或检查路径 |
| 78 | 节点未找到 | 用 `ssh_list_nodes` 查可用节点 |
| 124 | 超时 | 增加 `--timeout` 或排查慢命令 |
| 255 | SSH 协议错误 | 网络或服务端问题 |

---

## 4. 安全规约（重要！）

### 4.1 敏感数据防护

| 数据 | 暴露？ | 备注 |
|------|--------|------|
| `password` | ❌ 永不 | 工具 schema 不含此字段 |
| `passphrase` | ❌ 永不 | 同上 |
| `${ENV}` 变量名 | ❌ 永不 | 如 `MYSSHW_PWD_*` |
| `host` / `user` / `port` | ✅ | 元数据，安全 |
| `keypath` | ✅ | AI 可用于审计，但不含密钥本身 |
| 命令 stdout/stderr | ✅ | 包含实际输出，AI 应最小化回传 |

### 4.2 `--dry-run` 必用场景

AI 生成破坏性命令时，**必须**先调一次 `--dry-run: true`：

- `rm -rf ...`
- `systemctl restart ...`
- `dd if=... of=...`
- `mkfs /dev/...`
- `kill -9 ...`
- `iptables ...`

`dry-run` 模式下命令**不执行**，仅返回计划。AI 审查 `nodes[]` 列表后再去掉 `dry_run` 重跑。

### 4.3 `--rotate-known-hosts` 慎用

`rotate_known_hosts: true` 等价于本次调用 `StrictHostKeyChecking=no`。
**AI 无法区分 "合法 key rotation" 与 "MITM 攻击"**。仅在 admin 明确告知后启用。

**推荐 workflow**：
1. Admin 通过安全渠道（电话、Slack DM）告知新 fingerprint
2. AI 调 `ssh_hostkey_update` 带 `expect_fingerprint`
3. 该次调用把合法新 key 写入 known_hosts
4. 后续 `ssh_exec` / `ssh_copy` 走正常严格校验

### 4.4 不推荐用法

| 反模式 | 理由 |
|--------|------|
| 让 AI 调 `mysshw`（无参数）| 进入交互 TUI，AI 无法驱动 |
| AI 直接读 `~/.mysshw.toml` | 含明文密码/密钥路径；用 `ssh_list_nodes` 间接查询 |
| AI 调用 `tunnel` 子命令 | 长驻进程；MCP 同步语义不适合；走 shell 工具 |
| 在 `command` 里拼装不受信输入 | shell 注入风险；用 `ssh_copy` 传文件 |

---

## 5. 推荐 Workflow

### 5.1 AI Agent 启动时

```
1. 调 ssh_completions (flat=true) → 了解所有可用工具
2. 调 ssh_list_nodes → 了解当前可访问节点
3. 调 ssh_hostkey_audit (check_config=true) → 健康度自检
```

### 5.2 危险操作前

```
1. ssh_exec dry_run=true → 输出计划 JSON
2. AI 审查 nodes[] 与 command
3. ssh_exec (去掉 dry_run) → 实际执行
```

### 5.3 批量健康检查

```json
{
  "name": "ssh_exec",
  "arguments": {
    "node": ["prod-1", "prod-2", "prod-3"],
    "command": "uptime && df -h / | tail -1"
  }
}
```

返回每节点结果，AI 用 `exit_code` 字段判断哪些节点有异常。

### 5.4 主机密钥轮换（合规路径）

```
1. Admin 告知 AI 新 fingerprint（"新 key 是 SHA256:abc..."）
2. AI 调 ssh_hostkey_update host=prod-1 node=prod-1
                       expect_fingerprint=SHA256:abc...
                       confirm=true
3. 验证更新成功（result 退出码 0 + JSON 输出）
4. 后续 ssh_exec 走严格校验（无需 rotate flag）
```

---

## 6. 完整 Python SDK 示例

```python
import asyncio
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

async def main():
    # 启 mysshw mcp server
    params = StdioServerParameters(command="mysshw", args=["mcp"])
    
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            
            # 列出可用工具
            tools = await session.list_tools()
            print("Available tools:", [t.name for t in tools.tools])
            
            # 列出节点
            result = await session.call_tool("ssh_list_nodes", {})
            nodes = json.loads(result.content[0].text)
            
            # 在第一个节点上跑命令
            if nodes:
                first = nodes[0]
                result = await session.call_tool("ssh_exec", {
                    "node": [first["name"]],
                    "command": "uptime"
                })
                print("uptime result:", result.content[0].text)

asyncio.run(main())
```

---

## 7. 完整 Node.js SDK 示例

```typescript
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";

const transport = new StdioClientTransport({
  command: "mysshw",
  args: ["mcp"],
});

const client = new Client({
  name: "mysshw-demo",
  version: "1.0.0",
}, { capabilities: {} });

await client.connect(transport);

// 列出工具
const { tools } = await client.listTools();
console.log("Tools:", tools.map(t => t.name));

// 调用 ssh_list_nodes
const result = await client.callTool({
  name: "ssh_list_nodes",
  arguments: { search: "prod" },
});
const nodes = JSON.parse(result.content[0].text);
console.log("Found nodes:", nodes);
```

---

## 8. 协议细节

### 8.1 协议版本

mysshw 实现了 [MCP 协议 2024-11-05](https://modelcontextprotocol.io/specification/2024-11-05)。

### 8.2 Transport

仅实现 **stdio transport**（JSON-RPC over stdin/stdout）。HTTP / SSE transport 暂未实现。

> 注意：mysshw 进程**不向 stderr 写 JSON-RPC**。MCP 客户端可以读 stderr 读日志，
> 但 stdout 必须仅含 JSON-RPC 帧。

### 8.3 工具变更通知

当前实现**静态注册** 9 个工具。运行时不会增删工具。`listChanged` 标志设为 `false`。

### 8.4 超时与取消

- `ssh_exec` 单节点命令超时由 `--timeout` 控制
- 批量 exec 整体超时受 `cmd.Context()` 控制
- MCP 客户端可发送 `notifications/cancelled` 中断进行中的 tool call，mysshw 会关闭 SSH session

---

## 9. 调试

### 9.1 查看工具列表

```bash
mysshw mcp &
MCP_PID=$!
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"d","version":"1"}}}' | nc -U /proc/$MCP_PID/fd/0
kill $MCP_PID
```

### 9.2 开启 stderr 日志

```bash
RUST_LOG=debug mysshw mcp 2> mcp.log
```

stderr 输出：
- `mysshw:: MCP server started (stdio transport)` — 启动成功
- `mysshw:: stream encode: ...` — JSON 编码错误
- 来自 cobra 的命令错误信息

### 9.3 验证 JSON schema

调 `ssh_completions` 工具（参数 `flat: true`）返回完整 CLI schema，
AI 可用它做 schema validation / diff（升级前确认工具兼容性）。

---

## 10. 适用版本

mysshw **v26.07.04.0+**（MCP server 9 个工具全部可用）。

更早版本无 MCP server，但 `list` / `exec` / `copy` 等子命令可走 CLI subprocess 模式
（参见 `ai-agent.md`）。
