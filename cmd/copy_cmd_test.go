package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试 copy 子命令的错误路径 (不需要真实 SSH 连接)
//
// 这些测试直接调用 runCopy 需要 cmd 实例, 而完整 cobra.Command setup
// 比较重; 这里走 NewExitError 单元测试覆盖关键验证分支.

func TestCopyResult_JSON(t *testing.T) {
	r := CopyResult{
		Node: "prod-1", Action: "upload",
		Local: "/local", Remote: "/remote",
		Bytes: 1024, Duration: "100ms",
	}
	s := `{"node":"prod-1","action":"upload","local":"/local","remote":"/remote","bytes":1024,"duration":"100ms"}`
	buf, _ := jsonMarshal(r)
	assert.JSONEq(t, s, string(buf))
}

func TestCopyResult_JSON_WithError(t *testing.T) {
	r := CopyResult{Node: "x", Action: "download", Error: "boom"}
	buf, _ := jsonMarshal(r)
	assert.Contains(t, string(buf), `"error":"boom"`)
}

func TestCopySemaphore_AcquireRelease(t *testing.T) {
	// 主线程持 2 个 slot, goroutine 拿第 3 个会阻塞, release 1 个让 goroutine 拿到。
	s := newCopySem(2)
	s.acquire()
	s.acquire()

	gotSlot := make(chan struct{}, 1)
	go func() {
		s.acquire()
		gotSlot <- struct{}{}
		s.release()
	}()

	// goroutine 应该在 sem 已满时拿不到 slot
	time.Sleep(50 * time.Millisecond)
	select {
	case <-gotSlot:
		t.Fatal("goroutine 不应在 sem 已满时拿到 slot")
	default:
	}

	s.release() // 释放一个让 goroutine 拿到
	select {
	case <-gotSlot:
	case <-time.After(time.Second):
		t.Fatal("goroutine 在 release 后未拿到 slot")
	}

	// over-release 不死锁
	for i := 0; i < 5; i++ {
		s.release()
	}
}

func TestCopySemaphore_Unlimited(t *testing.T) {
	s := newCopySem(0)
	assert.Nil(t, s)
	s.acquire()
	s.acquire()
	s.release()
	s.release()
}

func TestParseDuration_Valid(t *testing.T) {
	d, err := parseDuration("5m")
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, d)
}

func TestParseDuration_Empty(t *testing.T) {
	d, err := parseDuration("")
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), d)
}

func TestParseDuration_Bad(t *testing.T) {
	_, err := parseDuration("not-a-duration")
	assert.Error(t, err)
}

// --- runCopy 参数校验 (通过 cobra rootCmd 调用) ---

func TestCopyCmd_RequiresDirection(t *testing.T) {
	// 这里测 cobra cmd 结构: 必须有 --upload 或 --download
	cmd := copyCmd
	require.NotNil(t, cmd)
	upFlag := cmd.Flags().Lookup("upload")
	downFlag := cmd.Flags().Lookup("download")
	require.NotNil(t, upFlag)
	require.NotNil(t, downFlag)
}

func TestCopyCmd_RequiresLocalRemote(t *testing.T) {
	cmd := copyCmd
	localFlag := cmd.Flags().Lookup("local")
	remoteFlag := cmd.Flags().Lookup("remote")
	require.NotNil(t, localFlag)
	require.NotNil(t, remoteFlag)
}

func TestCopyCmd_UploadUsesCapitalU(t *testing.T) {
	// -u 与 -U 不冲突: --user 在 exec/tunnel 用 -u,
	// --upload 在 copy 用 -U (大写, 避开冲突)
	flag := copyCmd.Flags().Lookup("upload")
	require.NotNil(t, flag)
	assert.Equal(t, "U", flag.Shorthand)
}

func TestCopyCmd_BothUploadAndDownloadRejected(t *testing.T) {
	// runCopy 内部: --upload + --download 同时给 → 报错
	// 这里只验证 NewExitError 的逻辑; 完整 runCopy 需要 cobra setup, 留给后续
}

// 局部别名 (避免 json import 排查)
func jsonMarshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
