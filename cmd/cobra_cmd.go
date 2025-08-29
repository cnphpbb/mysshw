package cmd

import (
	"context"
	"fmt"
	"os"

	"mysshw/config"

	"github.com/spf13/cobra"
)

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

		// 当没有子命令时执行 RunSSH
		RunSSH(cmd.Context())
	},
	Example: `  # Connect to SSH using default configuration
  mysshw

  # Connect using a custom configuration file
  mysshw --cfg /path/to/custom/config.toml
  or
  mysshw -c /path/to/custom/config.toml

  # Sync local config to remote server
  mysshw sync --upload | -u

  # Download remote config to local
  mysshw sync --down | -z

  # Custom configuration file path for sync command
  mysshw sync --cfg /path/to/custom/config.toml --upload or --down
  or
  mysshw sync -c /path/to/custom/config.toml -u or -z

  # Migrate from sshw YAML config to mysshw TOML config file
  mysshw yml -f ~/.sshw.yml
  or
  mysshw yml --file ~/.sshw.yml

  # Display version information
  mysshw version | -v | --version

  # Display help information
  mysshw help | -h | --help
  `,
}

// 初始化命令，添加标志等
func init() {
	// 添加全局标志
	rootCmd.PersistentFlags().StringP("cfg", "c", "", "Custom config file path (default is $HOME/.mysshw.toml)")

	rootCmd.PersistentFlags().BoolP("version", "v", false, "Print version for mysshw")

	// 添加子命令
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(VersionCmd)
	rootCmd.AddCommand(YMLCmd)

	// 为 sync 命令添加标志
	syncCmd.Flags().BoolP("upload", "u", false, "Upload local config to remote server")
	syncCmd.Flags().BoolP("down", "z", false, "Download remote config to local")
	//rootCmd.Context()
	rootCmd.SetContext(context.Background())
	rootCmd.SetContext(context.WithValue(rootCmd.Context(), cfgKey, config.CFG_PATH))

}

// Execute 执行根命令
func Execute() {
	err := rootCmd.ExecuteContext(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ExecuteCobra 供 cmd.go 调用的执行函数
func ExecuteCobra() {
	Execute()
}
