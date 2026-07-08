package cmd

import (
	"encoding/json"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ============================================================
// completionsCmd 动态输出完整的 CLI Schema (JSON)
//
// 设计目标:
//   - AI agent 无需硬编码 CLI 结构, 运行时动态获取
//   - 输出格式稳定, 与当前二进制版本完全对齐
//   - 用于 tool-use schema 校验、动态 tool 注册、CLI 版本适配
//
// 用法:
//
//	mysshw completions json                # 输出完整 schema
//	mysshw completions json list           # 只输出某个子命令的 schema
//	mysshw completions json -o json | jq   # 默认就是 json 输出
//	mysshw completions json --flat         # 展平模式（AI 遍历友好）
// ============================================================

var (
	completionsSubCmd string // 可选: 限定某个子命令
	completionsFlat   bool   // 展平模式
)

var completionsCmd = &cobra.Command{
	Use:   "completions [json] [subcommand]",
	Short: "Dump the full CLI schema as JSON for AI agents to discover the CLI",
	Long: `Print the complete CLI structure (commands, subcommands, flags, args policy, exit codes)
as a self-describing JSON document.

Designed for AI agents and dynamic tool-wrapping systems:
  - No hardcoded schema needed — query live from the running binary
  - The JSON output always matches the current version's CLI exactly
  - Flat mode (--flat) produces a single array for easy iteration

Exit code:
  0    Success
  64   Usage error (e.g. unknown subcommand name)

Examples:
  # Full CLI schema
  mysshw completions json

  # Only the "exec" subcommand
  mysshw completions json exec

  # Flat array (no nesting), easier for AI to iterate
  mysshw completions json --flat

  # Pipe through jq to extract flags for a specific command
  mysshw completions json copy | jq '.commands[] | .flags[] | .name'`,
	Args: cobra.MaximumNArgs(3),
	RunE: runCompletions,
}

func init() {
	completionsCmd.Flags().BoolVar(&completionsFlat, "flat", false,
		"Flatten all subcommands into a single top-level array (no nesting)")
	rootCmd.AddCommand(completionsCmd)
}

func runCompletions(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		completionsSubCmd = args[0]
		// 验证子命令是否存在
		if _, _, err := rootCmd.Find(strings.Fields(completionsSubCmd)); err != nil {
			return NewExitError(ExitUsage, "unknown subcommand: %q", completionsSubCmd)
		}
	}

	schema := buildSchema()

	if completionsFlat {
		flat := flattenCommands(schema.Commands)
		return WriteOut(cmd.OutOrStdout(), flat, OutputJSON)
	}
	return WriteOut(cmd.OutOrStdout(), schema, OutputJSON)
}

// ============================================================
// JSON Schema 数据结构
// ============================================================

// CLIJSONSchema 是顶级输出结构
type CLIJSONSchema struct {
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	Application   ApplicationInfo  `json:"application"`
	Commands      []CommandSchema  `json:"commands"`
	ExitCodes     []ExitCodeSchema `json:"exit_codes"`
}

// ApplicationInfo 应用信息
type ApplicationInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Build       string `json:"build,omitempty"`
	BuildTime   string `json:"build_time,omitempty"`
	GoVersion   string `json:"go_version,omitempty"`
	Description string `json:"description,omitempty"`
}

// CommandSchema 描述一个 cobra 命令
type CommandSchema struct {
	Name           string          `json:"name"`
	Path           []string        `json:"path"`                 // 从 root 到该命令的完整路径
	FullName       string          `json:"full_name"`            // 空格分隔的路径
	Short          string          `json:"short,omitempty"`      // 简短描述
	Long           string          `json:"long,omitempty"`       // 长描述
	Usage          string          `json:"usage,omitempty"`      // Use 字段
	UsageLine      string          `json:"usage_line,omitempty"` // UseLine()
	Example        string          `json:"example,omitempty"`    // Example 字段
	ArgsPolicy     string          `json:"args_policy"`          // noargs | arbitrary | exact(N) | min(N) | max(N) | range(a,b) | custom
	Subcommands    []CommandSchema `json:"subcommands"`
	Flags          []FlagSchema    `json:"flags"`
	HasSubcommands bool            `json:"has_subcommands"`
	IsLeaf         bool            `json:"is_leaf"` // 有 Run/RunE 执行体
}

