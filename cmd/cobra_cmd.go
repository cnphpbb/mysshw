package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path"
	"syscall"
	"time"

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

  # Display version information
  mysshw version | -v | --version

  # Display help information
  mysshw help | -h | --help
  `,
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
	Short: "Sync config file to remote server or download from remote server.",
	Long:  `Sync config file to remote server or download from remote server.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 处理配置文件路径
		cfgPath, _ := cmd.Flags().GetString("cfg")
		if cfgPath != "" {
			log.Println("started path changed to", cfgPath)
			config.CFG_PATH = cfgPath
		}
		log.Println("started path changed to", config.CFG_PATH)

		// 加载配置
		if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
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
	rootCmd.PersistentFlags().StringP("cfg", "c", "", "Custom config file path (default is $HOME/.mysshw.toml)")

	rootCmd.PersistentFlags().BoolP("version", "v", false, "Print version for mysshw")

	// 添加子命令
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(VersionCmd)

	// 为 sync 命令添加标志
	syncCmd.Flags().BoolP("upload", "u", false, "Upload local config to remote server")
	syncCmd.Flags().BoolP("down", "z", false, "Download remote config to local")
	//rootCmd.Context()
	rootCmd.SetContext(context.Background())
	rootCmd.SetContext(context.WithValue(rootCmd.Context(), cfgKey, config.CFG_PATH))

}

// 从 context 中获取配置路径，如果不存在则返回默认路径
func GetCtxConfigPath(ctx context.Context) string {
	if path, ok := ctx.Value(cfgKey).(string); ok {
		//fmt.Println("GetCfgPath::path", path)
		return path
	}
	// 默认配置路径: $HOME/.mysshw.toml
	usr, err := user.Current()
	if err != nil {
		//fmt.Println("GetCfgPath::user.Current().err", config.CFGPATH)
		return config.CFGPATH
	}
	//fmt.Println("GetCfgPath::return", path.Join(usr.HomeDir, ".mysshw.toml"))
	return path.Join(usr.HomeDir, ".mysshw.toml")
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

// printVersion 打印版本信息
func printVersion() {
	fmt.Println(" mysshw - a free and open source ssh cli client soft.")
	fmt.Printf("    - Version:: %s\n", Version)
	fmt.Printf("    - GitVersion:: %s\n", Build)
	if BuildTime != "" {
		fmt.Printf("    - BuildTime :: %s\n", BuildTime)
	}
	fmt.Printf("    - Go Version :: %s\n", GoVersion)
}

// SetVersion 设置版本信息
func SetVersion(version, build, buildTime, goVersion string) {
	Version = version
	Build = build
	BuildTime = buildTime
	GoVersion = goVersion
}

// RunSSH 执行 SSH 登录
// 修复初始化循环问题，将 context 作为参数传入
func RunSSH(ctx context.Context) {
	cfgPath := GetCtxConfigPath(ctx)
	fmt.Println("Config path changed to:", cfgPath)
	if err := config.LoadViperConfig(cfgPath); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// 设置信号处理捕获Ctrl+C和SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived termination signal, exiting...")
		os.Exit(0)
	}()

	// 创建一个读取器用于检测键盘输入
	reader := bufio.NewReader(os.Stdin)
	// 提示用户输入
	fmt.Println("请输入您的选择 (输入节点编号或按Ctrl+d退出):")

	for {
		// 显示菜单前提示按Ctrl+d退出
		fmt.Print("\033[H\033[2J")
		fmt.Println("Press 'Ctrl+d' or 'q' to quit, or select an SSH node:")
		node := ssh.Choose(config.CFG)
		if node == nil {
			fmt.Println("mysshw:: exiting...")
			return
		}

		// 检查是否按下q键
		select {
		case <-time.After(100 * time.Millisecond):
			// 没有按键输入，继续执行
		default:
			char, _, err := reader.ReadRune()
			if err != nil {
				// 检测到Ctrl+D (EOF)
				if err == io.EOF {
					fmt.Println("\nReceived Ctrl+D, exiting...")
					os.Exit(0)
				}
				continue
			}
			if char == 'q' || char == 'Q' {
				fmt.Println("Exiting...")
				os.Exit(0)
			}
		}

		client := ssh.NewClient(node)
		// 传递会话结束回调函数，在SSH会话结束后返回主界面
		client.Login(func() {
			fmt.Println("SSH session ended, returning to main menu...")
			// 清屏
			fmt.Print("\033[H\033[2J")
		})
		// 会话结束后继续循环，重新显示菜单
	}
}
