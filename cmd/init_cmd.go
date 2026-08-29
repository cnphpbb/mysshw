package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mysshw/config"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const lastSeenVersionFile = ".mysshw.lastseen"

// initCmd 向导式初始化 mysshw 配置 (用户决定 4: 首次运行显示本次更新 + 向导)
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize mysshw config (interactive wizard)",
	Long: `Run the interactive wizard to create or update ~/.mysshw.toml.

On first run (no config file), shows the "what's new" page and then the wizard.
On subsequent runs, asks to overwrite existing config.

Examples:
  mysshw init              # run wizard (interactive)
  mysshw init --force      # overwrite without confirmation`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		return runInitWizard(force)
	},
}

func init() {
	initCmd.Flags().BoolP("force", "f", false, "Overwrite existing config without confirmation")
	rootCmd.AddCommand(initCmd)
}

// runInitWizard 向导主流程
//
//	force=true:  覆盖已有配置, 不询问
//	force=false: 已存在 → 询问 y/N
func runInitWizard(force bool) error {
	cfgPath, err := config.GetCfgPath("")
	if err != nil {
		return fmt.Errorf("mysshw:: failed to get config path: %v", err)
	}

	// 检查是否已存在
	_, statErr := os.Stat(cfgPath)
	exists := statErr == nil
	if exists && !force {
		fmt.Printf("Config already exists at: %s\n", cfgPath)
		fmt.Print("Overwrite? [y/N]: ")
		var ans string
		_, _ = fmt.Scanln(&ans)
		if !strings.EqualFold(ans, "y") {
			fmt.Println("Aborted. Existing config kept.")
			return nil
		}
	}

	// 首次运行: 显示 What's New
	if !exists {
		showWhatsNew()
	}

	// 走向导
	cfg, err := runWizardSteps(cfgPath)
	if err != nil {
		return err
	}

	// 保存
	if err := saveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("mysshw:: failed to save config: %v", err)
	}

	// 更新 lastseen 标记
	_ = markVersionSeen()

	// 输出总结
	fmt.Println()
	fmt.Println("✓ Setup complete!")
	fmt.Printf("  Config saved to: %s\n", cfgPath)
	fmt.Printf("  Backup keep:     %d\n", cfg.BackupKeep)
	fmt.Printf("  Nodes:           %d\n", len(cfg.Nodes))
	fmt.Println()
	fmt.Println("⚠️  请退出后重新启动 mysshw 以加载配置。")
	fmt.Println()
	fmt.Print("Press Enter to exit...")
	_, _ = fmt.Scanln()
	return nil
}

