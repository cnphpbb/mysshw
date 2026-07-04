package ssh

import (
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"mysshw/config"
)

// withMockHome 临时替换 currentHomeDir 为临时目录
// 用于在跨平台（特别是 Windows，HOME 环境变量无效）下隔离测试
func withMockHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	orig := currentHomeDir
	currentHomeDir = func() (string, error) {
		return tmpDir, nil
	}
	t.Cleanup(func() { currentHomeDir = orig })
	return tmpDir
}

// generateTestPublicKey 生成一个临时 ed25519 SSH 公钥用于测试
func generateTestPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(priv)
	assert.NoError(t, err)

	return signer.PublicKey()
}

func TestGetKnownHostsPath(t *testing.T) {
	tmpHome := withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)
	assert.NotEmpty(t, path)

	expected := filepath.Join(tmpHome, ".ssh", "known_hosts")
	assert.Equal(t, expected, path)
}

func TestGetKnownHostsPath_CreatesSSHDir(t *testing.T) {
	tmpHome := withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	// 确认 .ssh 目录被自动创建
	sshDir := filepath.Join(tmpHome, ".ssh")
	info, err := os.Stat(sshDir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, sshDir, filepath.Dir(path))
}

func TestAppendKnownHost(t *testing.T) {
	withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	pubKey := generateTestPublicKey(t)

	// 第一次追加
	err = appendKnownHost(path, "example.com", pubKey)
	assert.NoError(t, err)

	// 验证文件存在且非空
	info, err := os.Stat(path)
	assert.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))

	// 验证内容包含主机名和密钥类型
	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "example.com")
	assert.Contains(t, string(content), "ssh-ed25519")
}

func TestAppendKnownHost_MultipleWrites(t *testing.T) {
	withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	pubKey1 := generateTestPublicKey(t)
	pubKey2 := generateTestPublicKey(t)

	// 追加两个不同主机
	err = appendKnownHost(path, "host1.example.com", pubKey1)
	assert.NoError(t, err)
	err = appendKnownHost(path, "host2.example.com", pubKey2)
	assert.NoError(t, err)

	// 验证两行都被写入
	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Len(t, lines, 2)
	assert.Contains(t, lines[0], "host1.example.com")
	assert.Contains(t, lines[1], "host2.example.com")
}

func TestTrustOnFirstUseCallback_CreatesEmpty(t *testing.T) {
	// 验证 callback 能基于空文件正常构造
	withMockHome(t)

	_, err := trustOnFirstUseCallback(&config.SSHNode{Name: "x"})
	assert.NoError(t, err)
}

func TestTrustOnFirstUseCallback_LoadsExistingFile(t *testing.T) {
	withMockHome(t)

	// 预先写入一条合法的 known_hosts 记录
	path, err := getKnownHostsPath()
	assert.NoError(t, err)
	pubKey := generateTestPublicKey(t)
	preExistLine := knownhosts.Line([]string{"pre-existing.example.com"}, pubKey) + "\n"
	err = os.WriteFile(path, []byte("# header comment\n"+preExistLine), 0600)
	assert.NoError(t, err)

	// callback 应该能正常加载已有内容
	_, err = trustOnFirstUseCallback(&config.SSHNode{Name: "x"})
	assert.NoError(t, err)
}

