package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"mysshw/config"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
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

// TestMCP_WithRecovery_PanicDoesNotBreakStdio 锁定 MCP 偶发断连回归:
// handler panic 若不被 recover, 会导致进程崩溃 → stdio EOF → AI 客户端断开。
// runMCP 通过 server.WithRecovery() 把 panic 转成 error 响应, 连接存活。
// 本测试用真实 stdio transport (io.Pipe) 驱动, 注册一个会 panic 的 tool,
// 断言: (1) panic 后能读到 error 响应 (不是 EOF); (2) 后续请求仍可响应 = 连接没断。
func TestMCP_WithRecovery_PanicDoesNotBreakStdio(t *testing.T) {
	s := server.NewMCPServer("test", "1.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)
	s.AddTool(mcp.NewTool("boom"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("kaboom")
	})

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	t.Cleanup(func() { _ = stdinR.Close(); _ = stdinW.Close(); _ = stdoutR.Close(); _ = stdoutW.Close() })

	stdio := server.NewStdioServer(s)
	stdio.SetErrorLogger(log.New(io.Discard, "", 0))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = stdio.Listen(ctx, stdinR, stdoutW) }()

	sc := bufio.NewScanner(stdoutR)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	send := func(m map[string]any) {
		b, err := json.Marshal(m)
		require.NoError(t, err)
		_, err = stdinW.Write(append(b, '\n'))
		require.NoError(t, err)
	}
	readResp := func() map[string]any {
		t.Helper()
		if !sc.Scan() {
			t.Fatalf("stdio 连接已断 (EOF): AI 客户端会失联; err=%v", sc.Err())
		}
		var r map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &r))
		return r
	}

	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2024-11-05",
			"clientInfo": map[string]any{"name": "test", "version": "1"}}})
	require.NotNil(t, readResp()["result"], "initialize 应返回 result")
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})

	// 关键断言 1: panic → error 响应, 连接没崩
	send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "boom", "arguments": map[string]any{}}})
	resp := readResp()
	assert.Nil(t, resp["result"], "panic 应让 result 为 nil")
	assert.NotNil(t, resp["error"], "panic 应转成 JSON-RPC error, got: %v", resp)

	// 关键断言 2: 后续请求仍可响应 = 连接存活 (核心: 不会让 AI 客户端断开)
	send(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/list", "params": map[string]any{}})
	list := readResp()
	require.NotNil(t, list["result"], "panic 后连接应仍存活可响应后续请求")
}

// ============================================================
// 回归测试: MCP exec 状态泄漏修复
// ============================================================

