package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecureConfigFile_NoChangeIfAlready0600(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "cfg.toml")
	err := os.WriteFile(tmp, []byte("dummy"), 0600)
	assert.NoError(t, err)

	secureConfigFile(tmp)

	info, err := os.Stat(tmp)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSecureConfigFile_TightensPermissions(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "cfg.toml")
	// 模拟旧版默认 0644 (group/other 可读)
	err := os.WriteFile(tmp, []byte("dummy"), 0644)
	assert.NoError(t, err)

	secureConfigFile(tmp)

	info, err := os.Stat(tmp)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(),
		"应将 0644 收紧为 0600, 实际: %v", info.Mode().Perm())
}

func TestSecureConfigFile_NonExistentPath(t *testing.T) {
	// 不存在的路径应静默忽略
	secureConfigFile(filepath.Join(t.TempDir(), "nonexistent.toml"))
}

func TestSecureConfigFile_SkipsDirectory(t *testing.T) {
	// 目录应跳过
	dir := t.TempDir()
	secureConfigFile(dir)

	info, err := os.Stat(dir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())
}