func TestGetKnownHostsPath_CustomPath(t *testing.T) {
	// mock config.CFG
	orig := config.CFG
	t.Cleanup(func() { config.CFG = orig })

	// 自定义路径在临时目录的子目录中
	customPath := filepath.Join(t.TempDir(), "subdir", "my_known_hosts")
	config.CFG = &config.Configs{
		KnownHostsPath: customPath,
	}

	path, err := getKnownHostsPath()
	assert.NoError(t, err)
	assert.Equal(t, customPath, path)

	// 父目录应被自动创建 (0700)
	parentDir := filepath.Dir(customPath)
	info, err := os.Stat(parentDir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetKnownHostsPath_CustomPathWithTilde(t *testing.T) {
	tmpHome := withMockHome(t)

	orig := config.CFG
	t.Cleanup(func() { config.CFG = orig })

	config.CFG = &config.Configs{
		KnownHostsPath: "~/.mysshw/known_hosts",
	}

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	expected := filepath.Join(tmpHome, ".mysshw", "known_hosts")
	assert.Equal(t, expected, path)
}

func TestGetKnownHostsPath_EmptyCustomPathFallsBackToDefault(t *testing.T) {
	tmpHome := withMockHome(t)

	orig := config.CFG
	t.Cleanup(func() { config.CFG = orig })

	// 显式空字符串应回退到默认 ~/.ssh/known_hosts
	config.CFG = &config.Configs{
		KnownHostsPath: "",
	}

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	expected := filepath.Join(tmpHome, ".ssh", "known_hosts")
	assert.Equal(t, expected, path)
}

func TestIsEnvTrue(t *testing.T) {
	cases := []struct {
		val      string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"  true  ", true}, // 带空格
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
		{"random", false},
	}
	for _, c := range cases {
		t.Run(c.val, func(t *testing.T) {
			t.Setenv("MYSSHW_TEST_BOOL", c.val)
			assert.Equal(t, c.expected, IsEnvTrue("MYSSHW_TEST_BOOL"))
		})
	}
}

func TestGetKnownHostsPath_EnvOverride(t *testing.T) {
	withMockHome(t)

	// 即使 config.CFG 有自定义路径, 环境变量也应优先
	orig := config.CFG
	t.Cleanup(func() { config.CFG = orig })
	config.CFG = &config.Configs{
		KnownHostsPath: "/should/be/ignored/by/env",
	}

	envPath := filepath.Join(t.TempDir(), "env_known_hosts")
	t.Setenv(EnvKnownHosts, envPath)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)
	assert.Equal(t, envPath, path)
}

func TestGetKnownHostsPath_EnvOverrideWithTilde(t *testing.T) {
	tmpHome := withMockHome(t)

	envPath := "~/.mysshw_env/known_hosts"
	t.Setenv(EnvKnownHosts, envPath)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	expected := filepath.Join(tmpHome, ".mysshw_env", "known_hosts")
	assert.Equal(t, expected, path)
}

func TestGetHostKeyCallback_EnvSkipReturnsInsecure(t *testing.T) {
	withMockHome(t)

	t.Setenv(EnvSkipHostKeyCheck, "true")

	node := &config.SSHNode{Name: "test", Host: "127.0.0.1", User: "root"}
	cb, err := getHostKeyCallback(node)
	assert.NoError(t, err)
	assert.NotNil(t, cb)
}

func TestGetHostKeyCallback_EnvSkipOverridesPerNode(t *testing.T) {
	// 环境变量 SKIP 应优先于 per-node 的 strict_host_key_checking = true
	withMockHome(t)
	t.Setenv(EnvSkipHostKeyCheck, "yes")

	strict := true
	node := &config.SSHNode{
		Name:                  "test",
		Host:                  "127.0.0.1",
		User:                  "root",
		StrictHostKeyChecking: &strict,
	}
	cb, err := getHostKeyCallback(node)
	assert.NoError(t, err)
	assert.NotNil(t, cb)
}

func TestGetHostKeyCallback_PerNodeSkip(t *testing.T) {
	withMockHome(t)
	t.Setenv(EnvSkipHostKeyCheck, "") // 显式清空

	skip := false
	node := &config.SSHNode{
		Name:                  "test",
		Host:                  "127.0.0.1",
		User:                  "root",
		StrictHostKeyChecking: &skip,
	}
	cb, err := getHostKeyCallback(node)
	assert.NoError(t, err)
	assert.NotNil(t, cb)
}

func TestGetHostKeyCallback_DefaultTOFU(t *testing.T) {
	withMockHome(t)
	t.Setenv(EnvSkipHostKeyCheck, "")

	// 默认 (nil StrictHostKeyChecking) → 走 TOFU
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1", User: "root"}
	cb, err := getHostKeyCallback(node)
	assert.NoError(t, err)
	assert.NotNil(t, cb)
}

