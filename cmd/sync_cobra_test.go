package cmd

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
	"mysshw/config"
)

func TestCreateSSHConfig_PlainPassword(t *testing.T) {
	syncCfg := newTestSyncInfo("root", "plainpass")

	cfg, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.NoError(t, err)
	assert.Equal(t, "root", cfg.User)
	assert.Len(t, cfg.Auth, 1)
}

func TestCreateSSHConfig_EnvPassword(t *testing.T) {
	t.Setenv("MYSSHW_TEST_PWD", "fromenv")

	syncCfg := newTestSyncInfo("admin", "${MYSSHW_TEST_PWD}")
	cfg, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.NoError(t, err)

	assert.Equal(t, "admin", cfg.User)
	assert.Len(t, cfg.Auth, 1)
}

func TestCreateSSHConfig_EnvPasswordUnset(t *testing.T) {
	// 未设置的 env 引用解析为空 → 没有任何认证方式 → 应返回明确错误
	// (而不是让 SSH 报 "attempted methods [none], no supported methods remain")
	t.Setenv("SSH_AUTH_SOCK", "")

	syncCfg := newTestSyncInfo("admin", "${MYSSHW_TEST_UNSET_PWD}")
	_, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no usable SSH authentication method")
	assert.Contains(t, err.Error(), "keypath")
	assert.Contains(t, err.Error(), "password")
}

func TestCreateSSHConfig_EnvPasswordWithDefault(t *testing.T) {
	t.Setenv("MYSSHW_TEST_PWD_DEFAULT", "")

	syncCfg := newTestSyncInfo("admin", "${MYSSHW_TEST_PWD_DEFAULT:-fallback_pwd}")
	cfg, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.NoError(t, err)

	assert.Equal(t, "admin", cfg.User)
	assert.Len(t, cfg.Auth, 1)
}

// TestCreateSSHConfig_KeyPath 私钥认证 (本次 bug 的回归测试)
//
// 背景: 旧实现只读 [sync.scp].password, 完全忽略 keypath,
// 导致"只配了私钥"的用户在握手阶段拿到
// "ssh: unable to authenticate, attempted methods [none], no supported methods remain"
func TestCreateSSHConfig_KeyPath(t *testing.T) {
	keyPath := writeTestKey(t, "")

	syncCfg := newTestSyncInfo("root", "")
	syncCfg.SCPConfig.KeyPath = keyPath

	cfg, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.NoError(t, err)

	assert.Equal(t, "root", cfg.User)
	assert.Len(t, cfg.Auth, 1, "私钥认证必须被注册")
}

// TestCreateSSHConfig_KeyPathWithPassphrase 带口令的私钥
func TestCreateSSHConfig_KeyPathWithPassphrase(t *testing.T) {
	keyPath := writeTestKey(t, "s3cret")

	syncCfg := newTestSyncInfo("root", "")
	syncCfg.SCPConfig.KeyPath = keyPath
	syncCfg.SCPConfig.Passphrase = "s3cret"

	cfg, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.NoError(t, err)
	assert.Len(t, cfg.Auth, 1)
}

// TestCreateSSHConfig_KeyPathPassphraseFromEnv 私钥口令支持 ${ENV} 引用
func TestCreateSSHConfig_KeyPathPassphraseFromEnv(t *testing.T) {
	t.Setenv("MYSSHW_TEST_KEY_PP", "s3cret")
	keyPath := writeTestKey(t, "s3cret")

	syncCfg := newTestSyncInfo("root", "")
	syncCfg.SCPConfig.KeyPath = keyPath
	syncCfg.SCPConfig.Passphrase = "${MYSSHW_TEST_KEY_PP}"

	cfg, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.NoError(t, err)
	assert.Len(t, cfg.Auth, 1)
}

// TestCreateSSHConfig_KeyPathWrongPassphrase 口令错误 → 明确报错
func TestCreateSSHConfig_KeyPathWrongPassphrase(t *testing.T) {
	keyPath := writeTestKey(t, "rightpass")

	syncCfg := newTestSyncInfo("root", "")
	syncCfg.SCPConfig.KeyPath = keyPath
	syncCfg.SCPConfig.Passphrase = "wrongpass"

	_, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "passphrase")
}

// TestCreateSSHConfig_KeyPathMissingFile 私钥文件不存在 → 明确报错
func TestCreateSSHConfig_KeyPathMissingFile(t *testing.T) {
	syncCfg := newTestSyncInfo("root", "")
	syncCfg.SCPConfig.KeyPath = filepath.Join(t.TempDir(), "nope_ed25519")

	_, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read private key")
}

// TestCreateSSHConfig_KeyPathAndPassword 私钥 + 密码同时配置 → 两种方式都注册
func TestCreateSSHConfig_KeyPathAndPassword(t *testing.T) {
	keyPath := writeTestKey(t, "")

	syncCfg := newTestSyncInfo("root", "plainpass")
	syncCfg.SCPConfig.KeyPath = keyPath

	cfg, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.NoError(t, err)
	assert.Len(t, cfg.Auth, 2)
}

// TestCreateSSHConfig_HostKeyCallbackNotNil 主机指纹校验回调必须存在 (TOFU)
//
// 旧实现用 InsecureIgnoreHostKey(), 备份通道可被 MITM 窃取配置
func TestCreateSSHConfig_HostKeyCallbackNotNil(t *testing.T) {
	syncCfg := newTestSyncInfo("root", "plainpass")

	cfg, cleanup, err := createSSHConfig(syncCfg)
	defer cleanup()
	require.NoError(t, err)
	assert.NotNil(t, cfg.HostKeyCallback)
}

// writeTestKey 生成临时 ed25519 私钥文件; passphrase 为空表示无口令
func writeTestKey(t *testing.T, passphrase string) string {
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

// newTestSyncInfo 构造一个测试用的 config.SyncInfo
func newTestSyncInfo(user, password string) config.SyncInfo {
	return config.SyncInfo{
		Type:      "scp",
		RemoteUri: "127.0.0.1:22",
		SCPConfig: config.SCPConfig{
			Username: user,
			Password: password,
		},
	}
}
