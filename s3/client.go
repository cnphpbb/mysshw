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
// 校验 remote_path 不以 '/' 开头 (S3 key 不应带前导 /, 避免与 ListObjects 前缀语义混淆)
func NewClient(ctx context.Context, cfg *config.SyncInfo) (*Client, error) {
	// 前置校验 remote_path: 必须在任何网络调用之前拒绝, 避免不必要的网络失败混淆错误
	if strings.HasPrefix(cfg.RemotePath, "/") {
		return nil, fmt.Errorf("S3 远程路径不应以 '/' 开头: %s", cfg.RemotePath)
	}

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

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// UploadFile 上传文件到S3服务器
func (c *Client) UploadFile(ctx context.Context, localPath string, remoteName string) error {
	// 构建远程文件路径
	remotePath := joinRemotePath(c.config.RemotePath, remoteName)

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
func (c *Client) DownloadFile(ctx context.Context, remoteName string, localPath string) error {
	// 构建远程文件路径
	remotePath := joinRemotePath(c.config.RemotePath, remoteName)

	// 检查文件是否存在
	_, err := c.client.StatObject(ctx, c.config.S3Config.BucketName, remotePath, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return fmt.Errorf("远程文件不存在: %s", remotePath)
		}
		return fmt.Errorf("检查远程文件失败: %w", err)
	}

	// 确保本地目录存在
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}

	// 下载文件
	if err := c.client.FGetObject(ctx, c.config.S3Config.BucketName, remotePath, localPath, minio.GetObjectOptions{}); err != nil {
		return fmt.Errorf("下载文件失败: %w", err)
	}

	return nil
}

// ListFiles 列出S3服务器上的文件
func (c *Client) ListFiles(ctx context.Context) ([]os.FileInfo, error) {
	// 构建前缀 (去除前导 '/', 保持与 UploadFile/DownloadFile 一致)
	prefix := strings.TrimPrefix(c.config.RemotePath, "/")

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
func (c *Client) DeleteFile(ctx context.Context, remoteName string) error {
	// 构建远程文件路径
	remotePath := joinRemotePath(c.config.RemotePath, remoteName)

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

// joinRemotePath 用 '/' 拼接 prefix + name (S3 key 始终用正斜杠, 跨平台一致)
func joinRemotePath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
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
