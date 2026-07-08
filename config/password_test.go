package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBcryptHash(t *testing.T) {
	// 真实 bcrypt 哈希示例 (cost 10, "password" 的哈希)
	realHash, _ := HashPassword("password")
	assert.Equal(t, 60, len(realHash), "bcrypt 哈希应为 60 字符")
	assert.True(t, IsBcryptHash(realHash), "真实 bcrypt 哈希应被识别")

	cases := []struct {
		input    string
		expected bool
	}{
		// 正例
		{"$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"$2b$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"$2y$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		// 负例: 长度不对
		{"$2a$10$short", false},
		{"$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhW", false},   // 59
		{"$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWyx", false}, // 61
		// 负例: 前缀不对
		{"$2x$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", false},
		{"$3a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", false},
		// 负例: 明文
		{"my-secret-password", false},
		{"", false},
		// 负例: env ref
		{"${MYSSHW_PWD_x}", false},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			assert.Equal(t, c.expected, IsBcryptHash(c.input), "input=%q", c.input)
		})
	}
}

func TestIsEnvReference(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"${MYSSHW_PWD_x}", true},
		{"${A}", true},
		{"${}", true}, // 空也算 (由 ResolveEnvRef 后续处理)
		{"plain-text", false},
		{"$2a$10$xxx", false},
		{"${unclosed", false},
		{"closed}", false},
		{"", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.expected, IsEnvReference(c.input), "input=%q", c.input)
	}
}

func TestHashPasswordAndVerify(t *testing.T) {
	plain := "my-secret-密码-🔐" // 测试非 ASCII + emoji

	hash, err := HashPassword(plain)
	assert.NoError(t, err)
	assert.Equal(t, 60, len(hash))
	assert.True(t, IsBcryptHash(hash), "HashPassword 返回值应通过 IsBcryptHash")

	// 正确密码
	err = VerifyPassword(hash, plain)
	assert.NoError(t, err)

	// 错误密码
	err = VerifyPassword(hash, "wrong-password")
	assert.Error(t, err)
}

func TestHashPassword_DifferentSalts(t *testing.T) {
	// 相同明文, 不同次调用 → 不同哈希 (bcrypt 自带随机 salt)
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	assert.NotEqual(t, h1, h2, "bcrypt 应每次生成不同 salt")
	assert.True(t, IsBcryptHash(h1))
	assert.True(t, IsBcryptHash(h2))

	// 但都应能验证通过
	assert.NoError(t, VerifyPassword(h1, "same"))
	assert.NoError(t, VerifyPassword(h2, "same"))
}

// ==================== Phase B: bcrypt-pepper ====================

func TestHashPasswordWithPepper_Basic(t *testing.T) {
	plain := "my-secret-pwd"
	pepper := "server-side-secret-32bytes-or-more"

	hash, err := HashPasswordWithPepper(plain, pepper)
	require.NoError(t, err)
	assert.Equal(t, 60, len(hash))
	assert.True(t, IsBcryptHash(hash))
}

func TestHashPasswordWithPepper_DifferentPeppers(t *testing.T) {
	// 相同明文 + 不同 pepper → 不同哈希
	plain := "same-pwd"
	h1, _ := HashPasswordWithPepper(plain, "pepper-A")
	h2, _ := HashPasswordWithPepper(plain, "pepper-B")
	assert.NotEqual(t, h1, h2, "不同 pepper 应产生不同哈希")
}

func TestHashPasswordWithPepper_EmptyPepperRejected(t *testing.T) {
	_, err := HashPasswordWithPepper("any", "")
	assert.Error(t, err, "空 pepper 必须被拒绝")
}

func TestVerifyPasswordWithPepper_RoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		plain  string
		pepper string
	}{
		{"ascii", "my-pwd", "secret-A"},
		{"unicode", "密码-🔐", "secret-B"},
		{"long pepper", "x", strings.Repeat("p", 64)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hash, err := HashPasswordWithPepper(c.plain, c.pepper)
			require.NoError(t, err)
			assert.NoError(t, VerifyPasswordWithPepper(hash, c.plain, c.pepper))
		})
	}
}

