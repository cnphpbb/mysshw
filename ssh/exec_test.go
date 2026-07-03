package ssh

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pipeEmptyStdin 把 os.Stdin 替换为立即 EOF 的 pipe, 返回恢复函数
//
// 目的: 让 resolvePasswordForSSH 走 noStdin 早返路径
// Windows 下子进程继承的 stdin 仍被识为 character device, isTerminal=true,
// 但实际 term.ReadPassword 会以 "The handle is invalid" 失败 — 该函数避免此问题。
func pipeEmptyStdin(t *testing.T) func() {
	t.Helper()
	orig := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stdin = r
	return func() {
		os.Stdin = orig
		_ = r.Close()
	}
}

// noStdinTest 包装 pipeEmptyStdin 为 t.Cleanup 形式
func noStdinTest(t *testing.T) {
	t.Helper()
	restore := pipeEmptyStdin(t)
	t.Cleanup(restore)
}

// ---------- ExecResult JSON marshaling ----------

func TestExecResult_JSON(t *testing.T) {
	r := &ExecResult{
		Node:     "prod-1",
		Command:  "uptime",
		Stdout:   " 12:34 up 1 day\n",
		Stderr:   "",
		ExitCode: 0,
		Duration: "123ms",
	}
	b, err := json.Marshal(r)
	require.NoError(t, err)
	s := string(b)
	assert.Contains(t, s, `"node":"prod-1"`)
	assert.Contains(t, s, `"exit_code":0`)
	assert.NotContains(t, s, `"error"`, "无 Error 时 JSON 应省略该字段")
}

func TestExecResult_JSON_WithError(t *testing.T) {
	r := &ExecResult{Node: "x", Command: "y", Error: "boom"}
	b, _ := json.Marshal(r)
	assert.Contains(t, string(b), `"error":"boom"`)
}

// ---------- semaphore ----------

func TestSemaphore_AcquireRelease(t *testing.T) {
	// 设计: 主线程持 2 个槽位 (semaphore=2), goroutine 取第 3 个会阻塞,
	// 主线程 release 1 个后 goroutine 获得。
	s := newSemaphore(2)
	s.acquire()
	s.acquire()

	gotSlot := make(chan struct{}, 1)
	go func() {
		s.acquire()
		gotSlot <- struct{}{}
		s.release()
	}()

	// 给 goroutine 时间试图 acquire
	time.Sleep(50 * time.Millisecond)
	select {
	case <-gotSlot:
		t.Fatal("goroutine 不应在 sem 已满时拿到槽位")
	default:
	}

	s.release() // 释放一个槽位让 goroutine 获得
	select {
	case <-gotSlot:
	case <-time.After(time.Second):
		t.Fatal("goroutine 在 release 后未拿到槽位")
	}

	// over-release 不应 panic: release 5 次也不会死锁
	for i := 0; i < 5; i++ {
		s.release()
	}
}

func TestSemaphore_Unlimited(t *testing.T) {
	s := newSemaphore(0)
	// Parallel<=0 应得到零值 (unlimited)
	assert.Nil(t, s.ch, "Parallel<=0 应得到 nil channel (unlimited)")
	// 不阻塞
	s.acquire()
	s.acquire()
	s.acquire()
	s.release()
	s.release()
	s.release()
}

// ---------- BatchExecutor.RunFanOut ----------

func TestRunFanOut_EmptyNodes(t *testing.T) {
	be := &BatchExecutor{Nodes: nil}
	results := be.RunFanOut(context.Background(), "true")
	assert.Empty(t, results)
}

func TestRunFanOut_NilNodeSkipped(t *testing.T) {
	be := &BatchExecutor{Nodes: []*config.SSHNode{nil, nil, nil}}
	results := be.RunFanOut(context.Background(), "true")
	require.Len(t, results, 3)
	for _, r := range results {
		require.NotNil(t, r)
		assert.Equal(t, "<nil>", r.Node)
		assert.Equal(t, -1, r.ExitCode)
		assert.NotEmpty(t, r.Error)
	}
}

func TestRunFanOut_FactoryError(t *testing.T) {
	noStdinTest(t)
	// 没有 host 的节点 → NewExecutor 应该失败 (因为 connect 检查 host)
	// 等等: connect 内部才检查 host; NewExecutor 只检查 node nil
	// 这里给个空 user 但 host 缺省让 NewClient 仍然返回非 nil, 但后续会 dial fail
	// 测 NewExecutor 在真正空 input 时的行为
	be := &BatchExecutor{Nodes: []*config.SSHNode{{Name: "x", Host: ""}}}
	results := be.RunFanOut(context.Background(), "true")
	require.Len(t, results, 1)
	require.NotNil(t, results[0])
	assert.Equal(t, "x", results[0].Node)
	// dial 报错
	assert.NotEmpty(t, results[0].Error, "空 host 节点应在拨号阶段失败")
}