// ============================================================
// rotate_known_hosts related tests
// ============================================================

func TestRotateKnownHost_NoExistingFile(t *testing.T) {
	withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	// 文件不存在时应成功 (等同于先 add)
	pubKey := generateTestPublicKey(t)
	err = rotateKnownHost(path, "new.example.com", pubKey)
	assert.NoError(t, err)

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "new.example.com")
}

func TestRotateKnownHost_RemovesOldKeepsOthers(t *testing.T) {
	withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	pubKeyA := generateTestPublicKey(t)
	pubKeyB := generateTestPublicKey(t)
	pubKeyC := generateTestPublicKey(t)

	// 初始状态: host A,B,C 都有 entries
	assert.NoError(t, appendKnownHost(path, "host-a.example.com", pubKeyA))
	assert.NoError(t, appendKnownHost(path, "host-b.example.com", pubKeyB))
	assert.NoError(t, appendKnownHost(path, "host-c.example.com", pubKeyC))

	// rotate host-b (用新 key 替换)
	pubKeyNewB := generateTestPublicKey(t)
	assert.NoError(t, rotateKnownHost(path, "host-b.example.com", pubKeyNewB))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	body := string(content)

	// host-a 和 host-c 应保留
	assert.Contains(t, body, "host-a.example.com")
	assert.Contains(t, body, "host-c.example.com")
	// host-b 新 key 应写入
	assert.Contains(t, body, "host-b.example.com")
	// 验证 old key 被删除: 不允许同名多行 (这里 host-b 只出现一行)
	lines := strings.Split(strings.TrimSpace(body), "\n")
	count := 0
	for _, ln := range lines {
		if lineContainsHostToken(strings.Fields(ln)[0], "host-b.example.com") {
			count++
		}
	}
	assert.Equal(t, 1, count, "host-b 应该只保留一条; 旧被删除")
}

func TestRotateKnownHost_RemovesMultipleEntriesForSameHost(t *testing.T) {
	withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	pubKeyA1 := generateTestPublicKey(t)
	pubKeyA2 := generateTestPublicKey(t)

	// 模拟服务器 key 轮换历史: 已知 2 个旧 key entry 都应删
	assert.NoError(t, appendKnownHost(path, "rotated.example.com", pubKeyA1))
	assert.NoError(t, appendKnownHost(path, "rotated.example.com", pubKeyA2))

	pubKeyNew := generateTestPublicKey(t)
	assert.NoError(t, rotateKnownHost(path, "rotated.example.com", pubKeyNew))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	body := string(content)

	// 只应有 1 条 rotated.example.com (新 key)
	lines := strings.Split(strings.TrimSpace(body), "\n")
	count := 0
	for _, ln := range lines {
		if strings.HasPrefix(ln, "|1|") {
			continue // hashed entries 跳过
		}
		if lineContainsHostToken(strings.Fields(ln)[0], "rotated.example.com") {
			count++
		}
	}
	assert.Equal(t, 1, count, "旧 key 全部删除，只保留新 key")
}

func TestRotateKnownHost_KeepsComments(t *testing.T) {
	withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	// 预先写入一行注释
	assert.NoError(t, os.WriteFile(path, []byte("# header\n"), 0600))

	pubKey := generateTestPublicKey(t)
	assert.NoError(t, rotateKnownHost(path, "fresh.example.com", pubKey))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	body := string(content)

	assert.Contains(t, body, "# header", "注释应保留")
	assert.Contains(t, body, "fresh.example.com")
}

func TestRotateKnownHost_NoMatchIsNoOp(t *testing.T) {
	withMockHome(t)

	path, err := getKnownHostsPath()
	assert.NoError(t, err)

	// 预先写入无关 host
	pubKey := generateTestPublicKey(t)
	assert.NoError(t, appendKnownHost(path, "other.example.com", pubKey))

	// rotate 不存在的 host: 等同于 add
	pubKey2 := generateTestPublicKey(t)
	assert.NoError(t, rotateKnownHost(path, "fresh.example.com", pubKey2))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	body := string(content)
	assert.Contains(t, body, "other.example.com", "无关 host 应保留")
	assert.Contains(t, body, "fresh.example.com")
}

