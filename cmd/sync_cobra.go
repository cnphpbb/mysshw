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
		opts := buildSyncCryptoOptions(cmd, syncCfg)

		// 预检: 本地配置是否已配置加密参数 (在连远程之前 fail-fast)
		if err := checkSyncEncryptionConfigured(syncCfg, opts, upload); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		switch syncCfg.Type {
		case "scp":
			runScpSync(syncCfg, upload, config.CFG_PATH, opts)
		case "webdav", "s3":
			runBackendSync(syncCfg, upload, config.CFG_PATH, opts)
		default:
			fmt.Printf("Unsupported sync type: %s\n", syncCfg.Type)
			os.Exit(1)
		}

		fmt.Println("Sync operation completed successfully.")
	},
}

// buildSyncCryptoOptions 组装同步加解密选项
//
// 优先级: CLI flag (显式指定时) > 配置文件 [sync] 字段 > 内置默认值
// 默认上传加密 (安全默认: 远程永远不落明文配置)
func buildSyncCryptoOptions(cmd *cobra.Command, syncCfg config.SyncInfo) syncCryptoOptions {
	opts := syncCryptoOptions{
		PassphraseCfg: syncCfg.Passphrase,
		PassphraseEnv: DefaultSyncPassphraseEnv,
	}

	// 上传是否加密
	if cmd.Flags().Changed("encrypt") {
		opts.Encrypt, _ = cmd.Flags().GetBool("encrypt")
	} else if syncCfg.Encrypt != nil {
		opts.Encrypt = *syncCfg.Encrypt
	} else {
		opts.Encrypt = true
	}

	// 下载后是否保留密文
	if cmd.Flags().Changed("keep-encrypted") {
		opts.KeepEncrypted, _ = cmd.Flags().GetBool("keep-encrypted")
		opts.KeepEncryptedSet = true
	}

	// 口令环境变量名
	if cmd.Flags().Changed("passphrase-env") {
		if v, _ := cmd.Flags().GetString("passphrase-env"); v != "" {
			opts.PassphraseEnv = v
		}
	}

	return opts
}

// runScpSync 处理 SCP 协议的同步 (走 SSH 通道, 不走 SyncBackend 接口)
func runScpSync(syncCfg config.SyncInfo, upload bool, cfgPath string, opts syncCryptoOptions) {
	sshCfg := createSSHConfig(syncCfg)

	client, err := createSCPclient(syncCfg.RemoteUri, sshCfg)
	if err != nil {
		fmt.Printf("Failed to create SCP client: %s\n", err)
		os.Exit(1)
	}
	defer client.Close()

	if upload {
		if err := uploadConfig(client, cfgPath, syncCfg.RemotePath, opts); err != nil {
			fmt.Printf("Upload failed: %s\n", err)
			os.Exit(1)
		}
	} else {
		if err := downloadConfig(client, cfgPath, syncCfg.RemotePath, opts); err != nil {
			fmt.Printf("Download failed: %s\n", err)
			os.Exit(1)
		}
	}
}

// runBackendSync 处理 WebDAV / S3 协议的同步 (走 SyncBackend 统一接口)
func runBackendSync(syncCfg config.SyncInfo, upload bool, cfgPath string, opts syncCryptoOptions) {
	ctx := context.Background()
	backend, err := newSyncBackend(ctx, &syncCfg)
	if err != nil {
		fmt.Printf("Failed to create %s client: %s\n", syncCfg.Type, err)
		os.Exit(1)
	}

	if upload {
		if err := uploadConfigSync(ctx, backend, cfgPath, syncCfg.RemotePath, opts); err != nil {
			fmt.Printf("Upload failed: %s\n", err)
			os.Exit(1)
		}
	} else {
		if err := downloadConfigSync(ctx, backend, cfgPath, syncCfg.RemotePath, opts); err != nil {
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
//
// opts.Encrypt=true 时先加密再上传, 远程只存密文 (防远程被入侵后备份被拖走)
func uploadConfig(client scp.Client, localCfgPath, remotePath string, opts syncCryptoOptions) error {
	fmt.Println("Starting to upload local config to remote server...")

	cfgBytes, err := config.LoadConfigBytes(localCfgPath)
	if err != nil {
		return fmt.Errorf("failed to load local config: %w", err)
	}

	cfgBytes, err = encryptUploadPayload(cfgBytes, opts)
	if err != nil {
		return err
	}

	if err := client.CopyFile(bytes.NewReader(cfgBytes), remotePath, "0644"); err != nil {
		return fmt.Errorf("error while copying file: %w", err)
	}

	fmt.Println("Successfully uploaded local config to remote server.")
	return nil
}

// downloadConfig SCP 协议专用: 从远程服务器下载配置到本地
//
// 远端是密文时解密后落盘明文 (--keep-encrypted 则保留密文)
// 解密失败不覆盖本地已有配置 (先落临时文件, 解密成功才替换)
func downloadConfig(client scp.Client, localCfgPath, remotePath string, opts syncCryptoOptions) error {
	fmt.Println("Starting to download remote config to local...")

	localPath, err := config.GetCfgPath(localCfgPath)
	if err != nil {
		return fmt.Errorf("resolve local config path: %w", err)
	}
	opts = preserveLocalEncryptionState(localPath, opts)

	// 先落到临时文件: 解密失败时不破坏本地已有配置
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("create local dir failed: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(localPath), ".mysshw-sync-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file failed: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := client.CopyFromRemote(tmp, remotePath); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to copy from remote: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file failed: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("read temp file failed: %w", err)
	}

	data, kind, err := decryptDownloaded(data, opts)
	if err != nil {
		return err
	}

	if err := saveDownloadedConfig(localPath, data); err != nil {
		return err
	}
	reportPayloadKind(kind)

	fmt.Println("Successfully downloaded remote config to local.")
	return nil
}
