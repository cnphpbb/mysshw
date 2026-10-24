package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"mysshw/config"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ============================================================
// mcpCmd 启动 MCP (Model Context Protocol) 服务器, stdio transport
//
// 设计目标:
//   - 让 Claude Desktop / Cursor / Cline / Zed 等 AI 工具原生集成 mysshw
//   - 不绕过 cobra: handler 把 MCP 参数翻译成 cobra args, 重新执行
//     `mysshw list` / `mysshw exec`, 复用 args validation / --dry-run / 退出码语义
//   - 工具列表严格静态注册, 不暴露 password / passphrase / ${ENV} 等敏感字段
//   - 退出码 → MCP ToolResultError, 业务错误透传 (BSD sysexits 不变)
//
// 用法 (Claude Desktop 配置示例):
//
//	{
//	  "mcpServers": {
//	    "mysshw": {
//	      "command": "/path/to/mysshw",
//	      "args": ["mcp"]
//	    }
//	  }
//	}
//
// 当前 Phase 暴露的工具:
//   - ssh_list_nodes        列出已配置的 SSH 节点
//   - ssh_exec              在一个或多个节点上执行命令
//   - ssh_copy              SCP 文件传输 (upload/download)
//   - ssh_hostkey_list      列出 known_hosts 条目
//   - ssh_hostkey_show      查看指定 host 的指纹
//   - ssh_hostkey_remove    删除指定 host 的条目
//   - ssh_hostkey_update    主动重取并更新 known_hosts
//   - ssh_hostkey_audit     健康度审计
//   - ssh_completions       动态查询 CLI 结构 (供 LLM 发现可用子命令)
// ============================================================

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server (stdio transport) for AI agents",
	Long: `Start a Model Context Protocol (MCP) server using stdio transport.

This exposes mysshw's SSH operations as MCP tools that AI agents (Claude Desktop,
Cursor, Cline, Zed, etc.) can invoke natively.

Once the MCP server is configured in your AI tool's settings, the agent will
discover tools automatically and can call them with structured arguments.

Examples:
  # Run as a subprocess (typically configured by the AI client, not run manually)
  mysshw mcp

  # Claude Desktop config (~/.config/Claude/claude_desktop_config.json):
  # {
  #   "mcpServers": {
  #     "mysshw": {
  #       "command": "/usr/local/bin/mysshw",
  #       "args": ["mcp"]
  #     }
  #   }
  # }

Exit codes (BSD sysexits):
  0    Server stopped normally
  1    Server error (transport, JSON-RPC, etc.)`,
	Args: cobra.NoArgs,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	// 配置全局: 强制走 JSON 输出 (MCP 客户端期望结构化数据)
	// 同时禁用 --cfg flag 的默认值 (避免 "~/.mysshw.toml" 在用户未配置时强制创建)
	// 这里只设置非交互标志, 实际 config path 由 MCP 客户端在 JSON-RPC args 中传入
	if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
		config.CFG_PATH = cfgPath
	}

	s := server.NewMCPServer(
		"mysshw",
		Version,
		server.WithToolCapabilities(false),
		// handler panic 不杀进程: recover → 返回 error 给客户端, 保持 stdio 连接存活
		// 否则任何 resolver/SSH 边界的 nil-deref 会导致 EOF 断开 (偶发现象之一)
		server.WithRecovery(),
		server.WithInstructions(`
mysshw MCP server exposes SSH operations as tools. Use ssh_list_nodes first to
discover available nodes, then ssh_exec to run commands. All tools return
JSON; check the 'error' field in each result item for per-node failures.

Important: Tools do NOT connect interactively. They execute one-shot commands
and return immediately. Long-running processes should use a separate shell.
`),
	)

	// 注册工具
	s.AddTool(sshListNodesTool(), sshListNodesHandler)
	s.AddTool(sshExecTool(), sshExecHandler)
	s.AddTool(sshCopyTool(), sshCopyHandler)
	s.AddTool(sshHostkeyListTool(), sshHostkeyListHandler)
	s.AddTool(sshHostkeyShowTool(), sshHostkeyShowHandler)
	s.AddTool(sshHostkeyRemoveTool(), sshHostkeyRemoveHandler)
	s.AddTool(sshHostkeyUpdateTool(), sshHostkeyUpdateHandler)
	s.AddTool(sshHostkeyAuditTool(), sshHostkeyAuditHandler)
	s.AddTool(sshCompletionsTool(), sshCompletionsHandler)

	// stdio transport: 通过 stdin 收 JSON-RPC, 通过 stdout 发 JSON-RPC
	// AI 客户端 (Claude Desktop 等) 把 mysshw mcp 作为子进程启动, 通过 stdio 通信
	fmt.Fprintln(os.Stderr, "mysshw:: MCP server started (stdio transport)")
	return server.ServeStdio(s)
}

