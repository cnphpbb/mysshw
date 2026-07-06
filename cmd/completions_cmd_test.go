package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// 测试策略:
//   不通过 cobra Execute() 调用 completionsCmd, 而是直接调用
//   buildSchema() / flattenCommands() / filterCommandsByPath() 等纯函数。
//
//   原因: cobra Execute() 会处理 parent 命令的 Run hook, 触发 rootCmd 的
//   RunSSH → config.LoadViperConfig, 测试环境没有 ~/.mysshw.toml 会失败。
//   我们的目标是测试 schema 构建逻辑, 而不是 cobra 命令路由。
// ============================================================

// runSchema 是测试辅助函数: 直接构建 schema (跳过 cobra Execute)
func runSchema(t *testing.T, args ...string) CLIJSONSchema {
	t.Helper()
	old := completionsSubCmd
	defer func() { completionsSubCmd = old }()
	completionsSubCmd = ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		completionsSubCmd = args[0]
	}
	return buildSchema()
}

// TestBuildSchema_HasExpectedFields 验证顶层 schema 结构
func TestBuildSchema_HasExpectedFields(t *testing.T) {
	// 测试环境下 SetVersion 未调用, 手动设置避免空检查失败
	SetVersion("v0.0.0-test", "test-build", "2026-01-01T00:00:00Z", "go1.23")

	s := runSchema(t)
	assert.Equal(t, "1.0", s.SchemaVersion)
	assert.NotEmpty(t, s.GeneratedAt)
	assert.Equal(t, "mysshw", s.Application.Name)
	assert.Equal(t, "v0.0.0-test", s.Application.Version)
	assert.Equal(t, "go1.23", s.Application.GoVersion)
	assert.NotEmpty(t, s.ExitCodes)
	assert.GreaterOrEqual(t, len(s.ExitCodes), 10, "should expose all BSD sysexits codes")
}

// TestBuildSchema_DiscoversAllTopLevelCommands 验证能发现所有顶级命令
func TestBuildSchema_DiscoversAllTopLevelCommands(t *testing.T) {
	s := runSchema(t)
	names := make(map[string]bool)
	for _, c := range s.Commands {
		names[c.Name] = true
	}
	for _, expected := range []string{"list", "exec", "copy", "tunnel", "hostkey",
		"sync", "init", "version", "yml", "completions"} {
		assert.True(t, names[expected], "missing command %q in schema (got %v)", expected, names)
	}
}

// TestBuildSchema_TunnelSubcommands 验证 tunnel 子命令被展开
func TestBuildSchema_TunnelSubcommands(t *testing.T) {
	s := runSchema(t)
	tunnel := findCommand(s.Commands, "tunnel")
	require.NotNil(t, tunnel, "tunnel not found")
	subs := subCommandNames(tunnel)
	for _, expected := range []string{"local", "reverse", "dynamic"} {
		assert.True(t, subs[expected], "missing tunnel.%s (got %v)", expected, subs)
	}
	assert.True(t, tunnel.HasSubcommands)
	// 注意: tunnelCmd 有 RunE = cmd.Help(), 技术上是 leaf, 但语义上是 router。
	// is_leaf 反映的是「是否有 Run/RunE 函数」, 这是正确的。
}

// TestBuildSchema_HostkeySubcommands 验证 hostkey 子命令
func TestBuildSchema_HostkeySubcommands(t *testing.T) {
	s := runSchema(t)
	hostkey := findCommand(s.Commands, "hostkey")
	require.NotNil(t, hostkey)
	subs := subCommandNames(hostkey)
	for _, expected := range []string{"list", "show", "remove", "update", "audit"} {
		assert.True(t, subs[expected], "missing hostkey.%s", expected)
	}
}

// TestBuildSchema_LeafCommands 验证 leaf 命令有 IsLeaf=true
func TestBuildSchema_LeafCommands(t *testing.T) {
	s := runSchema(t)
	// list/exec/copy/version/yml 都是 leaf
	leafs := []string{"list", "exec", "copy", "version", "yml", "init"}
	for _, name := range leafs {
		c := findCommand(s.Commands, name)
		require.NotNil(t, c, "%s not found", name)
		assert.True(t, c.IsLeaf, "%s should be a leaf", name)
		assert.False(t, c.HasSubcommands, "%s should not have subcommands", name)
	}
}

