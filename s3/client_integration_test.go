package s3

import (
	"context"
	"strings"
	"testing"
	"time"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeTestSyncInfo 构造测试用的 SyncInfo (s3 类型).
// endpoint 由调用方填充 (httptest.URL 或固定地址).
func makeTestSyncInfo(endpoint, bucket, remotePath string) *config.SyncInfo {
	return &config.SyncInfo{
		Type:       "s3",
		RemoteUri:  "127.0.0.1:0",
		RemotePath: remotePath,
		S3Config: config.S3Config{
			Endpoint:   endpoint,
			AccessKey:  "testaccess",
			SecretKey:  "testsecret",
			BucketName: bucket,
			Region:     "us-east-1",
		},
	}
}

func TestNewClient_LeadingSlashRejected(t *testing.T) {
	// remote_path 以 '/' 开头必须被拒绝 — 这是客户端代码层校验, 不依赖 server
	// 使用不存在的 endpoint, 确保 server 校验之前就被前置校验拦截
	cfg := makeTestSyncInfo("127.0.0.1:1", "test-bucket", "/bad/path")

	_, err := NewClient(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S3 远程路径不应以 '/' 开头")
}

func TestNewClient_InvalidEndpoint_FailsAtConnection(t *testing.T) {
	// 错误路径不在路径校验, 而是在连接层 — 防止未来重构时漏掉校验
	cfg := makeTestSyncInfo("nonexistent-host.invalid.local:9999", "bucket", "data")
	cfg.S3Config.Region = ""

	_, err := NewClient(context.Background(), cfg)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "不应以 '/' 开头")
}

func TestMinioFileInfo_AllMethods(t *testing.T) {
	// 验证 minioFileInfo 实现了 os.FileInfo 全部方法 (os.FileInfo 包含 7 个方法)
	modTime := time.Date(2026, 7, 12, 15, 30, 45, 0, time.UTC)
	fi := &minioFileInfo{
		name:    "remote-file.toml",
		size:    4096,
		modTime: modTime,
	}

	assert.Equal(t, "remote-file.toml", fi.Name())
	assert.Equal(t, int64(4096), fi.Size())
	assert.Equal(t, "-rw-r--r--", fi.Mode().String(), "Mode() 应固定返回 0644")
	assert.Equal(t, modTime, fi.ModTime())
	assert.False(t, fi.IsDir(), "S3 对象存储, 所有返回项视为非目录")
	assert.Nil(t, fi.Sys(), "Sys() 应返回 nil")
}

func TestMinioFileInfo_ZeroValue(t *testing.T) {
	// 零值也必须安全 (用于 new() 后立即调用)
	fi := &minioFileInfo{}
	assert.Equal(t, "", fi.Name())
	assert.Equal(t, int64(0), fi.Size())
	assert.True(t, fi.ModTime().IsZero())
	assert.False(t, fi.IsDir())
}

func TestJoinRemotePathPrefixVariants(t *testing.T) {
	// 全面的 joinRemotePath 场景 (基础覆盖 client_test.go 已存在, 这里补充边缘场景)
	cases := []struct {
		prefix, name, expected string
	}{
		{"", "", ""},
		{"data", "file", "data/file"},
		{"data/", "file", "data//file"}, // 重复斜杠, 上层负责清理
		{"a/b/c", "d.txt", "a/b/c/d.txt"},
		{"a/b/c", "", "a/b/c/"}, // 空前缀 + 名字, 仍应拼接
		{"", "file", "file"},
	}
	for _, c := range cases {
		t.Run(c.prefix+"|name|"+c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, joinRemotePath(c.prefix, c.name))
		})
	}
}

func TestConfigParse_AllFields(t *testing.T) {
	// 验证 SyncInfo 的所有 S3 字段都能正确构造和读取 (无运行时错误)
	cfg := makeTestSyncInfo("https://s3.amazonaws.com", "my-bucket", "config/mysshw.toml")
	assert.True(t, strings.HasPrefix(cfg.S3Config.Endpoint, "https"))
	assert.Equal(t, "my-bucket", cfg.S3Config.BucketName)
	assert.Equal(t, "config/mysshw.toml", cfg.RemotePath)
	assert.Equal(t, "us-east-1", cfg.S3Config.Region)
}
