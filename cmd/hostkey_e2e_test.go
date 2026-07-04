//go:build e2e
// +build e2e

// e2e 测试需要 gliderlabs/ssh (in-process SSH server)
// 仅在 `go test -tags e2e ./cmd/...` 时构建

package cmd

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func init() {
	// 防止测试中 cobra 副作用影响其他测试; 在 init 注册 rootCmd 的 hostkey 子命令
	// (它已在 hostkey_cmd.go 的 init() 注册, 这里只需确保 rootCmd 全局已加载)
}

// startE2ESSHServer 启动 in-process SSH server with ephemeral host key
// 返回 host, port, signer, cleanup
//
// 测试用凭据: testuser / testpwd
func startE2ESSHServer(t *testing.T) (host string, port int, signer ssh.Signer, cleanup func()) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}
	parsed, err := ssh.ParsePrivateKey(pem.EncodeToMemory(pemBlock))
	require.NoError(t, err)
	signer = parsed

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host = "127.0.0.1"
	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err = strconv.Atoi(portStr)
	require.NoError(t, err)

	server := &gliderssh.Server{
		HostSigners: []gliderssh.Signer{signer},
		PasswordHandler: func(ctx gliderssh.Context, password string) bool {
			return ctx.User() == "testuser" && password == "testpwd"
		},
	}
	server.Handle(func(s gliderssh.Session) {
		_, _ = s.Write([]byte("executed: " + strings.Join(s.Command(), " ") + "\n"))
		_ = s.Exit(0)
	})

	go func() { _ = server.Serve(listener) }()
	time.Sleep(150 * time.Millisecond) // wait for ready

	cleanup = func() { _ = server.Close() }
	return
}

// e2eConfigTOML 生成临时 TOML config, 包含指向测试 server 的 node
func e2eConfigTOML(t *testing.T, host string, port int) (configPath string, knownHostsPath string) {
	t.Helper()

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "mysshw.toml")
	knownHostsPath = filepath.Join(tmpDir, "kh")

	// known_hosts 文件提前创建 (callback factory 会用它)
	require.NoError(t, os.WriteFile(knownHostsPath, []byte{}, 0600))
	t.Setenv("MYSSHW_KNOWN_HOSTS", knownHostsPath)

	// config 内容 (mysshw TOML 兼容)
	// 注意: Windows 路径含反斜杠, 需要在 TOML string literal 中转义为 \\
	cfgDirEscaped := strings.ReplaceAll(configPath, `\`, `\\`)
	content := `cfg_dir = "` + cfgDirEscaped + `"

[sync]
type = "scp"
remote_uri = "` + host + `:22"
remote_path = "/tmp/backup.toml"

[sync.scp]
username = "testuser"
password = "testpwd"

[[nodes]]
groups = "Test"

[[nodes.ssh]]
name = "test-1"
host = "` + host + `"
user = "testuser"
port = ` + itoa(port) + `
password = "testpwd"
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0600))
	return configPath, knownHostsPath
}

// runHostkeyCmd 在隔离环境跑 cobra 命令, 捕获 stdout/stderr
//
// 注意: cobra rootCmd 是全局单例. 多次调用 rootCmd.Execute() 之间状态可能污染.
// 每个 test 函数在 setup/cleanup 处重置 cobra 全局状态。
func runHostkeyCmd(t *testing.T, args ...string) (stdout string, stderr string, exitCode int) {
	t.Helper()

	// 重要: cobra rootCmd 每次 Execute 之前先 reset, 否则上一次 SetArgs 残留
	// (e.g. cobra 内部解析 state 混用, --cfg 不会被新 args 覆盖)
	rootCmd.SetArgs([]string{}) // 先 reset
	rootCmd.SetArgs(args)

	var outBuf, errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)

	t.Cleanup(func() {
		rootCmd.SetArgs([]string{})
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		// 清理 cobra 内部 state 也需要 reset, 否则会导致一些奇怪的边角行为
		// cobra 没暴露 reset() API; 不过 cobra.Command 有 SilenceUsage / SilenceErrors,
		// 这里我们再设置 flag parser reset
		rootCmd.InitDefaultHelpFlag() // ensure fresh flags state
	})

	// Execute() 可能在出错时调用 os.Exit, 我们这里依靠 rootCmd 在出错时 set exit code
	exitCode = 0
	err := rootCmd.Execute()
	if err != nil {
		if ee, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = 1
		}
		stderr += "\n" + err.Error()
	}

	stdout = outBuf.String()
	stderr += errBuf.String()
	return stdout, stderr, exitCode
}

// ============================================================
// e2e 测试
// ============================================================

// TestHostkeyList_Empty 测试空 known_hosts 时的 list 输出
func TestHostkeyList_Empty(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, _ := e2eConfigTOML(t, host, port)

	stdout, _, _ := runHostkeyCmd(t, "hostkey", "list", "-o", "json", "--cfg", cfgPath)
	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &entries))
	assert.Empty(t, entries, "新空 known_hosts 应返回空数组")
}

