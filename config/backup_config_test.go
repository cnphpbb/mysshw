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

func TestWriteTomlWithBackup_NewFile(t *testing.T) {
	// 新文件 (不存在) → 直接写, 无备份
	tmp := filepath.Join(t.TempDir(), "mysshw.toml")
	err := WriteTomlWithBackup(tmp, []byte("content"))
	assert.NoError(t, err)

	data, err := os.ReadFile(tmp)
	assert.NoError(t, err)
	assert.Equal(t, "content", string(data))

	// 验证无备份
	matches, _ := filepath.Glob(tmp + ".*.bak")
	assert.Empty(t, matches, "新文件不应产生备份")
}

func TestWriteTomlWithBackup_ExistingFile(t *testing.T) {
	// 已存在文件 → 备份 + 写
	tmp := filepath.Join(t.TempDir(), "mysshw.toml")
	origContent := "original"
	err := os.WriteFile(tmp, []byte(origContent), 0600)
	assert.NoError(t, err)

	err = WriteTomlWithBackup(tmp, []byte("new"))
	assert.NoError(t, err)

	// 主文件是新内容
	data, _ := os.ReadFile(tmp)
	assert.Equal(t, "new", string(data))

	// 有一个备份, 内容是原内容
	matches, _ := filepath.Glob(tmp + ".*.bak")
	assert.Len(t, matches, 1)
	backup, _ := os.ReadFile(matches[0])
	assert.Equal(t, origContent, string(backup))
}

func TestWriteTomlWithBackup_PrunesOldBackups(t *testing.T) {
	// 创建 7 个旧备份 + 1 个当前 → 写新内容后应保留 3 个最新 (含刚生成的)
	tmp := filepath.Join(t.TempDir(), "mysshw.toml")
	for i := 0; i < 7; i++ {
		path := tmp + "." + padSeq(i) + ".bak"
		err := os.WriteFile(path, []byte("old"), 0600)
		assert.NoError(t, err)
	}
	// 当前文件 (不算备份, 但写入时也会被备份)
	err := os.WriteFile(tmp, []byte("current"), 0600)
	assert.NoError(t, err)

	// 设置 keep=3
	orig := CFG
	CFG = &Configs{BackupKeep: 3}
	t.Cleanup(func() { CFG = orig })

	err = WriteTomlWithBackup(tmp, []byte("new"))
	assert.NoError(t, err)

	// 验证: 7 旧 + 1 新 = 8 → 裁剪到 3 = 3 个 .bak
	matches, _ := filepath.Glob(tmp + ".*.bak")
	assert.Len(t, matches, 3, "应保留 3 个备份 (含刚生成的), 实际: %v", matches)
}

func TestPruneBackups_KeepZero(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mysshw.toml")
	os.WriteFile(tmp+"."+padSeq(0)+".bak", []byte("a"), 0600)

	err := pruneBackups(tmp, 0)
	assert.NoError(t, err)

	matches, _ := filepath.Glob(tmp + ".*.bak")
	assert.Len(t, matches, 1, "keep=0 不应裁剪")
}

func TestPruneBackups_KeepMoreThanExists(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mysshw.toml")
	os.WriteFile(tmp+"."+padSeq(0)+".bak", []byte("a"), 0600)
	os.WriteFile(tmp+"."+padSeq(1)+".bak", []byte("b"), 0600)

	err := pruneBackups(tmp, 10)
	assert.NoError(t, err)

	matches, _ := filepath.Glob(tmp + ".*.bak")
	assert.Len(t, matches, 2, "存在 2 个, keep=10 不应删除")
}

func TestPruneBackups_KeepOne(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mysshw.toml")
	for i := 0; i < 5; i++ {
		os.WriteFile(tmp+"."+padSeq(i)+".bak", []byte("x"), 0600)
	}

	err := pruneBackups(tmp, 1)
	assert.NoError(t, err)

	matches, _ := filepath.Glob(tmp + ".*.bak")
	assert.Len(t, matches, 1, "应只保留 1 个")
	// 保留最新的 (padSeq(4) > padSeq(0..3))
	assert.Contains(t, matches[0], padSeq(4))
}

func TestEffectiveBackupKeep(t *testing.T) {
	orig := CFG
	t.Cleanup(func() { CFG = orig })

	// CFG 为 nil
	CFG = nil
	assert.Equal(t, DefaultBackupKeep, effectiveBackupKeep())

	// BackupKeep <= 0
	CFG = &Configs{BackupKeep: 0}
	assert.Equal(t, DefaultBackupKeep, effectiveBackupKeep())

	CFG = &Configs{BackupKeep: -1}
	assert.Equal(t, DefaultBackupKeep, effectiveBackupKeep())

	// 正常值
	CFG = &Configs{BackupKeep: 7}
	assert.Equal(t, 7, effectiveBackupKeep())
}

// padSeq 返回固定宽度 (15 字符) 的数字字符串, 模拟时间戳字典序
// 真实场景中时间戳为 YYYYMMDD_HHMMSS (15 字符), 字典序 = 时间序
func padSeq(i int) string {
	return "20060102_15040" + string(rune('0'+i%10)) + "0"
}
