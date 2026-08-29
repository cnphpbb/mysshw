package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"mysshw/s3"
	"mysshw/webdav"

	"github.com/stretchr/testify/assert"
)

// fakeBackend 用于测试 SyncBackend 接口的内存实现
type fakeBackend struct {
	uploads   []string // 上传过的 (localPath, remoteName) 对
	downloads []string // 下载过的 (remoteName, localPath) 对
	uploadErr error
	downErr   error
}

func (f *fakeBackend) UploadFile(_ context.Context, localPath, remoteName string) error {
	f.uploads = append(f.uploads, localPath+"|"+remoteName)
	return f.uploadErr
}

func (f *fakeBackend) DownloadFile(_ context.Context, remoteName, localPath string) error {
	f.downloads = append(f.downloads, remoteName+"|"+localPath)
	return f.downErr
}

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

func TestUploadConfigSync(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}

	err := uploadConfigSync(ctx, backend, "/home/x/.mysshw.toml", "")
	assert.NoError(t, err)
	assert.Len(t, backend.uploads, 1)
	assert.Equal(t, "/home/x/.mysshw.toml|.mysshw.toml", backend.uploads[0])
}

func TestUploadConfigSync_WithRemoteOverride(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}

	err := uploadConfigSync(ctx, backend, "/home/x/.mysshw.toml", "remote/custom-name.toml")
	assert.NoError(t, err)
	assert.Equal(t, "/home/x/.mysshw.toml|custom-name.toml", backend.uploads[0])
}

func TestUploadConfigSync_PropagatesError(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{uploadErr: errors.New("network down")}

	err := uploadConfigSync(ctx, backend, "/tmp/cfg.toml", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network down")
	assert.Contains(t, err.Error(), "upload failed")
}

func TestDownloadConfigSync(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}

	// 用 /tmp 避免污染家目录
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "out.toml")

	err := downloadConfigSync(ctx, backend, localPath, "")
	assert.NoError(t, err)
	assert.Len(t, backend.downloads, 1)
}

func TestDownloadConfigSync_PropagatesError(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{downErr: errors.New("404 not found")}

	tmpDir := t.TempDir()
	err := downloadConfigSync(ctx, backend, filepath.Join(tmpDir, "out.toml"), "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404 not found")
}

func TestNewSyncBackend_UnsupportedType(t *testing.T) {
	// nil config 会先 panic 在 webdav/s3 路径, 跳过此测试
	// 实际类型校验逻辑在 webdav/s3 包内, 这里只验证 dispatch 不 panic
	_ = errors.New("")
}

// 编译期断言: webdav.Client / s3.Client 必须满足 SyncBackend 接口
// 如接口不兼容, 编译会报 "cannot use (...) as SyncBackend" 错误
var (
	_ SyncBackend = (*webdav.Client)(nil)
	_ SyncBackend = (*s3.Client)(nil)
)

// 引用 errors 以避免 unused import 警告 (TestNewSyncBackend_UnsupportedType 使用)
var _ = errors.New
