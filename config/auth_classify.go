package config

import "strings"

// ClassifyAuth 根据节点字段归类认证方式（仅展示用，不输出密码本身）。
//
//	空 password + 有 KeyPath       → "key"
//	空 password                    → "empty"
//	bcrypt 哈希 + 无 pepper        → "bcrypt"
//	bcrypt 哈希 + bcrypt-pepper 模式 → "bcrypt-pepper"
//	Argon2id 哈希                  → "argon2id"
//	PBKDF2-SHA256 哈希             → "pbkdf2-sha256"
//	${ENV} 引用                     → "env"
//	有 KeyPath 但 password 也有     → "<auth>+key" (e.g. "password+key")
//	其他明文                        → "password"
//
// 加密算法上下文由 cfg.EncryptionAlgorithm 决定:
//   - "bcrypt-pepper" → 所有 bcrypt 节点标记为 "bcrypt-pepper" (Phase B)
//   - 其他 / 空 → 标准 "bcrypt"
//
// 调用方:
//   - cmd/list_cmd.go::classifyAuth (历史遗留, 已迁移到此函数)
//   - cmd/config_cmd.go::classifyAuthForCLI
//   - MCP tools 输出的 auth 字段
func ClassifyAuth(n *SSHNode, cfg *Configs) string {
	if n == nil {
		return "unknown"
	}
	pwd := n.Password
	hasKey := n.KeyPath != ""

	if pwd == "" {
		if hasKey {
			return "key"
		}
		return "empty"
	}

	auth := ""
	if IsBcryptHash(pwd) {
		// Phase B: bcrypt-pepper 模式标记
		// 如果顶层 algorithm 显式指定 bcrypt-pepper, 所有 bcrypt 节点标记为 pepper
		// (因为运行时都会用 pepper 验证, 区别仅在 server 侧 secret_key 是否存在)
		if cfg != nil {
			if algo, _ := ResolveEncryptionAlgorithm(cfg.EncryptionAlgorithm); IsPepperRequired(algo) {
				auth = "bcrypt-pepper"
			} else {
				auth = "bcrypt"
			}
		} else {
			auth = "bcrypt"
		}
	} else if IsArgon2idHash(pwd) {
		// Phase B+ 占位识别: Argon2id 格式哈希
		auth = "argon2id"
	} else if IsPbkdf2Sha256Hash(pwd) {
		// Phase B+ 占位识别: PBKDF2-SHA256 格式哈希
		auth = "pbkdf2-sha256"
	} else if strings.HasPrefix(strings.TrimSpace(pwd), "${") {
		auth = "env"
	} else {
		auth = "password"
	}

	if hasKey {
		return auth + "+key"
	}
	return auth
}
