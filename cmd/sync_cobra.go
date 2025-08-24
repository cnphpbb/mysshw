package cmd

import (
	"bytes"
	"fmt"
	"log"
	"mysshw/auth"
	"mysshw/config"
	"mysshw/scp"
	"os"

	"github.com/spf13/cobra"
	crypto_ssh "golang.org/x/crypto/ssh"
)

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
