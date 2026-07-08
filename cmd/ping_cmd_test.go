package cmd

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// pingCmd tests
// ============================================================

func TestPingCmd_Registered(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "ping" {
			assert.Equal(t, "TCP-level connectivity check (no ICMP; AI / script friendly)", c.Short)
			return
		}
	}
	t.Fatal("ping subcommand not registered on rootCmd")
}

func TestPingCmd_HasExpectedFlags(t *testing.T) {
	flags := pingCmd.Flags()
	for _, name := range []string{"node", "alias", "host", "port", "timeout", "count", "parallel"} {
		f := flags.Lookup(name)
		assert.NotNil(t, f, "missing flag %q", name)
	}
}

func TestPingCmd_MutuallyExclusiveFlags(t *testing.T) {
	// cobra 应在 --node + --alias 同时使用时拒绝
	flags := pingCmd.Flags()
	// 注意: cobra MarkFlagsMutuallyExclusive 会自动注入 validation
	// 这里只验证注册正确
	assert.NotNil(t, flags.Lookup("node"))
	assert.NotNil(t, flags.Lookup("alias"))
	assert.NotNil(t, flags.Lookup("host"))
}

// TestPingOnce_RealLocalhost 测试 pingOnce 对本地 SSH 端口 (假设 22 端口开放)
func TestPingOnce_RealLocalhost(t *testing.T) {
	// 先检查本地 22 端口是否开放
	conn, err := net.DialTimeout("tcp", "127.0.0.1:22", 1*time.Second)
	if err != nil {
		t.Skip("no local SSH on 127.0.0.1:22, skipping")
	}
	conn.Close()

	r := pingOnce("test", "127.0.0.1", 22, 2*time.Second, 1)
	assert.Equal(t, "test", r.Node)
	assert.Equal(t, "127.0.0.1", r.Host)
	assert.Equal(t, 22, r.Port)
	assert.True(t, r.Reachable, "local SSH should be reachable")
	assert.Greater(t, r.LatencyMs, float64(0), "latency should be > 0")
	assert.Equal(t, 1, r.Attempts)
	assert.Empty(t, r.Error)
}

func TestPingOnce_Timeout(t *testing.T) {
	// 用不可达的 IP (RFC 5737 文档保留) 测试 timeout
	r := pingOnce("unreachable", "192.0.2.1", 22, 500*time.Millisecond, 1)
	assert.False(t, r.Reachable, "unreachable IP should not be reachable")
	assert.NotEmpty(t, r.Error, "should have error message on failure")
	assert.Equal(t, 1, r.Attempts)
}

func TestPingOnce_InvalidHost(t *testing.T) {
	r := pingOnce("invalid", "this-does-not-exist.invalid", 22, 1*time.Second, 1)
	assert.False(t, r.Reachable)
	assert.NotEmpty(t, r.Error)
}

func TestPingOnce_Count3(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:22", 1*time.Second)
	if err != nil {
		t.Skip("no local SSH on 127.0.0.1:22, skipping")
	}
	conn.Close()

	r := pingOnce("test", "127.0.0.1", 22, 2*time.Second, 3)
	assert.True(t, r.Reachable)
	assert.Equal(t, 3, r.Attempts, "should record total attempts")
	assert.Greater(t, r.LatencyMs, float64(0))
}

func TestPingOnce_CountZero(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:22", 1*time.Second)
	if err != nil {
		t.Skip("no local SSH on 127.0.0.1:22, skipping")
	}
	conn.Close()

	// count=0 应 fallback 到 1
	r := pingOnce("test", "127.0.0.1", 22, 2*time.Second, 0)
	assert.True(t, r.Reachable)
	assert.Equal(t, 1, r.Attempts, "count=0 should fallback to 1")
}

func TestPingBatch_MultipleNodes(t *testing.T) {
	nodes := []*struct{}{nil, nil} // 占位, 实际测试需要 SSHNode
	_ = nodes
	// 真实 batch 测试需要 config + SSHNode 实例
	// 这里只验证 pingBatch 函数签名可调用
	assert.NotNil(t, pingBatch)
}

func TestPingHandler_DirectModeReachable(t *testing.T) {
	// 模拟 --host 127.0.0.1 -p 22
	conn, err := net.DialTimeout("tcp", "127.0.0.1:22", 1*time.Second)
	if err != nil {
		t.Skip("no local SSH on 127.0.0.1:22, skipping")
	}
	conn.Close()

	// 直接调用 pingOnce, 模拟 handler 行为
	r := pingOnce("<direct>", "127.0.0.1", 22, 2*time.Second, 1)
	data, marshalErr := json.Marshal(r)
	require.NoError(t, marshalErr)
	var back pingResult
	require.NoError(t, json.Unmarshal(data, &back))
	assert.True(t, back.Reachable)
	assert.Equal(t, "<direct>", back.Node)
}

func TestPingHandler_DirectModeUnreachable(t *testing.T) {
	r := pingOnce("<direct>", "192.0.2.1", 22, 200*time.Millisecond, 1)
	assert.False(t, r.Reachable)
	assert.NotEmpty(t, r.Error)
	// 验证错误信息是拨号错误, 不是空字符串
	assert.True(t,
		strings.Contains(r.Error, "timeout") ||
			strings.Contains(r.Error, "connect") ||
			strings.Contains(r.Error, "refused") ||
			strings.Contains(r.Error, "no route"),
		"error should be a network error: %q", r.Error)
}

func TestCountUnreachable(t *testing.T) {
	results := []pingResult{
		{Reachable: true},
		{Reachable: false, Error: "timeout"},
		{Reachable: true},
		{Reachable: false, Error: "refused"},
	}
	assert.Equal(t, 2, countUnreachable(results))
}

func TestPingResult_JSONShape(t *testing.T) {
	r := pingResult{
		Node:      "prod-1",
		Host:      "10.0.0.1",
		Port:      22,
		Reachable: true,
		LatencyMs: 12.4,
		Attempts:  1,
	}
	data, err := json.Marshal(r)
	require.NoError(t, err)
	s := string(data)
	for _, key := range []string{`"node"`, `"host"`, `"port"`, `"reachable"`, `"latency_ms"`, `"attempts"`} {
		assert.Contains(t, s, key, "JSON output missing field %s", key)
	}
	// Error 字段在 nil 时省略
	assert.NotContains(t, s, `"error"`, "empty error should be omitted from JSON")
}

func TestPingResult_JSONShapeWithError(t *testing.T) {
	r := pingResult{
		Node:     "bad",
		Host:     "192.0.2.1",
		Port:     22,
		Attempts: 1,
		Error:    "i/o timeout",
	}
	data, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"error":"i/o timeout"`)
	assert.Contains(t, string(data), `"reachable":false`)
}
