package s3

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mysshw/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client S3客户端结构体
type Client struct {
	client *minio.Client
	config *config.SyncInfo
}

// NewClient 创建S3客户端
func NewClient(cfg *config.SyncInfo) (*Client, error) {
	// 使用context
	ctx := context.Background()

	// 获取endpoint，如果为空则使用remote_uri
	endpoint := cfg.S3Config.Endpoint
	if endpoint == "" {
		endpoint = cfg.RemoteUri
	}

	// 判断是否使用SSL（通过endpoint前缀判断）
	useSSL := false
	if len(endpoint) > 8 && endpoint[:8] == "https://" {
		useSSL = true
		endpoint = endpoint[8:]
	} else if len(endpoint) > 7 && endpoint[:7] == "http://" {
		useSSL = false
		endpoint = endpoint[7:]
	}

	// 创建S3客户端
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3Config.AccessKey, cfg.S3Config.SecretKey, ""),
		Secure: useSSL,
		Region: cfg.S3Config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("创建S3客户端失败: %w", err)
	}

	// 检查存储桶是否存在
	exists, err := client.BucketExists(ctx, cfg.S3Config.BucketName)
	if err != nil {
		return nil, fmt.Errorf("检查存储桶失败: %w", err)
	}

	// 如果存储桶不存在，创建存储桶
	if !exists {
		if err := client.MakeBucket(ctx, cfg.S3Config.BucketName, minio.MakeBucketOptions{
			Region: cfg.S3Config.Region,
		}); err != nil {
			return nil, fmt.Errorf("创建存储桶失败: %w", err)
		}
	}

	// 检查远程路径是否存在（S3是对象存储，这里只是验证格式）
	if !filepath.IsAbs(cfg.RemotePath) {
		return nil, fmt.Errorf("S3远程路径必须是绝对路径: %s", cfg.RemotePath)
	}

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// UploadFile 上传文件到S3服务器
func (c *Client) UploadFile(localPath string, remoteName string) error {
	// 使用context
	ctx := context.Background()

	// 构建远程文件路径
	remotePath := filepath.Join(c.config.RemotePath, remoteName)

	// 获取文件大小
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	// 上传文件
	uploadInfo, err := c.client.FPutObject(ctx, c.config.S3Config.BucketName, remotePath, localPath, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}

	// 验证上传大小
	if uploadInfo.Size != fileInfo.Size() {
		return fmt.Errorf("上传文件大小不匹配，预期: %d, 实际: %d", fileInfo.Size(), uploadInfo.Size)
	}

	return nil
}

// DownloadFile 从S3服务器下载文件
func (c *Client) DownloadFile(remoteName string, localPath string) error {
	// 使用context
	ctx := context.Background()

	// 构建远程文件路径
	remotePath := filepath.Join(c.config.RemotePath, remoteName)

	// 检查文件是否存在
	_, err := c.client.StatObject(ctx, c.config.S3Config.BucketName, remotePath, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return fmt.Errorf("远程文件不存在: %s", remotePath)
		}
		return fmt.Errorf("检查远程文件失败: %w", err)
	}

	// 确保本地目录存在
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}

	// 下载文件
	if err := c.client.FGetObject(ctx, c.config.S3Config.BucketName, remotePath, localPath, minio.GetObjectOptions{}); err != nil {
		return fmt.Errorf("下载文件失败: %w", err)
	}

	return nil
}

// ListFiles 列出S3服务器上的文件
func (c *Client) ListFiles() ([]os.FileInfo, error) {
	// 使用context
	ctx := context.Background()

	// 构建前缀
	prefix := c.config.RemotePath
	if !filepath.IsAbs(prefix) {
		return nil, fmt.Errorf("S3远程路径必须是绝对路径: %s", prefix)
	}

	// 去除路径前面的斜杠
	if len(prefix) > 0 && prefix[0] == '/' {
		prefix = prefix[1:]
	}

	// 初始化结果列表
	var fileInfos []os.FileInfo

	// 列出对象
	objectCh := c.client.ListObjects(ctx, c.config.S3Config.BucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
	})

	// 收集结果
	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("列出文件失败: %w", object.Err)
		}

		// 创建一个模拟的os.FileInfo
		fileInfo := &minioFileInfo{
			name:    object.Key[strings.LastIndex(object.Key, "/")+1:],
			size:    object.Size,
			modTime: object.LastModified,
		}

		fileInfos = append(fileInfos, fileInfo)
	}

	return fileInfos, nil
}

// DeleteFile 删除S3服务器上的文件
func (c *Client) DeleteFile(remoteName string) error {
	// 使用context
	ctx := context.Background()

	// 构建远程文件路径
	remotePath := filepath.Join(c.config.RemotePath, remoteName)

	// 检查文件是否存在
	_, err := c.client.StatObject(ctx, c.config.S3Config.BucketName, remotePath, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return fmt.Errorf("远程文件不存在: %s", remotePath)
		}
		return fmt.Errorf("检查远程文件失败: %w", err)
	}

	// 删除文件
	if err := c.client.RemoveObject(ctx, c.config.S3Config.BucketName, remotePath, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("删除文件失败: %w", err)
	}

	return nil
}

// SyncConfig 同步配置文件到S3服务器
func SyncConfig(cfg *config.Configs) error {
	// 只有当sync类型为s3时才执行同步
	if cfg.SyncCfg.Type != "s3" {
		return nil
	}

	// 创建S3客户端
	client, err := NewClient(&cfg.SyncCfg)
	if err != nil {
		return fmt.Errorf("创建S3客户端失败: %w", err)
	}

	// 同步配置文件
	fileName := filepath.Base(cfg.CfgDir)
	if err := client.UploadFile(cfg.CfgDir, fileName); err != nil {
		return fmt.Errorf("同步配置文件失败: %w", err)
	}

	return nil
}

// GetConfig 从S3服务器获取配置文件
func GetConfig(cfg *config.SyncInfo, localPath string) error {
	// 只有当sync类型为s3时才执行获取
	if cfg.Type != "s3" {
		return nil
	}

	// 创建S3客户端
	client, err := NewClient(cfg)
	if err != nil {
		return fmt.Errorf("创建S3客户端失败: %w", err)
	}

	// 获取配置文件
	fileName := filepath.Base(localPath)
	if err := client.DownloadFile(fileName, localPath); err != nil {
		return fmt.Errorf("获取配置文件失败: %w", err)
	}

	return nil
}

// minioFileInfo 模拟os.FileInfo接口的结构体
// 用于ListFiles函数返回的结果

type minioFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (m *minioFileInfo) Name() string {
	return m.name
}

func (m *minioFileInfo) Size() int64 {
	return m.size
}

func (m *minioFileInfo) Mode() os.FileMode {
	return 0644 // 默认权限
}

func (m *minioFileInfo) ModTime() time.Time {
	return m.modTime
}

func (m *minioFileInfo) IsDir() bool {
	return false // S3对象存储中，这里我们只返回文件
}

func (m *minioFileInfo) Sys() interface{} {
	return nil
}