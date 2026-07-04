package cmd

import (
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	mysshwSSH "mysshw/ssh"
)

// generateTestPublicKey 生成真实 ed25519 公钥 (供 parseKnownHosts 测试使用)
// mock 字符串 mockKHEd25519 不能通过 gossh.ParsePublicKey 验证,
// 所以单元测试需要真实 key
func generateTestPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer.PublicKey()
}

// knownhostLine 是 known_hosts 标准格式的一行 (明文)
func knownhostLine(t *testing.T, host string, key gossh.PublicKey) string {
	t.Helper()
	return knownhosts.Line([]string{host}, key)
}

// withMockKHPath 临时让 ssh.GetKnownHostsPath 返回测试目录中的路径
//
// 实现: 包级 env var MYSSHW_KNOWN_HOSTS 优先级最高;
//
//	我们 t.Setenv, 让 GetKnownHostsPath 返回 t.TempDir()/known_hosts
func withMockKHPath(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "known_hosts")
	t.Setenv("MYSSHW_KNOWN_HOSTS", path)
	return path
}

// mockKHContent 写一个示例 known_hosts 内容
func mockKHContent(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func TestExtractHostNames(t *testing.T) {
	cases := []struct {
		name  string
		field string
		want  []string
	}{
		{"simple", "example.com", []string{"example.com"}},
		{"with port", "example.com:2222", []string{"example.com"}},
		{"bracketed with port", "[example.com]:2222", []string{"example.com"}},
		{"comma list", "a.example.com,b.example.com", []string{"a.example.com", "b.example.com"}},
		{"hashed v1", "|1|salt|hash", []string{"<hashed>"}},
		{"empty", "", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractHostNames(tc.field)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEntryHostMatches(t *testing.T) {
	cases := []struct {
		name      string
		entryHost string
		query     string
		want      bool
	}{
		{"exact match", "example.com", "example.com", true},
		{"comma list contains query", "a.com,b.com,c.com", "b.com", true},
		{"comma list missing query", "a.com,b.com", "c.com", false},
		{"bracketed matches raw", "[example.com]:2222", "example.com", true},
		{"hashed never matches plain", "|1|salt|hash", "example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, entryHostMatches(tc.entryHost, tc.query))
		})
	}
}

func TestNormalizeHostArg(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"example.com", "example.com"},
		{"example.com:", "example.com"},
		{"example.com:2222", "example.com:2222"}, // port 保留
		{"  example.com  ", "example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := normalizeHostArg(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- parseKnownHosts 测试 ---

// 生成一个有效 ed25519 public key 的 known_hosts line (明文格式)
const (
	mockKHEd25519 = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB4OfTGEQ8F9hBxFtH3FKBjKaP5FQbdskT5+2NkJZaqGV"
)

func TestParseKnownHosts_EmptyFile(t *testing.T) {
	path := withMockKHPath(t)
	got, err := parseKnownHosts(path)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestParseKnownHosts_NonExistentFile(t *testing.T) {
	path := withMockKHPath(t) // 文件未创建
	got, err := parseKnownHosts(path)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestParseKnownHosts_OneEntry(t *testing.T) {
	path := withMockKHPath(t)
	// 用真实 ed25519 key (ssh 包辅助生成)
	pubKey := generateTestPublicKey(t)
	line := knownhostLine(t, "example.com", pubKey)
	mockKHContent(t, path, line+"\n")

	entries, err := parseKnownHosts(path)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "example.com", e.Host)
	assert.Equal(t, []string{"example.com"}, e.HostNames)
	assert.Equal(t, "ssh-ed25519", e.Algorithm)
	assert.False(t, e.IsHashed)
	assert.Equal(t, 1, e.LineNo)
	assert.True(t, strings.HasPrefix(e.Fingerprint, "SHA256:"))
}

func TestParseKnownHosts_MixedEntries(t *testing.T) {
	path := withMockKHPath(t)
	pubKey := generateTestPublicKey(t)
	mockKHContent(t, path,
		"# header comment\n"+
			knownhostLine(t, "example.com", pubKey)+"\n"+
			"\n"+
			knownhostLine(t, "other.example.com:2222", pubKey)+"\n"+
			"|1|salt123hash456 ssh-rsa AAAAB3...==\n",
	)

	entries, err := parseKnownHosts(path)
	require.NoError(t, err)
	require.Len(t, entries, 3)

	// 顺序按文件中出现顺序
	assert.Equal(t, "example.com", entries[0].Host)
	assert.Equal(t, []string{"example.com"}, entries[0].HostNames)

	assert.Equal(t, "[other.example.com]:2222", entries[1].Host,
		"非默认端口 known_hosts 自动 normalize 为 [host]:port 形式")
	assert.Equal(t, []string{"other.example.com"}, entries[1].HostNames)

	assert.True(t, entries[2].IsHashed)
	assert.Equal(t, []string{"<hashed>"}, entries[2].HostNames)
	assert.NotEmpty(t, entries[2].RawKey)
}

func TestParseKnownHosts_BracketedPort(t *testing.T) {
	path := withMockKHPath(t)
	pubKey := generateTestPublicKey(t)
	mockKHContent(t, path,
		knownhostLine(t, "[internal.example.com]:2222", pubKey)+"\n",
	)

	entries, err := parseKnownHosts(path)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, []string{"internal.example.com"}, entries[0].HostNames)
}

func TestParseKnownHosts_DamagedLine(t *testing.T) {
	path := withMockKHPath(t)
	// bad base64: line 只有 2 个 fields (host + algo), fields[2] 缺失
	mockKHContent(t, path,
		"broken.example.com notbase64\n",
	)
	entries, err := parseKnownHosts(path)
	require.NoError(t, err)
	// 损坏行被忽略 (fields < 3 跳过)
	assert.Empty(t, entries)
}

func TestParseKnownHosts_BadBase64Key(t *testing.T) {
	path := withMockKHPath(t)
	// host + algo + invalid base64
	mockKHContent(t, path,
		"bad.example.com ssh-rsa !@#$%^notbase64\n",
	)
	entries, err := parseKnownHosts(path)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "(decode error)", entries[0].Fingerprint)
	// host_names 仍只含原 host, 不加 (unparseable)
	assert.Equal(t, []string{"bad.example.com"}, entries[0].HostNames)
}

// 验证 ssh.GetKnownHostsPath 被 t.Setenv(MYSSHW_KNOWN_HOSTS) 影响
func TestSSHKnownHostsPathRespectsEnv(t *testing.T) {
	target := filepath.Join(t.TempDir(), "kh_test")
	t.Setenv("MYSSHW_KNOWN_HOSTS", target)
	got, err := mysshwSSH.GetKnownHostsPath()
	require.NoError(t, err)
	assert.Equal(t, target, got)
}

// Sanity: net.SplitHostPort 在测试环境下行为正确
func TestSplitHostPort(t *testing.T) {
	h, p, err := net.SplitHostPort("example.com:2222")
	assert.NoError(t, err)
	assert.Equal(t, "example.com", h)
	assert.Equal(t, "2222", p)

	_, _, err = net.SplitHostPort("example.com")
	assert.Error(t, err, "无 port 应报错")
}