// ============================================================
// Tool: ssh_list_nodes
// ============================================================

func sshListNodesTool() mcp.Tool {
	return mcp.NewTool("ssh_list_nodes",
		mcp.WithDescription(`List all configured SSH nodes in JSON format.

Use this as the first step to discover available nodes by name, alias, host, etc.
The JSON schema never includes passwords or passphrases (auth field shows the
authentication method category only).

Returns: Array of node objects with fields: group, name, alias, user, host,
port, auth (key|password|env|bcrypt|empty|unknown), keypath (optional), proxy
(optional, name of jump node).`),
		mcp.WithString("group",
			mcp.Description("Filter by group name (exact match, case-sensitive)"),
		),
		mcp.WithString("search",
			mcp.Description("Case-insensitive substring match against name/alias/host/user"),
		),
		mcp.WithString("cfg",
			mcp.Description("Custom config file path (defaults to ~/.mysshw.toml)"),
		),
	)
}

func sshListNodesHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdLine := []string{"list", "-o", "json"}
	if v := req.GetString("group", ""); v != "" {
		cmdLine = append(cmdLine, "--group", v)
	}
	if v := req.GetString("search", ""); v != "" {
		cmdLine = append(cmdLine, "--search", v)
	}
	if v := req.GetString("cfg", ""); v != "" {
		cmdLine = append(cmdLine, "--cfg", v)
	}

	out, err := runSubcommandCaptured(ctx, listCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// ============================================================
// Tool: ssh_exec
// ============================================================

func sshExecTool() mcp.Tool {
	return mcp.NewTool("ssh_exec",
		mcp.WithDescription(`Execute a command on one or more SSH nodes without TUI menu.

Selects nodes by name/alias (or by host+user+port). Returns a JSON array with
one entry per node containing stdout, stderr, exit_code, and duration. PTY is
enabled by default (for sudo / progress bars).

Always use ssh_list_nodes first to discover valid node names.

Important: For batch operations across many nodes, prefer --node with an
explicit list rather than relying on defaults.`),
		mcp.WithArray("node",
			mcp.Description("Node name or alias (repeatable for batch). Use ssh_list_nodes to discover valid values."),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("alias",
			mcp.Description("Single node by alias (exact match)"),
		),
		mcp.WithString("host",
			mcp.Description("Single node by host (combine with user/port to disambiguate)"),
		),
		mcp.WithString("user",
			mcp.Description("User for --host selector (e.g. 'app', 'root')"),
		),
		mcp.WithNumber("port",
			mcp.Description("Port for --host selector (e.g. 22, 2222)"),
		),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Command to execute on the remote node(s). Must be non-empty. Example: 'uptime', 'systemctl status nginx'"),
		),
		mcp.WithString("timeout",
			mcp.Description("Per-node timeout (e.g. '30s', '5m'). Default: 30s. Use '0' for no limit."),
		),
		mcp.WithNumber("parallel",
			mcp.Description("Max concurrent nodes for batch. Default: 10. Use 0 for unbounded."),
		),
		mcp.WithBoolean("no_pty",
			mcp.Description("Disable PTY. Default: PTY enabled (needed for sudo / progress bars / interactive programs)"),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description("Print the resolved plan (nodes, auth, command) WITHOUT actually connecting. Use this to verify AI-generated commands are pointing at the right nodes before destructive operations."),
		),
		mcp.WithBoolean("rotate_known_hosts",
			mcp.Description("On host key fingerprint mismatch, auto-rotate known_hosts (delete old + add new). Equivalent to OpenSSH StrictHostKeyChecking=no for THIS call. Cannot distinguish MITM from legitimate key rotation - use only when admin has rotated the server key."),
		),
		mcp.WithString("cfg",
			mcp.Description("Custom config file path (defaults to ~/.mysshw.toml)"),
		),
	)
}

func sshExecHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdLine := []string{"exec", "-o", "json"}

	// 节点选择 — 走 appendNodeArgFromRequest 兜底 mcp-go 0.32.0 GetStringSlice 的裸 string bug
	appendNodeArgFromRequest(req, &cmdLine)

	if v := req.GetString("alias", ""); v != "" {
		cmdLine = append(cmdLine, "--alias", v)
	}
	if v := req.GetString("host", ""); v != "" {
		cmdLine = append(cmdLine, "--host", v)
	}
	if v := req.GetString("user", ""); v != "" {
		cmdLine = append(cmdLine, "--user", v)
	}
	if v := req.GetInt("port", 0); v != 0 {
		cmdLine = append(cmdLine, "--port", fmt.Sprintf("%d", v))
	}

	// 命令
	command := req.GetString("command", "")
	if command == "" {
		return mcp.NewToolResultError("missing required parameter 'command'"), nil
	}

	// 其它选项
	if v := req.GetString("timeout", ""); v != "" {
		cmdLine = append(cmdLine, "--timeout", v)
	}
	if v := req.GetInt("parallel", 0); v != 0 {
		cmdLine = append(cmdLine, "--parallel", fmt.Sprintf("%d", v))
	}
	if req.GetBool("no_pty", false) {
		cmdLine = append(cmdLine, "--no-pty")
	}
	if req.GetBool("dry_run", false) {
		cmdLine = append(cmdLine, "--dry-run")
	}
	if req.GetBool("rotate_known_hosts", false) {
		cmdLine = append(cmdLine, "--rotate-known-hosts")
	}
	if v := req.GetString("cfg", ""); v != "" {
		cmdLine = append(cmdLine, "--cfg", v)
	}

	// 命令必须用 `--` 分隔 (cobra args)
	cmdLine = append(cmdLine, "--", command)

	out, err := runSubcommandCaptured(ctx, execCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// ============================================================
// Tool: ssh_copy
// ============================================================

func sshCopyTool() mcp.Tool {
	return mcp.NewTool("ssh_copy",
		mcp.WithDescription(`Copy a file between local and SSH nodes via SCP protocol.

Supports both upload (local -> remote) and download (remote -> local).
Batch upload across multiple nodes is supported; download is always single-node.
Output is JSON with one entry per node containing node/action/bytes/duration/error.`),
		mcp.WithArray("node",
			mcp.Description("Node name(s) or alias(es). Repeat for batch: --node X --node Y"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("alias",
			mcp.Description("Single node by alias (exact)"),
		),
		mcp.WithString("host",
			mcp.Description("Single node by host (combine with user/port)"),
		),
		mcp.WithString("user",
			mcp.Description("User for --host selector"),
		),
		mcp.WithNumber("port",
			mcp.Description("Port for --host selector"),
		),
		mcp.WithString("direction",
			mcp.Required(),
			mcp.Description("Either 'upload' (local -> remote on every node) or 'download' (remote -> local, single node only)"),
		),
		mcp.WithString("local",
			mcp.Required(),
			mcp.Description("Local file path (source for upload, destination for download)"),
		),
		mcp.WithString("remote",
			mcp.Required(),
			mcp.Description("Remote file path (destination for upload, source for download)"),
		),
		mcp.WithString("mode",
			mcp.Description("Remote file permissions as octal string (upload only). Example: '0644', '0755'. Default: 0644"),
		),
		mcp.WithString("timeout",
			mcp.Description("Per-node transfer timeout (e.g. '5m'). Default: 5m"),
		),
		mcp.WithNumber("parallel",
			mcp.Description("Max concurrent uploads (download always single-node). Default: 5"),
		),
		mcp.WithBoolean("rotate_known_hosts",
			mcp.Description("On key fingerprint mismatch, auto-rotate known_hosts. Use only on legitimate key rotation."),
		),
		mcp.WithString("cfg",
			mcp.Description("Custom config file path"),
		),
	)
}

func sshCopyHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	direction := req.GetString("direction", "")
	if direction != "upload" && direction != "download" {
		return mcp.NewToolResultError("'direction' must be 'upload' or 'download'"), nil
	}
	local := req.GetString("local", "")
	if local == "" {
		return mcp.NewToolResultError("missing required parameter 'local'"), nil
	}
	remote := req.GetString("remote", "")
	if remote == "" {
		return mcp.NewToolResultError("missing required parameter 'remote'"), nil
	}

	cmdLine := []string{"copy", "-o", "json"}

	// 节点选择 (跟 exec 一致; 同样绕开 mcp-go 0.32.0 GetStringSlice 的裸 string bug)
	appendNodeArgFromRequest(req, &cmdLine)

	if v := req.GetString("alias", ""); v != "" {
		cmdLine = append(cmdLine, "--alias", v)
	}
	if v := req.GetString("host", ""); v != "" {
		cmdLine = append(cmdLine, "--host", v)
	}
	if v := req.GetString("user", ""); v != "" {
		cmdLine = append(cmdLine, "--user", v)
	}
	if v := req.GetInt("port", 0); v != 0 {
		cmdLine = append(cmdLine, "--port", fmt.Sprintf("%d", v))
	}

	// 方向使用 -U/--upload 或 -d/--download
	if direction == "upload" {
		cmdLine = append(cmdLine, "--upload") // -U
	} else {
		cmdLine = append(cmdLine, "--download") // -d
	}

	cmdLine = append(cmdLine, "--local", local, "--remote", remote)

	if v := req.GetString("mode", ""); v != "" {
		cmdLine = append(cmdLine, "--mode", v)
	}
	if v := req.GetString("timeout", ""); v != "" {
		cmdLine = append(cmdLine, "--timeout", v)
	}
	if v := req.GetInt("parallel", 0); v != 0 {
		cmdLine = append(cmdLine, "--parallel", fmt.Sprintf("%d", v))
	}
	if req.GetBool("rotate_known_hosts", false) {
		cmdLine = append(cmdLine, "--rotate-known-hosts")
	}
	if v := req.GetString("cfg", ""); v != "" {
		cmdLine = append(cmdLine, "--cfg", v)
	}

	out, err := runSubcommandCaptured(ctx, copyCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// ============================================================
// Tool: ssh_hostkey_list
// ============================================================

func sshHostkeyListTool() mcp.Tool {
	return mcp.NewTool("ssh_hostkey_list",
		mcp.WithDescription(`List all entries in the SSH known_hosts file.

Returns JSON array of {host, host_names, algorithm, fingerprint, is_hashed}.
Default location: ~/.ssh/known_hosts (shared with OpenSSH). Can be overridden via
config.known_hosts_path in ~/.mysshw.toml.`),
		mcp.WithString("cfg",
			mcp.Description("Custom config file path"),
		),
	)
}

func sshHostkeyListHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdLine := []string{"hostkey", "list", "-o", "json"}
	if v := req.GetString("cfg", ""); v != "" {
		cmdLine = append(cmdLine, "--cfg", v)
	}
	out, err := runSubcommandCaptured(ctx, hostkeyListCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// ============================================================
// Tool: ssh_hostkey_show
// ============================================================

func sshHostkeyShowTool() mcp.Tool {
	return mcp.NewTool("ssh_hostkey_show",
		mcp.WithDescription(`Look up a specific host in known_hosts and return all matching entries.

Returns JSON with host, requested_as, entries[], found_in_file.
Exits 78 (EX_CONFIG) if the host is not in known_hosts.`),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("Host to look up (e.g. 'prod-1.example.com', '10.0.0.5:22')"),
		),
		mcp.WithString("cfg",
			mcp.Description("Custom config file path"),
		),
	)
}

func sshHostkeyShowHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host := req.GetString("host", "")
	if host == "" {
		return mcp.NewToolResultError("missing required parameter 'host'"), nil
	}
	cmdLine := []string{"hostkey", "show", host, "-o", "json"}
	if v := req.GetString("cfg", ""); v != "" {
		cmdLine = append(cmdLine, "--cfg", v)
	}
	out, err := runSubcommandCaptured(ctx, hostkeyShowCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// ============================================================
// Tool: ssh_hostkey_remove
// ============================================================

func sshHostkeyRemoveTool() mcp.Tool {
	return mcp.NewTool("ssh_hostkey_remove",
		mcp.WithDescription(`Remove all known_hosts entries for a specific host.

Use after a confirmed key rotation (admin has distributed new fingerprint via
out-of-band channel). Returns plain-text success message. Exits 0 even if
host was not found (no-op).`),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("Host whose entries should be removed (e.g. 'prod-1.example.com', '10.0.0.5:22')"),
		),
		mcp.WithString("cfg",
			mcp.Description("Custom config file path"),
		),
	)
}

func sshHostkeyRemoveHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host := req.GetString("host", "")
	if host == "" {
		return mcp.NewToolResultError("missing required parameter 'host'"), nil
	}
	cmdLine := []string{"hostkey", "remove", host}
	if v := req.GetString("cfg", ""); v != "" {
		cmdLine = append(cmdLine, "--cfg", v)
	}
	out, err := runSubcommandCaptured(ctx, hostkeyRemoveCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// ============================================================
// Tool: ssh_hostkey_update
// ============================================================

func sshHostkeyUpdateTool() mcp.Tool {
	return mcp.NewTool("ssh_hostkey_update",
		mcp.WithDescription(`Update known_hosts by actively re-fetching the key from a node.

Safer than --rotate-known-hosts flag because the operation is explicit and
audit-friendly. By default runs in DRY-RUN mode (prints planned action without
modifying known_hosts); pass --confirm to actually apply.

Security workflow:
  1. Admin rotates server key, distributes new fingerprint through trusted channel
  2. Optionally use --expect-fingerprint to enforce MITM protection
  3. Pass --confirm to apply

If --expect-fingerprint is set and server's actual fingerprint mismatches,
exits 124 without modifying known_hosts.`),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("Host entry to update in known_hosts (e.g. 'prod-1.example.com')"),
		),
		mcp.WithString("node",
			mcp.Required(),
			mcp.Description("Node name from ~/.mysshw.toml to connect to (must match <host>)"),
		),
		mcp.WithBoolean("confirm",
			mcp.Description("Actually modify known_hosts. Default: false (dry-run; print plan only)"),
		),
		mcp.WithString("expect_fingerprint",
			mcp.Description("Expected server fingerprint (SHA256:base64). If set and mismatch, exit 124 without modifying."),
		),
		mcp.WithString("timeout",
			mcp.Description("Dial timeout (e.g. '30s'). Default: 30s"),
		),
		mcp.WithString("cfg",
			mcp.Description("Custom config file path"),
		),
	)
}

func sshHostkeyUpdateHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host := req.GetString("host", "")
	if host == "" {
		return mcp.NewToolResultError("missing required parameter 'host'"), nil
	}
	node := req.GetString("node", "")
	if node == "" {
		return mcp.NewToolResultError("missing required parameter 'node'"), nil
	}

	cmdLine := []string{"hostkey", "update", host, "--node", node, "-o", "json"}
	if req.GetBool("confirm", false) {
		cmdLine = append(cmdLine, "--confirm")
	}
	if v := req.GetString("expect_fingerprint", ""); v != "" {
		cmdLine = append(cmdLine, "--expect-fingerprint", v)
	}
	if v := req.GetString("timeout", ""); v != "" {
		cmdLine = append(cmdLine, "--timeout", v)
	}
	if v := req.GetString("cfg", ""); v != "" {
		cmdLine = append(cmdLine, "--cfg", v)
	}

	out, err := runSubcommandCaptured(ctx, hostkeyUpdateCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// ============================================================
// Tool: ssh_hostkey_audit
// ============================================================

func sshHostkeyAuditTool() mcp.Tool {
	return mcp.NewTool("ssh_hostkey_audit",
		mcp.WithDescription(`Comprehensive audit of known_hosts health.

Returns JSON with:
  - file (path / size / modified_at)
  - total_entries, plain_entries, hashed_entries
  - algorithm_distribution (e.g. {ssh-ed25519: 5})
  - repeated_hosts (same host multiple entries, indicates leftover from key rotation)
  - configured_hosts (from --check-config mode)
  - missing_in_known_hosts (in config but not yet trusted)
  - warnings (human-readable)

Use for: security review (legacy algorithms), operational sync (find new nodes
not yet in known_hosts), rotation cleanup (find leftover old keys).`),
		mcp.WithBoolean("check_config",
			mcp.Description("Cross-check with ~/.mysshw.toml nodes; report hosts that are in config but missing from known_hosts"),
		),
		mcp.WithString("cfg",
			mcp.Description("Custom config file path"),
		),
	)
}

func sshHostkeyAuditHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdLine := []string{"hostkey", "audit", "-o", "json"}
	if req.GetBool("check_config", false) {
		cmdLine = append(cmdLine, "--check-config")
	}
	if v := req.GetString("cfg", ""); v != "" {
		cmdLine = append(cmdLine, "--cfg", v)
	}
	out, err := runSubcommandCaptured(ctx, hostkeyAuditCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// ============================================================
// Tool: ssh_completions (动态 CLI 发现)
// ============================================================

func sshCompletionsTool() mcp.Tool {
	return mcp.NewTool("ssh_completions",
		mcp.WithDescription(`Dynamically query mysshw CLI structure: commands, subcommands, flags, args policy, exit codes.

Output schema: see `+"`mysshw completions --help`"+`. Stable schema_version "1.0".

Use cases:
  - Discover what subcommands/flags are available at runtime
  - Validate that an expected tool/flag exists before calling it
  - Build dynamic tool-use schemas aligned with current binary

For LLM agents: prefer calling this tool at conversation start to learn the
full surface, instead of hardcoding command names from documentation.`),
		mcp.WithString("subcommand",
			mcp.Description("Filter to a specific subcommand tree (e.g. 'hostkey', 'tunnel'). Omit for full schema."),
		),
		mcp.WithBoolean("flat",
			mcp.Description("Flatten all subcommands into a single top-level array (no nesting). Easier for AI to iterate."),
		),
	)
}

func sshCompletionsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdLine := []string{"completions"}
	if v := req.GetString("subcommand", ""); v != "" {
		cmdLine = append(cmdLine, v)
	}
	if req.GetBool("flat", false) {
		cmdLine = append(cmdLine, "--flat")
	}

	// completions 总是输出 JSON, 不依赖 -o flag
	out, err := runSubcommandCaptured(ctx, completionsCmd, cmdLine)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// appendNodeArgFromRequest 把 MCP 参数 "node" 展开成 --node <v> 追加到 cmdLine.
//
// 必要性: mcp-go 0.32.0 的 GetStringSlice 对裸 string 值 (e.g. `node: "Manages-Node"`)
// 返回 nil — 只接受 []string / []any. AI 客户端 (hermes 等) 经常发裸 string 单节点,
// GetStringSlice 吞掉 → --node 不进 cmdLine → resolver fall-through 到 "全部节点"
// mass fan-out → 多节点 SSH 失败 → batch 完成失败.
//
// 修复: 直接读 req.GetArguments() (类型断言 map[string]any), switch 处理三种类型:
//   - string  (裸 string, AI 客户端发单节点常见)
//   - []any   (JSON 数组, mcp-go 0.32.0 已支持)
//   - []string (测试场景直接传 Go []string, mcp-go 0.32.0 也支持)
//
// nil / 其它类型不动 (前者表示未提供).
func appendNodeArgFromRequest(req mcp.CallToolRequest, cmdLine *[]string) {
	args := req.GetArguments()
	v, ok := args["node"]
	if !ok {
		return
	}
	switch x := v.(type) {
	case string:
		if x != "" {
			*cmdLine = append(*cmdLine, "--node", x)
		}
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok && s != "" {
				*cmdLine = append(*cmdLine, "--node", s)
			}
		}
	case []string:
		for _, s := range x {
			if s != "" {
				*cmdLine = append(*cmdLine, "--node", s)
			}
		}
	}
}

// ============================================================
// 内部 helper: 执行子命令并捕获输出
// ============================================================

// mcpSubcommandMu 串行化 runSubcommandCaptured 调用.
//
// 原因: 函数修改全局 rootCmd.SetArgs/SetOut/SetErr/SetContext 后调 Execute() 走 cobra.
// 并发 MCP tool call 会撕扯这些全局状态, 且 cobra 内部 (parsed flags, command tree)
// 非并发安全. 全局锁保证同一时刻只有一个 in-flight call.
//
// ponytail: 全局锁足够. 个人 SSH 客户端的 MCP 并发低 (单用户多次串行调用常见,
// 并行工具调用少见). 真要扛高并发再换成 exec.Command() 派生子进程.
var mcpSubcommandMu sync.Mutex

// runSubcommandCaptured 在隔离的子命令上下文中执行 cmd, 捕获 stdout/stderr 到 buf,
// 把 ExitError 序列化为可读消息。
//
// 重要: cobra 的 cmd.Execute() 内部会 redirect 到 c.Root().ExecuteC(), 并使用
// rootCmd.args 作为入参 (而不是 cmd.args)。所以必须先在 rootCmd 上 SetArgs,
// 然后 Execute rootCmd; cobra 会根据 args 中的子命令路径自动路由到 cmd。
//
// caller 负责传入完整路由路径 (e.g. hostkey 子命令需传 ["hostkey", "list", ...],
// 不是 ["list", ...])。args 可以为空, 表示 "执行 cmd 本身"。
//
// ctx 来自 MCP 请求 (per-request): 透传给 rootCmd.SetContext 让内层 SSH/cobra 收到
// 客户端取消/超时信号。否则 AI 客户端 cancel 后内层 SSH 仍跑到自然结束, 占用资源。
//
// 何时 reset:
//   - 必须 reset 再 Execute, 不然 cobra 的 flag Set() 把新值 append 到旧 slice 上
//     (e.g. execNodesFlag StringSlice 累积, execDryRun bool 残留), 第二次调用跑出来
//     还是第一次的参数, exec 完全失效。
//   - 历史上出过 bug: 用户先调 dry-run, 之后所有 exec 都成 dry-run, AI 以为自己执行了
//     实则没有。本函数统一 reset 解决, 不依赖每个 MCP handler 自觉。
//
// 何时 recover:
//   - 子命令内部任何 panic (SSH resolver / config / viper 边缘 case) 都不应杀进程。
//   - WithRecovery() 只 wrap handler, 不 cover runSubcommandCaptured 自身 + cobra 解析路径。
//   - 兜底: 内层 panic → 转 ExitError(70) 包 error 返回, MCP 连接存活。
func runSubcommandCaptured(ctx context.Context, _ *cobra.Command, args []string) (out string, retErr error) {
	mcpSubcommandMu.Lock()
	defer mcpSubcommandMu.Unlock()

	// reset 全局 flag 变量: 防止 MCP / CLI 多次调用之间状态泄漏
	resetCmdGlobals()

	var buf bytes.Buffer
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetContext(ctx)
	defer func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}()

	// 兜底 panic: 任何内层 panic (resolver / SSH client / viper 边缘 nil-deref)
	// 都不应杀掉 MCP 进程. WithRecovery() 只 wrap tool handler, 不覆盖此层.
	// 命中即转 ExitError(70) "internal error", AI 客户端拿错误而不是 EOF.
	defer func() {
		if r := recover(); r != nil {
			out = buf.String()
			retErr = WrapExit(ExitSoftware, fmt.Errorf("panic in subcommand: %v", r))
		}
	}()

	err := rootCmd.Execute()
	out = buf.String()

	if err != nil {
		if ee, ok := err.(ExitError); ok {
			if out != "" {
				return out, fmt.Errorf("exit %d: %s", ee.Code, ee.Err)
			}
			return "", fmt.Errorf("exit %d: %s", ee.Code, ee.Err)
		}
		if out != "" {
			return out, err
		}
		return "", err
	}
	return out, nil
}

// resetCmdGlobals 重置所有 cobra flag 全局变量到默认值.
//
// 必要性: cobra 用 StringSliceVar/BoolVar 等绑定到 Go 全局变量作为 flag 的值.
// flag 解析器 (pflag) 的 Set() 方法对 StringSlice 是 append, 对 Bool 只在见到 flag 时
// 才改值. 因此 cobra 不会"自动清零" — 上一轮的 flag 残留会污染下一轮.
//
// 典型 bug: MCP 客户端连续调两次 ssh_exec:
//   - 第 1 次: --node A --dry-run  → execNodesFlag=[A], execDryRun=true
//   - 第 2 次: --node B             → cobra append → execNodesFlag=[A,B], execDryRun=true 残留
//   - 结果: 第 2 次实际跑在 A+B 节点上, dry-run 模式, 看起来"exec 跑不通"
//
// reset 时机: runSubcommandCaptured 入口, Execute() 之前. 这样 cobra 解析新 args 时
// 从干净的默认值开始, 没有 append-on-stale 问题.
//
// 影响范围: reset 只在 MCP 路径 (runSubcommandCaptured) 调用, 不影响直接 CLI 调用.
// CLI 单次进程内多次调子命令本来也是 stateless (进程结束状态丢), reset 也不破坏.
// 命令 reset 时机是 rootCmd.Execute() 之前, 用 mutex 串行保证并发安全 (mcpSubcommandMu).
func resetCmdGlobals() {
	// exec 子命令
	execNodesFlag = nil
	execAlias = ""
	execHost = ""
	execUser = ""
	execPort = 0
	execTimeout = 30 * time.Second
	execParallel = 10
	execNoPTY = false
	execStopOnError = false
	execDryRun = false
	execRotateKnownHosts = false
	execStream = false

	// list 子命令
	listGroup = ""
	listSearch = ""

	// copy 子命令
	copyNodesFlag = nil
	copyAlias = ""
	copyHost = ""
	copyUser = ""
	copyPort = 0
	copyDirection = ""
	copyLocal = ""
	copyRemote = ""
	copyMode = "0644"
	copyTimeout = 5 * time.Minute
	copyParallel = 5
	copyRotateKnownHosts = false

	// completions 子命令
	completionsSubCmd = ""
	completionsFlat = false

	// hostkey update 子命令
	hostkeyUpdateNode = ""
	hostkeyUpdateTimeout = 30 * time.Second
	hostkeyUpdateConfirm = false
	hostkeyUpdateExpectFP = ""
	hostkeyAuditCheckConfig = false

	// config 全局: 防止上一次 --cfg 路径泄漏到下一次 (用户没显式传 --cfg 时)
	// 同 resetRootCmd 测试辅助函数的策略: 清回默认 + 清空已加载的 config
	config.CFG_PATH = "~/.mysshw.toml"
	config.CFG = nil
	// viper 全局: 防止上次缓存的 config path 影响下次 getConfigPath 解析
	viper.Reset()
}
