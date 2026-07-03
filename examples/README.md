# Examples

本目录是 mysshw 与外部工具集成的示例和参考。

## 文件

| 文件 | 用途 |
|------|------|
| [`anthropic-tool-use.json`](anthropic-tool-use.json) | Claude Tool Use schema，4 个工具：`ssh_list_nodes` / `ssh_exec` / `ssh_copy` / `ssh_tunnel`。可直接加载到 Anthropic SDK |
| [`ai-agent.md`](ai-agent.md) | 通用 AI Agent 集成指南（Claude / Cursor / Cline / Aider 等），含 Python 完整示例、退码约定、安全规约 |

## 适用版本

mysshw v26.06.13.3+（list / exec 子命令）。

## 其它平台

- **OpenAI Function Calling**: 转换 `anthropic-tool-use.json` 为 OpenAI `tools` 数组（结构几乎一致）
- **Cursor MCP**: 把 mysshw 包成 MCP server；详见 `ai-agent.md` § 5
- **LangChain**: 实现一个 `ShellTool` 类，包装 `subprocess.run(["mysshw", ...])`，parse JSON，返回 `ToolResult`
- **其他 CLI Agent**: 任何能 fork 子进程的工具都可；关键是 `-o json` + 退码
