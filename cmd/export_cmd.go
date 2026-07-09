package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"mysshw/config"

	"github.com/GuanceCloud/toml"
	"github.com/spf13/cobra"
)

// ============================================================
// exportCmd — 导出节点到可移植文件 (JSON/TOML)
//
// 用途: 把当前配置中选中的节点导出为独立文件, 方便分享或迁移。
// 密码字段统一替换为 "${IMPORTED}" 占位符, 避免敏感信息泄露。
//
// 兼容性: 导出的 JSON/TOML 可通过 `mysshw import` 导入到其他实例。
// JSON 格式适合 AI agent / 脚本处理; TOML 格式适合人类阅读和手动编辑。
// ============================================================

var (
	exportGroup  string
	exportNames  []string
	exportFormat string
	exportOutput string
)

// portableNode 导出/导入用的可移植节点结构
// 密码字段始终替换为 "${IMPORTED}" (导出时) 或由用户设置 (导入时)
type portableNode struct {
	Group            string `json:"group" toml:"group"`
	Name             string `json:"name" toml:"name"`
	Alias            string `json:"alias,omitempty" toml:"alias,omitempty"`
	Host             string `json:"host" toml:"host"`
	User             string `json:"user,omitempty" toml:"user,omitempty"`
	Port             int    `json:"port" toml:"port"`
	KeyPath          string `json:"keypath,omitempty" toml:"keypath,omitempty"`
	Password         string `json:"password,omitempty" toml:"password,omitempty"`
	Proxy            string `json:"proxy,omitempty" toml:"proxy,omitempty"`
	RotateKnownHosts bool   `json:"rotate_known_hosts,omitempty" toml:"rotate_known_hosts,omitempty"`
}

const importPasswordPlaceholder = "${IMPORTED}"

var exportCmd = &cobra.Command{
	Use:   "export [--group G] [--name N [--name M]] [--format json|toml] [--output FILE]",
	Short: "Export SSH nodes to a portable file (password fields redacted)",
	Long: `Export one or more SSH nodes from the current config to stdout or a file.

Password fields are always replaced with "${IMPORTED}" placeholder.
To restore passwords on import, edit the exported file and replace "${IMPORTED}"
with the actual password (or bcrypt hash / ${ENV} reference).

Flags:
  --group <name>     Filter by group (exact match)
  --name <name>      Filter by node name (repeatable; omit for all)
  --format <fmt>     Output format: "json" (default) or "toml"
  --output <file>    Write to file instead of stdout (optional)

Export format:
  JSON  — AI-agent friendly, easy to parse with jq
  TOML  — Human-readable, compatible with mysshw.toml [[nodes]] sections`,
	Example: `  # All nodes as JSON to stdout
  mysshw export

  # Production group as TOML
  mysshw export --group Production --format toml

  # Two specific nodes to a file
  mysshw export --name prod-1 --name prod-2 --output nodes.toml

  # Pipe to jq for AI agent
  mysshw export --group Production --format json | jq '. | length'`,
	Args: cobra.NoArgs,
	RunE: runExport,
}

func init() {
	f := exportCmd.Flags()
	f.StringVar(&exportGroup, "group", "",
		"Export only nodes in this group (exact match)")
	f.StringSliceVar(&exportNames, "name", nil,
		"Export only these node names (repeatable; omit for all)")
	f.StringVar(&exportFormat, "format", "json",
		"Output format: json (default) | toml")
	f.StringVar(&exportOutput, "output", "",
		"Write to file instead of stdout")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	// 1. 加载配置
	cfgPath := config.CFG_PATH
	if p, _ := cmd.Flags().GetString("cfg"); p != "" {
		cfgPath = p
	}
	cfg, err := config.ReadConfigOrEmpty(cfgPath)
	if err != nil {
		return WrapExit(ExitConfig, err)
	}

	// 2. 构建 name 集合 (--name 筛选)
	nameSet := make(map[string]bool, len(exportNames))
	for _, n := range exportNames {
		nameSet[n] = true
	}

	// 3. 收集节点
	var nodes []portableNode
	for _, g := range cfg.Nodes {
		if exportGroup != "" && g.Groups != exportGroup {
			continue
		}
		for _, n := range g.SSHNodes {
			if len(nameSet) > 0 && !nameSet[n.Name] {
				continue
			}
			nodes = append(nodes, portableNode{
				Group:            g.Groups,
				Name:             n.Name,
				Alias:            n.Alias,
				Host:             n.Host,
				User:             n.SetUser(),
				Port:             n.SetPort(),
				KeyPath:          n.KeyPath,
				Password:         importPasswordPlaceholder,
				Proxy:            n.Proxy,
				RotateKnownHosts: n.RotateKnownHosts,
			})
		}
	}

	if len(nodes) == 0 {
		return WrapExit(ExitConfig,
			fmt.Errorf("no nodes match the given filters"))
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Group != nodes[j].Group {
			return nodes[i].Group < nodes[j].Group
		}
		return nodes[i].Name < nodes[j].Name
	})

	// 4. 序列化
	var data []byte
	switch strings.ToLower(exportFormat) {
	case "json":
		data, err = json.MarshalIndent(nodes, "", "  ")
		if err != nil {
			return WrapExit(ExitIOErr, err)
		}
	case "toml":
		// 用 GuanceCloud/toml 的 NewEncoder 序列化
		var buf bytes.Buffer
		enc := toml.NewEncoder(&buf)
		if err := enc.Encode(nodes); err != nil {
			return WrapExit(ExitIOErr, fmt.Errorf("toml encode: %w", err))
		}
		header := fmt.Sprintf("# mysshw export — %d node(s)\n", len(nodes))
		data = append([]byte(header), buf.Bytes()...)
	default:
		return WrapExit(ExitUsage,
			fmt.Errorf("unsupported format %q (use json or toml)", exportFormat))
	}

	// 5. 输出
	if exportOutput != "" {
		if err := os.WriteFile(exportOutput, data, 0600); err != nil {
			return WrapExit(ExitIOErr, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "mysshw:: exported %d node(s) to %s\n", len(nodes), exportOutput)
	} else {
		cmd.OutOrStdout().Write(data)
		fmt.Fprintln(cmd.OutOrStdout())
	}
	return nil
}
