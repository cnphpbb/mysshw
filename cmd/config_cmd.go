package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"mysshw/config"

	"github.com/spf13/cobra"
)

// ============================================================
// configCmd 节点 CRUD 子命令 (Phase: config CRUD)
//
// 子命令:
//   list                  列出所有节点 (跟 `mysshw list` 语义相近, 但 schema 全)
//   get <name|alias>      查看节点详情 (含 password auth 类别, 不含明文)
//   add ...               新增节点 (含完整字段)
//   remove <name>          删除节点 (--confirm 才真删)
//   update <name>          增量改字段 (空值不改, --set-* 显式改)
//   diff <name> <other>    对比两个节点的字段差异
//
// 安全设计:
//   - 默认 --dry-run (add / remove / update), 需 --confirm 才写盘
//   - 写到磁盘前自动做 atomic rewrite (备份 + 临时文件 + rename)
//   - 密码字段推荐 bcrypt / env 引用, 明文会打印警告
//   - list / get 输出 JSON schema 不含 password 字段
//
// 注: 与现有的 `mysshw list` 子命令互补:
//   - `list`    → 输出 AI 友好的节点摘要 (auth 类别, 用于执行)
//   - `config list` → 输出节点详情 (含所有字段, 用于编辑)
// ============================================================

var (
	configListGroup  string
	configListFormat string

	configGetNode string

	configAddGroup    string
	configAddName     string
	configAddAlias    string
	configAddHost     string
	configAddUser     string
	configAddPort     int
	configAddPassword string
	configAddKeyPath  string
	configAddProxy    string
	configAddRotateKH bool

	configRemoveName    string
	configRemoveConfirm bool

	configUpdateName     string
	configUpdateGroup    string
	configUpdateAlias    string
	configUpdateHost     string
	configUpdateUser     string
	configUpdatePort     int
	configUpdatePassword string
	configUpdateKeyPath  string
	configUpdateProxy    string
	configUpdateConfirm  bool
	configUpdateRotateKH bool

	configDiffA string
	configDiffB string
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "CRUD for SSH nodes in mysshw.toml (list / get / add / remove / update / diff)",
	Long: `Manage SSH nodes directly in mysshw.toml without manual editing.

Subcommands:
  list                          List nodes (with all editable fields)
  get <name|alias>               Show one node's full config (password redacted)
  add ...                        Add a new SSH node (--confirm to write)
  remove <name>                  Delete a node (--confirm to apply)
  update <name>                  Patch one or more fields of a node (--confirm to write)
  diff <name> <other>            Compare fields of two nodes

Safety:
  - Most write commands default to --dry-run; pass --confirm to actually modify.
  - Every write goes through atomic-rewrite + backup (see backup_keep in mysshw.toml).
  - Password is recommended as bcrypt / ${ENV}; plain text triggers a stderr warning.
  - JSON output never contains the password field.

Examples:
  mysshw config list --group Production
  mysshw config get prod-1
  mysshw config add --group Production --name web-2 --host 10.0.0.2 --user app --password '${MYSSHW_PWD_web2}'
  mysshw config update prod-1 --port 2222 --confirm
  mysshw config remove prod-1 --confirm
  mysshw config diff prod-1 prod-2`,
	Args: cobra.ArbitraryArgs, // 子命令各自定义 Args, parent 不限制
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 从 rootCmd 的 PersistentFlags --cfg 中提取自定义配置路径
		// 其他子命令 (如 exec/list/hostkey) 在各自 RunE 中做同样的事,
		// config 子命令统一在此处理, 避免每个 runConfig* 重复.
		if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
			config.CFG_PATH = cfgPath
		}
		return nil
	},
}

// ============================================================
// list
// ============================================================

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all SSH nodes with full editable fields (password field redacted)",
	RunE:  runConfigList,
}

func init() {
	configListCmd.Flags().StringVar(&configListGroup, "group", "",
		"Filter by group name (exact match)")
	configListCmd.Flags().StringVar(&configListFormat, "format", "plain",
		"Output format: plain (table) | json")

	configCmd.AddCommand(configListCmd, configGetCmd, configAddCmd,
		configRemoveCmd, configUpdateCmd, configDiffCmd)

	rootCmd.AddCommand(configCmd)
}