// TestHostkeyUpdate_DryRun 测试无 --confirm 时不动 known_hosts
func TestHostkeyUpdate_DryRun(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, khPath := e2eConfigTOML(t, host, port)

	_, _, _ = runHostkeyCmd(t, "hostkey", "update", host, "--node", "test-1",
		"-o", "json", "--cfg", cfgPath)

	// known_hosts 应仍为空
	content, err := os.ReadFile(khPath)
	require.NoError(t, err)
	assert.Empty(t, content, "dry-run 不应写 known_hosts")
}

// TestHostkeyUpdate_WithConfirm 测试 --confirm 实际写入 server 的 key
func TestHostkeyUpdate_WithConfirm(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, khPath := e2eConfigTOML(t, host, port)

	stdout, stderr, exitCode := runHostkeyCmd(t, "hostkey", "update", host, "--node", "test-1",
		"--confirm", "-o", "json", "--cfg", cfgPath)
	t.Logf("exitCode=%d\nstdout=%s\nstderr=%s", exitCode, stdout, stderr)

	content, err := os.ReadFile(khPath)
	require.NoError(t, err)
	body := string(content)
	if body == "" {
		t.Logf("khPath=%s, ctx:\n%s", khPath, "config load path issue likely")
	}
	assert.NotEmpty(t, body, "--confirm 后 known_hosts 应有内容")
	assert.Contains(t, body, host, "条目应包含 host")
	assert.True(t,
		bytes.Contains([]byte(body), []byte("ssh-ed25519")) ||
			bytes.Contains([]byte(body), []byte("ecdsa-sha2-nistp")) ||
			bytes.Contains([]byte(body), []byte("ssh-rsa")),
		"条目应包含一个标准 algorithm: %s", body)
}

// TestHostkeyShow_AfterUpdate 测试 update 后 show 能查到 host
func TestHostkeyShow_AfterUpdate(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, _ := e2eConfigTOML(t, host, port)

	// 先 update
	_, _, _ = runHostkeyCmd(t, "hostkey", "update", host, "--node", "test-1",
		"--confirm", "-o", "json", "--cfg", cfgPath)

	// 再 show
	stdout, _, _ := runHostkeyCmd(t, "hostkey", "show", host,
		"-o", "json", "--cfg", cfgPath)

	var result struct {
		Host    string `json:"host"`
		Entries []struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	assert.Equal(t, host, result.Host)
	assert.NotEmpty(t, result.Entries, "show 应至少返回一条 entry")
	assert.NotEmpty(t, result.Entries[0].Fingerprint, "entry 必含 fingerprint")
}

// TestHostkeyUpdate_ExpectFingerprintMismatched 测试期望 fp 不匹配的安全路径
func TestHostkeyUpdate_ExpectFingerprintMismatched(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, khPath := e2eConfigTOML(t, host, port)

	// 期望一个明显不匹配的 fingerprint
	bogusFP := "SHA256:" + strings.Repeat("A", 43) // 43 base64 字符
	_, _, _ = runHostkeyCmd(t, "hostkey", "update", host, "--node", "test-1",
		"--confirm", "--expect-fingerprint", bogusFP,
		"-o", "json", "--cfg", cfgPath)

	// known_hosts 应仍为空 (mismatch 不写)
	content, err := os.ReadFile(khPath)
	require.NoError(t, err)
	assert.Empty(t, content, "fingerprint mismatch 不应写 known_hosts")
}

// TestHostkeyRemove_Cleanup 测试 remove 删除条目
//
// 已知问题: cobra 是全局单例, 多次 rootCmd.Execute() 之间 flag state 会积累.
// 为避免该竞态本测试不依赖 "update 然后 remove" 顺序, 而是直接
// 预写 fake known_hosts 文件, 仅测 remove 子命令。
func TestHostkeyRemove_Cleanup(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	_, khPath := e2eConfigTOML(t, host, port)

	// 预写 fake known_hosts 文件
	pubKey := generateE2EHostKey(t)
	fakeLine := knownhostsLine(t, host, pubKey)
	require.NoError(t, os.WriteFile(khPath, []byte(fakeLine+"\n"), 0600))

	beforeContent, err := os.ReadFile(khPath)
	require.NoError(t, err)
	require.NotEmpty(t, beforeContent)

	// remove
	stdoutRm, stderrRm, ecRm := runHostkeyCmd(t, "hostkey", "remove", host,
		"-o", "json", "--cfg", filepath.Dir(khPath)+"/mysshw.toml")
	t.Logf("remove: exitCode=%d\nstdout=%s\nstderr=%s", ecRm, stdoutRm, stderrRm)

	afterContent, err := os.ReadFile(khPath)
	require.NoError(t, err)
	assert.Empty(t, afterContent, "remove 后 known_hosts 应清空")
}

// ============================================================
// e2e helpers
// ============================================================

// generateE2EHostKey 生成测试用 ed25519 公钥
func generateE2EHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer.PublicKey()
}

// knownhostsLine 包装 knownhosts.Line 方便测试
func knownhostsLine(t *testing.T, host string, key ssh.PublicKey) string {
	t.Helper()
	return knownhosts.Line([]string{host}, key)
}

// itoa 包装 strconv.Itoa 减少 import 行数
func itoa(n int) string { return strconv.Itoa(n) }
