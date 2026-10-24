package config

import (
	"os"
	"regexp"
	"strings"
)

// envRefPattern 匹配 ${VAR} 或 ${VAR:-default} 形式的环境变量引用
// VAR 必须以字母或下划线开头, 后续字符可为字母/数字/下划线
// 默认值可包含除 } 外的任意字符 (含空格和标点)
var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// ResolveEnvRef 替换字符串中的 ${VAR} 和 ${VAR:-default} 引用为环境变量值
// 规则:
//   - ${VAR}: 取 $VAR; 若未设置或为空, 结果中该引用替换为空字符串
//   - ${VAR:-default}: 若 $VAR 未设置或为空, 使用 default
//   - 其他文本原样保留
//
// 示例:
//
//	ResolveEnvRef("${MYSSHW_PWD}")                → os.Getenv("MYSSHW_PWD")
//	ResolveEnvRef("${MYSSHW_PWD:-fallback}")     → env 值或 "fallback"
//	ResolveEnvRef("prefix-${A}-${B}")            → 拼接替换结果
//	ResolveEnvRef("plaintext")                    → "plaintext"
//
// 放在 config 包的原因: 配置检查 (config_check.go) 需要判断
// "[sync].passphrase 里的 ${VAR} 引用是否真的能取到值",
// 而 config 不能反向依赖 ssh 包 (ssh → config 已存在依赖)。
// ssh.ResolveEnvRef 保留为薄封装, 兼容既有调用方。
func ResolveEnvRef(s string) string {
	if !strings.Contains(s, "${") {
		return s // 快速路径
	}
	return envRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		groups := envRefPattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		name := groups[1]
		val, ok := os.LookupEnv(name)
		if ok && val != "" {
			return val
		}
		// groups[2] 存在即视为有默认段 (含空默认值 ${VAR:-})
		if len(groups) >= 3 {
			return groups[2]
		}
		return ""
	})
}

// HasEnvRef 判断字符串里是否含有 ${VAR} 形式的环境变量引用
func HasEnvRef(s string) bool {
	return strings.Contains(s, "${") && envRefPattern.MatchString(s)
}

// IsEnvRefPlaintext 判断字符串是否"看起来"是明文 (不是 env 引用、也不是 bcrypt 哈希)
// 用于配置检查里提示"密码明文落盘"
func IsEnvRefPlaintext(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if HasEnvRef(s) {
		return false
	}
	return !IsBcryptHash(s)
}
