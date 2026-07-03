package cmd

import (
	"strings"
	"time"
)

// isSoftErr 判断 ssh.ResolveNodes 返回的 soft error (部分 name 未找到, 但仍继续)
// 用于在 cmd 层决策是否吞掉错误继续
func isSoftErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "continuing with")
}

// isNodeNotFound 判断错误是否来自 ssh.ErrNodeNotFound (sentinal)
func isNodeNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "node not found")
}

// isCancelLike 判断是否是 ctx cancel / listener close 之类的"正常"错误
func isCancelLike(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "context canceled") ||
		strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "closed pipe")
}

// parseDuration 解析 duration 字符串 ("" 或 "0" 视为 0 = no limit)
func parseDuration(s string) (time.Duration, error) {
	if s == "" || s == "0" {
		return 0, nil
	}
	return time.ParseDuration(s)
}
