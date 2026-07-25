package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"mysshw/config"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// 测试策略:
//   直接调用 sshListNodesHandler / sshExecHandler, 用临时 config 文件
//   模拟完整 JSON-RPC 调用链。不通过 stdio transport 跑 (那需要 MCP client)。
//
//   runSubcommandCaptured 会设 rootCmd.args 并 Execute rootCmd;
//   每个测试结束后必须清理 rootCmd 的全局状态, 避免污染后续测试。
// ============================================================

// resetRootCmd 重置 rootCmd + viper + cmd 全局状态, 每个测试前/后调用
func resetRootCmd(t *testing.T) {
	t.Helper()
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	rootCmd.SetContext(context.Background())
	// 重置 config 全局状态 (CFG_PATH / CFG)
	config.CFG_PATH = "~/.mysshw.toml"
	config.CFG = nil
	// 重置 viper 全局状态 (路径缓存 / config 缓存)
	viper.Reset()
	// 重置 list 子命令的全局 filter 变量
	listGroup = ""
	listSearch = ""
	// 重置 exec 子命令的全局 flag 变量 (避免上个测试 --node 残留)
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
	// 重置 copy 子命令的全局 flag 变量
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
	// 重置 completions 子命令的全局 flag 变量
	completionsSubCmd = ""
	completionsFlat = false
}

// withTempConfig 创建临时配置文件并返回路径; t.Cleanup 自动清理
func withTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/mysshw.toml"
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

// ============================================================
// ssh_list_nodes tool tests
// ============================================================

func TestMCPListNodesTool_Schema(t *testing.T) {
	tool := sshListNodesTool()
	assert.Equal(t, "ssh_list_nodes", tool.Name)
	// 描述必填
	assert.NotEmpty(t, tool.Description)
	// cfg / group / search 是 string 类型参数
	for _, name := range []string{"cfg", "group", "search"} {
		prop, ok := tool.InputSchema.Properties[name].(map[string]any)
		assert.True(t, ok, "missing property %q", name)
		assert.Equal(t, "string", prop["type"], "%s should be string", name)
	}
}

func TestMCPListNodesHandler_Success(t *testing.T) {
	resetRootCmd(t)
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "node-1"
alias = "Alias1"
host = "10.0.0.1"
user = "app"
port = 2222

[[nodes.ssh]]
name = "node-2"
host = "10.0.0.2"
user = "root"
`)

	// 构造 MCP 请求
	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_list_nodes"
	req.Params.Arguments = map[string]any{"cfg": cfgPath}

	result, err := sshListNodesHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	// 解析 JSON 输出
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "content[0] should be TextContent")
	var nodes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &nodes))
	require.Len(t, nodes, 2)

	// 验证字段不含 password (安全规约)
	for _, n := range nodes {
		_, hasPassword := n["password"]
		assert.False(t, hasPassword, "list nodes JSON must not contain password field")
	}

	assert.Equal(t, "node-1", nodes[0]["name"])
	assert.Equal(t, "Alias1", nodes[0]["alias"])
	assert.Equal(t, float64(2222), nodes[0]["port"])
}

func TestMCPListNodesHandler_WithSearch(t *testing.T) {
	resetRootCmd(t)
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "web-1"
host = "10.0.0.1"

[[nodes.ssh]]
name = "db-1"
host = "10.0.0.2"
`)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_list_nodes"
	req.Params.Arguments = map[string]any{
		"cfg":    cfgPath,
		"search": "web",
	}

	result, err := sshListNodesHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	var nodes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &nodes))
	assert.Len(t, nodes, 1, "search 'web' should match only web-1")
	assert.Equal(t, "web-1", nodes[0]["name"])
}

func TestMCPListNodesHandler_MissingConfigFile(t *testing.T) {
	resetRootCmd(t)
	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_list_nodes"
	req.Params.Arguments = map[string]any{
		"cfg": "/nonexistent/mysshw.toml",
	}

	result, err := sshListNodesHandler(context.Background(), req)
	require.NoError(t, err, "handler should never return Go error, only ToolResultError")
	require.NotNil(t, result)
	assert.True(t, result.IsError, "missing config should return IsError=true")

	text := result.Content[0].(mcp.TextContent).Text
	// 应包含 exit code 78 (EX_CONFIG) 或 1
	assert.Contains(t, text, "exit 78", "should propagate EX_CONFIG exit code")
}

// ============================================================
// ssh_exec tool tests
// ============================================================