// runWizardSteps 走 huh 向导收集用户输入
// 返回组装好的 config.Configs
func runWizardSteps(cfgPath string) (*config.Configs, error) {
	var cfg config.Configs

	// Step 1: 同步类型
	var syncType string
	err := huh.NewSelect[string]().
		Title("Step 1/4: 同步类型 (sync type)?").
		Description("把配置备份到远程 (留 None 则只本地使用)").
		Options(
			huh.NewOption("None (仅本地使用)", ""),
			huh.NewOption("SCP", "scp"),
			huh.NewOption("WebDAV", "webdav"),
			huh.NewOption("S3", "s3"),
		).
		Value(&syncType).
		Run()
	if err != nil {
		return nil, err
	}

	if syncType != "" {
		cfg.SyncCfg.Type = syncType
		// 不同类型询问不同字段 (简化: 暂只问 uri, 详细信息由用户后续手动编辑)
		var uri string
		err = huh.NewInput().
			Title("Remote URI (e.g. user@host:port for scp, host:port for s3/webdav):").
			Value(&uri).
			Run()
		if err != nil {
			return nil, err
		}
		cfg.SyncCfg.RemoteUri = uri
	}

	// Step 2: 添加 SSH 节点
	fmt.Println()
	fmt.Println("Step 2/4: 添加第一个 SSH 节点")
	var groupName, nodeName, host, user string
	var port int
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("组名 (groups):").Value(&groupName).Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("不能为空")
				}
				return nil
			}),
			huh.NewInput().Title("节点名 (name):").Value(&nodeName).Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("不能为空")
				}
				return nil
			}),
			huh.NewInput().Title("主机 (host):").Value(&host).Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("不能为空")
				}
				return nil
			}),
			huh.NewInput().Title("用户 (user, 默认 root):").Value(&user),
			huh.NewInput().Title("端口 (port, 默认 22):").Value(itoaPtr(&port)),
		),
	).Run()
	if err != nil {
		return nil, err
	}
	if user == "" {
		user = "root"
	}
	if port == 0 {
		port = 22
	}

	// Step 3: 认证方式 + 密码处理
	var authMethod string
	err = huh.NewSelect[string]().
		Title("Step 3/4: 认证方式?").
		Options(
			huh.NewOption("Password (密码)", "password"),
			huh.NewOption("Public key (公钥)", "key"),
		).
		Value(&authMethod).
		Run()
	if err != nil {
		return nil, err
	}

	node := &config.SSHNode{
		Name: nodeName,
		Host: host,
		User: user,
		Port: port,
	}

	if authMethod == "password" {
		var pwdChoice string
		err = huh.NewSelect[string]().
			Title("密码处理方式?").
			Options(
				huh.NewOption("Enter now (bcrypt 哈希存入 TOML)", "bcrypt"),
				huh.NewOption("Skip (每次手动输入, 不写回)", "empty"),
				huh.NewOption("Use env var (推荐 CI/脚本)", "env"),
			).
			Value(&pwdChoice).
			Run()
		if err != nil {
			return nil, err
		}

		switch pwdChoice {
		case "bcrypt":
			hash, err := config.HashPassword(promptPwdFromStdin("请输入明文密码 (会 bcrypt 哈希存储):"))
			if err != nil {
				return nil, fmt.Errorf("bcrypt 哈希失败: %v", err)
			}
			node.Password = hash
		case "empty":
			node.Password = "" // 显式空 → 每次交互输入
		case "env":
			node.Password = fmt.Sprintf("${MYSSHW_PWD_%s}", nodeName)
		}
	} else {
		// 公钥认证
		var keypath string
		err = huh.NewInput().
			Title("私钥路径 (留空用 ~/.ssh/id_rsa):").
			Value(&keypath).
			Run()
		if err != nil {
			return nil, err
		}
		if keypath != "" {
			node.KeyPath = keypath
		}
	}

	// Step 4: 保存 (preview + confirm)
	fmt.Println()
	fmt.Println("Step 4/4: 预览配置")
	preview := previewNode(groupName, node)
	fmt.Println(preview)
	fmt.Println()
	var confirmed bool
	err = huh.NewConfirm().Title("保存上述配置?").Value(&confirmed).Run()
	if err != nil {
		return nil, err
	}
	if !confirmed {
		return nil, fmt.Errorf("用户取消保存")
	}

	cfg.Nodes = []config.Nodes{
		{Groups: groupName, SSHNodes: []*config.SSHNode{node}},
	}
	cfg.BackupKeep = config.DefaultBackupKeep
	return &cfg, nil
}

// previewNode 生成节点预览 (供向导步骤 4 展示给用户)
// 使用 config.SerializeConfigs 序列化 (避免手写 TOML 转义 bug)
func previewNode(group string, node *config.SSHNode) string {
	// 构造最小 Configs, 只包含当前节点, 便于预览
	cfg := &config.Configs{
		BackupKeep: config.DefaultBackupKeep,
		Nodes: []config.Nodes{
			{Groups: group, SSHNodes: []*config.SSHNode{node}},
		},
	}
	data, err := config.SerializeConfigs(cfg)
	if err != nil {
		// 预览失败不应阳断向导, 返回简洁文本代替
		return fmt.Sprintf("[[nodes]]\ngroups = %q\n\n[[nodes.ssh]]\nname = %q\nhost = %q\nuser = %q\nport = %d\n",
			group, node.Name, node.Host, node.User, node.Port)
	}
	return string(data)
}

// saveConfig 写 TOML (备份已有配置)
// 使用 config.SerializeConfigs (走 pelletier/go-toml/v2) + config.WriteTomlWithBackup
func saveConfig(path string, cfg *config.Configs) error {
	data, err := config.SerializeConfigs(cfg)
	if err != nil {
		return err
	}
	return config.WriteTomlWithBackup(path, data)
}

// initTomlHeader 返回向导生成的 TOML 文件顶部注释 (解释性文本)
// pelletier 的 Marshal 不产生注释, 手工追加以保持向导输出的可读性
func initTomlHeader() string {
	var b bytes.Buffer
	b.WriteString("# mysshw config generated by `mysshw init`\n")
	b.WriteString("# 完整字段参考: https://github.com/cnphpbb/mysshw/blob/master/readme.md#config\n")
	b.WriteString("# 密码推荐: bcrypt 哈希 / ${MYSSHW_PWD_xxx} env 引用 / 留空交互输入\n")
	return b.String()
}

