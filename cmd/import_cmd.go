package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mysshw/config"

	"github.com/GuanceCloud/toml"
	"github.com/spf13/cobra"
)

// ============================================================
// importCmd — 从可移植文件导入节点 (JSON/TOML)
//
// 用途: 读取 `mysshw export` 导出的文件, 或符合相同结构的
// 自定义 JSON/TOML 文件, 将节点批量添加到当前配置。
//
// 安全:
//   - 默认 --dry-run, --confirm 才写入
//   - 自动检测文件格式 (.json / .toml / 内容嗅探)
//   - 重复检测: 同 name 节点默认跳过 (--overwrite 则覆盖)
//   - 原子写 + 自动备份
// ============================================================

var (
	importConfirm   bool
	importOverwrite bool
	importGroup     string // 可选: 覆盖导入节点的 group
)

var importCmd = &cobra.Command{
	Use:   "import <file> [--confirm] [--overwrite] [--group G]",
	Short: "Import SSH nodes from a portable file (json or toml)",
	Long: `Import nodes from a file previously exported with "mysshw export" or
any file matching the same schema.

Format auto-detection:
  .json  → JSON array of nodes
  .toml  → TOML [[nodes]] sections or array of nodes
  other  → try JSON first, fallback to TOML

Password field ("${IMPORTED}" placeholder) is preserved as-is.
After import, edit the config and replace "${IMPORTED}" with the actual
password (or bcrypt hash / ${ENV} reference). Use "mysshw config update"
for individual updates.

Safety:
  - Default --dry-run; pass --confirm to actually write.
  - Nodes with duplicate names are skipped (use --overwrite to force update).
  - Atomic rewrite + backup on write.`,
	Example: `  # Dry-run preview
  mysshw import nodes.json

  # Actually import
  mysshw import nodes.toml --confirm

  # Override group for all imported nodes
  mysshw import nodes.json --group Staging --confirm

  # Overwrite existing nodes with same names
  mysshw import nodes.json --overwrite --confirm`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	f := importCmd.Flags()
	f.BoolVar(&importConfirm, "confirm", false,
		"Actually write to mysshw.toml (default: dry-run)")
	f.BoolVar(&importOverwrite, "overwrite", false,
		"Update existing nodes (default: skip duplicates)")
	f.StringVar(&importGroup, "group", "",
		"Override group for all imported nodes (optional)")
	rootCmd.AddCommand(importCmd)
}

// importStat 导入操作的统计摘要
type importStat struct {
	Total     int            `json:"total"`
	Added     int            `json:"added"`
	Skipped   int            `json:"skipped"`
	Overwrite int            `json:"overwrite,omitempty"`
	Errors    []string       `json:"errors,omitempty"`
	DryRun    bool           `json:"dry_run"`
	Nodes     []portableNode `json:"nodes,omitempty"`
}

// importResult 单节点导入结果
type importResult int

const (
	importAdded importResult = iota
	importSkipped
	importOverwritten
)

func runImport(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// 1. 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return WrapExit(ExitNoInput,
			fmt.Errorf("read file %q: %w", filePath, err))
	}
	if len(data) == 0 {
		return WrapExit(ExitNoInput,
			fmt.Errorf("file %q is empty", filePath))
	}

	// 2. 解析: 自动检测格式
	nodes, err := parseImportFile(data, filePath)
	if err != nil {
		return WrapExit(ExitConfig,
			fmt.Errorf("parse %q: %w", filePath, err))
	}
	if len(nodes) == 0 {
		return WrapExit(ExitConfig,
			fmt.Errorf("file %q contains no nodes", filePath))
	}

	// 3. 验证必填字段
	for _, n := range nodes {
		if n.Name == "" {
			return WrapExit(ExitConfig,
				fmt.Errorf("node missing required field 'name'"))
		}
		if n.Host == "" {
			return WrapExit(ExitConfig,
				fmt.Errorf("node %q missing required field 'host'", n.Name))
		}
	}

	// 4. 加载当前配置
	cfgPath := config.CFG_PATH
	if p, _ := cmd.Flags().GetString("cfg"); p != "" {
		cfgPath = p
	}
	cfg, err := config.ReadConfigOrEmpty(cfgPath)
	if err != nil {
		return WrapExit(ExitConfig, err)
	}

	// 5. 应用 --group 覆盖
	if importGroup != "" {
		for i := range nodes {
			nodes[i].Group = importGroup
		}
	}

	// 6. 逐个导入 (模拟添加)
	newCfg := cloneConfigs(cfg)
	stat := importStat{
		Total:  len(nodes),
		DryRun: !importConfirm,
		Nodes:  make([]portableNode, 0, len(nodes)),
	}

	for _, pn := range nodes {
		result, errStr := applyImportNode(newCfg, pn)
		switch result {
		case importAdded:
			stat.Added++
			stat.Nodes = append(stat.Nodes, pn)
		case importSkipped:
			stat.Skipped++
		case importOverwritten:
			stat.Overwrite++
			stat.Nodes = append(stat.Nodes, pn)
		}
		if errStr != "" {
			stat.Errors = append(stat.Errors, fmt.Sprintf("%s: %s", pn.Name, errStr))
		}
	}

	// 7. dry-run / confirm
	if !importConfirm {
		stat.DryRun = true
		fmt.Fprintln(cmd.OutOrStdout(), "mysshw:: dry-run mode, import would:")
		fmt.Fprintf(cmd.OutOrStdout(), "  total=%d added=%d skipped=%d",
			stat.Total, stat.Added, stat.Skipped)
		if stat.Overwrite > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), " overwritten=%d", stat.Overwrite)
		}
		fmt.Fprintln(cmd.OutOrStdout())
		for _, e := range stat.Errors {
			fmt.Fprintf(cmd.ErrOrStderr(), "  warning: %s\n", e)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "  pass --confirm to actually write")
		if stat.Added+stat.Overwrite > 0 {
			return WriteOut(cmd.OutOrStdout(), stat, OutputJSON)
		}
		return nil
	}

	// 8. 写入
	if stat.Added+stat.Overwrite > 0 {
		if err := config.WriteConfigAtomic(cfgPath, newCfg); err != nil {
			return WrapExit(ExitIOErr, err)
		}
	}

	// 9. 输出统计
	fmt.Fprintf(cmd.OutOrStdout(), "mysshw:: imported: +%d added", stat.Added)
	if stat.Overwrite > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), " ~%d updated", stat.Overwrite)
	}
	if stat.Skipped > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), " %d skipped", stat.Skipped)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	for _, e := range stat.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "mysshw:: warning: %s\n", e)
	}
	return nil
}