// configNodeDetail 节点详情 (JSON schema, 用于 list/get/diff)
// password 字段用 json:"-" 完全屏蔽
type configNodeDetail struct {
	Group   string `json:"group"`
	Name    string `json:"name"`
	Alias   string `json:"alias,omitempty"`
	Host    string `json:"host"`
	User    string `json:"user,omitempty"`
	Port    int    `json:"port"`
	KeyPath string `json:"keypath,omitempty"`
	// Password 字段用 json:"-" 永远不输出
	Password         string `json:"-"`
	Proxy            string `json:"proxy,omitempty"`
	RotateKnownHosts bool   `json:"rotate_known_hosts,omitempty"`
}

func configNodeFromSSHNode(group string, n *config.SSHNode) configNodeDetail {
	return configNodeDetail{
		Group:            group,
		Name:             n.Name,
		Alias:            n.Alias,
		Host:             n.Host,
		User:             n.SetUser(),
		Port:             n.SetPort(),
		KeyPath:          n.KeyPath,
		Password:         n.Password,
		Proxy:            n.Proxy,
		RotateKnownHosts: n.RotateKnownHosts,
	}
}

func runConfigList(cmd *cobra.Command, args []string) error {
	cfg, err := loadOrCreateConfig()
	if err != nil {
		return WrapExit(ExitConfig, err)
	}

	var details []configNodeDetail
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if configListGroup != "" && g.Groups != configListGroup {
				continue
			}
			details = append(details, configNodeFromSSHNode(g.Groups, n))
		}
	}

	sort.Slice(details, func(i, j int) bool {
		if details[i].Group != details[j].Group {
			return details[i].Group < details[j].Group
		}
		return details[i].Name < details[j].Name
	})

	if configListFormat == "json" {
		return WriteOut(cmd.OutOrStdout(), details, OutputJSON)
	}
	return printConfigList(cmd, details)
}

func printConfigList(cmd *cobra.Command, details []configNodeDetail) error {
	out := cmd.OutOrStdout()
	if len(details) == 0 {
		if out != nil {
			fmt.Fprintln(out, "(no nodes match filters)")
		}
		return nil
	}

	headers := []string{"GROUP", "NAME", "HOST", "PORT", "USER", "ALIAS"}
	rows := make([][]string, 0, len(details))
	for _, d := range details {
		rows = append(rows, []string{
			d.Group, d.Name, d.Host,
			fmt.Sprintf("%d", d.Port), d.User, d.Alias,
		})
	}
	return PrintTable(out, headers, rows)
}

// ============================================================
// get
// ============================================================

var configGetCmd = &cobra.Command{
	Use:   "get <name-or-alias>",
	Short: "Show one node's full config (password field always redacted)",
	RunE:  runConfigGet,
}

func init() {
	configGetCmd.Flags().StringVar(&configGetNode, "name", "",
		"Node name (overrides positional arg)")
	configGetCmd.Flags().StringVar(&configListFormat, "format", "plain",
		"Output format: plain | json")
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	target, err := getTargetArg(args, 0, configGetNode)
	if err != nil {
		return WrapExit(ExitUsage, err)
	}
	cfg, err := loadOrCreateConfig()
	if err != nil {
		return WrapExit(ExitConfig, err)
	}

	d, err := findNodeByNameOrAlias(cfg, target)
	if err != nil {
		return WrapExit(ExitConfig, err)
	}

	if configListFormat == "json" {
		return WriteOut(cmd.OutOrStdout(), d, OutputJSON)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Group:        %s\n", d.Group)
	fmt.Fprintf(out, "Name:         %s\n", d.Name)
	if d.Alias != "" {
		fmt.Fprintf(out, "Alias:        %s\n", d.Alias)
	}
	fmt.Fprintf(out, "Host:         %s\n", d.Host)
	fmt.Fprintf(out, "User:         %s\n", d.User)
	fmt.Fprintf(out, "Port:         %d\n", d.Port)
	if d.KeyPath != "" {
		fmt.Fprintf(out, "KeyPath:      %s\n", d.KeyPath)
	}
	if d.Proxy != "" {
		fmt.Fprintf(out, "Proxy:        %s\n", d.Proxy)
	}
	if d.Password != "" {
		auth := classifyAuthForCLI(&config.SSHNode{
			Name: d.Name, Alias: d.Alias, Host: d.Host, User: d.User,
			Port: d.Port, KeyPath: d.KeyPath, Password: d.Password,
		})
		fmt.Fprintf(out, "Password:     %s (value redacted)\n", auth)
	}
	return nil
}

// classifyAuthForCLI 返回密码字段的认证类别 (用于 get/show 输出)
//
// 已迁移到 config.ClassifyAuth, 保持薄包装
func classifyAuthForCLI(n *config.SSHNode) string {
	return config.ClassifyAuth(n, config.CFG)
}

// ============================================================
// add
// ============================================================

var configAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new SSH node (--confirm to write; default --dry-run)",
	RunE:  runConfigAdd,
}

