package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mysshw/config"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// saveRestoreCFGPath 保存/恢复全局 config.CFG_PATH (避免测试间相互污染)
func saveRestoreCFGPath(t *testing.T) {
	t.Helper()
	old := config.CFG_PATH
	t.Cleanup(func() { config.CFG_PATH = old })
}

// ============================================================
// 口令解析
// ============================================================

func TestResolveSyncPassphrase_FromEnv(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "env-pass")
	t.Setenv("MYSSHW_OTHER", "other-pass")

	// 环境变量优先于配置文件的 ${ENV} 引用
	pass, err := resolveSyncPassphrase(syncCryptoOptions{
		PassphraseEnv: DefaultSyncPassphraseEnv,
		PassphraseCfg: "${MYSSHW_OTHER}",
	}, false)
	assert.NoError(t, err)
	assert.Equal(t, "env-pass", pass)
}

func TestResolveSyncPassphrase_CustomEnvName(t *testing.T) {
	t.Setenv("MY_CUSTOM_PASSPHRASE", "custom-pass")

	pass, err := resolveSyncPassphrase(syncCryptoOptions{
		PassphraseEnv: "MY_CUSTOM_PASSPHRASE",
	}, false)
	assert.NoError(t, err)
	assert.Equal(t, "custom-pass", pass)
}

func TestResolveSyncPassphrase_FromConfig(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	t.Setenv("SYNC_PW", "from-env-ref")

	// 配置文件中的 ${ENV} 引用
	pass, err := resolveSyncPassphrase(syncCryptoOptions{
		PassphraseEnv: DefaultSyncPassphraseEnv,
		PassphraseCfg: "${SYNC_PW}",
	}, false)
	assert.NoError(t, err)
	assert.Equal(t, "from-env-ref", pass)
}

func TestResolveSyncPassphrase_ConfigPlainValue(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")

	pass, err := resolveSyncPassphrase(syncCryptoOptions{
		PassphraseEnv: DefaultSyncPassphraseEnv,
		PassphraseCfg: "literal-pass",
	}, false)
	assert.NoError(t, err)
	assert.Equal(t, "literal-pass", pass)
}

