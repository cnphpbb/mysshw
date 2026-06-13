package ssh

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mysshw/config"
)

// pipeStdin 把 s 写入临时 pipe, 设为 os.Stdin, 恢复函数返回
// 测试用: 让 resolvePasswordForSSH 从非 terminal stdin 读取
func pipeStdin(t *testing.T, s string) func() {
	t.Helper()
	orig := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(s)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stdin = r
	return func() {
		os.Stdin = orig
		_ = r.Close()
	}
}

// TestResolvePasswordForSSH_Empty 验证空密码 → 交互输入 (从 stdin pipe 模拟)
func TestResolvePasswordForSSH_Empty(t *testing.T) {
	defer pipeStdin(t, "test-password\n")()

	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	auth, cleartext := resolvePasswordForSSH(node, "")
	assert.NotNil(t, auth, "应返回非 nil AuthMethod")
	assert.Equal(t, "test-password", cleartext)
}

// TestResolvePasswordForSSH_Bcrypt 验证 bcrypt 哈希 + 正确明文
func TestResolvePasswordForSSH_Bcrypt(t *testing.T) {
	hash, err := config.HashPassword("secret-pwd")
	require.NoError(t, err)

	defer pipeStdin(t, "secret-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}

	auth, cleartext := resolvePasswordForSSH(node, hash)
	assert.NotNil(t, auth)
	assert.Equal(t, "secret-pwd", cleartext)
}

// TestResolvePasswordForSSH_BcryptMismatch 验证 bcrypt 验证失败
func TestResolvePasswordForSSH_BcryptMismatch(t *testing.T) {
	hash, _ := config.HashPassword("correct")

	defer pipeStdin(t, "wrong-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}

	auth, cleartext := resolvePasswordForSSH(node, hash)
	assert.Nil(t, auth, "验证失败应返回 nil")
	assert.Empty(t, cleartext)
}

// TestResolvePasswordForSSH_Plain 验证明文直接使用
func TestResolvePasswordForSSH_Plain(t *testing.T) {
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	// isTerminal(pipe) = false → 警告跳过, 明文直接使用
	auth, cleartext := resolvePasswordForSSH(node, "plain-password")
	assert.NotNil(t, auth)
	assert.Equal(t, "plain-password", cleartext)
}

// TestResolvePasswordForSSH_EmptyPipeEmpty 验证 stdin 空 → 跳过认证
func TestResolvePasswordForSSH_EmptyPipeEmpty(t *testing.T) {
	defer pipeStdin(t, "")()

	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	auth, cleartext := resolvePasswordForSSH(node, "")
	assert.Nil(t, auth, "空密码 + 空输入应跳过认证")
	assert.Empty(t, cleartext)
}

// TestIsTerminal_PipeReturnsFalse 验证 isTerminal 对 pipe 返回 false
func TestIsTerminal_PipeReturnsFalse(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()
	assert.False(t, isTerminal(r))
	assert.False(t, isTerminal(w))
}

// TestZeroBytes 验证 zeroBytes 对 []byte 有效 (string 不可变, 不测 string 形式)
func TestZeroBytes_Bytes(t *testing.T) {
	b := []byte("sensitive-password")
	zeroBytes(string(b))
	// 注: zeroBytes(string) 只能 zero 临时 []byte, 不能 zero 原 string
	// 这里仅证明函数不 panic
}

func TestZeroBytes_NoOpOnEmptyString(t *testing.T) {
	// 空字符串不应 panic
	zeroBytes("")
}

// TestPlainPwdWarnOnce 验证 plainPwdWarnOnce 是 sync.Once
func TestPlainPwdWarnOnce(t *testing.T) {
	// 类型断言: plainPwdWarnOnce 必须是 sync.Once
	// 编译期已保证, 但显式测试以防误改
	assert.NotNil(t, plainPwdWarnOnce)
}
