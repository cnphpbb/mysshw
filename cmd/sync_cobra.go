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
			log.Printf("Config path changed to %s", cfgPath)
			config.CFG_PATH = cfgPath
		}
		log.Printf("Using config path: %s", config.CFG_PATH)

		// 加载配置
		if err := loadConfig(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// 准备 SSH 配置
		syncCfg := config.CFG.SyncCfg
		sshCfg := createSSHConfig(syncCfg)

		// 创建 SCP 客户端并连接
		client, err := createSCPclient(syncCfg.RemoteUri, sshCfg)
		if err != nil {
			fmt.Printf("Failed to create SCP client: %s\n", err)
			os.Exit(1)
		}
		defer client.Close()

		// 处理上传或下载
		upload, _ := cmd.Flags().GetBool("upload")
		down, _ := cmd.Flags().GetBool("down")

		if upload {
			if err := uploadConfig(client, config.CFG_PATH, syncCfg.RemotePath); err != nil {
				fmt.Printf("Upload failed: %s\n", err)
				os.Exit(1)
			}
			return
		}

		if down {
			if err := downloadConfig(client, config.CFG_PATH, syncCfg.RemotePath); err != nil {
				fmt.Printf("Download failed: %s\n", err)
				os.Exit(1)
			}
			return
		}

		fmt.Println("Please specify either --upload or --down flag")
	},
}

// loadConfig 加载配置文件
func loadConfig() error {
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return nil
}

// createSSHConfig 创建 SSH 客户端配置
func createSSHConfig(syncCfg config.SyncInfo) *crypto_ssh.ClientConfig {
	return &crypto_ssh.ClientConfig{
		User: syncCfg.UserName,
		Auth: []crypto_ssh.AuthMethod{
			auth.PasswordKey(syncCfg.UserName, syncCfg.Password),
		},
		HostKeyCallback: crypto_ssh.InsecureIgnoreHostKey(),
	}
}

// createSCPclient 创建并连接 SCP 客户端
func createSCPclient(remoteURI string, sshCfg *crypto_ssh.ClientConfig) (scp.Client, error) {
	client := scp.NewClient(remoteURI, sshCfg)
	if err := client.Connect(); err != nil {
		return client, fmt.Errorf("couldn't establish connection to remote server: %w", err)
	}
	return client, nil
}

// uploadConfig 上传本地配置到远程服务器
func uploadConfig(client scp.Client, localCfgPath, remotePath string) error {
	fmt.Println("Starting to upload local config to remote server...")

	cfgBytes, err := config.LoadConfigBytes(localCfgPath)
	if err != nil {
		return fmt.Errorf("failed to load local config: %w", err)
	}

	if err := client.CopyFile(bytes.NewReader(cfgBytes), remotePath, "0644"); err != nil {
		return fmt.Errorf("error while copying file: %w", err)
	}

	fmt.Println("Successfully uploaded local config to remote server.")
	return nil
}

// downloadConfig 从远程服务器下载配置到本地
func downloadConfig(client scp.Client, localCfgPath, remotePath string) error {
	fmt.Println("Starting to download remote config to local...")

	localPath, _ := config.GetCfgPath(localCfgPath)
	f, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("couldn't open output file: %w", err)
	}
	defer f.Close()

	if err := client.CopyFromRemote(f, remotePath); err != nil {
		return fmt.Errorf("failed to copy from remote: %w", err)
	}

	fmt.Println("Successfully downloaded remote config to local.")
	return nil
}