func TestResolveSyncPassphrase_EmptyIsError(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")

	// 无环境变量、无配置、交互输入为空 (测试环境 stdin 为 EOF) → 报错
	_, err := resolveSyncPassphrase(syncCryptoOptions{
		PassphraseEnv: DefaultSyncPassphraseEnv,
	}, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty passphrase")
}

// ============================================================
// 加解密助手
// ============================================================

func TestEncryptForSync_Plaintext(t *testing.T) {
	plain := []byte("password = \"secret\"\n")

	enc, err := encryptForSync(plain, "pass")
	assert.NoError(t, err)
	assert.True(t, config.IsEncryptedFile(enc), "加密后应带 MSEC magic")
	assert.NotEqual(t, plain, enc)
	assert.NotContains(t, string(enc), "secret", "密文中不应出现明文")
}

func TestEncryptForSync_Idempotent(t *testing.T) {
	plain := []byte("password = \"secret\"\n")

	enc, err := encryptForSync(plain, "pass")
	assert.NoError(t, err)

	// 再次加密 → 幂等, 原样返回 (不双重加密)
	enc2, err := encryptForSync(enc, "pass")
	assert.NoError(t, err)
	assert.Equal(t, enc, enc2)
}

func TestDecryptAfterSync_RoundTrip(t *testing.T) {
	plain := []byte("password = \"secret\"\n")

	enc, err := encryptForSync(plain, "pass")
	assert.NoError(t, err)

	dec, encrypted, err := decryptAfterSync(enc, "pass", false)
	assert.NoError(t, err)
	assert.False(t, encrypted)
	assert.Equal(t, plain, dec)
}

func TestDecryptAfterSync_WrongPassphrase(t *testing.T) {
	enc, err := encryptForSync([]byte("data"), "right-pass")
	assert.NoError(t, err)

	_, _, err = decryptAfterSync(enc, "wrong-pass", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt remote config failed")
}

func TestDecryptAfterSync_LegacyPlaintext(t *testing.T) {
	plain := []byte("legacy = true\n")

	dec, encrypted, err := decryptAfterSync(plain, "pass", false)
	assert.NoError(t, err)
	assert.False(t, encrypted)
	assert.Equal(t, plain, dec, "旧版本明文备份应原样返回")
}

func TestDecryptAfterSync_KeepEncrypted(t *testing.T) {
	enc, err := encryptForSync([]byte("data"), "pass")
	assert.NoError(t, err)

	dec, encrypted, err := decryptAfterSync(enc, "pass", true)
	assert.NoError(t, err)
	assert.True(t, encrypted)
	assert.Equal(t, enc, dec, "--keep-encrypted 时应保留密文")
}

func TestEncryptUploadPayload_Disabled(t *testing.T) {
	plain := []byte("plain\n")

	out, err := encryptUploadPayload(plain, syncCryptoOptions{Encrypt: false})
	assert.NoError(t, err)
	assert.Equal(t, plain, out, "关闭加密时原样上传")
}

func TestEncryptUploadPayload_AlreadyEncrypted(t *testing.T) {
	enc, err := encryptForSync([]byte("data"), "pass")
	assert.NoError(t, err)

	// 无口令可用也不应报错 (幂等分支不解析口令)
	out, err := encryptUploadPayload(enc, syncCryptoOptions{Encrypt: true})
	assert.NoError(t, err)
	assert.Equal(t, enc, out)
}

func TestDecryptDownloaded_LegacyPlaintext(t *testing.T) {
	plain := []byte("legacy = true\n")

	out, kind, err := decryptDownloaded(plain, syncCryptoOptions{})
	assert.NoError(t, err)
	assert.Equal(t, payloadLegacyPlain, kind)
	assert.Equal(t, plain, out)
}

func TestDecryptDownloaded_KeepEncrypted(t *testing.T) {
	enc, err := encryptForSync([]byte("data"), "pass")
	assert.NoError(t, err)

	out, kind, err := decryptDownloaded(enc, syncCryptoOptions{KeepEncrypted: true})
	assert.NoError(t, err)
	assert.Equal(t, payloadEncrypted, kind)
	assert.Equal(t, enc, out)
}

// ============================================================
// 上传/下载全流程
// ============================================================

func TestUploadConfigSync_EncryptsRemotePayload(t *testing.T) {
	saveRestoreCFGPath(t)
	t.Setenv(DefaultSyncPassphraseEnv, "upload-pass")

	ctx := context.Background()
	backend := &fakeBackend{}
	plain := "password = \"secret\"\n"
	cfgPath := writeTempSyncConfig(t, plain)

	err := uploadConfigSync(ctx, backend, cfgPath, "", syncCryptoOptions{Encrypt: true, PassphraseEnv: DefaultSyncPassphraseEnv})
	assert.NoError(t, err)
	assert.Len(t, backend.uploads, 1)

	// 远端存的必须是密文
	assert.True(t, config.IsEncryptedFile(backend.uploaded), "远端必须是密文")
	assert.NotContains(t, string(backend.uploaded), "secret")

	// 用同一口令能还原出原始明文
	dec, err := config.DecryptConfig(backend.uploaded, []byte("upload-pass"))
	assert.NoError(t, err)
	assert.Equal(t, plain, string(dec))

	// 本地文件保持明文, 不被改写
	local, err := os.ReadFile(cfgPath)
	assert.NoError(t, err)
	assert.Equal(t, plain, string(local))
}

func TestUploadConfigSync_EncryptDisabledKeepsPlaintext(t *testing.T) {
	saveRestoreCFGPath(t)

	ctx := context.Background()
	backend := &fakeBackend{}
	cfgPath := writeTempSyncConfig(t, "plain = true\n")

	err := uploadConfigSync(ctx, backend, cfgPath, "", syncCryptoOptions{Encrypt: false})
	assert.NoError(t, err)
	assert.False(t, config.IsEncryptedFile(backend.uploaded))
}

func TestDownloadConfigSync_DecryptsToLocal(t *testing.T) {
	saveRestoreCFGPath(t)
	t.Setenv(DefaultSyncPassphraseEnv, "download-pass")

	ctx := context.Background()
	plain := "password = \"secret\"\n"
	enc, err := config.EncryptConfig([]byte(plain), []byte("download-pass"))
	assert.NoError(t, err)

	backend := &fakeBackend{remoteData: enc}
	localPath := filepath.Join(t.TempDir(), "restored.toml")

	err = downloadConfigSync(ctx, backend, localPath, "", syncCryptoOptions{PassphraseEnv: DefaultSyncPassphraseEnv})
	assert.NoError(t, err)

	data, err := os.ReadFile(localPath)
	assert.NoError(t, err)
	assert.Equal(t, plain, string(data), "本地应落盘明文")

	// 落地文件权限 0600
	info, err := os.Stat(localPath)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestDownloadConfigSync_KeepEncryptedStaysEncrypted(t *testing.T) {
	saveRestoreCFGPath(t)

	ctx := context.Background()
	enc, err := config.EncryptConfig([]byte("secret = 1\n"), []byte("pw"))
	assert.NoError(t, err)

	backend := &fakeBackend{remoteData: enc}
	localPath := filepath.Join(t.TempDir(), "restored.toml")

	err = downloadConfigSync(ctx, backend, localPath, "", syncCryptoOptions{KeepEncrypted: true})
	assert.NoError(t, err)

	data, err := os.ReadFile(localPath)
	assert.NoError(t, err)
	assert.True(t, config.IsEncryptedFile(data), "--keep-encrypted 时本地也应是密文")
}

func TestDownloadConfigSync_WrongPassphraseKeepsLocalFile(t *testing.T) {
	saveRestoreCFGPath(t)
	t.Setenv(DefaultSyncPassphraseEnv, "wrong-pass")

	ctx := context.Background()
	enc, err := config.EncryptConfig([]byte("remote = 1\n"), []byte("right-pass"))
	assert.NoError(t, err)

	backend := &fakeBackend{remoteData: enc}
	localPath := filepath.Join(t.TempDir(), "restored.toml")
	// 预先写入本地配置
	assert.NoError(t, os.WriteFile(localPath, []byte("local = 1\n"), 0600))

	err = downloadConfigSync(ctx, backend, localPath, "", syncCryptoOptions{PassphraseEnv: DefaultSyncPassphraseEnv})
	assert.Error(t, err)

	// 口令错误时本地配置不得被覆盖
	data, readErr := os.ReadFile(localPath)
	assert.NoError(t, readErr)
	assert.Equal(t, "local = 1\n", string(data))
}

// ============================================================
// 选项组装 (flag > 配置 > 默认)
// ============================================================

func TestBuildSyncCryptoOptions_Defaults(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("encrypt", true, "")
	cmd.Flags().Bool("keep-encrypted", false, "")
	cmd.Flags().String("passphrase-env", DefaultSyncPassphraseEnv, "")

	opts := buildSyncCryptoOptions(cmd, config.SyncInfo{})
	assert.True(t, opts.Encrypt, "默认应加密上传 (安全默认)")
	assert.False(t, opts.KeepEncrypted)
	assert.Equal(t, DefaultSyncPassphraseEnv, opts.PassphraseEnv)
}

func TestBuildSyncCryptoOptions_ConfigOverridesDefault(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("encrypt", true, "")
	cmd.Flags().Bool("keep-encrypted", false, "")
	cmd.Flags().String("passphrase-env", DefaultSyncPassphraseEnv, "")

	no := false
	opts := buildSyncCryptoOptions(cmd, config.SyncInfo{Encrypt: &no, Passphrase: "cfg-pass"})
	assert.False(t, opts.Encrypt, "配置文件应能关闭加密")
	assert.Equal(t, "cfg-pass", opts.PassphraseCfg)
}

func TestBuildSyncCryptoOptions_FlagOverridesConfig(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("encrypt", true, "")
	cmd.Flags().Bool("keep-encrypted", false, "")
	cmd.Flags().String("passphrase-env", DefaultSyncPassphraseEnv, "")
	assert.NoError(t, cmd.Flags().Set("encrypt", "false"))
	assert.NoError(t, cmd.Flags().Set("keep-encrypted", "true"))
	assert.NoError(t, cmd.Flags().Set("passphrase-env", "MY_PW"))

	yes := true
	opts := buildSyncCryptoOptions(cmd, config.SyncInfo{Encrypt: &yes})
	assert.False(t, opts.Encrypt, "显式 flag 应优先于配置文件")
	assert.True(t, opts.KeepEncrypted)
	assert.True(t, opts.KeepEncryptedSet)
	assert.Equal(t, "MY_PW", opts.PassphraseEnv)
}

func TestBuildSyncCryptoOptions_KeepEncryptedNotSet(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("encrypt", true, "")
	cmd.Flags().Bool("keep-encrypted", false, "")
	cmd.Flags().String("passphrase-env", DefaultSyncPassphraseEnv, "")

	opts := buildSyncCryptoOptions(cmd, config.SyncInfo{})
	assert.False(t, opts.KeepEncrypted)
	assert.False(t, opts.KeepEncryptedSet, "未显式指定时不应锁定加密状态")
}

// ============================================================
// 本地加密状态保留
// ============================================================

func TestPreserveLocalEncryptionState_PlainLocalKeepsDecrypt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	assert.NoError(t, os.WriteFile(path, []byte("plain = 1\n"), 0600))

	opts := preserveLocalEncryptionState(path, syncCryptoOptions{})
	assert.False(t, opts.KeepEncrypted, "本地是明文 → 下载后解密落盘")
}

func TestPreserveLocalEncryptionState_EncryptedLocalKeepsCiphertext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	enc, err := config.EncryptConfig([]byte("secret = 1\n"), []byte("pw"))
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(path, enc, 0600))

	opts := preserveLocalEncryptionState(path, syncCryptoOptions{})
	assert.True(t, opts.KeepEncrypted, "本地是密文 → 下载后仍保持密文")
}

