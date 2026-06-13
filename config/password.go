package config

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// 密码字段格式 (Phase 2 引入):
//   - 明文:    "my-secret-password"
//   - 环境变量: "${MYSSHW_PWD_node1}"  (由 ssh.ResolveEnvRef 处理)
//   - bcrypt 哈希: "$2a$10$..."  (60 字符, 必须 bcrypt 验证后使用)
//   - 空字符串: ""  (由 ssh 包交互提示输入)
//
// 自动检测策略 (B1 决策):
//   1. 长度 60 且以 "$2a$"/"$2b$"/"$2y$" 开头 → bcrypt 哈希
//   2. 以 "${" 开头且以 "}" 结尾 → 环境变量引用 (由 ResolveEnvRef 后续处理)
//   3. 否则 → 明文 (兼容老版本)

// BcryptHashPrefixes bcrypt 哈希的有效前缀 (算法版本标识)
var BcryptHashPrefixes = []string{"$2a$", "$2b$", "$2y$"}

// BcryptHashLength 标准 bcrypt 哈希长度 (60 字符: 2a$10$[22 salt][31 hash])
const BcryptHashLength = 60

// IsBcryptHash 判断字符串是否为有效 bcrypt 哈希
// 规则: 必须以 $2a$/$2b$/$2y$ 之一开头 + 长度恰好 60
func IsBcryptHash(s string) bool {
	if len(s) != BcryptHashLength {
		return false
	}
	for _, prefix := range BcryptHashPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// IsEnvReference 判断是否为 ${...} 环境变量引用
func IsEnvReference(s string) bool {
	return strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}")
}

// HashPassword 用 bcrypt 哈希明文密码
// cost 使用 bcrypt.DefaultCost (10), ~60ms 单次哈希 (足够慢以抗暴力)
// 返回: 完整 bcrypt 哈希字符串 (60 字符), 可直接存入 TOML
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword 验证明文密码是否与 bcrypt 哈希匹配
// 返回 nil 表示匹配, ErrMismatchedHashAndPassword 表示不匹配
func VerifyPassword(hash, plaintext string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
}
