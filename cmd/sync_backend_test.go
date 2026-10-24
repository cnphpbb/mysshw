package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mysshw/s3"
	"mysshw/webdav"

	"github.com/stretchr/testify/assert"
)

// fakeBackend 用于测试 SyncBackend 接口的内存实现
type fakeBackend struct {
	uploads    []string // 上传过的 (remoteName) 列表
	uploaded   []byte   // 最近一次上传的字节
	downloads  []string // 下载过的 (remoteName) 列表
	remoteData []byte   // 下载时返回的字节
	uploadErr  error
	downErr    error
}

func (f *fakeBackend) UploadBytes(_ context.Context, data []byte, remoteName string) error {
	f.uploads = append(f.uploads, remoteName)
	f.uploaded = append([]byte(nil), data...)
	return f.uploadErr
}

func (f *fakeBackend) DownloadBytes(_ context.Context, remoteName string) ([]byte, error) {
	f.downloads = append(f.downloads, remoteName)
	if f.downErr != nil {
		return nil, f.downErr
	}
	return f.remoteData, nil
}

// 编译期断言: webdav.Client / s3.Client 必须满足 SyncBackend 接口
// 如接口不兼容, 编译会报 "cannot use (...) as SyncBackend" 错误
var (
	_ SyncBackend = (*webdav.Client)(nil)
	_ SyncBackend = (*s3.Client)(nil)
)

func TestResolveRemoteFilename(t *testing.T) {
	tests := []struct {
		name     string
		local    string
		remote   string
		expected string
	}{
		{"本地名 (无远程)", "/home/x/.mysshw.toml", "", ".mysshw.toml"},
		{"远程覆盖 (basename 优先)", "/home/x/.mysshw.toml", "backups/cfg.toml", "cfg.toml"},
		{"远程目录 (无文件名 → 退回本地)", "/home/x/.mysshw.toml", "backups/", ".mysshw.toml"},
		{"相对路径", "./config.toml", "", "config.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveRemoteFilename(tt.local, tt.remote))
		})
	}
}

// writeTempSyncConfig 写一个临时配置文件, 返回路径
func writeTempSyncConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mysshw.toml")
	assert.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

func TestUploadConfigSync_Plaintext(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}
	cfgPath := writeTempSyncConfig(t, "hello = \"world\"\n")

	err := uploadConfigSync(ctx, backend, cfgPath, "", syncCryptoOptions{Encrypt: false})
	assert.NoError(t, err)
	assert.Len(t, backend.uploads, 1)
	assert.Equal(t, "hello = \"world\"\n", string(backend.uploaded))
}

func TestUploadConfigSync_WithRemoteOverride(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}
	cfgPath := writeTempSyncConfig(t, "hello = \"world\"\n")

	err := uploadConfigSync(ctx, backend, cfgPath, "remote/custom-name.toml", syncCryptoOptions{Encrypt: false})
	assert.NoError(t, err)
	assert.Len(t, backend.uploads, 1)
	assert.Equal(t, "custom-name.toml", backend.uploads[0])
}

func TestUploadConfigSync_PropagatesError(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{uploadErr: errors.New("network down")}
	cfgPath := writeTempSyncConfig(t, "hello = \"world\"\n")

	err := uploadConfigSync(ctx, backend, cfgPath, "", syncCryptoOptions{Encrypt: false})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network down")
	assert.Contains(t, err.Error(), "upload failed")
}

func TestDownloadConfigSync(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{remoteData: []byte("hello = \"world\"\n")}
	localPath := filepath.Join(t.TempDir(), "out.toml")

	err := downloadConfigSync(ctx, backend, localPath, "", syncCryptoOptions{})
	assert.NoError(t, err)
	assert.Len(t, backend.downloads, 1)

	data, err := os.ReadFile(localPath)
	assert.NoError(t, err)
	assert.Equal(t, "hello = \"world\"\n", string(data))
}

func TestDownloadConfigSync_PropagatesError(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{downErr: errors.New("404 not found")}

	tmpDir := t.TempDir()
	err := downloadConfigSync(ctx, backend, filepath.Join(tmpDir, "out.toml"), "", syncCryptoOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404 not found")
}
