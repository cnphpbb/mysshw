package ssh

import (
	"os"
	"strings"

	"mysshw/config"
)

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
//
//	ResolveEnvRef("${MYSSHW_PWD}")                → os.Getenv("MYSSHW_PWD")
//	ResolveEnvRef("${MYSSHW_PWD:-fallback}")     → env 值或 "fallback"
//	ResolveEnvRef("prefix-${A}-${B}")            → 拼接替换结果
//	ResolveEnvRef("plaintext")                    → "plaintext"
//
// 实际实现见 config.ResolveEnvRef (config 不能反向依赖 ssh 包)
func ResolveEnvRef(s string) string {
	return config.ResolveEnvRef(s)
}
