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
	"github.com/spf13/pflag"
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
// 处理:
//  1. reset 所有 flag value 为 default (flag.Values 在 Set 后会保留)
//  2. reset SetArgs + PersistentFlags
//  3. 重设 Out/Err 到独立 buffer
//  4. cleanup 时恢复
func runHostkeyCmd(t *testing.T, args ...string) (stdout string, stderr string, exitCode int) {
	t.Helper()

	// Reset 所有 flag value 到 default — 这是 state pollution 的根源
	resetCobraFlags(t)

	// Cobra rootCmd SetArgs
	rootCmd.SetArgs(args)

	var outBuf, errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)

	t.Cleanup(func() {
		rootCmd.SetArgs([]string{})
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
	})

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

// resetCobraFlags 将 rootCmd 所有 flag 值重置为 default.
//
// cobra + pflag 不提供 clean reset API; Set+Value.Set(default) 是最直接的重置途径.
// 包括 PersistentFlags + Flags + 子命令继承的 flags。
//
// 在 subtest / 顺序敏感场景中这个 reset 避免了 --cfg / --output / --timeout 等
// 在前一个 test 中被设置后仍残留的问题。
func resetCobraFlags(t *testing.T) {
	t.Helper()
	// Direct VisitAll on Flags / PersistentFlags
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Value != nil {
			_ = f.Value.Set(f.DefValue)
		}
	})
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Value != nil {
			_ = f.Value.Set(f.DefValue)
		}
	})
}

// ============================================================
// e2e 测试
// ============================================================

// TestHostkeyE2E 是 hostkey 子命令端到端测试的容器, 通过 t.Run subtests 隔离.
//
// 每个 subtest 享有独立 setup (fresh cfg + kh path + fresh SSH server)。这样
// subtest 间的失败不互相影响 (--check-config, --cfg, --output 等 flag 状态
// 通过 runHostkeyCmd 内部的 resetCobraFlags 清隔)。
func TestHostkeyE2E(t *testing.T) {
	t.Run("List_Empty", func(t *testing.T) {
		testHostkeyListEmpty(t)
	})
	t.Run("Update_DryRun", func(t *testing.T) {
		testHostkeyUpdateDryRun(t)
	})
	t.Run("Update_WithConfirm", func(t *testing.T) {
		testHostkeyUpdateWithConfirm(t)
	})
	t.Run("Show_AfterUpdate", func(t *testing.T) {
		testHostkeyShowAfterUpdate(t)
	})
	t.Run("Update_ExpectFingerprintMismatch", func(t *testing.T) {
		testHostkeyUpdateExpectFingerprintMismatch(t)
	})
	t.Run("Remove_Cleanup", func(t *testing.T) {
		testHostkeyRemoveCleanup(t)
	})
	t.Run("Audit", func(t *testing.T) {
		testHostkeyAudit(t)
	})
}

// --- 每个 subtest function 都接受 (t *testing.T), 独立 setup/teardown ---

func testHostkeyListEmpty(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, _ := e2eConfigTOML(t, host, port)

	stdout, _, _ := runHostkeyCmd(t, "hostkey", "list", "-o", "json", "--cfg", cfgPath)
	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &entries))
	assert.Empty(t, entries, "新空 known_hosts 应返回空数组")
}

func testHostkeyUpdateDryRun(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, khPath := e2eConfigTOML(t, host, port)

	_, _, _ = runHostkeyCmd(t, "hostkey", "update", host, "--node", "test-1",
		"-o", "json", "--cfg", cfgPath)

	content, err := os.ReadFile(khPath)
	require.NoError(t, err)
	assert.Empty(t, content, "dry-run 不应写 known_hosts")
}

func testHostkeyUpdateWithConfirm(t *testing.T) {
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
		t.Fatalf("update 未写盘: kh empty")
	}
	assert.Contains(t, body, host, "条目应包含 host")
	assert.True(t,
		bytes.Contains([]byte(body), []byte("ssh-ed25519")) ||
			bytes.Contains([]byte(body), []byte("ecdsa-sha2-nistp")) ||
			bytes.Contains([]byte(body), []byte("ssh-rsa")),
		"条目应包含一个标准 algorithm: %s", body)
}