func TestVerifyPasswordWithPepper_WrongPepper(t *testing.T) {
	hash, _ := HashPasswordWithPepper("pwd", "correct-pepper")
	// 错误 pepper → 验证失败
	err := VerifyPasswordWithPepper(hash, "pwd", "wrong-pepper")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, bcryptErrMismatched()) || err != nil)
}

func TestVerifyPasswordWithPepper_WrongPlain(t *testing.T) {
	hash, _ := HashPasswordWithPepper("correct", "pepper")
	err := VerifyPasswordWithPepper(hash, "wrong", "pepper")
	assert.Error(t, err)
}

func TestVerifyPasswordWithPepper_EmptyPepperRejected(t *testing.T) {
	hash, _ := HashPasswordWithPepper("pwd", "any-pepper")
	err := VerifyPasswordWithPepper(hash, "pwd", "")
	assert.Error(t, err, "验证时空 pepper 必须被拒绝")
}

// bcryptErrMismatched 辅助函数, 避免直接 import bcrypt 在测试中
func bcryptErrMismatched() error {
	// 走一次错误验证以获取 err 类型
	h, _ := HashPassword("x")
	return VerifyPassword(h, "y")
}

func TestPepperRequired(t *testing.T) {
	cases := []struct {
		algorithm string
		want      bool
	}{
		{"", false},
		{"bcrypt", false},
		{"bcrypt-pepper", true},
		{"argon2id", false}, // 不需要 bcrypt-pepper
		{"pbkdf2-sha256", false},
		{"unknown", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, IsPepperRequired(c.algorithm), "algorithm=%q", c.algorithm)
	}
}

func TestIsArgon2idHash(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		// 标准 Argon2id 格式 (示例, 不一定能通过实际验证)
		{"$argon2id$v=19$m=65536,t=3,p=4$YWJjZGVmZ2hpamtsbW5vcA$P3dVem5Oem5", true},
		{"$argon2id$v=19$m=65536,t=3,p=4$xxxx", true},
		// 负例
		{"$2a$10$xxxxx", false},
		{"$bcrypt$xxxx", false},
		{"plain-password", false},
		{"", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, IsArgon2idHash(c.input), "input=%q", c.input)
	}
}

func TestIsPbkdf2Sha256Hash(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"$pbkdf2-sha256$100000$YWJjZGVmZ2hpamtsbW5vcA$xxxx", true},
		{"$pbkdf2-sha256$xxxxx", true},
		{"$2a$10$xxxxx", false},
		{"plain", false},
		{"", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, IsPbkdf2Sha256Hash(c.input), "input=%q", c.input)
	}
}

func TestResolveEncryptionAlgorithm(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", EncryptionAlgorithmAuto, false},
		{"bcrypt", EncryptionAlgorithmBcrypt, false},
		{"bcrypt-pepper", EncryptionAlgorithmBcryptPepper, false},
		{"bcrypt_pepper", EncryptionAlgorithmBcryptPepper, false}, // 允许下划线别名
		{"BCRYPT-PEPPER", EncryptionAlgorithmBcryptPepper, false}, // 大小写无关
		{"  bcrypt  ", EncryptionAlgorithmBcrypt, false},          // 允许 trim
		{"argon2id", EncryptionAlgorithmArgon2id, false},
		{"pbkdf2-sha256", EncryptionAlgorithmPbkdf2Sha256, false},
		{"unknown-algo", "", true},
		{"md5", "", true},
	}
	for _, c := range cases {
		got, err := ResolveEncryptionAlgorithm(c.input)
		if c.wantErr {
			assert.Error(t, err, "input=%q", c.input)
		} else {
			assert.NoError(t, err, "input=%q", c.input)
			assert.Equal(t, c.want, got, "input=%q", c.input)
		}
	}
}