// parseImportFile 尝试解析导入文件, 自动检测 JSON / TOML
func parseImportFile(data []byte, filePath string) ([]portableNode, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	// 按扩展名优先
	if ext == ".json" {
		return parseJSONImport(data)
	}
	if ext == ".toml" {
		return parseTOMLImport(data)
	}

	// 未知扩展名: 先试 JSON, 再试 TOML
	nodes, err := parseJSONImport(data)
	if err == nil {
		return nodes, nil
	}
	return parseTOMLImport(data)
}

// parseJSONImport 解析 JSON 数组 -> []portableNode
func parseJSONImport(data []byte) ([]portableNode, error) {
	var nodes []portableNode
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return nodes, nil
}

// parseTOMLImport 解析 TOML -> []portableNode
// 支持两种格式:
//  1. [[nodes]] sections (同 mysshw.toml)
//  2. 直接顶层数组
func parseTOMLImport(data []byte) ([]portableNode, error) {
	// 方案 A: 先试顶层数组
	var nodes []portableNode
	if err := toml.Unmarshal(data, &nodes); err == nil && len(nodes) > 0 {
		return nodes, nil
	}

	// 方案 B: 试 [[nodes]] 结构
	type importConfig struct {
		Nodes []struct {
			Groups   string         `toml:"groups"`
			SSHNodes []portableNode `toml:"ssh"`
		} `toml:"nodes"`
	}
	var cfg importConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("toml: not valid node array or [[nodes]] format: %w", err)
	}

	var out []portableNode
	for _, g := range cfg.Nodes {
		group := g.Groups
		for _, n := range g.SSHNodes {
			n.Group = group
			out = append(out, n)
		}
	}
	return out, nil
}

// applyImportNode 将 portableNode 应用到 config 中
// 返回 (结果, 错误描述)
func applyImportNode(cfg *config.Configs, pn portableNode) (importResult, string) {
	node := &config.SSHNode{
		Name:             pn.Name,
		Alias:            pn.Alias,
		Host:             pn.Host,
		User:             pn.User,
		Port:             pn.Port,
		KeyPath:          pn.KeyPath,
		Password:         pn.Password,
		Proxy:            pn.Proxy,
		RotateKnownHosts: pn.RotateKnownHosts,
	}

	// 检查 name 重复
	existingGroup := ""
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == pn.Name {
				existingGroup = g.Groups
				break
			}
		}
		if existingGroup != "" {
			break
		}
	}

	if existingGroup != "" {
		if !importOverwrite {
			return importSkipped, ""
		}
		// overwrite: 删除旧节点
		newNodes := cfg.Nodes
		for gi, g := range newNodes {
			for ni, n := range g.SSHNodes {
				if n.Name == pn.Name {
					newNodes[gi].SSHNodes = append(g.SSHNodes[:ni], g.SSHNodes[ni+1:]...)
					break
				}
			}
		}
	}

	// 检查 alias 重复 (非阻塞警告)
	if pn.Alias != "" {
		for _, g := range cfg.Nodes {
			for _, n := range g.SSHNodes {
				if n.Alias == pn.Alias && n.Name != pn.Name {
					return importSkipped,
						fmt.Sprintf("alias %q already used by %q in group %q", pn.Alias, n.Name, g.Groups)
				}
			}
		}
	}

	// 添加节点
	group := pn.Group
	if group == "" {
		group = "Imported"
	}
	_ = configWithNodeAdded(cfg, group, node)
	if existingGroup != "" && importOverwrite {
		return importOverwritten, ""
	}
	return importAdded, ""
}
