package cmd

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"mysshw/config"
	"mysshw/scp"
	"mysshw/ssh"
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

		// 获取同步配置
		syncCfg := config.CFG.SyncCfg

		// 处理上传或下载
		upload, _ := cmd.Flags().GetBool("upload")
		down, _ := cmd.Flags().GetBool("down")

		if !upload && !down {
			fmt.Println("Please specify either --upload or --down flag")
			os.Exit(1)
		}

		// 根据同步类型选择不同的客户端
		switch syncCfg.Type {
		case "scp":
			runScpSync(syncCfg, upload, config.CFG_PATH)
		case "webdav", "s3":
			runBackendSync(syncCfg, upload, config.CFG_PATH)
		default:
			fmt.Printf("Unsupported sync type: %s\n", syncCfg.Type)
			os.Exit(1)
		}

		fmt.Println("Sync operation completed successfully.")
	},
}

// runScpSync 处理 SCP 协议的同步 (走 SSH 通道, 不走 SyncBackend 接口)
func runScpSync(syncCfg config.SyncInfo, upload bool, cfgPath string) {
	sshCfg := createSSHConfig(syncCfg)

	client, err := createSCPclient(syncCfg.RemoteUri, sshCfg)
	if err != nil {
		fmt.Printf("Failed to create SCP client: %s\n", err)
		os.Exit(1)
	}
	defer client.Close()

	if upload {
		if err := uploadConfig(client, cfgPath, syncCfg.RemotePath); err != nil {
			fmt.Printf("Upload failed: %s\n", err)
			os.Exit(1)
		}
	} else {
		if err := downloadConfig(client, cfgPath, syncCfg.RemotePath); err != nil {
			fmt.Printf("Download failed: %s\n", err)
			os.Exit(1)
		}
	}
}

// runBackendSync 处理 WebDAV / S3 协议的同步 (走 SyncBackend 统一接口)
func runBackendSync(syncCfg config.SyncInfo, upload bool, cfgPath string) {
	ctx := context.Background()
	backend, err := newSyncBackend(ctx, &syncCfg)
	if err != nil {
		fmt.Printf("Failed to create %s client: %s\n", syncCfg.Type, err)
		os.Exit(1)
	}

	if upload {
		if err := uploadConfigSync(ctx, backend, cfgPath, syncCfg.RemotePath); err != nil {
			fmt.Printf("Upload failed: %s\n", err)
			os.Exit(1)
		}
	} else {
		if err := downloadConfigSync(ctx, backend, cfgPath, syncCfg.RemotePath); err != nil {
			fmt.Printf("Download failed: %s\n", err)
			os.Exit(1)
		}
	}
}

// loadConfig 加载配置文件
func loadConfig() error {
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return nil
}

// createSSHConfig 创建 SSH 客户端配置
// 密码支持 ${ENV_VAR} 引用 (与 ssh/client.go 一致)
func createSSHConfig(syncCfg config.SyncInfo) *crypto_ssh.ClientConfig {
	resolvedPwd := ssh.ResolveEnvRef(syncCfg.SCPConfig.Password)
	authMethods := []crypto_ssh.AuthMethod{}
	if resolvedPwd != "" {
		authMethods = append(authMethods, crypto_ssh.Password(resolvedPwd))
	}
	return &crypto_ssh.ClientConfig{
		User:            syncCfg.SCPConfig.Username,
		Auth:            authMethods,
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

// uploadConfig SCP 协议专用: 上传本地配置到远程服务器
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

// downloadConfig SCP 协议专用: 从远程服务器下载配置到本地
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
