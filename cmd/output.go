package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// OutputFormat 输出格式枚举
type OutputFormat string

const (
	OutputPlain OutputFormat = "plain"
	OutputJSON  OutputFormat = "json"
)

// outputFlagName 全局 --output flag 名（注册于 rootCmd.PersistentFlags）
const outputFlagName = "output"

// RegisterOutputFlag 在 rootCmd 注册 --output 全局 flag
//
// 默认 plain; 用于 list/exec 等 AI 友好子命令的机器可读输出。
func RegisterOutputFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP(outputFlagName, "o", string(OutputPlain),
		"Output format: plain (default, human-readable) or json (machine-readable)")
}

// GetOutputFormat 从 cmd 读取 --output, 回退默认 plain
func GetOutputFormat(cmd *cobra.Command) OutputFormat {
	if cmd == nil {
		return OutputPlain
	}
	v, err := cmd.Flags().GetString(outputFlagName)
	if err != nil || v == "" {
		return OutputPlain
	}
	switch OutputFormat(v) {
	case OutputJSON:
		return OutputJSON
	default:
		return OutputPlain
	}
}

// WriteOut 统一输出: plain 直接 v.(string); 否则 JSON encode
//
// 约定:
//   - string/error 类型, plain 模式下按行打印
//   - slice/struct/json.Marshaler, plain 模式下 fallback 为单行 JSON
//   - JSON 模式: 全对象 indent 输出
func WriteOut(w io.Writer, v any, format OutputFormat) error {
	if w == nil {
		w = io.Discard
	}
	switch format {
	case OutputJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	default:
		switch x := v.(type) {
		case string:
			_, err := fmt.Fprintln(w, x)
			return err
		case error:
			_, err := fmt.Fprintln(w, x.Error())
			return err
		default:
			enc := json.NewEncoder(w)
			return enc.Encode(v)
		}
	}
}