// FlagSchema 描述一个 flag
type FlagSchema struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
	Usage       string `json:"usage,omitempty"`
	Persistent  bool   `json:"persistent"` // 是否 persistent (继承给子命令)
	Required    bool   `json:"required"`   // 标记为必填
	Deprecated  string `json:"deprecated,omitempty"`
	NoOptDefVal string `json:"no_opt_default,omitempty"` // --flag=val 形式的默认值
}

// ExitCodeSchema 退出码描述
type ExitCodeSchema struct {
	Code        int    `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ============================================================
// Schema 构建
// ============================================================

func buildSchema() CLIJSONSchema {
	now := time.Now().UTC().Format(time.RFC3339)

	root := walkCommand(rootCmd, []string{})

	schema := CLIJSONSchema{
		SchemaVersion: "1.0",
		GeneratedAt:   now,
		Application: ApplicationInfo{
			Name:        rootCmd.Name(),
			Version:     Version,
			Build:       Build,
			BuildTime:   BuildTime,
			GoVersion:   GoVersion,
			Description: rootCmd.Short,
		},
		Commands: root.Subcommands,
		ExitCodes: []ExitCodeSchema{
			{Code: 0, Name: "OK", Description: "成功"},
			{Code: 1, Name: "GENERIC", Description: "未分类错误"},
			{Code: 2, Name: "AUTH_FAILED", Description: "认证失败（密码错/密钥错/ENV未设置/bcrypt mismatch）"},
			{Code: 64, Name: "EX_USAGE", Description: "命令行参数错误"},
			{Code: 65, Name: "EX_DATAERR", Description: "配置 TOML 格式错"},
			{Code: 66, Name: "EX_NOINPUT", Description: "输入文件缺失"},
			{Code: 70, Name: "EX_SOFTWARE", Description: "内部错误（panic、nil deref 等）"},
			{Code: 73, Name: "EX_CANTCREAT", Description: "无法创建输出文件"},
			{Code: 74, Name: "EX_IOERR", Description: "IO 错误"},
			{Code: 78, Name: "EX_CONFIG", Description: "配置错误/节点未找到"},
			{Code: 124, Name: "EX_TIMEOUT", Description: "命令超时"},
			{Code: 126, Name: "EX_CMD_NOT_EXEC", Description: "命令无法执行"},
			{Code: 127, Name: "EX_CMD_NOT_FOUND", Description: "命令未找到"},
			{Code: 255, Name: "EX_SSH_ERROR", Description: "底层 SSH 协议错误"},
		},
	}

	if completionsSubCmd != "" {
		schema.Commands = filterCommandsByPath(schema.Commands, strings.Fields(completionsSubCmd))
	}

	sortCommands(schema.Commands)
	return schema
}

// walkCommand 递归遍历 cobra 命令树
func walkCommand(cmd *cobra.Command, parentPath []string) CommandSchema {
	path := append(parentPath, cmd.Name())
	fullName := strings.Join(path, " ")

	s := CommandSchema{
		Name:           cmd.Name(),
		Path:           copyPath(path),
		FullName:       fullName,
		Short:          cmd.Short,
		Long:           cmd.Long,
		Usage:          cmd.Use,
		UsageLine:      cmd.UseLine(),
		Example:        cmd.Example,
		ArgsPolicy:     inferArgsPolicy(cmd.Args),
		Flags:          collectFlags(cmd),
		HasSubcommands: cmd.HasSubCommands(),
		IsLeaf:         cmd.RunE != nil || cmd.Run != nil,
	}

	for _, sub := range cmd.Commands() {
		if sub.Name() == "help" || sub.Hidden {
			continue
		}
		s.Subcommands = append(s.Subcommands, walkCommand(sub, path))
	}

	return s
}

// collectFlags 收集指定命令的所有 flag (本地 + inherited)
func collectFlags(cmd *cobra.Command) []FlagSchema {
	seen := make(map[string]bool)
	var flags []FlagSchema

	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if seen[f.Name] {
			return
		}
		seen[f.Name] = true
		flags = append(flags, flagToSchema(f, false))
	})

	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if seen[f.Name] {
			return
		}
		seen[f.Name] = true
		flags = append(flags, flagToSchema(f, true))
	})

	sort.Slice(flags, func(i, j int) bool {
		return flags[i].Name < flags[j].Name
	})
	return flags
}

// flagToSchema 将 pflag.Flag 转为 FlagSchema
func flagToSchema(f *pflag.Flag, persistent bool) FlagSchema {
	required := false
	if annotations := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]; annotations != nil {
		for _, a := range annotations {
			if a == "true" {
				required = true
				break
			}
		}
	}

	deprecated := ""
	if f.Deprecated != "" {
		deprecated = f.Deprecated
	}

	defVal := f.DefValue
	// 隐藏密码类默认值
	if strings.Contains(strings.ToLower(f.Name), "password") ||
		strings.Contains(strings.ToLower(f.Name), "passphrase") ||
		strings.Contains(strings.ToLower(f.Name), "secret") ||
		strings.Contains(strings.ToLower(f.Name), "token") {
		if defVal != "" {
			defVal = "(redacted)"
		}
	}

	return FlagSchema{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Type:        f.Value.Type(),
		Default:     defVal,
		Usage:       f.Usage,
		Persistent:  persistent,
		Required:    required,
		Deprecated:  deprecated,
		NoOptDefVal: f.NoOptDefVal,
	}
}

// inferArgsPolicy 通过反射推断命令的 Args validator 策略
//
// cobra 的 Args 是 PositionalArgs (函数类型), 无法直接 switch 比较,
// 这里通过 runtime.FuncForPC 获取函数名来推断策略文本。
func inferArgsPolicy(argsFunc interface{}) string {
	if argsFunc == nil {
		return "arbitrary"
	}

	pc := reflect.ValueOf(argsFunc).Pointer()
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "custom"
	}
	name := fn.Name()

	// 常见 cobra 函数签名
	switch {
	case strings.HasSuffix(name, "NoArgs"):
		return "noargs"
	case strings.HasSuffix(name, "ArbitraryArgs"):
		return "arbitrary"
	case strings.HasSuffix(name, "OnlyValidArgs"):
		return "only_valid"
	case strings.HasSuffix(name, "MinimumNArgs"):
		// 需要提取 int 参数
		return "min(N)"
	case strings.HasSuffix(name, "MaximumNArgs"):
		return "max(N)"
	case strings.HasSuffix(name, "ExactArgs"):
		return "exact(N)"
	case strings.HasSuffix(name, "RangeArgs"):
		return "range(min,max)"
	case strings.HasSuffix(name, "MatchAll"):
		return "match_all"
	default:
		return "custom"
	}
}

// sortCommands 递归排序命令 (按路径字典序)
func sortCommands(commands []CommandSchema) {
	sort.Slice(commands, func(i, j int) bool {
		return pathStr(commands[i].Path) < pathStr(commands[j].Path)
	})
	for i := range commands {
		sortCommands(commands[i].Subcommands)
	}
}

func pathStr(p []string) string { return strings.Join(p, "/") }

func copyPath(p []string) []string {
	out := make([]string, len(p))
	copy(out, p)
	return out
}

// flattenCommands 递归展平所有子命令到单层数组
func flattenCommands(commands []CommandSchema) []CommandSchema {
	var out []CommandSchema
	for _, c := range commands {
		flat := c
		flat.Subcommands = nil
		out = append(out, flat)
		out = append(out, flattenCommands(c.Subcommands)...)
	}
	return out
}

// filterCommandsByPath 在命令树中筛选匹配路径的命令
func filterCommandsByPath(commands []CommandSchema, path []string) []CommandSchema {
	if len(path) == 0 {
		return commands
	}
	var result []CommandSchema
	for _, c := range commands {
		if c.Name == path[0] {
			if len(path) == 1 {
				result = append(result, c)
			} else {
				result = append(result, filterCommandsByPath(c.Subcommands, path[1:])...)
			}
		}
	}
	return result
}

// 确保 json 包在 plain 模式下也被间接使用
var _ = json.Marshal
