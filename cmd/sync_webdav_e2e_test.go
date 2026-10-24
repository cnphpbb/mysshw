package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mysshw/config"
	"mysshw/webdav"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// webdavTestServer 最小 WebDAV 服务器 (支持 OPTIONS / PROPFIND / PUT / GET / MKCOL)
//
// 目的: 端到端验证 "远程只存密文, 下载后本地解密" 这条真实 HTTP 链路
type webdavTestServer struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newWebdavTestServer() *webdavTestServer {
	return &webdavTestServer{files: map[string][]byte{}}
}

func (s *webdavTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := r.URL.Path
	switch r.Method {
	case "OPTIONS":
		w.Header().Set("DAV", "1, 2")
		w.WriteHeader(http.StatusOK)

	case "PROPFIND":
		data, ok := s.files[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeMultiStatus(w, path, filepath.Base(path), len(data), false)

	case "PUT":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.files[path] = body
		w.WriteHeader(http.StatusCreated)

	case "GET":
		data, ok := s.files[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)

	case "MKCOL":
		w.WriteHeader(http.StatusCreated)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// writeMultiStatus 输出 PROPFIND 所需的 207 multistatus 响应
func writeMultiStatus(w http.ResponseWriter, href, displayName string, size int, isDir bool) {
	resourcetype := ""
	if isDir {
		resourcetype = "<D:collection/>"
	}
	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:">
  <D:response>
    <D:href>%s</D:href>
    <D:propstat>
      <D:prop>
        <D:displayname>%s</D:displayname>
        <D:resourcetype>%s</D:resourcetype>
        <D:getcontentlength>%d</D:getcontentlength>
        <D:getcontenttype>application/octet-stream</D:getcontenttype>
        <D:getetag>"etag"</D:getetag>
        <D:getlastmodified>Mon, 02 Jan 2006 15:04:05 GMT</D:getlastmodified>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>`, href, displayName, resourcetype, size)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(body))
}

// newTestWebdavBackend 启动测试服务器并创建 webdav backend
func newTestWebdavBackend(t *testing.T) (*webdavTestServer, SyncBackend) {
	t.Helper()
	srv := newWebdavTestServer()
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	uri := strings.TrimPrefix(ts.URL, "http://")
	cfg := &config.SyncInfo{
		Type:       "webdav",
		RemoteUri:  uri,
		RemotePath: "backup",
		WebDAVConfig: config.WebDAVConfig{
			Auth:     "Basic",
			Username: "root",
			Password: "pass",
		},
	}

	backend, err := webdav.NewClient(context.Background(), cfg)
	require.NoError(t, err)
	return srv, backend
}

func TestSyncWebDAV_E2E_RemoteStoresCiphertextOnly(t *testing.T) {
	saveRestoreCFGPath(t)
	t.Setenv(DefaultSyncPassphraseEnv, "e2e-pass")

	srv, backend := newTestWebdavBackend(t)

	plain := "password = \"super-secret\"\n"
	cfgPath := writeTempSyncConfig(t, plain)

	// --- 上传 ---
	err := uploadConfigSync(context.Background(), backend, cfgPath, "", syncCryptoOptions{
		Encrypt:       true,
		PassphraseEnv: DefaultSyncPassphraseEnv,
	})
	require.NoError(t, err)

	// 远程文件里只能是密文
	stored, ok := srv.files["/backup/mysshw.toml"]
	require.True(t, ok, "远程应存在 /backup/mysshw.toml, 实际: %v", srv.files)
	assert.True(t, config.IsEncryptedFile(stored), "远程必须是密文")
	assert.NotContains(t, string(stored), "super-secret", "远程不得出现明文密码")

	// --- 下载 ---
	restored := filepath.Join(t.TempDir(), "mysshw.toml")
	err = downloadConfigSync(context.Background(), backend, restored, "", syncCryptoOptions{
		PassphraseEnv: DefaultSyncPassphraseEnv,
	})
	require.NoError(t, err)

	data, err := os.ReadFile(restored)
	require.NoError(t, err)
	assert.Equal(t, plain, string(data), "下载后本地应还原为明文")

	// 本地文件权限 0600
	info, err := os.Stat(restored)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSyncWebDAV_E2E_WrongPassphraseKeepsRemoteAndLocal(t *testing.T) {
	saveRestoreCFGPath(t)
	t.Setenv(DefaultSyncPassphraseEnv, "e2e-pass")

	srv, backend := newTestWebdavBackend(t)

	plain := "password = \"super-secret\"\n"
	cfgPath := writeTempSyncConfig(t, plain)

	require.NoError(t, uploadConfigSync(context.Background(), backend, cfgPath, "", syncCryptoOptions{
		Encrypt:       true,
		PassphraseEnv: DefaultSyncPassphraseEnv,
	}))

	// 用错误口令下载 → 报错, 且本地文件不被覆盖
	t.Setenv(DefaultSyncPassphraseEnv, "wrong-pass")
	restored := filepath.Join(t.TempDir(), "mysshw.toml")
	require.NoError(t, os.WriteFile(restored, []byte("local = 1\n"), 0600))

	err := downloadConfigSync(context.Background(), backend, restored, "", syncCryptoOptions{
		PassphraseEnv: DefaultSyncPassphraseEnv,
	})
	assert.Error(t, err)

	data, readErr := os.ReadFile(restored)
	require.NoError(t, readErr)
	assert.Equal(t, "local = 1\n", string(data), "本地配置不得被覆盖")

	// 远程密文保持不变
	stored := srv.files["/backup/mysshw.toml"]
	assert.True(t, config.IsEncryptedFile(stored))
}
