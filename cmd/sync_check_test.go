package cmd

import (
	"os"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
)

// ============================================================
// sync 加密参数预检
// ============================================================

func TestDetectPassphraseSource_FromEnv(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "env-pass")

	src, detail := detectPassphraseSource(syncCryptoOptions{PassphraseEnv: DefaultSyncPassphraseEnv})
	assert.Equal(t, passphraseFromEnv, src)
	assert.Contains(t, detail, DefaultSyncPassphraseEnv)
}

func TestDetectPassphraseSource_FromConfig(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)
	t.Setenv("MYSSHW_CFG_PW", "cfg-pass")

	src, _ := detectPassphraseSource(syncCryptoOptions{
		PassphraseEnv: DefaultSyncPassphraseEnv,
		PassphraseCfg: "${MYSSHW_CFG_PW}",
	})
	assert.Equal(t, passphraseFromConfig, src)
}

func TestDetectPassphraseSource_None(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)

	src, _ := detectPassphraseSource(syncCryptoOptions{
		PassphraseEnv: DefaultSyncPassphraseEnv,
		PassphraseCfg: "${MYSSHW_MISSING_PW_XYZ}",
	})
	assert.Equal(t, passphraseNone, src)
}

func TestCheckSyncEncryptionConfigured_UploadWithPassphrase(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "pw")

	err := checkSyncEncryptionConfigured(
		config.SyncInfo{Type: "scp"},
		syncCryptoOptions{Encrypt: true, PassphraseEnv: DefaultSyncPassphraseEnv},
		true,
	)
	assert.NoError(t, err)
}

func TestCheckSyncEncryptionConfigured_UploadNoPassphraseNonInteractive(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)

	err := checkSyncEncryptionConfigured(
		config.SyncInfo{Type: "scp"},
		syncCryptoOptions{Encrypt: true, PassphraseEnv: DefaultSyncPassphraseEnv},
		true,
	)
	// 测试环境下 stdin 不是 terminal → 必须 fail-fast, 不能挂起等输入
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no passphrase is configured")
}

func TestCheckSyncEncryptionConfigured_UploadEncryptDisabled(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")

	// 关闭加密 → 不报错 (只告警)
	err := checkSyncEncryptionConfigured(
		config.SyncInfo{Type: "scp"},
		syncCryptoOptions{Encrypt: false, PassphraseEnv: DefaultSyncPassphraseEnv},
		true,
	)
	assert.NoError(t, err)
}

func TestCheckSyncEncryptionConfigured_DownloadNoPassphrase(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)

	// 下载: 远端可能是明文, 只告警不报错
	err := checkSyncEncryptionConfigured(
		config.SyncInfo{Type: "scp"},
		syncCryptoOptions{Encrypt: true, PassphraseEnv: DefaultSyncPassphraseEnv},
		false,
	)
	assert.NoError(t, err)
}

func TestCheckSyncEncryptionConfigured_DownloadWithPassphrase(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "pw")

	err := checkSyncEncryptionConfigured(
		config.SyncInfo{Type: "scp"},
		syncCryptoOptions{Encrypt: true, PassphraseEnv: DefaultSyncPassphraseEnv},
		false,
	)
	assert.NoError(t, err)
}
