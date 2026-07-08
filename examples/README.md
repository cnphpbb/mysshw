# Examples

mysshw 与外部工具集成的示例和参考。

## 集成模式

mysshw 提供 **2 种 AI Agent 集成模式**，按推荐度从高到低：

| 模式 | 文档 | 适用场景 |
|------|------|---------|
| **MCP (Model Context Protocol)** | [`mcp-integration.md`](mcp-integration.md) | Claude Desktop / Cursor / Cline / Zed 等原生支持 MCP 的工具。**首选** |
| **CLI subprocess + JSON** | [`ai-agent.md`](ai-agent.md) | 不支持 MCP 的 Agent（如 LangChain、AutoGPT）。仍可用但需自己解析 |

## 文件

| 文件 | 用途 |
|------|------|
| [`mcp-integration.md`](mcp-integration.md) | **MCP 集成指南**（推荐）：9 个工具、4 个客户端配置、安全规约、Python/Node.js SDK |
| [`anthropic-tool-use.json`](anthropic-tool-use.json) | Claude Tool Use schema（5 个工具，已弃用，新版请用 MCP 集成） |
| [`ai-agent.md`](ai-agent.md) | CLI subprocess 模式集成指南（退码约定、Python SDK、安全规约） |

## 适用版本

- **MCP 模式**：mysshw **v26.07.04.0+**（9 个 MCP 工具）
- **CLI subprocess 模式**：mysshw v26.06.13.3+（list / exec 子命令）

## 其它平台

- **OpenAI Function Calling**: 与 Anthropic Tool Use 几乎一致；MCP 模式下 OpenAI Agent SDK 已支持 MCP
- **LangChain**: 用 `ShellTool` 包装 `subprocess.run(["mysshw", ...])`；详见 `ai-agent.md` § 6
- **其它 CLI Agent**: 任何能 fork 子进程的工具都可；关键是 `-o json` + 退码
