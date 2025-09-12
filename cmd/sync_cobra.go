package cmd

import (
	"bytes"
	"fmt"
	"log"
	"mysshw/auth"
	"mysshw/config"
	"mysshw/s3"
	"mysshw/scp"
	"mysshw/webdav"
	"os"
	"path/filepath"

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
			// 准备 SSH 配置
			sshCfg := createSSHConfig(syncCfg)

			// 创建 SCP 客户端并连接
			client, err := createSCPclient(syncCfg.RemoteUri, sshCfg)
			if err != nil {
				fmt.Printf("Failed to create SCP client: %s\n", err)
				os.Exit(1)
			}
			defer client.Close()

			if upload {
				if err := uploadConfig(client, config.CFG_PATH, syncCfg.RemotePath); err != nil {
					fmt.Printf("Upload failed: %s\n", err)
					os.Exit(1)
				}
			} else {
				if err := downloadConfig(client, config.CFG_PATH, syncCfg.RemotePath); err != nil {
					fmt.Printf("Download failed: %s\n", err)
					os.Exit(1)
				}
			}
		case "webdav":
			// 创建 WebDAV 客户端并连接
			client, err := webdav.NewClient(&syncCfg)
			if err != nil {
				fmt.Printf("Failed to create WebDAV client: %s\n", err)
				os.Exit(1)
			}

			if upload {
				if err := uploadWebDAVConfig(client, config.CFG_PATH, syncCfg.RemotePath); err != nil {
					fmt.Printf("Upload failed: %s\n", err)
					os.Exit(1)
				}
			} else {
				if err := downloadWebDAVConfig(client, config.CFG_PATH, syncCfg.RemotePath); err != nil {
					fmt.Printf("Download failed: %s\n", err)
					os.Exit(1)
				}
			}
		case "s3":
			// 创建 S3 客户端并连接
			client, err := s3.NewClient(&syncCfg)
			if err != nil {
				fmt.Printf("Failed to create S3 client: %s\n", err)
				os.Exit(1)
			}

			if upload {
				if err := uploadS3Config(client, config.CFG_PATH, syncCfg.RemotePath); err != nil {
					fmt.Printf("Upload failed: %s\n", err)
					os.Exit(1)
				}
			} else {
				if err := downloadS3Config(client, config.CFG_PATH, syncCfg.RemotePath); err != nil {
					fmt.Printf("Download failed: %s\n", err)
					os.Exit(1)
				}
			}
		default:
			fmt.Printf("Unsupported sync type: %s\n", syncCfg.Type)
			os.Exit(1)
		}

		fmt.Println("Sync operation completed successfully.")
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
		User: syncCfg.SCPConfig.Username,
		Auth: []crypto_ssh.AuthMethod{
			auth.PasswordKey(syncCfg.SCPConfig.Username, syncCfg.SCPConfig.Password),
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

// uploadWebDAVConfig 上传本地配置到WebDAV服务器
func uploadWebDAVConfig(client *webdav.Client, localCfgPath, remotePath string) error {
	fmt.Println("Starting to upload local config to WebDAV server...")

	// 获取配置文件名
	_, filename := filepath.Split(localCfgPath)

	// 如果提供了远程路径，则使用该路径中的文件名
	if remotePath != "" {
		_, remoteFilename := filepath.Split(remotePath)
		if remoteFilename != "" {
			filename = remoteFilename
		}
	}

	// 上传配置文件
	if err := client.UploadFile(localCfgPath, filename); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	fmt.Println("Successfully uploaded local config to WebDAV server.")
	return nil
}

// downloadWebDAVConfig 从WebDAV服务器下载配置到本地
func downloadWebDAVConfig(client *webdav.Client, localCfgPath, remotePath string) error {
	fmt.Println("Starting to download config from WebDAV server...")

	// 获取配置文件名
	_, filename := filepath.Split(localCfgPath)

	// 如果提供了远程路径，则使用该路径中的文件名
	if remotePath != "" {
		_, remoteFilename := filepath.Split(remotePath)
		if remoteFilename != "" {
			filename = remoteFilename
		}
	}

	// 获取本地配置文件路径
	localPath, _ := config.GetCfgPath(localCfgPath)

	// 下载配置文件
	if err := client.DownloadFile(filename, localPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	fmt.Println("Successfully downloaded config from WebDAV server.")
	return nil
}

// uploadS3Config 上传本地配置到S3服务器
func uploadS3Config(client *s3.Client, localCfgPath, remotePath string) error {
	fmt.Println("Starting to upload local config to S3 server...")

	// 获取配置文件名
	_, filename := filepath.Split(localCfgPath)

	// 如果提供了远程路径，则使用该路径中的文件名
	if remotePath != "" {
		_, remoteFilename := filepath.Split(remotePath)
		if remoteFilename != "" {
			filename = remoteFilename
		}
	}

	// 上传配置文件
	if err := client.UploadFile(localCfgPath, filename); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	fmt.Println("Successfully uploaded local config to S3 server.")
	return nil
}

// downloadS3Config 从S3服务器下载配置到本地
func downloadS3Config(client *s3.Client, localCfgPath, remotePath string) error {
	fmt.Println("Starting to download config from S3 server...")

	// 获取配置文件名
	_, filename := filepath.Split(localCfgPath)

	// 如果提供了远程路径，则使用该路径中的文件名
	if remotePath != "" {
		_, remoteFilename := filepath.Split(remotePath)
		if remoteFilename != "" {
			filename = remoteFilename
		}
	}

	// 获取本地配置文件路径
	localPath, _ := config.GetCfgPath(localCfgPath)

	// 下载配置文件
	if err := client.DownloadFile(filename, localPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	fmt.Println("Successfully downloaded config from S3 server.")
	return nil
}