func TestMCPExecTool_Schema(t *testing.T) {
	tool := sshExecTool()
	assert.Equal(t, "ssh_exec", tool.Name)
	assert.NotEmpty(t, tool.Description)

	// command 是必填 string
	cmdProp, ok := tool.InputSchema.Properties["command"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", cmdProp["type"])
	assert.Contains(t, tool.InputSchema.Required, "command", "command must be required")

	// node 是 array of string
	nodeProp, ok := tool.InputSchema.Properties["node"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "array", nodeProp["type"])
	items, ok := nodeProp["items"].(map[string]any)
	require.True(t, ok, "node items should be object schema")
	assert.Equal(t, "string", items["type"], "node items should be string")

	// 数字 / bool 参数
	for name, typ := range map[string]string{
		"port":               "number",
		"parallel":           "number",
		"no_pty":             "boolean",
		"dry_run":            "boolean",
		"rotate_known_hosts": "boolean",
	} {
		prop, ok := tool.InputSchema.Properties[name].(map[string]any)
		require.True(t, ok, "missing property %q", name)
		assert.Equal(t, typ, prop["type"], "%s should be %s", name, typ)
	}
}

func TestMCPExecHandler_MissingCommand(t *testing.T) {
	resetRootCmd(t)
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "node-1"
host = "10.0.0.1"
`)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_exec"
	req.Params.Arguments = map[string]any{
		"cfg":  cfgPath,
		"node": []string{"node-1"},
		// command 故意缺失
	}

	result, err := sshExecHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "missing command should return IsError=true")
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "command")
}

func TestMCPExecHandler_DryRun(t *testing.T) {
	resetRootCmd(t)
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "prod-1"
host = "10.0.0.1"
user = "app"
port = 22
`)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_exec"
	req.Params.Arguments = map[string]any{
		"cfg":     cfgPath,
		"node":    []string{"prod-1"},
		"command": "uptime",
		"dry_run": true,
		"timeout": "30s",
	}

	result, err := sshExecHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "expected success on dry-run, got: %v", result.Content)

	text := result.Content[0].(mcp.TextContent).Text
	var plan map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &plan))
	assert.True(t, plan["dry_run"].(bool))
	assert.Equal(t, "uptime", plan["command"])
	assert.Equal(t, "30s", plan["timeout"])
}

func TestMCPExecHandler_NodeNotFound(t *testing.T) {
	resetRootCmd(t)
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "node-1"
host = "10.0.0.1"
`)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_exec"
	req.Params.Arguments = map[string]any{
		"cfg":     cfgPath,
		"node":    []string{"nonexistent"},
		"command": "uptime",
		"dry_run": true,
	}

	result, err := sshExecHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "exit 78", "should propagate EX_CONFIG (78) for node not found")
}

func TestMCPExecHandler_AliasMatching(t *testing.T) {
	// 验证 --node 同时匹配 alias (v26.07.04.1+ 行为)
	resetRootCmd(t)
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "prod-web-1"
alias = "Web01"
host = "10.0.0.1"
`)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_exec"
	req.Params.Arguments = map[string]any{
		"cfg":     cfgPath,
		"node":    []string{"Web01"}, // 用 alias 找节点
		"command": "uptime",
		"dry_run": true,
	}

	result, err := sshExecHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError, "alias match should work: %v", result.Content)
}

func TestMCPExecHandler_BatchWithMultipleNodes(t *testing.T) {
	resetRootCmd(t)
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "node-1"
host = "10.0.0.1"

[[nodes.ssh]]
name = "node-2"
host = "10.0.0.2"

[[nodes.ssh]]
name = "node-3"
host = "10.0.0.3"
`)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_exec"
	req.Params.Arguments = map[string]any{
		"cfg":     cfgPath,
		"node":    []string{"node-1", "node-2", "node-3"},
		"command": "uptime",
		"dry_run": true,
	}

	result, err := sshExecHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)
}

func TestMCPExecHandler_AllArgsTranslateCorrectly(t *testing.T) {
	// 验证所有参数都能正确翻译为 cobra args (除了 dry_run)
	resetRootCmd(t)
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "prod-1"
host = "10.0.0.1"
`)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_exec"
	req.Params.Arguments = map[string]any{
		"cfg":                cfgPath,
		"node":               []string{"prod-1"},
		"command":            "ls /tmp",
		"timeout":            "5m",
		"parallel":           20,
		"no_pty":             true,
		"dry_run":            true,
		"alias":              "should_be_ignored_since_node_is_set",
		"host":               "10.0.0.1",
		"user":               "app",
		"port":               2222,
		"rotate_known_hosts": true,
	}

	result, err := sshExecHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success, got: %v", result.Content)
}

