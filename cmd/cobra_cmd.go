package cmd

import (
	"context"
	"fmt"
	"os"

	"mysshw/config"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// isTerminalStdout 检查 stdout 是否关联到 terminal (用于向导向导只在交互式 shell 触发)
func isTerminalStdout() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// 版本信息变量，将由 main 包设置
var (
	Version   string
	Build     string
	BuildTime string
	GoVersion string
)

// 定义自定义类型作为 context key，避免与其他包的 key 冲突
type cfgKeyType string

const cfgKey cfgKeyType = "cfg"

// rootCmd 代表没有调用子命令时的基础命令
var rootCmd = &cobra.Command{
	Use:     "mysshw",
	Version: Version,
	Short:   "CLI mysshw: A free and open source SSH command line client software.",
	Long: `CLI mysshw: A free and open source SSH command line client software.

Use "mysshw help" for more information about a specific command.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 检查是否请求版本信息
		versionFlag, _ := cmd.Flags().GetBool("version")
		if versionFlag {
			printVersion()
			return
		}

		// 处理配置文件路径
		cfgPath, _ := cmd.Flags().GetString("cfg")
		if cfgPath != "" {
			//log.Println("Config path changed to", cfgPath)
			config.CFG_PATH = cfgPath
			// 更新上下文中的配置路径
			cmd.SetContext(context.WithValue(cmd.Context(),
				cfgKey, cfgPath))
		}

		// 首次运行检测 (用户决定 4: 显示本次更新 + 走向导)
		// 仅在 stdout 是 terminal 时触发, 避免 CI/管道噪音
		if IsFirstRun() && isTerminalStdout() {
			// 无 TOML → 显示 What's New + 自动跳 initCmd
			showWhatsNew()
			if err := runInitWizard(false); err != nil {
				fmt.Fprintf(os.Stderr, "mysshw:: init wizard failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Re-run `mysshw` to load the new config.")
			return
		}
		// 升级检测 (TOML 存在但 lastseen 旧版本) → 仅显示 What's New
		if IsUpgradedFromOlder() && isTerminalStdout() {
			showWhatsNew()
			_ = markVersionSeen()
		}

		// 当没有子命令时执行 RunSSH
		RunSSH(cmd.Context())
	},
	Example: `  # Default: interactive SSH session (TUI menu to pick node)
  mysshw

  # Custom config file
  mysshw --cfg /path/to/config.toml

  # First-run wizard
  mysshw init

  # List nodes (human-readable or JSON)
  mysshw list
  mysshw list -o json --search prod

  # Non-interactive: run command on a node
  mysshw exec --node prod-1 -- uptime
  mysshw exec --node prod-1 --node prod-2 -- "df -h"          # batch
  mysshw exec -f --node prod-1 -- "journalctl -f"             # stream

  # SCP: copy files to/from nodes
  mysshw copy --upload --node prod-1 --local ./file --remote /tmp/
  mysshw copy --download --node prod-1 --remote /var/log/app.log --local ./

  # Port forwarding (local / reverse / SOCKS5)
  mysshw tunnel local --node prod-1 --local 8080 --remote 80
  mysshw tunnel reverse --node prod-1 --remote 9090 --local 90
  mysshw tunnel dynamic --node prod-1 --local 1080               # SOCKS5

  # Ping: TCP connectivity check
  mysshw ping --node prod-1
  mysshw ping --host 10.0.0.1 --port 22

  # Info: remote system information
  mysshw info --node prod-1
  mysshw info --node prod-1 --node prod-2 -o json        # batch

  # CRUD: manage SSH nodes in config
  mysshw config list
  mysshw config get prod-1
  mysshw config add --group Production --name web-2 --host 10.0.0.2 --user app
  mysshw config update prod-1 --port 2222 --confirm
  mysshw config remove prod-1 --confirm
  mysshw config diff prod-1 prod-2

  # Export/Import: portable node config
  mysshw export --group Production --format toml --output nodes.toml
  mysshw import nodes.toml --confirm

  # Host key management
  mysshw hostkey list
  mysshw hostkey show 10.0.0.1
  mysshw hostkey remove 10.0.0.1

  # Password & encryption tools
  mysshw hash gen                    # generate bcrypt hash
  mysshw hash verify                 # verify password against hash
  mysshw hash pepper                 # generate secret_key for bcrypt-pepper
  mysshw encrypt                     # encrypt mysshw.toml with passphrase
  mysshw encrypt decrypt             # decrypt mysshw.toml

  # Config sync (SCP / WebDAV / S3)
  mysshw sync --upload
  mysshw sync --down

  # Migrate from sshw YAML
  mysshw yml -f ~/.sshw.yml

  # AI integration: MCP server (stdio)
  mysshw mcp

  # AI integration: CLI schema discovery
  mysshw completions json
  mysshw completions json --flat
  mysshw completions json exec        # filter by subcommand

  # Display version or help
  mysshw version
  mysshw -v`,
}

// 初始化命令，添加标志等
func init() {
	// 添加全局标志
	rootCmd.PersistentFlags().StringP("cfg", "c", "", "Custom config file path (default is $HOME/.mysshw.toml)")

	rootCmd.PersistentFlags().BoolP("version", "v", false, "Print version for mysshw")

	// AI 友好子命令使用的全局输出格式 (list / exec 等)
	RegisterOutputFlag(rootCmd)

	// 添加子命令
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(VersionCmd)
	rootCmd.AddCommand(YMLCmd)
	// listCmd / execCmd 由各自文件 init() 独占注册 (避免重复)

	// 为 sync 命令添加标志
	syncCmd.Flags().BoolP("upload", "u", false, "Upload local config to remote server")
	syncCmd.Flags().BoolP("down", "z", false, "Download remote config to local")
	// 加密相关 (默认上传加密: 远程只存密文, 防远程被入侵后备份被拖走)
	syncCmd.Flags().Bool("encrypt", true, "Encrypt config before upload (remote stores ciphertext only). Use --encrypt=false to upload plaintext")
	syncCmd.Flags().Bool("keep-encrypted", false, "Keep downloaded config encrypted on disk (skip local decryption)")
	syncCmd.Flags().String("passphrase-env", DefaultSyncPassphraseEnv, "Environment variable holding the sync encryption passphrase")
	//rootCmd.Context()
	rootCmd.SetContext(context.Background())
	rootCmd.SetContext(context.WithValue(rootCmd.Context(), cfgKey, config.CFG_PATH))

}

// Execute 执行根命令
//
// 退出码策略:
//   - RunE 返回 nil                  → exit 0
//   - RunE 返回 *ExitError            → exit ExitError.Code (BSD sysexits)
//   - RunE 返回其他 error 或 flag 错  → exit ExitGeneric (1)
func Execute() {
	err := rootCmd.ExecuteContext(context.Background())
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	if ee, ok := err.(ExitError); ok {
		os.Exit(ee.Code)
	}
	os.Exit(ExitGeneric)
}

// ExecuteCobra 供 cmd.go 调用的执行函数
func ExecuteCobra() {
	Execute()
}