func TestTrustOnFirstUseCallback_RotateOnMismatch(t *testing.T) {
	withMockHome(t)

	// 预先写一条 host example.com 的"旧" key
	path, err := getKnownHostsPath()
	assert.NoError(t, err)
	oldKey := generateTestPublicKey(t)
	oldEncoded := knownhosts.Line([]string{"example.com"}, oldKey)
	assert.NoError(t, os.WriteFile(path, []byte(oldEncoded+"\n"), 0600))

	// node 开启 rotate
	node := &config.SSHNode{Name: "x", RotateKnownHosts: true}
	cb, err := trustOnFirstUseCallback(node)
	assert.NoError(t, err)

	newKey := generateTestPublicKey(t)
	newEncoded := knownhosts.Line([]string{"example.com"}, newKey)
	remote := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 22}
	cbErr := cb("example.com", remote, newKey)
	// rotate 路径应返回 nil (接受)
	assert.NoError(t, cbErr, "rotate 路径应接受新 key (返回 nil)")

	// 验证文件已被 rotate:
	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	body := string(content)

	assert.NotContains(t, body, oldEncoded, "旧 key line 应被删除")
	assert.Contains(t, body, newEncoded, "新 key line 应被写入")

	// 唯一一条以 example.com 开头的 line
	count := countHostLines(t, body, "example.com")
	assert.Equal(t, 1, count, "example.com 应仅有一行 (rotate 后)")
}

func TestTrustOnFirstUseCallback_NoRotateByDefault(t *testing.T) {
	withMockHome(t)

	// 预先写一条 host 的 key
	path, err := getKnownHostsPath()
	assert.NoError(t, err)
	oldKey := generateTestPublicKey(t)
	oldEncoded := knownhosts.Line([]string{"example.com"}, oldKey)
	assert.NoError(t, os.WriteFile(
		path,
		[]byte(oldEncoded+"\n"),
		0600,
	))

	// node 不开启 rotate (默认值)
	node := &config.SSHNode{Name: "x"}
	cb, err := trustOnFirstUseCallback(node)
	assert.NoError(t, err)

	newKey := generateTestPublicKey(t)
	newEncoded := knownhosts.Line([]string{"example.com"}, newKey)
	remote := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 22}
	cbErr := cb("example.com", remote, newKey)
	// 默认严格拒绝
	assert.Error(t, cbErr, "默认应对 fingerprint mismatch 拒绝")

	// 验证旧 key 还在, 新 key 未写入
	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	body := string(content)
	assert.Contains(t, body, oldEncoded, "旧 key line 应保留")
	assert.NotContains(t, body, newEncoded, "默认不写新 key")
}

// countHostLines 辅助: 数 known_hosts body 中以 host 字段开头的明文行数 (hashed 跳过)
func countHostLines(t *testing.T, body, host string) int {
	t.Helper()
	n := 0
	for _, ln := range strings.Split(body, "\n") {
		if ln == "" || strings.HasPrefix(ln, "|1|") {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) == 0 {
			continue
		}
		if lineContainsHostToken(fields[0], host) {
			n++
		}
	}
	return n
}

func TestLineContainsHostToken(t *testing.T) {
	cases := []struct {
		name     string
		field    string
		target   string
		expected bool
	}{
		{"simple match", "example.com", "example.com", true},
		{"no match", "other.com", "example.com", false},
		{"with port", "example.com:2222", "example.com", true},
		{"bracketed + port", "[example.com]:2222", "example.com", true},
		{"comma list match", "a.example.com,b.example.com", "b.example.com", true},
		{"comma list no match", "a.example.com,b.example.com", "c.example.com", false},
		{"empty field", "", "example.com", false},
		{"partial match", "xexample.com", "example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lineContainsHostToken(tc.field, tc.target)
			assert.Equal(t, tc.expected, got, "field=%q target=%q", tc.field, tc.target)
		})
	}
}
