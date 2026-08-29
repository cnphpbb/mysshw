package ssh

import (
	"os"
	"sync"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	auth, cleartext := resolvePasswordForSSH(node, "", true, nil)
	assert.NotNil(t, auth, "应返回非 nil AuthMethod")
	assert.Equal(t, "test-password", cleartext)
}

// TestResolvePasswordForSSH_Bcrypt 验证 bcrypt 哈希 + 正确明文
func TestResolvePasswordForSSH_Bcrypt(t *testing.T) {
	hash, err := config.HashPassword("secret-pwd")
	require.NoError(t, err)

	defer pipeStdin(t, "secret-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}

	auth, cleartext := resolvePasswordForSSH(node, hash, true, nil)
	assert.NotNil(t, auth)
	assert.Equal(t, "secret-pwd", cleartext)
}

// TestResolvePasswordForSSH_BcryptMismatch 验证 bcrypt 验证失败
func TestResolvePasswordForSSH_BcryptMismatch(t *testing.T) {
	hash, _ := config.HashPassword("correct")

	defer pipeStdin(t, "wrong-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}

	auth, cleartext := resolvePasswordForSSH(node, hash, true, nil)
	assert.Nil(t, auth, "验证失败应返回 nil")
	assert.Empty(t, cleartext)
}

// TestResolvePasswordForSSH_Plain 验证明文直接使用
func TestResolvePasswordForSSH_Plain(t *testing.T) {
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	// isTerminal(pipe) = false → 警告跳过, 明文直接使用
	auth, cleartext := resolvePasswordForSSH(node, "plain-password", true, nil)
	assert.NotNil(t, auth)
	assert.Equal(t, "plain-password", cleartext)
}

// TestResolvePasswordForSSH_EmptyPipeEmpty 验证 stdin 空 → 跳过认证
func TestResolvePasswordForSSH_EmptyPipeEmpty(t *testing.T) {
	defer pipeStdin(t, "")()

	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	auth, cleartext := resolvePasswordForSSH(node, "", true, nil)
	assert.Nil(t, auth, "空密码 + 空输入应跳过认证")
	assert.Empty(t, cleartext)
}

// TestResolvePasswordForSSH_NonInteractiveSkipsStdin 验证 allowStdin=false 时
// 不读 stdin (AI/exec 场景)。即使 stdin 里有内容也直接返回 nil。
func TestResolvePasswordForSSH_NonInteractiveSkipsStdin(t *testing.T) {
	defer pipeStdin(t, "ignored-input\n")()

	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	auth, cleartext := resolvePasswordForSSH(node, "", false, nil)
	assert.Nil(t, auth, "非交互 + 空密码 → nil auth")
	assert.Empty(t, cleartext)
}

// ==================== Phase B: bcrypt-pepper 验证 ====================

// TestResolvePasswordForSSH_BcryptPepper_Success 验证 bcrypt-pepper 正确明文 + secret_key
func TestResolvePasswordForSSH_BcryptPepper_Success(t *testing.T) {
	pepper := "server-side-secret-32bytes"
	hash, err := config.HashPasswordWithPepper("secret-pwd", pepper)
	require.NoError(t, err)

	defer pipeStdin(t, "secret-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	cfg := &config.Configs{
		EncryptionAlgorithm: "bcrypt-pepper",
		SecretKey:           pepper, // 明文 pepper (实际部署应用 env 引用)
	}

	auth, cleartext := resolvePasswordForSSH(node, hash, true, cfg)
	assert.NotNil(t, auth, "bcrypt-pepper 验证通过应返回非 nil")
	assert.Equal(t, "secret-pwd", cleartext)
}

// TestResolvePasswordForSSH_BcryptPepper_WrongPepper 验证错误 pepper → 验证失败
func TestResolvePasswordForSSH_BcryptPepper_WrongPepper(t *testing.T) {
	hash, _ := config.HashPasswordWithPepper("secret-pwd", "correct-pepper")

	defer pipeStdin(t, "secret-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	cfg := &config.Configs{
		EncryptionAlgorithm: "bcrypt-pepper",
		SecretKey:           "WRONG-PEPPER",
	}

	auth, cleartext := resolvePasswordForSSH(node, hash, true, cfg)
	assert.Nil(t, auth, "错误 pepper 应验证失败")
	assert.Empty(t, cleartext)
}

// TestResolvePasswordForSSH_BcryptPepper_EmptySecretKey 验证 algorithm=bcrypt-pepper 但
// secret_key 缺失 → 跳过认证 (避免退化到标准 bcrypt 让攻击者得逞)
func TestResolvePasswordForSSH_BcryptPepper_EmptySecretKey(t *testing.T) {
	hash, _ := config.HashPasswordWithPepper("secret-pwd", "any-pepper")

	defer pipeStdin(t, "secret-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	cfg := &config.Configs{
		EncryptionAlgorithm: "bcrypt-pepper",
		SecretKey:           "", // 故意空
	}

	auth, cleartext := resolvePasswordForSSH(node, hash, true, cfg)
	assert.Nil(t, auth, "bcrypt-pepper 节点 + 空 secret_key 应拒绝")
	assert.Empty(t, cleartext)
}

// TestResolvePasswordForSSH_BcryptWithoutPepper 验证 algorithm="" 时按标准 bcrypt 处理
// (兼容 v26.07.04.0 行为, 即便 config 里有 secret_key 也不使用)
func TestResolvePasswordForSSH_BcryptWithoutPepper(t *testing.T) {
	// 标准 bcrypt 哈希 (无 pepper)
	hash, _ := config.HashPassword("secret-pwd")

	defer pipeStdin(t, "secret-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	cfg := &config.Configs{
		EncryptionAlgorithm: "",           // 留空 → 兼容模式
		SecretKey:           "any-pepper", // 配置了但 algorithm 没指定, 不使用
	}

	auth, cleartext := resolvePasswordForSSH(node, hash, true, cfg)
	assert.NotNil(t, auth, "algorithm 留空时按标准 bcrypt 验证")
	assert.Equal(t, "secret-pwd", cleartext)
}

// TestResolvePasswordForSSH_BcryptPepper_EnvRef 验证 secret_key 走 ${ENV} 引用
func TestResolvePasswordForSSH_BcryptPepper_EnvRef(t *testing.T) {
	pepper := "env-pepper-32bytes"
	t.Setenv("MYSSHW_TEST_PEPPER", pepper)

	hash, err := config.HashPasswordWithPepper("secret-pwd", pepper)
	require.NoError(t, err)

	defer pipeStdin(t, "secret-pwd\n")()
	node := &config.SSHNode{Name: "test", Host: "127.0.0.1"}
	cfg := &config.Configs{
		EncryptionAlgorithm: "bcrypt-pepper",
		SecretKey:           "${MYSSHW_TEST_PEPPER}", // 引用 env
	}

	auth, cleartext := resolvePasswordForSSH(node, hash, true, cfg)
	assert.NotNil(t, auth, "${ENV} 引用的 secret_key 应被解析")
	assert.Equal(t, "secret-pwd", cleartext)
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
	// 编译期类型检查: plainPwdWarnOnce 必须是 sync.Once
	// 不能用 assert.NotNil (会拷贝 sync.Once, 触发 vet noCopy 警告)
	var _ *sync.Once = &plainPwdWarnOnce
	assert.NotNil(t, &plainPwdWarnOnce, "plainPwdWarnOnce address must be non-nil (it's a global)")
}
