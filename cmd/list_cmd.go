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
// 已迁移到 config.ClassifyAuth, 此处保留薄包装以避免改动调用面
func classifyAuth(n *config.SSHNode) string {
	return config.ClassifyAuth(n, config.CFG)
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
	// 写入会污染协议流导致客户端断开。nil writer 直接 no-op (PrintTable 内部处理)。
	if len(summaries) == 0 {
		if w != nil {
			fmt.Fprintln(w, "(no nodes match filters)")
		}
		return nil
	}

	headers := []string{"GROUP", "NAME", "ALIAS", "USER", "HOST", "PORT", "AUTH"}
	rows := make([][]string, 0, len(summaries))
	for _, s := range summaries {
		rows = append(rows, []string{
			s.Group, s.Name, s.Alias, s.User, s.Host,
			fmt.Sprintf("%d", s.Port), s.Auth,
		})
	}
	return PrintTable(w, headers, rows)
}
