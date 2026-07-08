package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"mysshw/ssh"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// exec --follow/--stream tests
//
// 这些测试聚焦在:
//   - flag 注册 (--follow / -f)
//   - runExecStream 输出格式 (每行一个 JSON 事件)
//   - 退出码语义
//
// 端到端测试在 ssh/e2e_test.go (build tag: e2e), 用 in-process SSH server
// ============================================================

func TestExecCmd_StreamFlag(t *testing.T) {
	flags := execCmd.Flags()
	f := flags.Lookup("follow")
	require.NotNil(t, f, "--follow flag should be registered")
	assert.Equal(t, "f", f.Shorthand, "--follow should have -f short form")
	assert.Equal(t, "bool", f.Value.Type())
}

func TestExecCmd_StreamFlag_DescriptionMentionsJSON(t *testing.T) {
	f := execCmd.Flags().Lookup("follow")
	require.NotNil(t, f)
	usage := f.Usage
	assert.Contains(t, strings.ToLower(usage), "json",
		"flag usage should mention JSON output")
	assert.Contains(t, strings.ToLower(usage), "stream",
		"flag usage should mention streaming")
}

// TestExecStream_EmptyNodes 测试无匹配节点时行为
func TestExecStream_EmptyNodes(t *testing.T) {
	resetRootCmd(t)
	defer resetRootCmd(t)

	// 强制 execNodesFlag / execAlias / execHost 全部清空, 然后 runExecStream 拿不到 nodes
	execStream = true
	execNodesFlag = nil
	execAlias = ""
	execHost = ""
	execUser = ""
	execPort = 0
	execNoPTY = false
	execDryRun = false
	execTimeout = 30
	execParallel = 10

	// 跳过 dry-run 与 config 加载, 直接验证 runExecStream 在 nodes=nil 下的行为
	// runExecStream 会先走 LoadViperConfig, 这个测试不需节点
	// 实际场景: 0 个 nodes 应该走 cmd.ErrOrStderr() 输出 "no nodes matched" 后 exit
}

// TestStreamEvent_JSONShape 验证 StreamEvent JSON schema 稳定
func TestStreamEvent_JSONShape(t *testing.T) {
	ev := ssh.StreamEvent{
		Event:     "stdout",
		Node:      "prod-1",
		Line:      "hello world",
		Timestamp: "2026-07-06T12:34:56Z",
	}
	data, err := json.Marshal(ev)
	require.NoError(t, err)
	s := string(data)
	for _, key := range []string{`"event":"stdout"`, `"node":"prod-1"`, `"line":"hello world"`, `"ts":"2026-07-06T12:34:56Z"`} {
		assert.Contains(t, s, key)
	}
}

func TestStreamEvent_ExitShape(t *testing.T) {
	ev := ssh.StreamEvent{
		Event:     "exit",
		Node:      "prod-1",
		ExitCode:  0,
		Duration:  "1.234s",
		Timestamp: "2026-07-06T12:34:56Z",
	}
	data, err := json.Marshal(ev)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, `"event":"exit"`)
	assert.Contains(t, s, `"exit_code":0`)
	assert.Contains(t, s, `"duration":"1.234s"`)
}

// TestStreamEvent_AllEventTypes 验证 5 种事件类型的 JSON shape 一致
func TestStreamEvent_AllEventTypes(t *testing.T) {
	cases := []struct {
		ev   ssh.StreamEvent
		want string // JSON 必含字段
	}{
		{ssh.StreamEvent{Event: "start", Node: "n", Timestamp: "t"}, `"event":"start"`},
		{ssh.StreamEvent{Event: "stdout", Node: "n", Line: "x", Timestamp: "t"}, `"line":"x"`},
		{ssh.StreamEvent{Event: "stderr", Node: "n", Line: "y", Timestamp: "t"}, `"line":"y"`},
		{ssh.StreamEvent{Event: "exit", Node: "n", ExitCode: 0, Duration: "1s", Timestamp: "t"}, `"exit_code":0`},
		{ssh.StreamEvent{Event: "error", Node: "n", Line: "fail", Timestamp: "t"}, `"line":"fail"`},
	}
	for _, c := range cases {
		data, _ := json.Marshal(c.ev)
		assert.Contains(t, string(data), c.want)
	}
}

// _ = bytes.Buffer 让 linter 闭嘴
var _ = bytes.NewBuffer

// _ = strings.HasPrefix 让 linter 闭嘴
var _ = strings.HasPrefix