// ============================================================
// runSubcommandCaptured helper tests
// ============================================================

// TestRunSubcommandCaptured_DoesNotPolluteState 验证 runSubcommandCaptured
// 不会污染 rootCmd 的 SetArgs 状态 (前一个测试设了 args 但提前 return)
// 这是一个回归测试。
func TestRunSubcommandCaptured_DoesNotPolluteState(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	// 故意设置错误 args[0]
	_, _ = runSubcommandCaptured(context.Background(), listCmd, []string{"wrong"})

	// 验证 rootCmd 的 args 已被清理
	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "T"

[[nodes.ssh]]
name = "n1"
host = "1.2.3.4"
`)
	out, err := runSubcommandCaptured(context.Background(), listCmd,
		[]string{"list", "-o", "json", "--cfg", cfgPath})
	require.NoError(t, err, "if state polluted, runSubcommandCaptured would fail: %s", out)
	assert.Contains(t, out, "n1")
}

// TestRunSubcommandCaptured_NestedSubcommand 验证嵌套子命令 (e.g. hostkey list) 正确路由
func TestRunSubcommandCaptured_NestedSubcommand(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	// nested 子命令需要传完整路径
	_, err := runSubcommandCaptured(context.Background(), hostkeyListCmd,
		[]string{"hostkey", "list", "-o", "json"})
	// 可能在缺少 ~/.ssh/known_hosts 时报错, 但 errors.Is 检查路由错误类型
	// 如果 args 被错误路由, 会报 "unknown command" 而不是 ExitError
	if err != nil {
		// 接受 ExitError (业务错误, 路由成功) 或其他正常错误
		assert.NotContains(t, err.Error(), "unknown command",
			"nested subcommand should route correctly")
	}
}

func TestRunSubcommandCaptured_ExecutesCorrectCommand(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "n1"
host = "1.2.3.4"
`)

	out, err := runSubcommandCaptured(context.Background(), listCmd,
		[]string{"list", "-o", "json", "--cfg", cfgPath})
	require.NoError(t, err)
	assert.Contains(t, out, "n1")
	assert.Contains(t, out, "1.2.3.4")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ============================================================
// mcpCmd 注册测试
// ============================================================

func TestMCPCmd_Registered(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "mcp" {
			assert.Equal(t, "Start MCP server (stdio transport) for AI agents", c.Short)
			return
		}
	}
	t.Fatal("mcp subcommand not registered on rootCmd")
}

// ============================================================
// ssh_copy tool tests
// ============================================================

func TestMCPCopyTool_Schema(t *testing.T) {
	tool := sshCopyTool()
	assert.Equal(t, "ssh_copy", tool.Name)
	assert.NotEmpty(t, tool.Description)

	// 必填参数
	assert.Contains(t, tool.InputSchema.Required, "direction")
	assert.Contains(t, tool.InputSchema.Required, "local")
	assert.Contains(t, tool.InputSchema.Required, "remote")

	// node 是 array of string
	nodeProp, ok := tool.InputSchema.Properties["node"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "array", nodeProp["type"])
}

func TestMCPCopyHandler_InvalidDirection(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_copy"
	req.Params.Arguments = map[string]any{
		"direction": "sideways",
		"local":     "/tmp/x",
		"remote":    "/tmp/y",
	}

	result, err := sshCopyHandler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "direction")
}

func TestMCPCopyHandler_MissingLocal(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_copy"
	req.Params.Arguments = map[string]any{
		"direction": "upload",
		"remote":    "/tmp/y",
	}

	result, err := sshCopyHandler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "local")
}

func TestMCPCopyHandler_MissingRemote(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_copy"
	req.Params.Arguments = map[string]any{
		"direction": "upload",
		"local":     "/tmp/x",
	}

	result, err := sshCopyHandler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "remote")
}