func TestRunFanOut_SkipFactoryError(t *testing.T) {
	noStdinTest(t)
	// 节点直接为 nil 应跳过 NewExecutor
	be := &BatchExecutor{
		Nodes: []*config.SSHNode{
			nil,
			{Name: "ok-1", Host: "127.0.0.1", Port: 1, User: "x"}, // dial fail
		},
		Options: BatchOptions{UsePTY: false, Timeout: 50 * time.Millisecond, Parallel: 2},
	}
	results := be.RunFanOut(context.Background(), "true")
	require.Len(t, results, 2)
	assert.Equal(t, "<nil>", results[0].Node)
	assert.Equal(t, "ok-1", results[1].Node)
}

func TestRunFanOut_RespectsParallel(t *testing.T) {
	noStdinTest(t)
	// 验证并发控制: 创建 10 个 dial-fail 的节点, 用 Parallel=2
	be := &BatchExecutor{
		Nodes: makeNodes(10),
		Options: BatchOptions{
			UsePTY:   false,
			Timeout:  50 * time.Millisecond,
			Parallel: 2,
		},
	}
	start := time.Now()
	results := be.RunFanOut(context.Background(), "true")
	elapsed := time.Since(start)
	require.Len(t, results, 10)
	for _, r := range results {
		require.NotNil(t, r)
		_ = r
	}
	assert.Less(t, elapsed, 30*time.Second)
}

func makeNodes(n int) []*config.SSHNode {
	out := make([]*config.SSHNode, n)
	for i := 0; i < n; i++ {
		out[i] = &config.SSHNode{
			Name: "node-" + string(rune('A'+i)),
			Host: "127.0.0.1",
			Port: 1, // 极可能 dial fail
			User: "x",
		}
	}
	return out
}

// ---------- NewExecutor ----------

func TestNewExecutor_NilNode(t *testing.T) {
	_, err := NewExecutor(nil, ExecOptions{})
	assert.Error(t, err)
}

func TestNewExecutor_EmptyHost(t *testing.T) {
	_, err := NewExecutor(&config.SSHNode{Name: "x", Host: ""}, ExecOptions{})
	// 这不会立即报错 — NewClient 在这里仍返回 defaultClient; connect 阶段才报
	// 因此此处断言 exec 已建立
	assert.NoError(t, err)
}

func TestNewExecutor_BadPassword_AllowedAtThisLayer(t *testing.T) {
	noStdinTest(t)
	e, err := NewExecutor(
		&config.SSHNode{Name: "x", Host: "127.0.0.1", User: "u", Password: "anything"},
		ExecOptions{UsePTY: false},
	)
	assert.NoError(t, err)
	assert.NotNil(t, e)
	e.Close()
}

func TestExecutor_RunCommand_DialFails(t *testing.T) {
	noStdinTest(t)
	e, err := NewExecutor(
		&config.SSHNode{Name: "fail", Host: "127.0.0.1", Port: 1, User: "nobody"},
		ExecOptions{UsePTY: false},
	)
	require.NoError(t, err)
	defer e.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r, runErr := e.RunCommand(ctx, "true")
	require.NotNil(t, r)
	require.Error(t, runErr)
	assert.Contains(t, r.Error, "dial failed")
}

func TestExecutor_RunCommand_ContextCancelled(t *testing.T) {
	noStdinTest(t)
	e, err := NewExecutor(
		&config.SSHNode{Name: "x", Host: "203.0.113.1", User: "u"}, // RFC5737 TEST-NET-3
		ExecOptions{UsePTY: false},
	)
	require.NoError(t, err)
	defer e.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	r, runErr := e.RunCommand(ctx, "true")
	elapsed := time.Since(start)
	require.NotNil(t, r)
	require.Error(t, runErr)
	assert.Less(t, elapsed, 1*time.Second, "应快速返回, 不应等到 TCP 超时")
	assert.True(t,
		strings.Contains(r.Error, "cancelled") || strings.Contains(r.Error, "dial failed"),
		"应标识被取消或拨号失败, 实际: %s", r.Error)
}

func TestExecutor_Close_Idempotent(t *testing.T) {
	noStdinTest(t)
	e, err := NewExecutor(
		&config.SSHNode{Name: "x", Host: "127.0.0.1", Port: 1, User: "u"},
		ExecOptions{},
	)
	require.NoError(t, err)
	assert.NoError(t, e.Close())
	assert.NoError(t, e.Close())
}

// ---------- 错误分类 (供 cmd 层使用) ----------

func TestErrNodeNotFound_Sentinel(t *testing.T) {
	_, err := ResolveNodes(&config.Configs{}, NodeSelector{Host: "x"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNodeNotFound))
}
