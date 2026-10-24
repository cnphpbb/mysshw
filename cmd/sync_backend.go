package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"mysshw/config"
	"mysshw/s3"
	"mysshw/webdav"
)

// SyncBackend 配置文件同步的统一接口
//
// 目前由 webdav.Client 和 s3.Client 实现。
// 抽取后, sync 命令主循环只需关心 "上传" / "下载" 两个动作,
// 不再需要为每个协议手写 wrapper (避免 uploadWebDAVConfig / uploadS3Config
// 这种 N×2 复制粘贴)。
//
// 采用字节级接口而非文件级: 上传前需要加密 (远程只存密文),
// 下载后需要解密再落地, 字节级最直接。
//
// SCP 协议有自己的传输层 (走 SSH 通道), 不走此接口。
type SyncBackend interface {
	UploadBytes(ctx context.Context, data []byte, remoteName string) error
	DownloadBytes(ctx context.Context, remoteName string) ([]byte, error)
}

// newSyncBackend 根据 sync 配置创建对应的 backend 实例
func newSyncBackend(ctx context.Context, syncCfg *config.SyncInfo) (SyncBackend, error) {
	switch syncCfg.Type {
	case "webdav":
		return webdav.NewClient(ctx, syncCfg)
	case "s3":
		return s3.NewClient(ctx, syncCfg)
	default:
		return nil, fmt.Errorf("unsupported sync backend type: %s", syncCfg.Type)
	}
}

// resolveRemoteFilename 根据本地路径 + 可选远程路径前缀推导上传文件名
//
//	本地:  /home/x/.mysshw.toml
//	远端:  ""               → "mysshw.toml"
//	远端:  "backups/abc.toml" → "abc.toml" (取远端路径的 basename)
func resolveRemoteFilename(localCfgPath, remotePath string) string {
	_, localName := filepath.Split(localCfgPath)
	if remotePath != "" {
		_, remoteName := filepath.Split(remotePath)
		if remoteName != "" {
			return remoteName
		}
	}
	return localName
}

// uploadConfigSync 通用上传流程 (WebDAV / S3 共用)
//
// opts.Encrypt=true 时先加密再上传, 远程只存密文
func uploadConfigSync(ctx context.Context, backend SyncBackend, localCfgPath, remotePath string, opts syncCryptoOptions) error {
	fmt.Println("Starting to upload local config to remote server...")

	filename := resolveRemoteFilename(localCfgPath, remotePath)

	data, err := config.LoadConfigBytes(localCfgPath)
	if err != nil {
		return fmt.Errorf("failed to load local config: %w", err)
	}

	data, err = encryptUploadPayload(data, opts)
	if err != nil {
		return err
	}

	if err := backend.UploadBytes(ctx, data, filename); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	fmt.Println("Successfully uploaded local config to remote server.")
	return nil
}

// downloadConfigSync 通用下载流程 (WebDAV / S3 共用)
//
// 远端是密文时解密后落盘明文 (--keep-encrypted 则保留密文)
func downloadConfigSync(ctx context.Context, backend SyncBackend, localCfgPath, remotePath string, opts syncCryptoOptions) error {
	fmt.Println("Starting to download config from remote server...")

	filename := resolveRemoteFilename(localCfgPath, remotePath)
	localPath, err := config.GetCfgPath(localCfgPath)
	if err != nil {
		return fmt.Errorf("resolve local config path: %w", err)
	}
	opts = preserveLocalEncryptionState(localPath, opts)

	data, err := backend.DownloadBytes(ctx, filename)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	data, kind, err := decryptDownloaded(data, opts)
	if err != nil {
		return err
	}

	if err := saveDownloadedConfig(localPath, data); err != nil {
		return err
	}
	reportPayloadKind(kind)

	fmt.Println("Successfully downloaded config from local.")
	return nil
}
