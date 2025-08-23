package cmd

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"mysshw/auth"
	"mysshw/config"
	"mysshw/scp"
	"mysshw/ssh"

	"github.com/spf13/cobra"
	crypto_ssh "golang.org/x/crypto/ssh"
)

// 版本信息变量，将由 main 包设置
var (
	Version   string
	Build     string
	BuildTime string
	GoVersion string
)

// rootCmd 代表没有调用子命令时的基础命令
var rootCmd = &cobra.Command{
	Use:     "mysshw",
	Version: Version,
	Short:   "A free and open source ssh cli client soft.",
	Long: `A free and open source ssh cli client soft.

Usage:
  mysshw [command]

Available Commands:
  sync       Sync config file to remote
  help       Help about any command

Flags:
  -c, --cfg string    config file (default is $HOME/.mysshw.toml)
  -h, --help          help for mysshw
  -v, --version       version for mysshw

Use "mysshw [command] --help" for more information about a command.
`,
	Run: func(cmd *cobra.Command, args []string) {
		// 检查是否请求版本信息
		versionFlag, _ := cmd.Flags().GetBool("version")
		if versionFlag {
			printVersion()
			return
		}
		// 当没有子命令时执行 RunSSH
		RunSSH()
	},
}

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of mysshw",
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

// syncCmd 同步配置文件到远程
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync config file to remote",
	Long:  `Sync config file to remote server or download from remote.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 处理配置文件路径
		cfgPath, _ := cmd.Flags().GetString("cfg")
		if cfgPath != "" {
			log.Println("started path changed to", cfgPath)
			config.CFG_PATH = cfgPath
		}
		log.Println("started path changed to", config.CFG_PATH)

		// 加载配置
		if err := config.LoadViperConfig(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// 准备 SSH 配置
		syncCfg := config.CFG.SyncCfg
		sshCfg := crypto_ssh.ClientConfig{
			User: syncCfg.UserName,
			Auth: []crypto_ssh.AuthMethod{
				auth.PasswordKey(syncCfg.UserName, syncCfg.Password),
			},
			HostKeyCallback: crypto_ssh.InsecureIgnoreHostKey(),
		}

		// 创建 SCP 客户端
		client := scp.NewClient(syncCfg.RemoteUri, &sshCfg)
		if err := client.Connect(); err != nil {
			fmt.Printf("Couldn't establish a connection to the remote server: %s \n", err)
			os.Exit(1)
		}
		defer client.Close()

		// 处理上传或下载
		upload, _ := cmd.Flags().GetBool("upload")
		down, _ := cmd.Flags().GetBool("down")

		if upload {
			fmt.Println("mysshw:: Use Upload Local Config Remote Server!! Begin... ")
			cfgBytes, _ := config.LoadConfigBytes(config.CFG_PATH)
			if err := client.CopyFile(bytes.NewReader(cfgBytes), syncCfg.RemotePath, "0644"); err != nil {
				fmt.Printf("Error while copying file: %s", err)
				os.Exit(1)
			}
			fmt.Println("mysshw:: Use Upload Local Config Remote Server!! End... ")
			return
		}

		if down {
			fmt.Println("mysshw:: Use Remote Config Download Local!!  Begin... ")
			localPath, _ := config.GetCfgPath(config.CFG_PATH)
			f, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				fmt.Printf("Couldn't open the output file: %s  \n", err)
				os.Exit(1)
			}
			defer f.Close()
			if err := client.CopyFromRemote(f, syncCfg.RemotePath); err != nil {
				fmt.Printf("Error Copy failed from remote: %s \n", err)
				os.Exit(1)
			}
			fmt.Println("mysshw:: Use Remote Config Download Local!!  End...  ")
			return
		}

		fmt.Println("Please specify either --upload or --down flag")
	},
}

// 初始化命令，添加标志等
func init() {
	// 添加全局标志
	rootCmd.PersistentFlags().StringP("cfg", "c", "", "config file (default is $HOME/.mysshw.toml)")
	rootCmd.PersistentFlags().BoolP("version", "v", false, "version for mysshw")

	// 添加子命令
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(VersionCmd)

	// 为 sync 命令添加标志
	syncCmd.Flags().BoolP("upload", "u", false, "Update mysshw config")
	syncCmd.Flags().BoolP("down", "z", false, "Download mysshw config")
}

// Execute 执行根命令
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ExecuteCobra 供 cmd.go 调用的执行函数
func ExecuteCobra() {
	Execute()
}

// printVersion 打印版本信息
func printVersion() {
	fmt.Printf("mysshw version: %s\n", Version)
	fmt.Printf("Build: %s\n", Build)
	if BuildTime != "" {
		fmt.Printf("Build Time: %s\n", BuildTime)
	}
	fmt.Printf("Go Version: %s\n", GoVersion)
}

// SetVersion 设置版本信息
func SetVersion(version, build, buildTime, goVersion string) {
	Version = version
	Build = build
	BuildTime = buildTime
	GoVersion = goVersion
}

// RunSSH 执行 SSH 登录
func RunSSH() {
	if err := config.LoadViperConfig(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	node := ssh.Choose(config.CFG)
	client := ssh.NewClient(node)
	client.Login()
}
