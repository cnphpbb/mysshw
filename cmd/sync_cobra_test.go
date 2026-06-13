package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"mysshw/config"
)

func TestCreateSSHConfig_PlainPassword(t *testing.T) {
	syncCfg := newTestSyncInfo("root", "plainpass")

	cfg := createSSHConfig(syncCfg)
	assert.Equal(t, "root", cfg.User)
	assert.Len(t, cfg.Auth, 1)
}

func TestCreateSSHConfig_EnvPassword(t *testing.T) {
	t.Setenv("MYSSHW_TEST_PWD", "fromenv")

	syncCfg := newTestSyncInfo("admin", "${MYSSHW_TEST_PWD}")
	cfg := createSSHConfig(syncCfg)

	assert.Equal(t, "admin", cfg.User)
	assert.Len(t, cfg.Auth, 1)
}

func TestCreateSSHConfig_EnvPasswordUnset(t *testing.T) {
	syncCfg := newTestSyncInfo("admin", "${MYSSHW_TEST_UNSET_PWD}")
	cfg := createSSHConfig(syncCfg)

	assert.Equal(t, "admin", cfg.User)
	// 解析后为空, 不应添加任何 AuthMethod
	assert.Len(t, cfg.Auth, 0)
}

func TestCreateSSHConfig_EnvPasswordWithDefault(t *testing.T) {
	t.Setenv("MYSSHW_TEST_PWD_DEFAULT", "")

	syncCfg := newTestSyncInfo("admin", "${MYSSHW_TEST_PWD_DEFAULT:-fallback_pwd}")
	cfg := createSSHConfig(syncCfg)

	assert.Equal(t, "admin", cfg.User)
	assert.Len(t, cfg.Auth, 1)
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
