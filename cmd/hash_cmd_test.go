package cmd

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// hashCmd 单元测试
//
// 实现策略: hash 子命令的输出走 outputWriter() (默认 stderr), 通过
// --stdout 切到 stdout。我们用 os.Pipe 替换 os.Stdout 捕获, 然后调
// cobra cmd.Execute() (会 redirect 到 rootCmd 但 hash subcommand 走 RunE
// 不依赖 config 加载, 所以没问题)。
// ============================================================

// captureStdout 临时替换 os.Stdout, 执行 fn, 返回输出
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	// 异步读
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	out := <-done
	os.Stdout = origStdout
	return out
}

// captureStderr 临时替换 os.Stderr, 执行 fn, 返回输出
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
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
	os.Stderr = origStderr
	return out
}

// captureStdin 临时替换 os.Stdin
func captureStdin(t *testing.T, input string) {
	t.Helper()
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, _ = w.WriteString(input)
	_ = w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
}

// ==================== 注册测试 ====================

func TestHashCmd_Registered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "hash" {
			assert.Equal(t, "Password hashing utilities: gen / verify / pepper", c.Short)
			found = true
		}
	}
	assert.True(t, found, "hash subcommand not registered on rootCmd")
}

func TestHashCmd_HasSubcommands(t *testing.T) {
	names := make(map[string]bool)
	for _, c := range hashCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"gen", "verify", "pepper"} {
		assert.True(t, names[want], "missing subcommand %q", want)
	}
}

// ==================== algoFromHash 单元测试 ====================

func TestAlgoFromHash(t *testing.T) {
	// 真实 60 字符 bcrypt 哈希 (Phase 2 测试已验证)
	realBcrypt := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	cases := []struct {
		hash string
		want string
	}{
		{realBcrypt, "bcrypt"},
		{"$argon2id$v=19$m=65536,t=3,p=4$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcA", "argon2id"},
		{"$pbkdf2-sha256$600000$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcA", "pbkdf2-sha256"},
		{"plain-password", "unknown"},
		{"", "unknown"},
		{"$2a$10$short", "unknown"}, // 长度不够 60 字符
	}
	for _, c := range cases {
		assert.Equal(t, c.want, algoFromHash(c.hash), "hash=%q", c.hash)
	}
}

// ==================== readHashInput 单元测试 ====================

func TestReadHashInput_Arg(t *testing.T) {
	got, err := readHashInput("from-arg")
	require.NoError(t, err)
	assert.Equal(t, "from-arg", got)
}