func init() {
	f := configAddCmd.Flags()
	f.StringVar(&configAddGroup, "group", "", "Group name (required)")
	f.StringVar(&configAddName, "name", "", "Node name (required, unique within group)")
	f.StringVar(&configAddAlias, "alias", "", "Alias (optional, must be globally unique)")
	f.StringVar(&configAddHost, "host", "", "Host address (required)")
	f.StringVar(&configAddUser, "user", "", "SSH user (default: root)")
	f.IntVar(&configAddPort, "port", 0, "SSH port (default: 22)")
	f.StringVar(&configAddPassword, "password", "",
		"Password (recommended: bcrypt hash or ${ENV} reference; plain text warns)")
	f.StringVar(&configAddKeyPath, "keypath", "", "Path to SSH private key")
	f.StringVar(&configAddProxy, "proxy", "", "Jump node name (proxy)")
	f.BoolVar(&configAddRotateKH, "rotate-known-hosts", false,
		"⚠️ Auto-rotate known_hosts on key mismatch (MITM risk)")
	f.Bool("confirm", false, "Actually write to mysshw.toml (default: dry-run)")
}

func runConfigAdd(cmd *cobra.Command, args []string) error {
	cfg, err := loadOrCreateConfig()
	if err != nil {
		return WrapExit(ExitConfig, err)
	}

	// 校验必填
	if configAddGroup == "" || configAddName == "" || configAddHost == "" {
		return NewExitError(ExitUsage,
			"--group, --name, --host are required")
	}

	// 唯一性校验
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == configAddName {
				return WrapExit(ExitConfig,
					fmt.Errorf("name %q already exists in group %q", configAddName, g.Groups))
			}
			if configAddAlias != "" && n.Alias == configAddAlias {
				return WrapExit(ExitConfig,
					fmt.Errorf("alias %q already used by %q", configAddAlias, n.Name))
			}
		}
	}

	// 明文密码警告
	if !configAddRotateKH {
		warnPlainPassword(configAddPassword)
	}

	newNode := &config.SSHNode{
		Name:             configAddName,
		Alias:            configAddAlias,
		Host:             configAddHost,
		User:             configAddUser,
		Port:             configAddPort,
		Password:         configAddPassword,
		KeyPath:          configAddKeyPath,
		Proxy:            configAddProxy,
		RotateKnownHosts: configAddRotateKH,
	}

	// 默认 --dry-run, 需 --confirm 才写
	addConfirm, _ := cmd.Flags().GetBool("confirm")
	if !addConfirm {
		newCfg := configWithNodeAdded(cfg, configAddGroup, newNode)
		fmt.Fprintln(cmd.OutOrStdout(), "mysshw:: dry-run mode, add would create:")
		fmt.Fprintf(cmd.OutOrStdout(), "  group=%s name=%s host=%s\n",
			configAddGroup, configAddName, configAddHost)
		fmt.Fprintln(cmd.OutOrStdout(), "  pass --confirm to actually write")
		return WriteOut(cmd.OutOrStdout(), configListFromCfg(newCfg, false), OutputJSON)
	}

	newCfg := configWithNodeAdded(cfg, configAddGroup, newNode)
	if err := config.WriteConfigAtomic(config.CFG_PATH, newCfg); err != nil {
		return WrapExit(ExitIOErr, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"mysshw:: added node %q to group %q\n", configAddName, configAddGroup)
	return nil
}

