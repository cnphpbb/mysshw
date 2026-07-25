package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"mysshw/config"

	"github.com/spf13/cobra"
)

var (
	listGroup  string
	listSearch string
)

// listCmd 列出配置中的所有 SSH 节点
//
// 设计目标:
//   - 默认人类可读（表格）
//   - --json 输出结构化数据，供 AI / jq 解析
//   - 绝不输出 password / passphrase 等敏感字段
//
// JSON schema 稳定，向后兼容优先：新增字段时放末尾；删除字段前需 deprecate。
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured SSH nodes",
	Long: `Print all configured SSH nodes in human-readable table (default) or JSON.

JSON schema is stable and safe to pipe into jq / parse by AI agents.

Examples:
  mysshw list
  mysshw list --json
  mysshw list --json --search prod
  mysshw list --group Production
  mysshw list -o json | jq '.[] | select(.auth=="bcrypt")'`,
	Args: cobra.NoArgs,
	RunE: runList,
}

func init() {
	listCmd.Flags().StringVar(&listGroup, "group", "",
		"Filter by group name (exact match, case-sensitive)")
	listCmd.Flags().StringVar(&listSearch, "search", "",
		"Substring match against name/alias/host/user (case-insensitive)")
	rootCmd.AddCommand(listCmd)
}

// nodeSummary list 命令的对外数据结构（JSON schema）
//
// ⚠️ 不要新增 password / passphrase 字段。AI 工具可能把此 JSON 复制到上下文中，
// 暴露 secret 会导致泄露。
type nodeSummary struct {
	Group   string `json:"group"`
	Name    string `json:"name"`
	Alias   string `json:"alias,omitempty"`
	User    string `json:"user,omitempty"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Auth    string `json:"auth"`              // key | password | env | bcrypt | empty | unknown
	KeyPath string `json:"keypath,omitempty"` // 仅当 KeyPath 非空
	Proxy   string `json:"proxy,omitempty"`   // 仅当 SSHNode.Proxy 非空 (节点通过跳板访问)
}

// classifyAuth 根据节点字段归类认证方式（仅展示用，不输出密码本身）
//
//	空 password + 有 KeyPath       → "key"
//	空 password                    → "empty"
//	bcrypt 哈希                     → "bcrypt"
//	bcrypt-pepper 哈希 (Phase B)    → "bcrypt-pepper" (需顶层 secret_key)
//	${ENV} 引用                     → "env"
//	有 KeyPath 但 password 也有     → "key+password"（少见）
//	其他明文                        → "password"
//
// Phase B: 如果 cfg.EncryptionAlgorithm == "bcrypt-pepper", 则所有 bcrypt 节点
//
//	都显示为 "bcrypt-pepper" (因为所有节点都需 pepper 验证)
func classifyAuth(n *config.SSHNode) string {
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

	// 先检测特定格式
	auth := ""
	if config.IsBcryptHash(pwd) {
		// Phase B: bcrypt-pepper 模式标记
		// 如果顶层 algorithm 显式指定 bcrypt-pepper, 所有 bcrypt 节点标记为 pepper
		// (因为运行时都会用 pepper 验证, 区别仅在 server 侧 secret_key 是否存在)
		if config.CFG != nil {
			if algo, _ := config.ResolveEncryptionAlgorithm(config.CFG.EncryptionAlgorithm); config.IsPepperRequired(algo) {
				auth = "bcrypt-pepper"
			} else {
				auth = "bcrypt"
			}
		} else {
			auth = "bcrypt"
		}
	} else if config.IsArgon2idHash(pwd) {
		// Phase B+ 占位识别: Argon2id 格式哈希
		auth = "argon2id"
	} else if config.IsPbkdf2Sha256Hash(pwd) {
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

func runList(cmd *cobra.Command, args []string) error {
	if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
		config.CFG_PATH = cfgPath
	}
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("load config: %w", err))
	}

	summaries := buildNodeSummaries(config.CFG)
	summaries = filterSummaries(summaries, listGroup, listSearch)
	sortSummaries(summaries)

	format := GetOutputFormat(cmd)
	if format == OutputPlain {
		return printNodeTable(cmd.OutOrStdout(), summaries)
	}
	return WriteOut(cmd.OutOrStdout(), summaries, format)
}

func buildNodeSummaries(cfg *config.Configs) []nodeSummary {
	var out []nodeSummary
	if cfg == nil {
		return out
	}
	for _, group := range cfg.Nodes {
		for _, n := range group.SSHNodes {
			if n == nil {
				continue
			}
			s := nodeSummary{
				Group:   group.Groups,
				Name:    n.Name,
				Alias:   n.Alias,
				User:    n.SetUser(),
				Host:    n.Host,
				Port:    n.SetPort(),
				Auth:    classifyAuth(n),
				KeyPath: n.KeyPath,
				Proxy:   n.Proxy,
			}
			out = append(out, s)
		}
	}
	return out
}

func filterSummaries(in []nodeSummary, group, search string) []nodeSummary {
	if group == "" && search == "" {
		return in
	}
	searchLower := strings.ToLower(search)
	out := make([]nodeSummary, 0, len(in))
	for _, s := range in {
		if group != "" && s.Group != group {
			continue
		}
		if search != "" {
			hay := strings.ToLower(strings.Join([]string{s.Name, s.Alias, s.Host, s.User}, " "))
			if !strings.Contains(hay, searchLower) {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// sortSummaries 稳定排序：(group, name) 字典序, 便于 git diff / AI 解析一致
func sortSummaries(s []nodeSummary) {
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].Group != s[j].Group {
			return s[i].Group < s[j].Group
		}
		return s[i].Name < s[j].Name
	})
}

func printNodeTable(w io.Writer, summaries []nodeSummary) error {
	// 不兜底到 os.Stdout: MCP stdio 模式下 os.Stdout = JSON-RPC 通道,
	// 写入会污染协议流导致客户端断开。nil writer 直接 no-op。
	if w == nil {
		return nil
	}
	if len(summaries) == 0 {
		fmt.Fprintln(w, "(no nodes match filters)")
		return nil
	}

	// 计算列宽
	widths := []int{len("GROUP"), len("NAME"), len("ALIAS"), len("USER"), len("HOST"), len("PORT"), len("AUTH")}
	for _, s := range summaries {
		if l := len(s.Group); l > widths[0] {
			widths[0] = l
		}
		if l := len(s.Name); l > widths[1] {
			widths[1] = l
		}
		if l := len(s.Alias); l > widths[2] {
			widths[2] = l
		}
		if l := len(s.User); l > widths[3] {
			widths[3] = l
		}
		if l := len(s.Host); l > widths[4] {
			widths[4] = l
		}
		if l := len(fmt.Sprintf("%d", s.Port)); l > widths[5] {
			widths[5] = l
		}
		if l := len(s.Auth); l > widths[6] {
			widths[6] = l
		}
	}
	rowFmt := fmt.Sprintf("%%-%ds  %%%ds  %%%ds  %%%ds  %%%ds  %%%ds  %%s\n",
		widths[0], widths[1], widths[2], widths[3], widths[4], widths[5])

	fmt.Fprintf(w, rowFmt, "GROUP", "NAME", "ALIAS", "USER", "HOST", "PORT", "AUTH")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", sumInts(widths)+12))
	for _, s := range summaries {
		fmt.Fprintf(w, rowFmt, s.Group, s.Name, s.Alias, s.User, s.Host, fmt.Sprintf("%d", s.Port), s.Auth)
	}
	return nil
}

func sumInts(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}
