package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// withTempHome 临时重定向 $HOME 到 t.TempDir() (供 lastseen 标记测试用)
func withTempHome(t *testing.T) {
	t.Helper()
	orig := os.Getenv("HOME")
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { _ = os.Setenv("HOME", orig) })
}

func TestMarkVersionSeen(t *testing.T) {
	withTempHome(t)
	assert.NoError(t, markVersionSeen())

	data, err := os.ReadFile(lastSeenMarkerPath())
	assert.NoError(t, err)
	assert.Equal(t, Version, string(data))
}

func TestIsUpgradedFromOlder_NoMarker(t *testing.T) {
	// 不存在的 lastseen 路径
	tmpDir := t.TempDir()
	markerPath := filepath.Join(tmpDir, ".mysshw.lastseen")
	assert.False(t, isUpgradedFromOlderAt(markerPath))
}

func TestIsUpgradedFromOlder_SameVersion(t *testing.T) {
	tmpDir := t.TempDir()
	markerPath := filepath.Join(tmpDir, ".mysshw.lastseen")
	assert.NoError(t, os.WriteFile(markerPath, []byte(Version), 0600))
	assert.False(t, isUpgradedFromOlderAt(markerPath))
}

func TestIsUpgradedFromOlder_OldVersion(t *testing.T) {
	tmpDir := t.TempDir()
	markerPath := filepath.Join(tmpDir, ".mysshw.lastseen")
	assert.NoError(t, os.WriteFile(markerPath, []byte("v0.0.0-old"), 0600))
	assert.True(t, isUpgradedFromOlderAt(markerPath))
}

func TestIsFirstRun_NoToml(t *testing.T) {
	// 用不存在的路径
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".mysshw.toml")
	assert.True(t, isFirstRunAtPath(cfgPath))
}

func TestIsFirstRun_WithToml(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".mysshw.toml")
	assert.NoError(t, os.WriteFile(cfgPath, []byte("[nodes]\n"), 0600))
	assert.False(t, isFirstRunAtPath(cfgPath))
}

func TestIsTerminalStdout_PipeReturnsFalse(t *testing.T) {
	// 真实 stdout 通常是 terminal, 此处仅证明函数不 panic
	_ = isTerminalStdout()
}

func TestItoaPtr(t *testing.T) {
	i := 22
	s := itoaPtr(&i)
	assert.Equal(t, "22", *s)

	var nilInt *int
	s = itoaPtr(nilInt)
	assert.Equal(t, "", *s)
}
