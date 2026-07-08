package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== LoadViperConfig 加密文件集成测试 ====================

// TestLoadViperConfig_Encrypted 验证 LoadViperConfig 能自动识别加密文件
// 并提示 passphrase 解密
func TestLoadViperConfig_Encrypted(t *testing.T) {
	// 准备: 加密 DefaultTomlConfig
	original := []byte(DefaultTomlConfig)
	passphrase := []byte("integration-test-pass-32bytes")

	cipher, err := EncryptConfig(original, passphrase)
	require.NoError(t, err)

	// 写到临时文件
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(cfgPath, cipher, 0600))

	// 模拟 stdin 输入 passphrase
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Write(passphrase)
	w.Write([]byte("\n"))
	w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	// 加载: 应自动检测加密, 提示输入 passphrase, 解密后解析
	err = LoadViperConfig(cfgPath)
	require.NoError(t, err, "LoadViperConfig should handle encrypted file")

	// 验证: 配置已加载 (CFG 不为空, 至少有一个 group)
	assert.NotNil(t, CFG)
	assert.NotEmpty(t, CFG.Nodes, "decrypted config should have nodes")
	t.Cleanup(func() { CFG = nil; CFG_PATH = "~/.mysshw.toml" })
}

func TestLoadViperConfig_Encrypted_WrongPassphrase(t *testing.T) {
	// 加密文件
	cipher, err := EncryptConfig([]byte(DefaultTomlConfig), []byte("correct-pass"))
	require.NoError(t, err)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(cfgPath, cipher, 0600))

	// 模拟 stdin 输入错误 passphrase
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Write([]byte("wrong-pass"))
	w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	err = LoadViperConfig(cfgPath)
	assert.Error(t, err, "wrong passphrase should fail load")
}

func TestLoadViperConfig_PlainFile_StillWorks(t *testing.T) {
	// 验证: 旧的不加密配置文件仍可正常加载 (兼容性)
	plain := []byte(DefaultTomlConfig)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(cfgPath, plain, 0600))

	// 不需要 stdin (plain 文件无 passphrase prompt)
	err := LoadViperConfig(cfgPath)
	require.NoError(t, err, "plain file should still work (backward compat)")
	assert.NotEmpty(t, CFG.Nodes)
	t.Cleanup(func() { CFG = nil; CFG_PATH = "~/.mysshw.toml" })
}

// ==================== File header 检测 ====================

func TestIsEncryptedFile_RealConfig(t *testing.T) {
	plain := []byte(DefaultTomlConfig)
	cipher, _ := EncryptConfig(plain, []byte("pass"))

	// 写到文件再读, 模拟实际加载流程
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(path, cipher, 0600))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, IsEncryptedFile(raw))
	assert.False(t, IsEncryptedFile(plain), "plain TOML should not be detected")
}

// ==================== 内存 round-trip (无需文件) ====================

func TestDecryptConfig_MemoryRoundTrip(t *testing.T) {
	cases := [][]byte{
		[]byte("simple"),
		[]byte("with\nnewlines\nand\ttabs"),
		bytes.Repeat([]byte("X"), 10000),
		[]byte("密码-🔐-emoji-测试"),
	}
	pass := []byte("test-pass")

	for i, plain := range cases {
		cipher, err := EncryptConfig(plain, pass)
		require.NoError(t, err, "case %d: encrypt", i)

		decrypted, err := DecryptConfig(cipher, pass)
		require.NoError(t, err, "case %d: decrypt", i)
		assert.Equal(t, plain, decrypted, "case %d: round-trip", i)
	}
}
