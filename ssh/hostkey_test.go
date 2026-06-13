package ssh

import (
	"crypto/rand"
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

	_, err := trustOnFirstUseCallback()
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
	_, err = trustOnFirstUseCallback()
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
