package ssh

import (
	"os"
	"regexp"
	"strings"
)

// envRefPattern 匹配 ${VAR} 或 ${VAR:-default} 形式的环境变量引用
// VAR 必须以字母或下划线开头, 后续字符可为字母/数字/下划线
// 默认值可包含除 } 外的任意字符 (含空格和标点)
var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// IsEnvTrue 解析布尔型环境变量 (true/1/yes, 大小写不敏感, 忽略前后空格)
// 未设置或值非 true/1/yes 视为 false
func IsEnvTrue(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "true" || v == "1" || v == "yes"
}

// ResolveEnvRef 替换字符串中的 ${VAR} 和 ${VAR:-default} 引用为环境变量值
// 规则:
//   - ${VAR}: 取 $VAR; 若未设置或为空, 结果中该引用替换为空字符串
//   - ${VAR:-default}: 若 $VAR 未设置或为空, 使用 default
//   - 其他文本原样保留
//
// 示例:
//   ResolveEnvRef("${MYSSHW_PWD}")                → os.Getenv("MYSSHW_PWD")
//   ResolveEnvRef("${MYSSHW_PWD:-fallback}")     → env 值或 "fallback"
//   ResolveEnvRef("prefix-${A}-${B}")            → 拼接替换结果
//   ResolveEnvRef("plaintext")                    → "plaintext"
//
// 导出供 cmd 等其他包复用
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
