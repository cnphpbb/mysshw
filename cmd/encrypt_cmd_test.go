package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== 注册测试 ====================

func TestEncryptCmd_Registered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "encrypt" {
			assert.Equal(t, "Encrypt mysshw.toml with a passphrase (Argon2id + AES-256-GCM)", c.Short)
			found = true
		}
	}
	assert.True(t, found, "encrypt subcommand not registered")
}

func TestEncryptCmd_HasDecryptSubcommand(t *testing.T) {
	for _, c := range encryptCmd.Commands() {
		if c.Name() == "decrypt" {
			return
		}
	}
	t.Fatal("decrypt subcommand not registered under encrypt")
}

// ==================== readPassphraseInteractive ====================

func TestReadPassphraseInteractive_NormalStdin(t *testing.T) {
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("my-secret-pass\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	pass, err := readPassphraseInteractive("prompt: ")
	require.NoError(t, err)
	assert.Equal(t, "my-secret-pass", pass)
}

func TestReadPassphraseInteractive_StripsCRLF(t *testing.T) {
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("pass\r\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	pass, err := readPassphraseInteractive("")
	require.NoError(t, err)
	assert.Equal(t, "pass", pass, "should strip trailing \\r\\n")
}

func TestReadPassphraseInteractive_EmptyStdin(t *testing.T) {
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Close() // 立即 EOF
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	pass, err := readPassphraseInteractive("")
	// empty stdin: err 是 nil (bufio.Scan 返回 false, Err() 也是 nil)
	// 但 passphrase 是空字符串 — caller (encrypt) 会检测并报错
	assert.NoError(t, err)
	assert.Empty(t, pass, "应返回空字符串, 由调用方检测")
}

// ==================== runEncryptOrDecrypt 集成测试 ====================

// 创建一个临时明文 config 文件
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

// 读 passphrase 到 stdin
func feedStdin(t *testing.T, s string) {
	t.Helper()
	orig := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString(s)
	w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
}

// captureStderrEnc captures os.Stderr for the duration of fn
func captureStderrEnc(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	out := <-done
	os.Stderr = orig
	return out
}

func TestEncryptDecrypt_RoundTrip_File(t *testing.T) {
	original := "[nodes]\ngroups = \"Test\"\n[[nodes.ssh]]\nname = \"x\"\nhost = \"1.2.3.4\"\n"
	inputPath := writeTempConfig(t, original)

	// 加密
	feedStdin(t, "test-passphrase-32bytes\n")
	encryptInput = inputPath
	encryptOutput = inputPath + ".enc"
	encryptInPlace = false
	encryptStdout = false

	err := runEncryptOrDecrypt(encryptCmd, true)
	require.NoError(t, err, "encrypt should succeed")

	// 验证输出文件存在且是加密格式
	cipher, err := os.ReadFile(inputPath + ".enc")
	require.NoError(t, err)
	assert.True(t, config.IsEncryptedFile(cipher), "output should be encrypted")

	// 解密
	feedStdin(t, "test-passphrase-32bytes\n")
	encryptInput = inputPath + ".enc"
	encryptOutput = inputPath + ".dec"
	err = runEncryptOrDecrypt(encryptDecryptCmd, false)
	require.NoError(t, err, "decrypt should succeed")

	// 验证解密后与原文件一致
	decrypted, err := os.ReadFile(inputPath + ".dec")
	require.NoError(t, err)
	assert.Equal(t, original, string(decrypted))
}

func TestEncryptDecrypt_WrongPassphrase(t *testing.T) {
	original := "[nodes]\ngroups = \"T\"\n"
	inputPath := writeTempConfig(t, original)

	// 加密
	feedStdin(t, "correct-pass\n")
	encryptInput = inputPath
	encryptOutput = inputPath + ".enc"
	encryptInPlace = false
	encryptStdout = false
	require.NoError(t, runEncryptOrDecrypt(encryptCmd, true))

	// 错 passphrase 解密
	feedStdin(t, "wrong-pass\n")
	encryptInput = inputPath + ".enc"
	encryptOutput = ""
	err := runEncryptOrDecrypt(encryptDecryptCmd, false)
	assert.Error(t, err, "wrong passphrase should fail")
}

func TestEncrypt_RejectsAlreadyEncrypted(t *testing.T) {
	// 先创建一个加密文件
	original := "test"
	cipher, err := config.EncryptConfig([]byte(original), []byte("pass"))
	require.NoError(t, err)

	inputPath := writeTempConfig(t, string(cipher))

	// 尝试再加密
	feedStdin(t, "any-pass\n")
	encryptInput = inputPath
	encryptOutput = ""
	encryptInPlace = false
	err = runEncryptOrDecrypt(encryptCmd, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already encrypted")
}

func TestDecrypt_RejectsPlainFile(t *testing.T) {
	plain := "[nodes]\ngroups = \"T\"\n"
	inputPath := writeTempConfig(t, plain)

	feedStdin(t, "any-pass\n")
	encryptInput = inputPath
	encryptOutput = ""
	err := runEncryptOrDecrypt(encryptDecryptCmd, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not encrypted")
}

func TestEncrypt_EmptyPassphrase(t *testing.T) {
	original := "test"
	inputPath := writeTempConfig(t, original)

	feedStdin(t, "\n") // 空 passphrase
	encryptInput = inputPath
	encryptOutput = inputPath + ".enc"
	err := runEncryptOrDecrypt(encryptCmd, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty passphrase")
}

func TestEncrypt_StdoutFlag(t *testing.T) {
	original := "stdout-test"
	inputPath := writeTempConfig(t, original)

	feedStdin(t, "test-pass\n")
	encryptInput = inputPath
	encryptOutput = ""
	encryptStdout = true

	// capture stdout
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	err := runEncryptOrDecrypt(encryptCmd, true)
	require.NoError(t, err)

	_ = w.Close()
	out := <-done
	os.Stdout = origStdout

	// 验证: 输出是加密格式
	assert.True(t, config.IsEncryptedFile([]byte(out)), "stdout output should be encrypted")

	// 验证: 没有写文件
	_, err = os.Stat(inputPath + ".enc")
	assert.True(t, os.IsNotExist(err), "no file should be written when --stdout is set")
}

func TestEncrypt_NonexistentInputFile(t *testing.T) {
	feedStdin(t, "pass\n")
	encryptInput = "/nonexistent/mysshw.toml"
	encryptOutput = ""
	err := runEncryptOrDecrypt(encryptCmd, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read input")
}

// ==================== Atomic Write ====================

func TestEncrypt_AtomicWrite_TempFileRemovedOnSuccess(t *testing.T) {
	original := "atomic-test"
	inputPath := writeTempConfig(t, original)

	feedStdin(t, "pass\n")
	encryptInput = inputPath
	encryptOutput = filepath.Join(filepath.Dir(inputPath), "out.toml")
	encryptInPlace = false
	encryptStdout = false
	require.NoError(t, runEncryptOrDecrypt(encryptCmd, true))

	// temp file 不应残留
	_, err := os.Stat(encryptOutput + ".tmp")
	assert.True(t, os.IsNotExist(err), "temp file should be removed on success")
}