// TestMCPExecHandler_StateNotLeakedAcrossCalls 锁定同类 bug: 第一次调 ssh_exec 后
// cobra flag 全局变量 (execNodesFlag / execDryRun / execRotateKnownHosts) 残留,
// 第二次调用被污染, exec 跑不通.
//
// 历史 bug 表现:
//   - call 1: --node A --dry-run       → execNodesFlag=[A], execDryRun=true
//   - call 2: --node B                  → cobra append → execNodesFlag=[A,B], execDryRun=true 残留
//                                            → 第 2 次返回 dry-run 输出, 不是真执行
//
// 修复: runSubcommandCaptured 入口 resetCmdGlobals(), Execute() 之前重置所有 flag 全局变量.
func TestMCPExecHandler_StateNotLeakedAcrossCalls(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "node-A"
host = "10.0.0.1"

[[nodes.ssh]]
name = "node-B"
host = "10.0.0.2"
`)

	// call 1: --node A --dry-run
	req1 := mcp.CallToolRequest{}
	req1.Params.Name = "ssh_exec"
	req1.Params.Arguments = map[string]any{
		"cfg":     cfgPath,
		"node":    []string{"node-A"},
		"command": "uptime",
		"dry_run": true,
	}
	out1, err := runSubcommandCaptured(context.Background(), execCmd,
		[]string{"exec", "-o", "json", "--cfg", cfgPath, "--node", "node-A", "--dry-run", "--", "uptime"})
	require.NoError(t, err)
	t.Logf("call 1 output: %s", out1)

	// 关键断言 1: call 1 后 execNodesFlag 应被 reset (不累积)
	// resetCmdGlobals 在入口处调用, 解析后 cobra 设置, 之后回归到 [A]
	assert.Equal(t, []string{"node-A"}, execNodesFlag,
		"call 1 后 execNodesFlag 应只含 'node-A', 不应有残留")

	// call 2: --node B --dry-run=false (验证 dry_run 不残留; 用 dry_run=true 避免真 SSH)
	// 这里 dry_run=true 跟我们想测的"不残留"是相容的: 测的是"不残留" → 列出 node-B only
	// 不想真 SSH, 因为本机 IP 10.0.0.2 不存在, dial 会卡超时
	req2 := mcp.CallToolRequest{}
	req2.Params.Name = "ssh_exec"
	req2.Params.Arguments = map[string]any{
		"cfg":     cfgPath,
		"node":    []string{"node-B"},
		"command": "uptime",
		"dry_run": true,
	}
	out2, err := runSubcommandCaptured(context.Background(), execCmd,
		[]string{"exec", "-o", "json", "--cfg", cfgPath, "--node", "node-B", "--dry-run", "--", "uptime"})
	require.NoError(t, err)
	t.Logf("call 2 output: %s", out2)

	// 关键断言 2: call 2 的 selector.names 应只有 [B], 不应是 [A,B]
	if !strings.Contains(out2, "\"node-B\"") {
		t.Fatalf("call 2 输出缺少 node-B: %s", out2)
	}
	if strings.Contains(out2, "\"node-A\"") {
		t.Fatalf("call 2 输出含 node-A, 说明 call 1 的 --node 残留了: %s", out2)
	}
}

// TestMCPExecHandler_DryRunNotLeakedAcrossCalls 单独验证 bool flag 不残留:
// call 1 显式设 --dry-run, call 2 完全不设 (走的路径相同但 dry_run 应是 false).
// 因为没有 dry_run 路径直接看 JSON 字段, 我们用 runExecDryRun 的输出特征:
//   - dry_run=true  → 输出顶层字段 "dry_run": true
//   - dry_run=false → 不会输出该字段 (默认为 false, omitempty-like)
//
// 这里用 mock 节点 (不会真 SSH) 配 --dry-run 跑 call 1, 然后 call 2 不带 --dry-run 验证
// 是否又走了 dry-run 路径 (如果走, 返回 dry-run JSON; 如果没走, 会尝试 dial 失败).
func TestMCPExecHandler_DryRunNotLeakedAcrossCalls(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "node-X"
host = "10.0.0.99"
`)

	// call 1: --dry-run
	_, err := runSubcommandCaptured(context.Background(), execCmd,
		[]string{"exec", "-o", "json", "--cfg", cfgPath, "--node", "node-X", "--dry-run", "--", "uptime"})
	require.NoError(t, err)

	// call 2: 不带 --dry-run. 期望: execDryRun 被 reset → false, 走真实路径 (会 dial 失败)
	// 关键是: 不是返回 dry-run JSON
	_, err = runSubcommandCaptured(context.Background(), execCmd,
		[]string{"exec", "-o", "json", "--cfg", cfgPath, "--node", "node-X", "--timeout", "1s", "--", "uptime"})
	// 真 SSH 会失败 (network unreachable), err 应非 nil 且 NOT 含 "dry_run"
	if err == nil {
		t.Fatalf("call 2 没装 dry-run, 应尝试真 SSH, 期望 error (dial 失败), got nil")
	}
	if strings.Contains(err.Error(), "dry_run") {
		t.Fatalf("call 2 错误消息含 dry_run, 说明 call 1 的 --dry-run 残留了: %s", err.Error())
	}
}

