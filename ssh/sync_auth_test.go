package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	crypto_ssh "golang.org/x/crypto/ssh"
)

// writeSyncTestKey 生成临时 ed25519 私钥; passphrase 为空表示无口令
func writeSyncTestKey(t *testing.T, passphrase string) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	var block *pem.Block
	if passphrase == "" {
		block, err = crypto_ssh.MarshalPrivateKey(priv, "mysshw-test")
	} else {
		block, err = crypto_ssh.MarshalPrivateKeyWithPassphrase(priv, "mysshw-test", []byte(passphrase))
	}
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0600))
	return path
}

func TestLoadKeySigner_NoPassphrase(t *testing.T) {
	keyPath := writeSyncTestKey(t, "")

	signer, err := LoadKeySigner(keyPath, "")
	require.NoError(t, err)
	assert.NotNil(t, signer)
	assert.NotEmpty(t, signer.PublicKey().Marshal())
}

func TestLoadKeySigner_WithPassphrase(t *testing.T) {
	keyPath := writeSyncTestKey(t, "s3cret")

	signer, err := LoadKeySigner(keyPath, "s3cret")
	require.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestLoadKeySigner_PassphraseFromEnv(t *testing.T) {
	t.Setenv("MYSSHW_TEST_KEY_PP", "s3cret")
	keyPath := writeSyncTestKey(t, "s3cret")

	signer, err := LoadKeySigner(keyPath, "${MYSSHW_TEST_KEY_PP}")
	require.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestLoadKeySigner_WrongPassphrase(t *testing.T) {
	keyPath := writeSyncTestKey(t, "rightpass")

	_, err := LoadKeySigner(keyPath, "wrongpass")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "passphrase")
}

func TestLoadKeySigner_MissingFile(t *testing.T) {
	_, err := LoadKeySigner(filepath.Join(t.TempDir(), "nope"), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read private key")
}

// TestLoadKeySigner_ExpandHomeDir ~ 路径应被展开为家目录
func TestLoadKeySigner_ExpandHomeDir(t *testing.T) {
	home := withMockHome(t)

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := crypto_ssh.MarshalPrivateKey(priv, "mysshw-test")
	require.NoError(t, err)

	sshDir := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(sshDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(sshDir, "id_ed25519"), pem.EncodeToMemory(block), 0600))

	signer, err := LoadKeySigner("~/.ssh/id_ed25519", "")
	require.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestAgentAuthMethod_NoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	_, _, err := AgentAuthMethod()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SSH_AUTH_SOCK")
}

// TestSyncHostKeyCallback 默认应返回 TOFU 回调 (非 nil)
func TestSyncHostKeyCallback(t *testing.T) {
	home := withMockHome(t)
	t.Setenv(EnvSkipHostKeyCheck, "")

	cb, err := SyncHostKeyCallback()
	require.NoError(t, err)
	assert.NotNil(t, cb)

	// known_hosts 文件应已在 mock 家目录下创建
	_, err = os.Stat(filepath.Join(home, ".ssh", "known_hosts"))
	assert.NoError(t, err)
}

// TestSyncHostKeyCallback_SkipEnv 逃生开关: MYSSHW_SKIP_HOST_KEY_CHECK=true → 跳过校验
func TestSyncHostKeyCallback_SkipEnv(t *testing.T) {
	t.Setenv(EnvSkipHostKeyCheck, "true")

	cb, err := SyncHostKeyCallback()
	require.NoError(t, err)
	assert.NotNil(t, cb)
}