// configWithNodeAdded 返回 cfg 的拷贝, 在指定 group 末尾添加 node (或新建 group)
func configWithNodeAdded(cfg *config.Configs, group string, node *config.SSHNode) *config.Configs {
	newCfg := cloneConfigs(cfg)
	for i, g := range newCfg.Nodes {
		if g.Groups == group {
			newCfg.Nodes[i].SSHNodes = append(g.SSHNodes, node)
			return newCfg
		}
	}
	newCfg.Nodes = append(newCfg.Nodes, config.Nodes{Groups: group, SSHNodes: []*config.SSHNode{node}})
	return newCfg
}

// cloneConfigs 深拷贝 *Configs (避免修改共享 slice)
func cloneConfigs(c *config.Configs) *config.Configs {
	if c == nil {
		return &config.Configs{}
	}
	out := *c
	out.Nodes = make([]config.Nodes, len(c.Nodes))
	for i, g := range c.Nodes {
		newGroup := g
		newGroup.SSHNodes = make([]*config.SSHNode, len(g.SSHNodes))
		for j, n := range g.SSHNodes {
			if n == nil {
				continue
			}
			nodeCopy := *n
			newGroup.SSHNodes[j] = &nodeCopy
		}
		out.Nodes[i] = newGroup
	}
	return &out
}

// ============================================================
// remove / update / diff
// ============================================================

var configRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a node by name (default --dry-run; --confirm to apply)",
	RunE:  runConfigRemove,
}

func init() {
	configRemoveCmd.Flags().BoolVar(&configRemoveConfirm, "confirm", false,
		"Actually write to mysshw.toml (default: dry-run)")
}

