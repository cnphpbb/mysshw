package webdav

import (
	"context"
	"fmt"
	"os"

	"mysshw/config"

	"github.com/studio-b12/gowebdav"
)

// Client WebDAV客户端结构体
type Client struct {
	client *gowebdav.Client
	config *config.SyncInfo
}

// NewClient 创建WebDAV客户端
// 校验 remote_path 不以 '/' 开头 (WebDAV 路径是 server-relative, 前导 / 在某些服务器上会丢失)
func NewClient(ctx context.Context, cfg *config.SyncInfo) (*Client, error) {
	// 构建WebDAV服务器URL
	baseURL := fmt.Sprintf("http://%s", cfg.RemoteUri)
	if cfg.RemotePath != "" && cfg.RemotePath[0] == '/' {
		return nil, fmt.Errorf("WebDAV 远程路径不应以 '/' 开头: %s", cfg.RemotePath)
	}

	// 创建WebDAV客户端
	client := gowebdav.NewClient(baseURL, cfg.WebDAVConfig.Username, cfg.WebDAVConfig.Password)

	// 测试连接
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("连接WebDAV服务器失败: %w", err)
	}

	// 检查远程路径是否存在
	_, err := client.Stat(cfg.RemotePath)
	if gowebdav.IsErrNotFound(err) {
		// 如果路径不存在，则创建
		if err := client.MkdirAll(cfg.RemotePath, 0755); err != nil {
			return nil, fmt.Errorf("创建远程路径失败: %w", err)
		}
	}

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// UploadBytes 上传内存中的字节到WebDAV服务器
// 用于上传加密后的配置 (远程只存密文)
func (c *Client) UploadBytes(ctx context.Context, data []byte, remoteName string) error {
	// 构建远程文件路径
	remotePath := joinRemotePath(c.config.RemotePath, remoteName)

	// 上传文件
	if err := c.client.Write(remotePath, data, 0644); err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}

	return nil
}

// DownloadBytes 从WebDAV服务器下载文件内容到内存
func (c *Client) DownloadBytes(ctx context.Context, remoteName string) ([]byte, error) {
	// 构建远程文件路径
	remotePath := joinRemotePath(c.config.RemotePath, remoteName)

	// 检查文件是否存在
	_, err := c.client.Stat(remotePath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, fmt.Errorf("远程文件不存在: %s", remotePath)
		}
		return nil, fmt.Errorf("检查远程文件失败: %w", err)
	}

	// 读取远程文件内容
	content, err := c.client.Read(remotePath)
	if err != nil {
		return nil, fmt.Errorf("读取远程文件失败: %w", err)
	}

	return content, nil
}

// ListFiles 列出WebDAV服务器上的文件
func (c *Client) ListFiles(ctx context.Context) ([]os.FileInfo, error) {
	files, err := c.client.ReadDir(c.config.RemotePath)
	if err != nil {
		return nil, fmt.Errorf("列出远程文件失败: %w", err)
	}

	return files, nil
}

// DeleteFile 删除WebDAV服务器上的文件
func (c *Client) DeleteFile(ctx context.Context, remoteName string) error {
	// 构建远程文件路径
	remotePath := joinRemotePath(c.config.RemotePath, remoteName)

	// 检查文件是否存在
	_, err := c.client.Stat(remotePath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return fmt.Errorf("远程文件不存在: %s", remotePath)
		}
		return fmt.Errorf("检查远程文件失败: %w", err)
	}

	// 删除文件
	if err := c.client.Remove(remotePath); err != nil {
		return fmt.Errorf("删除远程文件失败: %w", err)
	}

	return nil
}

// joinRemotePath 用 '/' 拼接 prefix + name (WebDAV 路径始终用正斜杠)
func joinRemotePath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}
