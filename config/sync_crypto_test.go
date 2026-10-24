package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSyncConfig 写一个只含 [sync] 段的配置文件
func writeSyncConfig(t *testing.T, syncSection string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mysshw.toml")
	content := syncSection + "\n[sync.scp]\nusername = \"root\"\npassword = \"${MYSSHW_PWD_sync}\"\n\n[[nodes]]\ngroups = \"Test\"\n\n[[nodes.ssh]]\nname = \"n1\"\nhost = \"127.0.0.1\"\nuser = \"root\"\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	t.Cleanup(func() { CFG = nil; CFG_PATH = "~/.mysshw.toml" })
	return path
}

func TestSyncInfo_EncryptAbsent(t *testing.T) {
	path := writeSyncConfig(t, "[sync]\ntype = \"scp\"\nremote_uri = \"127.0.0.1:22\"\nremote_path = \"/tmp/x.toml\"")

	require.NoError(t, LoadViperConfig(path))
	assert.Nil(t, CFG.SyncCfg.Encrypt, "未配置 encrypt 时应为 nil (由上层决定默认值)")
}

func TestSyncInfo_EncryptFalse(t *testing.T) {
	path := writeSyncConfig(t, "[sync]\ntype = \"scp\"\nremote_uri = \"127.0.0.1:22\"\nremote_path = \"/tmp/x.toml\"\nencrypt = false")

	require.NoError(t, LoadViperConfig(path))
	require.NotNil(t, CFG.SyncCfg.Encrypt, "encrypt = false 应被解析为非 nil 指针")
	assert.False(t, *CFG.SyncCfg.Encrypt)
}

func TestSyncInfo_EncryptTrue(t *testing.T) {
	path := writeSyncConfig(t, "[sync]\ntype = \"scp\"\nremote_uri = \"127.0.0.1:22\"\nremote_path = \"/tmp/x.toml\"\nencrypt = true")

	require.NoError(t, LoadViperConfig(path))
	require.NotNil(t, CFG.SyncCfg.Encrypt)
	assert.True(t, *CFG.SyncCfg.Encrypt)
}

func TestSyncInfo_Passphrase(t *testing.T) {
	path := writeSyncConfig(t, "[sync]\ntype = \"scp\"\nremote_uri = \"127.0.0.1:22\"\nremote_path = \"/tmp/x.toml\"\npassphrase = \"${MYSSHW_SYNC_PW}\"")

	require.NoError(t, LoadViperConfig(path))
	assert.Equal(t, "${MYSSHW_SYNC_PW}", CFG.SyncCfg.Passphrase)
}
