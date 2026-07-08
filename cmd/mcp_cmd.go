package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"mysshw/config"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
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

	// 节点选择
	for _, n := range req.GetStringSlice("node", nil) {
		cmdLine = append(cmdLine, "--node", n)
	}
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

	// 节点选择 (跟 exec 一致)
	for _, n := range req.GetStringSlice("node", nil) {
		cmdLine = append(cmdLine, "--node", n)
	}
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

// ============================================================
// 内部 helper: 执行子命令并捕获输出
// ============================================================

// runSubcommandCaptured 在隔离的子命令上下文中执行 cmd, 捕获 stdout/stderr 到 buf,
// 把 ExitError 序列化为可读消息。
//
// 重要: cobra 的 cmd.Execute() 内部会 redirect 到 c.Root().ExecuteC(), 并使用
// rootCmd.args 作为入参 (而不是 cmd.args)。所以必须先在 rootCmd 上 SetArgs,
// 然后 Execute rootCmd; cobra 会根据 args 中的子命令路径自动路由到 cmd。
//
// caller 负责传入完整路由路径 (e.g. hostkey 子命令需传 ["hostkey", "list", ...],
// 不是 ["list", ...])。args 可以为空, 表示 "执行 cmd 本身"。
func runSubcommandCaptured(_ context.Context, cmd *cobra.Command, args []string) (string, error) {
	var buf bytes.Buffer
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetContext(context.Background())
	defer func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}()

	err := rootCmd.Execute()
	out := buf.String()

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