func TestReadHashInput_StdinMarker(t *testing.T) {
	// 临时重定向 stdin
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("from-stdin\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	got, err := readHashInput("-")
	require.NoError(t, err)
	assert.Equal(t, "from-stdin", got)
}

// ==================== Hash 生成 + 验证集成测试 (跨函数) ====================

func TestHashGenAndVerifyRoundTrip_Bcrypt(t *testing.T) {
	plain := "round-trip-pwd-密码-🔐"
	hash, err := config.HashPassword(plain)
	require.NoError(t, err)
	assert.True(t, config.IsBcryptHash(hash))
	assert.NoError(t, config.VerifyPassword(hash, plain))
}

func TestHashGenAndVerifyRoundTrip_BcryptPepper(t *testing.T) {
	plain := "round-trip-pwd"
	pepper := "server-side-secret-32bytes"

	hash, err := config.HashPasswordWithPepper(plain, pepper)
	require.NoError(t, err)
	assert.True(t, config.IsBcryptHash(hash))

	// 正确 pepper 验证通过
	assert.NoError(t, config.VerifyPasswordWithPepper(hash, plain, pepper))

	// 错误 pepper 验证失败
	assert.Error(t, config.VerifyPasswordWithPepper(hash, plain, "wrong-pepper"))

	// 错误明文验证失败
	assert.Error(t, config.VerifyPasswordWithPepper(hash, "wrong-plain", pepper))
}

// ==================== 直接调 RunE 函数 (绕开 cobra 副作用) ====================

func TestRunHashGen_DirectBcrypt(t *testing.T) {
	origAlgo := hashAlgo
	origPepper := hashPepper
	origStdout := hashStdout
	defer func() {
		hashAlgo = origAlgo
		hashPepper = origPepper
		hashStdout = origStdout
	}()
	hashAlgo = "bcrypt"
	hashPepper = ""
	hashStdout = true

	// 调 RunE 函数 (RunE 的签名: func(cmd *cobra.Command, args []string) error)
	// 注意: cobra 期望 RunE 通过 cmd.Execute() 调用, 但 RunE 也可以直接调
	hashGenCmd.SetArgs([]string{"test-pwd"})
	err := hashGenCmd.RunE(hashGenCmd, []string{"test-pwd"})
	require.NoError(t, err)
}

func TestRunHashGen_DirectBcryptPepper_RequiresPepper(t *testing.T) {
	origAlgo := hashAlgo
	origPepper := hashPepper
	defer func() {
		hashAlgo = origAlgo
		hashPepper = origPepper
	}()
	hashAlgo = "bcrypt-pepper"
	hashPepper = "" // 故意空

	err := hashGenCmd.RunE(hashGenCmd, []string{"test-pwd"})
	assert.Error(t, err, "bcrypt-pepper without --pepper should fail")
	assert.Contains(t, err.Error(), "pepper")
}

func TestRunHashGen_DirectBcryptPepper_Success(t *testing.T) {
	origAlgo := hashAlgo
	origPepper := hashPepper
	defer func() {
		hashAlgo = origAlgo
		hashPepper = origPepper
	}()
	hashAlgo = "bcrypt-pepper"
	hashPepper = "test-pepper-32bytes"

	err := hashGenCmd.RunE(hashGenCmd, []string{"my-pwd"})
	assert.NoError(t, err)
}

func TestRunHashGen_DirectUnknownAlgo(t *testing.T) {
	origAlgo := hashAlgo
	defer func() { hashAlgo = origAlgo }()
	hashAlgo = "made-up-algo"

	err := hashGenCmd.RunE(hashGenCmd, []string{"pwd"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown --algo")
}

func TestRunHashGen_DirectArgon2id_FallbackToBcrypt(t *testing.T) {
	origAlgo := hashAlgo
	defer func() { hashAlgo = origAlgo }()
	hashAlgo = "argon2id"

	err := hashGenCmd.RunE(hashGenCmd, []string{"pwd"})
	assert.NoError(t, err, "argon2id 应该 fallback 到 bcrypt (Phase B+)")
}

// ==================== verify 单元 (直接 RunE) ====================

func TestRunHashVerify_DirectMatch(t *testing.T) {
	hash, _ := config.HashPassword("correct")

	origPepper := hashPepper
	defer func() { hashPepper = origPepper }()
	hashPepper = ""

	// 临时替换 stdin
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("correct\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	err := hashVerifyCmd.RunE(hashVerifyCmd, []string{hash})
	assert.NoError(t, err)
}

func TestRunHashVerify_DirectMismatch_Exits(t *testing.T) {
	hash, _ := config.HashPassword("correct")

	origPepper := hashPepper
	defer func() { hashPepper = origPepper }()
	hashPepper = ""

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("wrong\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	err := hashVerifyCmd.RunE(hashVerifyCmd, []string{hash})
	assert.Error(t, err, "wrong password should fail verify")
}

func TestRunHashVerify_DirectBcryptPepper_Match(t *testing.T) {
	hash, _ := config.HashPasswordWithPepper("my-pwd", "my-pepper")

	origPepper := hashPepper
	defer func() { hashPepper = origPepper }()
	hashPepper = "my-pepper"

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("my-pwd\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	err := hashVerifyCmd.RunE(hashVerifyCmd, []string{hash})
	assert.NoError(t, err)
}

func TestRunHashVerify_DirectBcryptPepper_WrongPepper(t *testing.T) {
	hash, _ := config.HashPasswordWithPepper("my-pwd", "my-pepper")

	origPepper := hashPepper
	defer func() { hashPepper = origPepper }()
	hashPepper = "wrong-pepper"

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("my-pwd\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	err := hashVerifyCmd.RunE(hashVerifyCmd, []string{hash})
	assert.Error(t, err)
}

func TestRunHashVerify_DirectInvalidHash(t *testing.T) {
	origPepper := hashPepper
	defer func() { hashPepper = origPepper }()
	hashPepper = ""

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("pwd\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	err := hashVerifyCmd.RunE(hashVerifyCmd, []string{"not-a-hash"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hash")
}

func TestRunHashVerify_DirectArgon2idNotImplemented(t *testing.T) {
	argon2idHash := "$argon2id$v=19$m=65536,t=3,p=4$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcA"

	origPepper := hashPepper
	defer func() { hashPepper = origPepper }()
	hashPepper = ""

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("pwd\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	err := hashVerifyCmd.RunE(hashVerifyCmd, []string{argon2idHash})
	assert.Error(t, err, "argon2id verify not yet implemented should fail")
	assert.Contains(t, err.Error(), "not yet implemented")
}

// ==================== pepper 单元 (直接 RunE) ====================

func TestRunHashPepper_Direct(t *testing.T) {
	// RunE 写到 os.Stdout (强制), 验证输出
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	err := hashPepperCmd.RunE(hashPepperCmd, []string{})
	require.NoError(t, err)
	_ = w.Close()
	out := <-done

	// 应输出 32 字节 base64 编码 (44 字符, 含 padding)
	out = strings.TrimSpace(out)
	assert.Len(t, out, 44, "32-byte base64 = 44 chars: %q", out)

	// 解码后应该是 32 字节
	decoded, err := base64.StdEncoding.DecodeString(out)
	require.NoError(t, err)
	assert.Len(t, decoded, 32, "decoded pepper should be 32 bytes")
}

func TestRunHashPepper_Direct_Unique(t *testing.T) {
	// 多次调用应产生不同的 pepper
	peppers := make(map[string]bool)
	for i := 0; i < 5; i++ {
		origStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		done := make(chan string)
		go func() {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			done <- buf.String()
		}()

		err := hashPepperCmd.RunE(hashPepperCmd, []string{})
		require.NoError(t, err)
		_ = w.Close()
		out := strings.TrimSpace(<-done)
		os.Stdout = origStdout

		assert.False(t, peppers[out], "pepper should be unique: %q", out)
		peppers[out] = true
	}
}

// ==================== 引用以避免 unused import 警告 ====================

var _ = io.Discard