func testHostkeyShowAfterUpdate(t *testing.T) {
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

func testHostkeyUpdateExpectFingerprintMismatch(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, khPath := e2eConfigTOML(t, host, port)

	bogusFP := "SHA256:" + strings.Repeat("A", 43)
	_, _, _ = runHostkeyCmd(t, "hostkey", "update", host, "--node", "test-1",
		"--confirm", "--expect-fingerprint", bogusFP,
		"-o", "json", "--cfg", cfgPath)

	content, err := os.ReadFile(khPath)
	require.NoError(t, err)
	assert.Empty(t, content, "fingerprint mismatch 不应写 known_hosts")
}

func testHostkeyRemoveCleanup(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	_, khPath := e2eConfigTOML(t, host, port)

	// 预写 fake known_hosts 文件 (避免依赖 update-then-remove 顺序不稳定)
	pubKey := generateE2EHostKey(t)
	fakeLine := knownhostsLine(t, host, pubKey)
	require.NoError(t, os.WriteFile(khPath, []byte(fakeLine+"\n"), 0600))

	beforeContent, err := os.ReadFile(khPath)
	require.NoError(t, err)
	require.NotEmpty(t, beforeContent)

	cfgDir := filepath.Dir(khPath)
	cfgFile := filepath.Join(cfgDir, "mysshw.toml")
	stdoutRm, _, _ := runHostkeyCmd(t, "hostkey", "remove", host,
		"-o", "json", "--cfg", cfgFile)
	t.Logf("remove stdout: %s", stdoutRm)

	afterContent, err := os.ReadFile(khPath)
	require.NoError(t, err)
	assert.Empty(t, afterContent, "remove 后 known_hosts 应清空")
}

// testHostkeyAudit 测试 audit 子命令 — read-only, 不应改 known_hosts
func testHostkeyAudit(t *testing.T) {
	host, port, _, cleanup := startE2ESSHServer(t)
	defer cleanup()
	cfgPath, khPath := e2eConfigTOML(t, host, port)

	// 预写一个简单 known_hosts (1 entry, ssh-ed25519)
	pubKey := generateE2EHostKey(t)
	entry := knownhostsLine(t, host, pubKey)
	require.NoError(t, os.WriteFile(khPath, []byte(entry+"\n"), 0600))

	// 不带 --check-config
	stdout, _, _ := runHostkeyCmd(t, "hostkey", "audit", "-o", "json", "--cfg", cfgPath)
	var r1 struct {
		File                  map[string]any `json:"file"`
		TotalEntries          int            `json:"total_entries"`
		PlainEntries          int            `json:"plain_entries"`
		AlgorithmDistribution map[string]int `json:"algorithm_distribution"`
		Warnings              []string       `json:"warnings"`
		ConfiguredHosts       []string       `json:"configured_hosts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &r1))
	assert.Equal(t, 1, r1.TotalEntries, "应有 1 entry")
	assert.Equal(t, 1, r1.PlainEntries)
	assert.NotNil(t, r1.AlgorithmDistribution["ssh-ed25519"])
	assert.Nil(t, r1.ConfiguredHosts, "不带 --check-config 不应读 config")

	// 带 --check-config
	stdout2, _, _ := runHostkeyCmd(t, "hostkey", "audit", "--check-config", "-o", "json", "--cfg", cfgPath)
	var r2 struct {
		ConfiguredHosts []string `json:"configured_hosts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout2), &r2))
	assert.NotEmpty(t, r2.ConfiguredHosts, "--check-config 应填 configured_hosts 字段")

	// audit 不应改 known_hosts
	afterContent, err := os.ReadFile(khPath)
	require.NoError(t, err)
	assert.Equal(t, entry+"\n", string(afterContent), "audit 是 read-only, 不应改 known_hosts")
}

// 兼容旧 test 路径 (deprecated 路径; 请用 TestHostkeyE2E 容器跑 subtest)
//
// 保留为 stub 避免 import 警告 - 不再跑这些, 用 TestHostkeyE2E/Audit 代替
func TestHostkeyList_Empty(t *testing.T)    { t.Skip("use TestHostkeyE2E/List_Empty subtest") }
func TestHostkeyUpdate_DryRun(t *testing.T) { t.Skip("use TestHostkeyE2E/Update_DryRun subtest") }
func TestHostkeyUpdate_WithConfirm(t *testing.T) {
	t.Skip("use TestHostkeyE2E/Update_WithConfirm subtest")
}
func TestHostkeyShow_AfterUpdate(t *testing.T) { t.Skip("use TestHostkeyE2E/Show_AfterUpdate subtest") }
func TestHostkeyUpdate_ExpectFingerprintMismatched(t *testing.T) {
	t.Skip("use TestHostkeyE2E/Update_ExpectFingerprintMismatch subtest")
}
func TestHostkeyRemove_Cleanup(t *testing.T) { t.Skip("use TestHostkeyE2E/Remove_Cleanup subtest") }

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
