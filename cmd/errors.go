package cmd

import (
	"fmt"
	"os"
)

// BSD sysexits.h 风格的退出码常量
//
// 参考:
//   - BSD sysexits.h (https://man.openbsd.org/sysexits)
//   - GNU coreutils 惯例 (timeout → 124)
//
// 设计意图: AI agent / 脚本可直接读退出码判断错误类别，
//
//	无需解析 stderr 文案。Error() 输出包含人类可读描述。
//
// 错误码总览:
//
//	  0 OK              成功
//	  1 GENERIC         未分类错误
//	  2 AUTH_FAILED     认证失败（密码错 / 密钥错 / ${ENV} 未设置 / bcrypt mismatch）
//	 64 EX_USAGE        命令行参数错误
//	 65 EX_DATAERR      配置 TOML 格式错
//	 66 EX_NOINPUT      输入文件缺失
//	 70 EX_SOFTWARE     内部错误（panic、nil deref 等）
//	 73 EX_CANTCREAT    无法创建输出文件
//	 74 EX_IOERR        IO 错误
//	 78 EX_CONFIG       配置错误 / 节点未找到 (alias: NODE_NOT_FOUND)
//	124 TIMEOUT         命令超时（GNU coreutils 惯例）
//	126 CMD_NOT_EXEC    命令无法执行
//	127 CMD_NOT_FOUND   命令未找到
//	255 SSH_ERROR       底层 SSH 协议错误
const (
	ExitOK          = 0
	ExitGeneric     = 1
	ExitAuthFailed  = 2
	ExitUsage       = 64
	ExitDataErr     = 65
	ExitNoInput     = 66
	ExitSoftware    = 70
	ExitCantCreate  = 73
	ExitIOErr       = 74
	ExitConfig      = 78
	ExitTimeout     = 124
	ExitCmdNotExec  = 126
	ExitCmdNotFound = 127
	ExitSSHError    = 255
)

// ExitError 携带退出码的错误类型，供 RunE 返回
//
// 用法:
//
//	return ExitError{Code: ExitAuthFailed, Err: err}
//	// 或
//	return NewExitError(ExitUsage, "missing --node")
type ExitError struct {
	Code int
	Err  error
}

func (e ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return fmt.Sprintf("exit %d: %v", e.Code, e.Err)
}

func (e ExitError) Unwrap() error { return e.Err }

// NewExitError 工厂方法 (printf 风格)
func NewExitError(code int, format string, args ...any) ExitError {
	return ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}

// WrapExit 包装已有 error 加上退出码
func WrapExit(code int, err error) ExitError {
	return ExitError{Code: code, Err: err}
}

// HandleExit 处理 RunE 返回值
//   - nil         → 退出 0
//   - ExitError   → 用其 Code
//   - 其他 error  → 退出 ExitGeneric (=1), stderr 打印描述
func HandleExit(err error) {
	if err == nil {
		os.Exit(ExitOK)
	}
	if ee, ok := err.(ExitError); ok {
		fmt.Fprintln(os.Stderr, ee.Error())
		os.Exit(ee.Code)
	}
	fmt.Fprintln(os.Stderr, "mysshw::", err)
	os.Exit(ExitGeneric)
}

// ExitCode 工具: 若 err 是 ExitError 返回其 Code, 否则 ExitGeneric
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if ee, ok := err.(ExitError); ok {
		return ee.Code
	}
	return ExitGeneric
}