func runConfigRemove(cmd *cobra.Command, args []string) error {
	cfg, err := loadOrCreateConfig()
	if err != nil {
		return WrapExit(ExitConfig, err)
	}
	name, err := getTargetArg(args, 0, configRemoveName)
	if err != nil {
		return WrapExit(ExitUsage, err)
	}

	// 找出节点
	found := false
	newCfg := cloneConfigs(cfg)
	for i, g := range newCfg.Nodes {
		for j, n := range g.SSHNodes {
			if n.Name == name {
				if !configRemoveConfirm {
					fmt.Fprintf(cmd.OutOrStdout(),
						"mysshw:: dry-run, would remove %q from group %q (--confirm to apply)\n",
						name, g.Groups)
					return nil
				}
				newCfg.Nodes[i].SSHNodes = append(g.SSHNodes[:j], g.SSHNodes[j+1:]...)
				// 如果组内没有更多节点了, 移除整个空组
				if len(newCfg.Nodes[i].SSHNodes) == 0 {
					newCfg.Nodes = append(newCfg.Nodes[:i], newCfg.Nodes[i+1:]...)
				}
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return WrapExit(ExitConfig,
			fmt.Errorf("node %q not found", name))
	}

	if err := config.WriteConfigAtomic(config.CFG_PATH, newCfg); err != nil {
		return WrapExit(ExitIOErr, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "mysshw:: removed node %q\n", name)
	return nil
}

var configUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Patch one or more fields of a node (default --dry-run; --confirm to apply)",
	RunE:  runConfigUpdate,
}

func init() {
	f := configUpdateCmd.Flags()
	f.StringVar(&configUpdateGroup, "group", "",
		"Move node to new group")
	f.StringVar(&configUpdateAlias, "alias", "",
		"Set new alias")
	f.StringVar(&configUpdateHost, "host", "",
		"Set new host")
	f.StringVar(&configUpdateUser, "user", "",
		"Set new user")
	f.IntVar(&configUpdatePort, "port", 0,
		"Set new port (use --port=0 to keep current)")
	f.StringVar(&configUpdatePassword, "password", "",
		"Set new password (recommended: bcrypt / ${ENV})")
	f.StringVar(&configUpdateKeyPath, "keypath", "",
		"Set new keypath (use --keypath=\"\" to clear)")
	f.StringVar(&configUpdateProxy, "proxy", "",
		"Set new proxy (use --proxy=\"\" to clear)")
	f.BoolVar(&configUpdateRotateKH, "rotate-known-hosts", false,
		"⚠️ Auto-rotate known_hosts (reads stdin if persistent flag)")
	f.BoolVar(&configUpdateConfirm, "confirm", false,
		"Actually write to mysshw.toml (default: dry-run)")
}

func runConfigUpdate(cmd *cobra.Command, args []string) error {
	cfg, err := loadOrCreateConfig()
	if err != nil {
		return WrapExit(ExitConfig, err)
	}
	name, err := getTargetArg(args, 0, configUpdateName)
	if err != nil {
		return WrapExit(ExitUsage, err)
	}

	// 找出节点位置
	nodeIdx, groupIdx, found := locateNode(cfg, name)
	if !found {
		return WrapExit(ExitConfig, fmt.Errorf("node %q not found", name))
	}

	oldNode := cfg.Nodes[groupIdx].SSHNodes[nodeIdx]
	flagSet := cmd.Flags()

	newCfg := cloneConfigs(cfg)
	newNode := newCfg.Nodes[groupIdx].SSHNodes[nodeIdx]

	// 增量应用: 只改通过 flag 显式指定的字段
	// Cobra flag.Changed 检测用户是否真的传了该 flag
	changed := false
	if flagSet.Changed("group") {
		newGroup := configUpdateGroup
		// 检查新 group 是否已存在
		exists := false
		for _, g := range newCfg.Nodes {
			if g.Groups == newGroup {
				exists = true
				break
			}
		}
		if !exists {
			newCfg.Nodes = append(newCfg.Nodes, config.Nodes{
				Groups:   newGroup,
				SSHNodes: []*config.SSHNode{},
			})
			// 把节点移到新 group
			targetGroupIdx := len(newCfg.Nodes) - 1
			newCfg.Nodes[targetGroupIdx].SSHNodes = append(
				newCfg.Nodes[targetGroupIdx].SSHNodes, newNode)
			newCfg.Nodes[groupIdx].SSHNodes = append(
				newCfg.Nodes[groupIdx].SSHNodes[:nodeIdx],
				newCfg.Nodes[groupIdx].SSHNodes[nodeIdx+1:]...)
		} else {
			return WrapExit(ExitConfig,
				fmt.Errorf("group %q already exists, can't move", newGroup))
		}
		changed = true
	}

	if flagSet.Changed("alias") {
		newNode.Alias = configUpdateAlias
		changed = true
	}
	if flagSet.Changed("host") {
		newNode.Host = configUpdateHost
		changed = true
	}
	if flagSet.Changed("user") {
		newNode.User = configUpdateUser
		changed = true
	}
	if flagSet.Changed("port") {
		newNode.Port = configUpdatePort
		changed = true
	}
	if flagSet.Changed("password") {
		newNode.Password = configUpdatePassword
		warnPlainPassword(configUpdatePassword)
		changed = true
	}
	if flagSet.Changed("keypath") {
		newNode.KeyPath = configUpdateKeyPath
		changed = true
	}
	if flagSet.Changed("proxy") {
		newNode.Proxy = configUpdateProxy
		changed = true
	}
	if flagSet.Changed("rotate-known-hosts") {
		newNode.RotateKnownHosts = configUpdateRotateKH
		changed = true
	}

	if !changed {
		return NewExitError(ExitUsage,
			"no fields specified (use --alias / --host / --port etc. to modify)")
	}

	if !configUpdateConfirm {
		fmt.Fprintf(cmd.OutOrStdout(),
			"mysshw:: dry-run, would patch node %q (--confirm to apply)\n", name)
		return WriteOut(cmd.OutOrStdout(), []string{configDiffNodes(oldNode, newNode)}, OutputJSON)
	}

	if err := config.WriteConfigAtomic(config.CFG_PATH, newCfg); err != nil {
		return WrapExit(ExitIOErr, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "mysshw:: updated node %q\n", name)
	return nil
}

var configDiffCmd = &cobra.Command{
	Use:   "diff <name-or-a> <name-or-b>",
	Short: "Compare two nodes field-by-field",
	RunE:  runConfigDiff,
}

func init() {
	configDiffCmd.Flags().StringVar(&configDiffA, "a", "", "First node (name or alias)")
	configDiffCmd.Flags().StringVar(&configDiffB, "b", "", "Second node (name or alias)")
}

func runConfigDiff(cmd *cobra.Command, args []string) error {
	cfg, err := loadOrCreateConfig()
	if err != nil {
		return WrapExit(ExitConfig, err)
	}

	a, err := getTargetArg(args, 0, configDiffA)
	if err != nil {
		return WrapExit(ExitUsage, fmt.Errorf("--a: %w", err))
	}
	b, err := getTargetArg(args, 1, configDiffB)
	if err != nil {
		return WrapExit(ExitUsage, fmt.Errorf("--b: %w", err))
	}

	da, err := findNodeByNameOrAlias(cfg, a)
	if err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("a: %w", err))
	}
	db, err := findNodeByNameOrAlias(cfg, b)
	if err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("b: %w", err))
	}

	return WriteOut(cmd.OutOrStdout(), []configNodeDetail{da, db}, OutputJSON)
}