// TestBuildSchema_FlagsHaveMetadata 验证 flag 元数据完整
func TestBuildSchema_FlagsHaveMetadata(t *testing.T) {
	s := runSchema(t)
	exec := findCommand(s.Commands, "exec")
	require.NotNil(t, exec)

	// --timeout: duration 类型, 默认 30s
	timeout := findFlag(exec.Flags, "timeout")
	require.NotNil(t, timeout, "exec --timeout not found")
	assert.Equal(t, "duration", timeout.Type)
	assert.Equal(t, "30s", timeout.Default)

	// --no-pty: bool 类型, 默认 false
	noPty := findFlag(exec.Flags, "no-pty")
	require.NotNil(t, noPty, "exec --no-pty not found")
	assert.Equal(t, "bool", noPty.Type)

	// --dry-run: bool
	dryRun := findFlag(exec.Flags, "dry-run")
	require.NotNil(t, dryRun, "exec --dry-run not found")
}

// TestBuildSchema_PersistentFlagMarked 验证 persistent flag
func TestBuildSchema_PersistentFlagMarked(t *testing.T) {
	s := runSchema(t)
	exec := findCommand(s.Commands, "exec")
	require.NotNil(t, exec)

	cfg := findFlag(exec.Flags, "cfg")
	require.NotNil(t, cfg)
	assert.True(t, cfg.Persistent, "--cfg should be persistent")
	assert.Equal(t, "c", cfg.Shorthand)

	output := findFlag(exec.Flags, "output")
	require.NotNil(t, output)
	assert.True(t, output.Persistent, "--output should be persistent")
}

// TestBuildSchema_FilterBySubcommand 验证按子命令筛选
func TestBuildSchema_FilterBySubcommand(t *testing.T) {
	s := runSchema(t, "tunnel")
	require.Len(t, s.Commands, 1, "filtered should have 1 command")
	assert.Equal(t, "tunnel", s.Commands[0].Name)
	assert.GreaterOrEqual(t, len(s.Commands[0].Subcommands), 3)

	s2 := runSchema(t, "hostkey")
	require.Len(t, s2.Commands, 1)
	assert.Equal(t, "hostkey", s2.Commands[0].Name)
}

// TestBuildSchema_FilterByDeepPath 验证深层路径筛选
func TestBuildSchema_FilterByDeepPath(t *testing.T) {
	// 直接测试 filterCommandsByPath 的深层路径能力
	tree := []CommandSchema{
		{
			Name: "tunnel",
			Subcommands: []CommandSchema{
				{Name: "local"},
				{Name: "reverse"},
			},
		},
	}
	filtered := filterCommandsByPath(tree, []string{"tunnel", "local"})
	require.Len(t, filtered, 1)
	assert.Equal(t, "local", filtered[0].Name)

	// 不存在的路径返回空
	filtered = filterCommandsByPath(tree, []string{"tunnel", "dynamic"})
	assert.Empty(t, filtered)
}

// TestFlattenCommands 验证展平函数
func TestFlattenCommands(t *testing.T) {
	tree := []CommandSchema{
		{
			Name: "a",
			Subcommands: []CommandSchema{
				{Name: "a1"},
				{
					Name: "a2",
					Subcommands: []CommandSchema{
						{Name: "a2x"},
					},
				},
			},
		},
		{Name: "b"},
	}
	flat := flattenCommands(tree)
	assert.Equal(t, 5, len(flat), "should flatten a/a1/a2/a2x/b → 5 entries")
	for _, c := range flat {
		assert.Nil(t, c.Subcommands, "flattened should have nil subcommands")
	}
}

// TestFilterCommandsByPath 验证路径筛选边界
func TestFilterCommandsByPath(t *testing.T) {
	tree := []CommandSchema{
		{Name: "a", Subcommands: []CommandSchema{{Name: "a1"}, {Name: "a2"}}},
		{Name: "b"},
	}

	t.Run("nil path returns all", func(t *testing.T) {
		got := filterCommandsByPath(tree, nil)
		assert.Equal(t, len(tree), len(got))
	})

	t.Run("empty path returns all", func(t *testing.T) {
		got := filterCommandsByPath(tree, []string{})
		assert.Equal(t, len(tree), len(got))
	})

	t.Run("exact match", func(t *testing.T) {
		got := filterCommandsByPath(tree, []string{"a"})
		require.Len(t, got, 1)
		assert.Equal(t, "a", got[0].Name)
	})

	t.Run("deep match", func(t *testing.T) {
		got := filterCommandsByPath(tree, []string{"a", "a1"})
		require.Len(t, got, 1)
		assert.Equal(t, "a1", got[0].Name)
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := filterCommandsByPath(tree, []string{"nonexistent"})
		assert.Empty(t, got)
	})
}