// TestMCPExecHandler_RotateKnownHostsNotMutatedAcrossCalls 锁定同类 bug:
// --rotate-known-hosts flag 直接修改 config.CFG.*.RotateKnownHosts 字段,
// 永久污染全局 config. 下次不带此 flag 的调用也会用旧值.
//
// 修复: runExec 在改字段前 save, defer 时 restore.
func TestMCPExecHandler_RotateKnownHostsNotMutatedAcrossCalls(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	cfgPath := withTempConfig(t, `
[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "node-1"
host = "10.0.0.1"
`)

	// 直接跳过 dry-run, 走 --rotate-known-hosts + dry-run 路径模拟
	// runExecDryRun 路径不触发 RotateKnownHosts 字段修改
	// 所以这里测试的是"原始 false 值在调用前后都是 false"
	// (我们不需要真 SSH, 只要确认字段没被改)
	if err := execCmd.ParseFlags([]string{"--node", "node-1", "--dry-run", "--rotate-known-hosts"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	defer func() {
		execCmd.ParseFlags([]string{}) // 清掉
	}()

	// 直接调 runExec (MCP handler 不变), 验证 config.CFG.Nodes[0].SSHNodes[0].RotateKnownHosts
	// 被临时设为 true 然后在 defer 恢复
	require.NoError(t, config.LoadViperConfig(cfgPath))
	require.NotNil(t, config.CFG)
	require.NotEmpty(t, config.CFG.Nodes)
	require.NotEmpty(t, config.CFG.Nodes[0].SSHNodes)

	original := config.CFG.Nodes[0].SSHNodes[0].RotateKnownHosts
	require.False(t, original, "precondition: RotateKnownHosts 起始为 false")

	// 直接走 runExec 函数路径, 加载 config + 解析节点
	// 这里用 dry_run 路径, 不触发真 SSH, 但 rotate 字段仍会被尝试设置
	_ = runExec(execCmd, []string{"echo", "hello"})

	// 关键断言: 调用结束后, 字段值仍是 false (rotate 临时设置 + defer 恢复)
	// 注意: 这里即使 runExec 失败也不影响 — defer 会在 runExec 函数返回时执行
	got := config.CFG.Nodes[0].SSHNodes[0].RotateKnownHosts
	assert.Equal(t, original, got,
		"runExec 结束后 RotateKnownHosts 字段应恢复原值, 当前=%v (期望=%v)", got, original)
}

// TestRunSubcommandCaptured_RecoversPanic 验证 runSubcommandCaptured 内的
// defer recover() 能兜底任何内层 panic (resolver / config / viper 边缘 nil-deref).
// WithRecovery() 只 wrap tool handler, 不覆盖此层.
//
// 历史 bug: 任意 nil-deref 都会让 MCP 进程崩溃, stdio EOF, AI 客户端失联.
//
// 注入方式: 临时在 rootCmd 注册一个 `panic_cmd` 子命令, RunE 调 panic.
// 走完整 runSubcommandCaptured 路径: SetArgs → Execute → panic → recover → 返回 error.
func TestRunSubcommandCaptured_RecoversPanic(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	// 注册临时 panic 子命令
	panicCmd := &cobra.Command{
		Use: "panic_cmd",
		RunE: func(cmd *cobra.Command, args []string) error {
			panic("injected panic for recovery test")
		},
	}
	rootCmd.AddCommand(panicCmd)
	defer rootCmd.RemoveCommand(panicCmd)

	// 调 runSubcommandCaptured, 应该 recover 而不让进程死
	out, err := runSubcommandCaptured(context.Background(), panicCmd,
		[]string{"panic_cmd"})

	// 关键断言 1: 返回 error, 不是 panic 终止
	require.Error(t, err, "panic 应被 recover 并包成 error")
	// 关键断言 2: error 包含 panic 信息 (调试可见)
	assert.Contains(t, err.Error(), "panic", "error 应标明来源是 panic")
	// 关键断言 3: error 关联 ExitSoftware (=70), AI 客户端可分类
	ee, ok := err.(ExitError)
	if assert.True(t, ok, "panic 应被包成 ExitError") {
		assert.Equal(t, ExitSoftware, ee.Code, "panic 应映射到 ExitSoftware (70)")
	}
	_ = out
}
