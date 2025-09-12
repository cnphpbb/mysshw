package webdav

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mysshw/config"

	"github.com/studio-b12/gowebdav"
)

// Client WebDAV客户端结构体
type Client struct {
	client *gowebdav.Client
	config *config.SyncInfo
}

// NewClient 创建WebDAV客户端
func NewClient(cfg *config.SyncInfo) (*Client, error) {
	// 构建WebDAV服务器URL
	baseURL := fmt.Sprintf("http://%s", cfg.RemoteUri)
	if !filepath.IsAbs(cfg.RemotePath) {
		return nil, fmt.Errorf("WebDAV远程路径必须是绝对路径: %s", cfg.RemotePath)
	}

	// 创建WebDAV客户端
	client := gowebdav.NewClient(baseURL, cfg.WebDAVConfig.Username, cfg.WebDAVConfig.Password)

	// 测试连接
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("连接WebDAV服务器失败: %w", err)
	}

	// 检查远程路径是否存在
	_, err := client.Stat(cfg.RemotePath)
	if os.IsNotExist(err) {
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

// UploadFile 上传文件到WebDAV服务器
func (c *Client) UploadFile(localPath string, remoteName string) error {
	// 打开本地文件
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer file.Close()

	// 读取文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("读取文件内容失败: %w", err)
	}

	// 构建远程文件路径
	remotePath := filepath.Join(c.config.RemotePath, remoteName)

	// 上传文件
	if err := c.client.Write(remotePath, content, 0644); err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}

	return nil
}

// DownloadFile 从WebDAV服务器下载文件
func (c *Client) DownloadFile(remoteName string, localPath string) error {
	// 构建远程文件路径
	remotePath := filepath.Join(c.config.RemotePath, remoteName)

	// 检查文件是否存在
	_, err := c.client.Stat(remotePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("远程文件不存在: %s", remotePath)
		}
		return fmt.Errorf("检查远程文件失败: %w", err)
	}

	// 读取远程文件内容
	content, err := c.client.Read(remotePath)
	if err != nil {
		return fmt.Errorf("读取远程文件失败: %w", err)
	}

	// 确保本地目录存在
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}

	// 写入本地文件
	if err := os.WriteFile(localPath, content, 0644); err != nil {
		return fmt.Errorf("写入本地文件失败: %w", err)
	}

	return nil
}

// ListFiles 列出WebDAV服务器上的文件
func (c *Client) ListFiles() ([]os.FileInfo, error) {
	files, err := c.client.ReadDir(c.config.RemotePath)
	if err != nil {
		return nil, fmt.Errorf("列出远程文件失败: %w", err)
	}

	return files, nil
}

// DeleteFile 删除WebDAV服务器上的文件
func (c *Client) DeleteFile(remoteName string) error {
	// 构建远程文件路径
	remotePath := filepath.Join(c.config.RemotePath, remoteName)

	// 检查文件是否存在
	_, err := c.client.Stat(remotePath)
	if err != nil {
		if os.IsNotExist(err) {
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

// SyncConfig 同步配置文件到WebDAV服务器
func SyncConfig(cfg *config.Configs) error {
	// 只有当sync类型为webdav时才执行同步
	if cfg.SyncCfg.Type != "webdav" {
		return nil
	}

	// 创建WebDAV客户端
	client, err := NewClient(&cfg.SyncCfg)
	if err != nil {
		return fmt.Errorf("创建WebDAV客户端失败: %w", err)
	}

	// 同步配置文件
	fileName := filepath.Base(cfg.CfgDir)
	if err := client.UploadFile(cfg.CfgDir, fileName); err != nil {
		return fmt.Errorf("同步配置文件失败: %w", err)
	}

	return nil
}

// GetConfig 从WebDAV服务器获取配置文件
func GetConfig(cfg *config.SyncInfo, localPath string) error {
	// 创建WebDAV客户端
	client, err := NewClient(cfg)
	if err != nil {
		return fmt.Errorf("创建WebDAV客户端失败: %w", err)
	}

	// 获取配置文件
	fileName := filepath.Base(localPath)
	if err := client.DownloadFile(fileName, localPath); err != nil {
		return fmt.Errorf("获取配置文件失败: %w", err)
	}

	return nil
}