// TestInferArgsPolicy 验证 args policy 推断
func TestInferArgsPolicy(t *testing.T) {
	// nil → arbitrary
	assert.Equal(t, "arbitrary", inferArgsPolicy(nil))

	// 自定义函数 → custom
	custom := func(a, b string) {}
	assert.Equal(t, "custom", inferArgsPolicy(custom))
}

// TestCollectFlags_NoHiddenFlags 验证 hidden flag 被过滤
func TestCollectFlags_NoHiddenFlags(t *testing.T) {
	// 在 rootCmd 注册一个临时 hidden flag
	rootCmd.PersistentFlags().Bool("test-completions-hidden", false, "")
	flag := rootCmd.PersistentFlags().Lookup("test-completions-hidden")
	require.NotNil(t, flag)
	flag.Hidden = true

	flags := collectFlags(rootCmd)
	for _, f := range flags {
		assert.NotEqual(t, "test-completions-hidden", f.Name,
			"hidden flag should not be in schema")
	}

	// 清理
	rootCmd.PersistentFlags().MarkHidden("test-completions-hidden")
	flag.Hidden = false
	rootCmd.PersistentFlags().Lookup("test-completions-hidden").Hidden = false
}

// TestBuildSchema_ExitCodesConsistent 验证 exit codes 与 errors.go 一致
func TestBuildSchema_ExitCodesConsistent(t *testing.T) {
	s := runSchema(t)

	codes := make(map[int]bool)
	for _, c := range s.ExitCodes {
		codes[c.Code] = true
	}

	// 关键的 BSD sysexits 必须存在
	for _, expected := range []int{0, 1, 2, 64, 65, 66, 70, 73, 74, 78, 124, 126, 127, 255} {
		assert.True(t, codes[expected], "exit code %d should be in schema", expected)
	}

	// 验证名称和 errors.go 中的常量含义一致
	expectedNames := map[int]string{
		0:   "OK",
		64:  "EX_USAGE",
		78:  "EX_CONFIG",
		124: "EX_TIMEOUT",
		255: "EX_SSH_ERROR",
	}
	for _, c := range s.ExitCodes {
		if want, ok := expectedNames[c.Code]; ok {
			assert.Equal(t, want, c.Name, "code %d name mismatch", c.Code)
		}
	}
}

// TestBuildSchema_JSONRoundTrip 验证 JSON 可序列化/反序列化
func TestBuildSchema_JSONRoundTrip(t *testing.T) {
	s := runSchema(t)
	data, err := json.Marshal(s)
	require.NoError(t, err)

	var out CLIJSONSchema
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, s.SchemaVersion, out.SchemaVersion)
	assert.Equal(t, len(s.Commands), len(out.Commands))
	assert.Equal(t, len(s.ExitCodes), len(out.ExitCodes))
}

// TestBuildSchema_AllFlagsHaveNames 验证所有 flag 都有非空 name
func TestBuildSchema_AllFlagsHaveNames(t *testing.T) {
	s := runSchema(t)

	var check func(c CommandSchema, path string)
	check = func(c CommandSchema, path string) {
		fullPath := path + " " + c.Name
		for _, f := range c.Flags {
			assert.NotEmpty(t, f.Name, "flag with empty name in %s", fullPath)
			assert.NotEmpty(t, f.Type, "flag %s in %s has empty type", f.Name, fullPath)
		}
		for _, sub := range c.Subcommands {
			check(sub, fullPath)
		}
	}
	for _, c := range s.Commands {
		check(c, "")
	}
}

// ============================================================
// Helpers
// ============================================================

func findCommand(cmds []CommandSchema, name string) *CommandSchema {
	for i := range cmds {
		if cmds[i].Name == name {
			return &cmds[i]
		}
	}
	return nil
}

func findFlag(flags []FlagSchema, name string) *FlagSchema {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}

func subCommandNames(c *CommandSchema) map[string]bool {
	out := make(map[string]bool, len(c.Subcommands))
	for _, sub := range c.Subcommands {
		out[sub.Name] = true
	}
	return out
}