// ============================================================
// 共享 helpers
// ============================================================

// loadOrCreateConfig 读配置文件, 失败时返回空 *Configs (首次 init 流程一致)
func loadOrCreateConfig() (*config.Configs, error) {
	cfgPath := config.CFG_PATH
	if cfgPath == "" {
		cfgPath = "~/.mysshw.toml"
	}
	cfg, err := config.ReadConfigOrEmpty(cfgPath)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &config.Configs{}
	}
	return cfg, nil
}

// getTargetArg 取位置参数中第 idx 个, 或 fallback 到 flag
func getTargetArg(args []string, idx int, fallback string) (string, error) {
	if idx < len(args) {
		return args[idx], nil
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("missing node name or alias (provide as argument or --name)")
}

// locateNode 找节点位置, 返回 (nodeIdx, groupIdx, found)
func locateNode(cfg *config.Configs, name string) (int, int, bool) {
	for gi, g := range cfg.Nodes {
		for ni, n := range g.SSHNodes {
			if n.Name == name {
				return ni, gi, true
			}
		}
	}
	return -1, -1, false
}

// findNodeByNameOrAlias 按 name 或 alias 查找, 找不到返回错误
func findNodeByNameOrAlias(cfg *config.Configs, nameOrAlias string) (configNodeDetail, error) {
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == nameOrAlias || (n.Alias != "" && n.Alias == nameOrAlias) {
				return configNodeFromSSHNode(g.Groups, n), nil
			}
		}
	}
	return configNodeDetail{}, fmt.Errorf("node %q not found (by name or alias)", nameOrAlias)
}

// configListFromCfg 从 cfg 生成 []configNodeDetail (供 --dry-run 预览用)
// 只包含指定 group (空字符串 = 全部)
func configListFromCfg(c *config.Configs, all bool) []configNodeDetail {
	var out []configNodeDetail
	for _, g := range c.Nodes {
		for _, n := range g.SSHNodes {
			if all || g.Groups == configAddGroup {
				out = append(out, configNodeFromSSHNode(g.Groups, n))
			}
		}
	}
	return out
}

// configDiffNodes 返回差异描述字符串 (为 update dry-run 用)
func configDiffNodes(oldN, newN *config.SSHNode) string {
	// 这里简化为只返回字段差异; 更复杂的实现留给未来
	return fmt.Sprintf("updated %q: host=%q->%q port=%d->%d",
		oldN.Name, oldN.Host, newN.Host, oldN.Port, newN.Port)
}

// warnPlainPassword 在 stderr 输出明文密码警告 (不阻塞, 仅警告)
func warnPlainPassword(p string) {
	if p == "" {
		return
	}
	if config.IsBcryptHash(p) {
		return
	}
	if strings.HasPrefix(strings.TrimSpace(p), "${") && strings.HasSuffix(strings.TrimSpace(p), "}") {
		return
	}
	fmt.Fprintln(os.Stderr,
		"mysshw:: warning: password is plaintext, recommend bcrypt hash or ${ENV} reference (see `mysshw hash gen`)")
}

// 保留 json 引用, 避免 unused import (configListFromCfg 内部使用)
var _ = json.Marshal