func TestMCPCopyHandler_UploadLocalFileMissing(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "T"

[[nodes.ssh]]
name = "n1"
host = "10.0.0.1"
`)
	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_copy"
	req.Params.Arguments = map[string]any{
		"cfg":       cfgPath,
		"node":      []string{"n1"},
		"direction": "upload",
		"local":     "/nonexistent/path/that/does/not/exist",
		"remote":    "/tmp/remote.txt",
	}

	result, err := sshCopyHandler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError, "upload with missing local file should fail")
	// exit 66 (EX_NOINPUT) 应用于本地文件缺失
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "exit 66", "should propagate EX_NOINPUT (66) for missing local file")
}

// ============================================================
// ssh_hostkey_* tool tests
// ============================================================

func TestMCPHostkeyListTool_Schema(t *testing.T) {
	tool := sshHostkeyListTool()
	assert.Equal(t, "ssh_hostkey_list", tool.Name)
	assert.NotEmpty(t, tool.Description)
}

func TestMCPHostkeyShowTool_RequiresHost(t *testing.T) {
	tool := sshHostkeyShowTool()
	assert.Contains(t, tool.InputSchema.Required, "host")
}

func TestMCPHostkeyShowHandler_MissingHost(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_hostkey_show"
	req.Params.Arguments = map[string]any{}

	result, err := sshHostkeyShowHandler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "host")
}

func TestMCPHostkeyRemoveTool_RequiresHost(t *testing.T) {
	tool := sshHostkeyRemoveTool()
	assert.Contains(t, tool.InputSchema.Required, "host")
}

func TestMCPHostkeyRemoveHandler_MissingHost(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_hostkey_remove"
	req.Params.Arguments = map[string]any{}

	result, err := sshHostkeyRemoveHandler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "host")
}

func TestMCPHostkeyUpdateTool_RequiresHostAndNode(t *testing.T) {
	tool := sshHostkeyUpdateTool()
	assert.Contains(t, tool.InputSchema.Required, "host")
	assert.Contains(t, tool.InputSchema.Required, "node")
}

func TestMCPHostkeyUpdateHandler_MissingHost(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_hostkey_update"
	req.Params.Arguments = map[string]any{
		"node": "some-node",
	}

	result, err := sshHostkeyUpdateHandler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "host")
}

func TestMCPHostkeyUpdateHandler_MissingNode(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_hostkey_update"
	req.Params.Arguments = map[string]any{
		"host": "some-host",
	}

	result, err := sshHostkeyUpdateHandler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "node")
}

func TestMCPHostkeyAuditTool_Schema(t *testing.T) {
	tool := sshHostkeyAuditTool()
	assert.Equal(t, "ssh_hostkey_audit", tool.Name)
	// check_config 应该是 bool
	cc, ok := tool.InputSchema.Properties["check_config"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "boolean", cc["type"])
}

// ============================================================
// ssh_completions tool tests
// ============================================================

func TestMCPCompletionsTool_Schema(t *testing.T) {
	tool := sshCompletionsTool()
	assert.Equal(t, "ssh_completions", tool.Name)
	assert.NotEmpty(t, tool.Description)
	// subcommand 是 string (可选)
	sub, ok := tool.InputSchema.Properties["subcommand"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", sub["type"])
	// flat 是 bool
	flat, ok := tool.InputSchema.Properties["flat"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "boolean", flat["type"])
}

func TestMCPCompletionsHandler_FullSchema(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_completions"
	req.Params.Arguments = map[string]any{}

	result, err := sshCompletionsHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// 验证输出是合法 JSON schema
	text := result.Content[0].(mcp.TextContent).Text
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &schema))
	assert.Equal(t, "1.0", schema["schema_version"])
	assert.NotNil(t, schema["commands"])
}

func TestMCPCompletionsHandler_FilteredSubcommand(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_completions"
	req.Params.Arguments = map[string]any{
		"subcommand": "hostkey",
	}

	result, err := sshCompletionsHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &schema))
	cmds := schema["commands"].([]any)
	require.Len(t, cmds, 1)
	assert.Equal(t, "hostkey", cmds[0].(map[string]any)["name"])
}

func TestMCPCompletionsHandler_FlatMode(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "ssh_completions"
	req.Params.Arguments = map[string]any{
		"flat": true,
	}

	result, err := sshCompletionsHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	// flat mode 返回数组, 不是对象
	var arr []any
	require.NoError(t, json.Unmarshal([]byte(text), &arr))
	assert.Greater(t, len(arr), 10, "flat mode should expose all commands as array")
}

// ============================================================
// 服务器注册检查: 所有工具都已注册
// ============================================================

func TestMCPServer_AllToolsRegistered(t *testing.T) {
	expected := []string{
		"ssh_list_nodes",
		"ssh_exec",
		"ssh_copy",
		"ssh_hostkey_list",
		"ssh_hostkey_show",
		"ssh_hostkey_remove",
		"ssh_hostkey_update",
		"ssh_hostkey_audit",
		"ssh_completions",
	}

	tools := map[string]bool{
		"ssh_list_nodes":     true,
		"ssh_exec":           true,
		"ssh_copy":           true,
		"ssh_hostkey_list":   true,
		"ssh_hostkey_show":   true,
		"ssh_hostkey_remove": true,
		"ssh_hostkey_update": true,
		"ssh_hostkey_audit":  true,
		"ssh_completions":    true,
	}

	// 验证数量
	assert.Equal(t, len(expected), len(tools))

	// 验证每个工具都能成功调用 (不会因 schema 问题报错)
	for _, name := range expected {
		assert.True(t, tools[name], "%s should be registered", name)
	}
}

// ============================================================
// 回归测试: MCP server 偶发断开修复
// ============================================================

// TestRunSubcommandCaptured_ExitErrorSurvives 验证内层 RunE 返回 ExitError 时
// runSubcommandCaptured 把错误返回给 caller 而不是调用 os.Exit 杀掉 MCP 进程。
//
// 历史 bug: cmd/exec_cmd.go 旧版对 result.ExitCode > 0 直接 os.Exit(result.ExitCode),
// 通过 runSubcommandCaptured 重入时整个 mysshw mcp 进程被杀, AI 客户端 EOF 断开。
// 修复: 改 return WrapExit(result.ExitCode, nil)。
func TestRunSubcommandCaptured_ExitErrorSurvives(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	// 触发 ExitError 路径: list 加载不存在的 config → WrapExit(ExitConfig=78)
	out, err := runSubcommandCaptured(context.Background(), listCmd,
		[]string{"list", "-o", "json", "--cfg", "/nonexistent/path/that/does/not/exist.toml"})

	// 必须返回 error 而非让 process 死亡
	require.Error(t, err, "runSubcommandCaptured 应返回 ExitError 包装的 error 而不是 os.Exit 杀进程")
	assert.Contains(t, err.Error(), "exit 78",
		"ExitError code 78 (EX_CONFIG) 应被透传到 error message")
	// 进程存活 = 测试还能继续跑 = os.Exit 没被调用
	assert.NotNil(t, out, "已写到 buffer 的输出应仍能返回 (即使 inner cmd 失败)")
}

// TestPrintNodeTable_NilWriterNoOp 验证 nil writer 时不写到 os.Stdout,
// 否则 MCP stdio 模式下会污染 JSON-RPC 流导致客户端断开。
func TestPrintNodeTable_NilWriterNoOp(t *testing.T) {
	// 重定向 os.Stdout 捕获是否被写入
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	// 调用方传 nil writer — 不应触达 os.Stdout
	res := printNodeTable(nil, []nodeSummary{{Group: "G", Name: "n1"}})
	w.Close()

	// 读 pipe: 如果 fallback 到 os.Stdout, 这里会读到内容
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	r.Close()

	assert.NoError(t, res)
	assert.Equal(t, 0, n, "nil writer 不应写到 os.Stdout (会污染 MCP JSON-RPC 流)")
}

// TestRunSubcommandCaptured_Concurrent 验证 mcpSubcommandMu 正确串行化并发调用.
//
// 历史 bug: 并发 MCP tool call 同时修改 rootCmd.SetArgs/SetOut/SetErr/SetContext
// 后调 rootCmd.Execute(), 全局状态互相撕扯导致 race / panic / 输出串台.
// 修复: 加 sync.Mutex.
//
// 测试策略: 全部 goroutine 共享同一 config (避免 viper.AddConfigPath 累加污染).
// 加 -race 跑测试时, Go runtime 会检测 rootCmd / mcpSubcommandMu 周围的真 race.
// 不验证 per-goroutine 输出内容 — 锁保证串行, 内容正确性靠单线程测试覆盖.
func TestRunSubcommandCaptured_Concurrent(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "G"

[[nodes.ssh]]
name = "shared-node"
host = "10.0.0.1"
`)

	const N = 10
	var wg sync.WaitGroup
	errs := make(chan error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs <- fmt.Errorf("goroutine %d panicked: %v", idx, r)
				}
			}()

			out, err := runSubcommandCaptured(context.Background(), listCmd,
				[]string{"list", "-o", "json", "--cfg", cfgPath})
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: %w", idx, err)
				return
			}
			if !strings.Contains(out, "shared-node") {
				errs <- fmt.Errorf("goroutine %d: missing shared-node in output: %s", idx, out)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		t.Errorf("%v", e)
	}
}