func TestPreserveLocalEncryptionState_ExplicitFlagWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	enc, err := config.EncryptConfig([]byte("secret = 1\n"), []byte("pw"))
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(path, enc, 0600))

	// 显式 --keep-encrypted=false → 强制解密落盘
	opts := preserveLocalEncryptionState(path, syncCryptoOptions{KeepEncryptedSet: true})
	assert.False(t, opts.KeepEncrypted, "显式 flag 应优先于本地加密状态")
}

func TestDownloadConfigSync_EncryptedLocalStaysEncrypted(t *testing.T) {
	saveRestoreCFGPath(t)
	t.Setenv(DefaultSyncPassphraseEnv, "dl-pass")

	ctx := context.Background()
	enc, err := config.EncryptConfig([]byte("remote = 1\n"), []byte("dl-pass"))
	assert.NoError(t, err)

	backend := &fakeBackend{remoteData: enc}
	localPath := filepath.Join(t.TempDir(), "mysshw.toml")
	// 预置本地密文配置
	assert.NoError(t, os.WriteFile(localPath, enc, 0600))

	err = downloadConfigSync(ctx, backend, localPath, "", syncCryptoOptions{PassphraseEnv: DefaultSyncPassphraseEnv})
	assert.NoError(t, err)

	data, err := os.ReadFile(localPath)
	assert.NoError(t, err)
	assert.True(t, config.IsEncryptedFile(data), "本地已是密文 → 下载后不应降级为明文")
}
