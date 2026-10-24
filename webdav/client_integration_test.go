package webdav

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/webdav"
)

// startWebDAVMock 启动一个内存 WebDAV server, 返回 base URL + cleanup 函数.
// 使用 golang.org/x/net/webdav (标准库) 实现 server, gowebdav 实现 client.
// 这是真实端到端集成测试, 覆盖 NewClient / UploadBytes / DownloadBytes / ListFiles / DeleteFile 全流程.
func startWebDAVMock(t *testing.T) (baseURL string, cleanup func()) {
	t.Helper()
	fs := webdav.NewMemFS()
	ls := webdav.NewMemLS()
	handler := &webdav.Handler{
		FileSystem: fs,
		LockSystem: ls,
	}
	server := httptest.NewServer(handler)
	return server.URL, server.Close
}

// makeTestSyncInfo 构造测试用的 SyncInfo (webdav 类型).
// remote_uri 留空 (由调用者通过 baseURL 设置), 调用前需设置 cfg.RemoteUri.
func makeTestSyncInfo(baseURL, remotePath string) *config.SyncInfo {
	return &config.SyncInfo{
		Type:       "webdav",
		RemoteUri:  strings.TrimPrefix(baseURL, "http://"),
		RemotePath: remotePath,
		WebDAVConfig: config.WebDAVConfig{
			Auth:     "Basic",
			Username: "testuser",
			Password: "testpass",
		},
	}
}

// writeTestFile 写入临时文件, 返回路径 + cleanup.
func writeTestFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	return p
}

// readTestFile 读取测试文件内容为字节.
func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestNewClient_LeadingSlashRejected(t *testing.T) {
	// 远程路径以 '/' 开头应被拒绝 (不依赖 server)
	baseURL, cleanup := startWebDAVMock(t)
	defer cleanup()

	cfg := makeTestSyncInfo(baseURL, "/bad/path")
	_, err := NewClient(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不应以 '/' 开头")
}

func TestNewClient_EmptyPath_OK(t *testing.T) {
	// 空远程路径 = 根目录, 应允许
	baseURL, cleanup := startWebDAVMock(t)
	defer cleanup()

	cfg := makeTestSyncInfo(baseURL, "")
	client, err := NewClient(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestWebDAV_RoundTrip_UploadDownloadListDelete(t *testing.T) {
	baseURL, cleanup := startWebDAVMock(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 准备测试文件
	localPath := writeTestFile(t, "mysshw.toml", "[nodes]\nname=\"web1\"\n")
	remoteName := "mysshw.toml"
	remotePath := "backup/mysshw.toml" // 不带前导 /

	// 1. NewClient - 自动创建 remotePath 目录
	cfg := makeTestSyncInfo(baseURL, remotePath)
	client, err := NewClient(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	// 2. UploadBytes - 上传
	err = client.UploadBytes(ctx, readTestFile(t, localPath), remoteName)
	require.NoError(t, err, "upload 应成功")

	// 3. ListFiles - 列出
	files, err := client.ListFiles(ctx)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "mysshw.toml", files[0].Name())

	// 4. DownloadBytes - 下载到内存 (再落到另一临时位置校验)
	downloaded, err := client.DownloadBytes(ctx, remoteName)
	require.NoError(t, err)
	assert.Equal(t, "[nodes]\nname=\"web1\"\n", string(downloaded))

	downloadDir := t.TempDir()
	downloadPath := filepath.Join(downloadDir, "downloaded.toml")
	require.NoError(t, os.WriteFile(downloadPath, downloaded, 0644))

	// 5. DeleteFile - 删除
	err = client.DeleteFile(ctx, remoteName)
	require.NoError(t, err)

	// 6. 删除后 ListFiles 应为空
	files, err = client.ListFiles(ctx)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestWebDAV_DownloadMissingFile_ReturnsError(t *testing.T) {
	baseURL, cleanup := startWebDAVMock(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := makeTestSyncInfo(baseURL, "nonexistent")
	client, err := NewClient(ctx, cfg)
	require.NoError(t, err)

	// 下载不存在的文件应返回错误
	_, err = client.DownloadBytes(ctx, "nope.toml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "远程文件不存在")
}

func TestWebDAV_DeleteMissingFile_ReturnsError(t *testing.T) {
	baseURL, cleanup := startWebDAVMock(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := makeTestSyncInfo(baseURL, "")
	client, err := NewClient(ctx, cfg)
	require.NoError(t, err)

	// 删除不存在的文件应返回错误
	err = client.DeleteFile(ctx, "nonexistent.toml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "远程文件不存在")
}

func TestWebDAV_UploadOverwrite(t *testing.T) {
	baseURL, cleanup := startWebDAVMock(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := makeTestSyncInfo(baseURL, "data")
	client, err := NewClient(ctx, cfg)
	require.NoError(t, err)

	// 第一次上传
	localPath1 := writeTestFile(t, "v1.toml", "v1")
	err = client.UploadBytes(ctx, readTestFile(t, localPath1), "config.toml")
	require.NoError(t, err)

	// 第二次上传覆盖
	localPath2 := writeTestFile(t, "v2.toml", "v2")
	err = client.UploadBytes(ctx, readTestFile(t, localPath2), "config.toml")
	require.NoError(t, err)

	// 验证下载的是 v2
	content, err := client.DownloadBytes(ctx, "config.toml")
	require.NoError(t, err)
	assert.Equal(t, "v2", string(content))
}

func TestWebDAV_ConnectFailure_InvalidHost(t *testing.T) {
	// 指向一个不存在的 host, Connect 应失败
	cfg := &config.SyncInfo{
		Type:       "webdav",
		RemoteUri:  "127.0.0.1:1", // 几乎一定没服务
		RemotePath: "test",
		WebDAVConfig: config.WebDAVConfig{
			Username: "x",
			Password: "y",
		},
	}
	// 使用短超时 ctx
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := NewClient(ctx, cfg)
	require.Error(t, err)
	assert.Nil(t, client)
}
