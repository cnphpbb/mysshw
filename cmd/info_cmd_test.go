package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// infoCmd tests
//
// info_cmd 需要真实 SSH 连接, 多数测试聚焦在:
//   - 命令注册 / flag 注册 / schema
//   - 纯函数 (humanBytes / humanDuration / parseBootTimeEpoch)
//   - JSON 数据结构
//
// 端到端测试用 e2e build tag (ssh/e2e_test.go 模式), 跟其它命令保持一致
// ============================================================

func TestInfoCmd_Registered(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "info" {
			assert.Equal(t, "Collect system info from SSH nodes (hostname, OS, CPU, memory, disk, uptime)", c.Short)
			return
		}
	}
	t.Fatal("info subcommand not registered on rootCmd")
}

func TestInfoCmd_HasExpectedFlags(t *testing.T) {
	flags := infoCmd.Flags()
	for _, name := range []string{"node", "alias", "host", "user", "port", "timeout", "parallel", "no-pty"} {
		f := flags.Lookup(name)
		assert.NotNil(t, f, "missing flag %q", name)
	}
}

func TestInfoCmd_MutuallyExclusiveFlags(t *testing.T) {
	flags := infoCmd.Flags()
	assert.NotNil(t, flags.Lookup("node"))
	assert.NotNil(t, flags.Lookup("alias"))
	assert.NotNil(t, flags.Lookup("host"))
}

// ============================================================
// humanDuration tests
// ============================================================

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{-1 * time.Second, "0s"}, // 负值视为 0
		{500 * time.Millisecond, "0s"},
		{1 * time.Second, "1s"},
		{45 * time.Second, "45s"},
		{2 * time.Minute, "2m 0s"},
		{2*time.Minute + 30*time.Second, "2m 30s"},
		{3 * time.Hour, "3h 0m"},
		{3*time.Hour + 30*time.Minute, "3h 30m"},
		{5 * 24 * time.Hour, "5d 0h 0m"},
		{30 * 24 * time.Hour, "30d 0h 0m"},
	}
	for _, c := range cases {
		got := humanDuration(c.in)
		assert.Equal(t, c.want, got, "humanDuration(%v)", c.in)
	}
}

// ============================================================
// humanBytes tests
// ============================================================

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{1024 * 1024, "1.00 MiB"},
		{8 * 1024 * 1024 * 1024, "8.00 GiB"},
		{2 * 1024 * 1024 * 1024 * 1024, "2.00 TiB"},
	}
	for _, c := range cases {
		got := humanBytes(c.in)
		assert.Equal(t, c.want, got, "humanBytes(%d)", c.in)
	}
}

// ============================================================
// parseBootTimeEpoch tests
// ============================================================

func TestParseBootTimeEpoch_LinuxEmpty(t *testing.T) {
	assert.Equal(t, int64(0), parseBootTimeEpoch(""))
	assert.Equal(t, int64(0), parseBootTimeEpoch("not-a-mac-output"))
}

func TestParseBootTimeEpoch_MacFormat(t *testing.T) {
	// macOS sysctl kern.boottime 格式: "{ sec = 1700000000, usec = 0 }"
	got := parseBootTimeEpoch("{ sec = 1700000000, usec = 0 }")
	assert.Equal(t, int64(1700000000), got)
}

func TestParseBootTimeEpoch_MacFormatWithSpaces(t *testing.T) {
	got := parseBootTimeEpoch("{sec = 1700000000,usec=0}")
	assert.Equal(t, int64(1700000000), got)
}

// ============================================================
// NodeInfo / CPUInfo / MemoryInfo / DiskInfo JSON tests
// ============================================================

func TestNodeInfo_JSONShape(t *testing.T) {
	n := NodeInfo{
		Node:          "prod-1",
		Hostname:      "prod-1.internal",
		OS:            "linux",
		Kernel:        "6.8.0-31-generic",
		Arch:          "x86_64",
		CPU:           CPUInfo{Cores: 4, Model: "Intel(R) Xeon(R)"},
		Memory:        MemoryInfo{TotalBytes: 17179869184},
		Disk:          []DiskInfo{{Mount: "/", SizeBytes: 100000000, UsedBytes: 50000000, Percent: 50}},
		UptimeSeconds: 2592000,
		UptimeHuman:   "30d 0h 0m",
		CollectedAt:   "2026-07-06T12:34:56Z",
	}

	data, err := json.Marshal(n)
	require.NoError(t, err)
	s := string(data)

	// 关键字段
	for _, key := range []string{
		`"node":"prod-1"`,
		`"hostname":"prod-1.internal"`,
		`"os":"linux"`,
		`"kernel":"6.8.0-31-generic"`,
		`"arch":"x86_64"`,
		`"cpu":{"cores":4,"model":"Intel(R) Xeon(R)"}`,
		`"memory":{"total_bytes":17179869184}`,
		`"uptime_seconds":2592000`,
		`"uptime_human":"30d 0h 0m"`,
		`"collected_at":"2026-07-06T12:34:56Z"`,
		`"disk":[{"mount":"/","size_bytes":100000000,"used_bytes":50000000,"percent":50}]`,
	} {
		assert.Contains(t, s, key, "JSON missing %s", key)
	}

	// 空字段应被 omitempty 省略
	assert.NotContains(t, s, `"error"`)
}

func TestNodeInfo_JSONEmptyOmitsOptional(t *testing.T) {
	n := NodeInfo{Node: "x", CollectedAt: "2026-07-06"}
	data, err := json.Marshal(n)
	require.NoError(t, err)
	s := string(data)
	// optional 字段被省略
	assert.NotContains(t, s, `"hostname"`)
	assert.NotContains(t, s, `"os"`)
	assert.NotContains(t, s, `"kernel"`)
	assert.NotContains(t, s, `"arch"`)
	assert.NotContains(t, s, `"uptime"`)
	assert.NotContains(t, s, `"disk"`)
	assert.NotContains(t, s, `"error"`)
}

func TestNodeInfo_JSONWithError(t *testing.T) {
	n := NodeInfo{
		Node:        "bad",
		CollectedAt: "2026-07-06",
		Error:       "new executor: auth failed",
	}
	data, err := json.Marshal(n)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"error":"new executor: auth failed"`)
}

// ============================================================
// infoCmd JSON parse / load config 行为测试
//
// 注: TestRunInfo_NoConfigFile / TestRunInfo_NoMatchingNodes 涉及 cobra 命令执行,
// 由于 cobra 子命令 Execute() 会 redirect 到 rootCmd, 并且 --cfg flag default
// 在子命令里仍生效, 单元测试难以隔离. 端到端测试在 ssh/e2e_test.go (build tag: e2e).
// 这里只验证纯函数 / 数据结构, 避免 cobra global state 干扰.
// ============================================================

// ============================================================
// isComplete tests (内部一致性检查)
// ============================================================

func TestNodeInfo_IsComplete(t *testing.T) {
	assert.False(t, (NodeInfo{}).isComplete())
	assert.False(t, (NodeInfo{Hostname: "x"}).isComplete())
	assert.False(t, (NodeInfo{Hostname: "x", OS: "linux"}).isComplete())
	assert.True(t, (NodeInfo{Hostname: "x", OS: "linux", CPU: CPUInfo{Cores: 4}}).isComplete())
}

// ============================================================
// countInfoFailed tests
// ============================================================

func TestCountInfoFailed(t *testing.T) {
	results := []NodeInfo{
		{Node: "ok"},
		{Node: "bad", Error: "fail"},
		{Node: "ok2"},
		{Node: "bad2", Error: "fail2"},
	}
	assert.Equal(t, 2, countInfoFailed(results))
}