// showWhatsNew 显示本次版本主要更新 (用户决定 4)
func showWhatsNew() {
	fmt.Println("╭─────────────────────────────────────────────────────────╮")
	fmt.Println("│  欢迎使用 mysshw v" + Version + "                            │")
	fmt.Println("╰─────────────────────────────────────────────────────────╯")
	fmt.Println()
	fmt.Println("本次版本主要更新:")
	fmt.Println("  ✨ 主机指纹校验 (TOFU): 防 MITM 中间人攻击")
	fmt.Println("  ✨ ${ENV_VAR} 密码引用: 避免明文落盘 (CI/脚本友好)")
	fmt.Println("  ✨ 配置文件自动 chmod 0600: 防同机其他用户读取")
	fmt.Println("  ✨ SCP / WebDAV / S3 三种远程同步协议")
	fmt.Println("  ✨ Docker 镜像 (~21 MB): 解决 Windows 兼容性问题")
	fmt.Println("  ✨ 本地备份机制: 写 TOML 自动备份, 可回滚 (默认保留 5 个)")
	fmt.Println("  ✨ bcrypt 密码存储: 哈希存储, 抗暴力破解")
	fmt.Println("  ✨ 交互输入密码: 空密码可每次手动输入")
	fmt.Println("  ✨ 首次运行向导: 帮你快速生成配置 (当前)")
	fmt.Println()
	fmt.Println("⚠️  即将进入配置向导, 回答几个问题即可生成 ~/.mysshw.toml")
	fmt.Println()
	fmt.Print("按 Enter 继续...")
	_, _ = fmt.Scanln()
}

// markVersionSeen 写入 lastseen 标记文件
func markVersionSeen() error {
	cfgDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(cfgDir, lastSeenVersionFile)
	return os.WriteFile(path, []byte(Version), 0600)
}

// lastSeenMarkerPath 返回 lastseen 标记文件路径 (测试用)
func lastSeenMarkerPath() string {
	cfgDir, _ := os.UserHomeDir()
	return filepath.Join(cfgDir, lastSeenVersionFile)
}

// IsFirstRun 检查是否首次运行 (无 TOML)
//
//	返回: (无 TOML, 无 lastseen) → true
func IsFirstRun() bool {
	cfgPath, _ := config.GetCfgPath("")
	return isFirstRunAtPath(cfgPath)
}

// isFirstRunAtPath 检查指定路径是否首次运行 (供测试用)
func isFirstRunAtPath(cfgPath string) bool {
	if _, err := os.Stat(cfgPath); err == nil {
		return false // TOML 存在, 不是首次
	}
	// TOML 不存在, 但 lastseen 标记存在 → 之前用过, 配置文件被删了
	//   此时: 显示向导
	// 严格 "首次": 也没 lastseen 标记
	// 宽松: TOML 不存在 → 一律走向导
	return true
}

// IsUpgradedFromOlder 检查是否从旧版本升级 (lastseen 版本 != 当前)
func IsUpgradedFromOlder() bool {
	return isUpgradedFromOlderAt(lastSeenMarkerPath())
}

// isUpgradedFromOlderAt 检查指定路径的 lastseen (供测试用)
func isUpgradedFromOlderAt(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false // 无 lastseen → 不算升级, 可能是首次
	}
	lastSeen := strings.TrimSpace(string(data))
	return lastSeen != Version
}

// promptPwdFromStdin 从 stdin 安全读取密码 (不回显)
//
// 使用 term.ReadPassword 关闭逐字回显 (避免密码被旁人肩窥)。
// 安全性增强: 原实现用 fmt.Scanln, 密码会逐字出现在终端上。
func promptPwdFromStdin(prompt string) string {
	fmt.Print(prompt)
	fd := int(os.Stdin.Fd())
	b, err := term.ReadPassword(fd)
	fmt.Println() // 补一个换行 (ReadPassword 吃掉原始 '\n')
	if err != nil {
		// 降级到 Scanln (仅在 terminal 不可用时, 例如 CI 环境)
		fmt.Fprintln(os.Stderr, "mysshw:: term.ReadPassword failed, falling back to Scanln")
		var pw string
		_, _ = fmt.Scanln(&pw)
		return pw
	}
	return string(b)
}

// itoaPtr 辅助函数 (huh.NewInput 不直接支持 int, 用 *string 包装)
func itoaPtr(p *int) *string {
	if p == nil {
		s := ""
		return &s
	}
	s := strconv.Itoa(*p)
	return &s
}
