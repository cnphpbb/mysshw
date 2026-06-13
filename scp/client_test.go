package scp

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// startMockSSHServer 启动一个内存中的 SSH server, 接受 ed25519 公钥认证
// serverHandler 在 SSH session 中被调用, 根据执行的命令返回 stdin/stdout
// 返回:
//   - addr: 监听地址 (host:port)
//   - signer: 客户端用于认证的 signer
//   - stop: 关闭 server 的函数
func startMockSSHServer(t *testing.T, serverHandler func(cmd string, stdin io.Writer, stdout io.Reader) error) (addr string, signer ssh.Signer, stop func()) {
	t.Helper()

	// 生成 ed25519 密钥对
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err = ssh.NewSignerFromKey(priv)
	require.NoError(t, err)

	// 配置 SSH server
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			// 接受任意公钥 (测试用)
			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(signer)

	// 监听随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	// 接受连接
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener 关闭
			}
			go func(conn net.Conn) {
				sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					_ = conn.Close()
					return
				}
				go ssh.DiscardRequests(reqs)
				go func() {
					_ = sshConn.Wait()
				}()
				for newCh := range chans {
					if newCh.ChannelType() != "session" {
						_ = newCh.Reject(ssh.UnknownChannelType, "unknown")
						continue
					}
					ch, requests, err := newCh.Accept()
					if err != nil {
						continue
					}
					go handleMockSession(ch, requests, serverHandler)
				}
			}(conn)
		}
	}()

	stop = func() { _ = listener.Close() }
	addr = listener.Addr().String()
	return
}

// handleMockSession 处理 SSH session 请求, 找到 "exec" 请求后调用 serverHandler
// handler 的 stdin/stdout 直接绑定到 SSH channel (简化: scp 的 stdin/stdout 就是 channel 本身)
func handleMockSession(ch ssh.Channel, requests <-chan *ssh.Request, handler func(cmd string, stdin io.Writer, stdout io.Reader) error) {
	defer ch.Close()

	for req := range requests {
		switch req.Type {
		case "exec":
			// 解析命令
			var payload struct {
				Command string
			}
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)

			// 直接将 channel 作为 stdin/stdout 传给 handler
			err := handler(payload.Command, ch, ch)

			status := uint32(0)
			if err != nil {
				status = 1
			}
			var exitPayload = struct{ Status uint32 }{Status: status}
			payloadBytes := ssh.Marshal(exitPayload)
			_, _ = ch.SendRequest("exit-status", false, payloadBytes)
			return
		default:
			_ = req.Reply(false, nil)
		}
	}
}

// scpSinkHandler 模拟 scp -t (接收) 模式
// 读取 C0644 <size> <name>\n + 0x00 ack, 然后读取 size 字节, 再 0x00 ack
func scpSinkHandler(stdin io.Writer, stdout io.Reader) (string, []byte, error) {
	// 读取 header
	header := make([]byte, 1)
	if _, err := io.ReadFull(stdout, header); err != nil {
		return "", nil, err
	}
	if header[0] != 'C' {
		return "", nil, fmt.Errorf("expected 'C', got %q", header[0])
	}

	// 读取 "C0644 <size> <name>\n"
	headerLine, err := readLine(stdout)
	if err != nil {
		return "", nil, err
	}
	parts := strings.Fields(headerLine)
	if len(parts) < 3 {
		return "", nil, fmt.Errorf("invalid header: %q", headerLine)
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", nil, err
	}
	name := parts[2]

	// 发送 ack
	if _, err := stdin.Write([]byte{0x00}); err != nil {
		return name, nil, err
	}

	// 读取 size 字节
	data := make([]byte, size)
	if _, err := io.ReadFull(stdout, data); err != nil {
		return name, nil, err
	}

	// 读取末尾 0x00
	tail := make([]byte, 1)
	if _, err := io.ReadFull(stdout, tail); err != nil {
		return name, nil, err
	}
	if tail[0] != 0x00 {
		return name, nil, fmt.Errorf("expected trailing 0x00, got %q", tail[0])
	}

	// 发送 ack
	if _, err := stdin.Write([]byte{0x00}); err != nil {
		return name, data, err
	}

	return name, data, nil
}

// scpSourceHandler 模拟 scp -f (发送) 模式
func scpSourceHandler(name string, content []byte, stdin io.Writer, stdout io.Reader) error {
	// 等待客户端 ack
	ack := make([]byte, 1)
	if _, err := io.ReadFull(stdout, ack); err != nil {
		return err
	}
	if ack[0] != 0x00 {
		return fmt.Errorf("expected initial ack, got %q", ack[0])
	}

	// 发送 header
	header := fmt.Sprintf("C0644 %d %s\n", len(content), name)
	if _, err := stdin.Write([]byte(header)); err != nil {
		return err
	}

	// 等待 ack
	if _, err := io.ReadFull(stdout, ack); err != nil {
		return err
	}
	if ack[0] != 0x00 {
		return fmt.Errorf("expected post-header ack, got %q", ack[0])
	}

	// 发送数据
	if _, err := stdin.Write(content); err != nil {
		return err
	}
	if _, err := stdin.Write([]byte{0x00}); err != nil {
		return err
	}

	// 等待 ack
	if _, err := io.ReadFull(stdout, ack); err != nil {
		return err
	}
	if ack[0] != 0x00 {
		return fmt.Errorf("expected post-data ack, got %q", ack[0])
	}

	return nil
}

// readLine 读取到 \n (不含)
func readLine(r io.Reader) (string, error) {
	var buf bytes.Buffer
	b := make([]byte, 1)
	for {
		_, err := io.ReadFull(r, b)
		if err != nil {
			return "", err
		}
		if b[0] == '\n' {
			return buf.String(), nil
		}
		buf.WriteByte(b[0])
	}
}

// TestCopyFile_UploadToMockServer 验证上传文件到 mock SSH server
func TestCopyFile_UploadToMockServer(t *testing.T) {
	receivedName := ""
	receivedData := []byte(nil)

	addr, signer, stop := startMockSSHServer(t, func(cmd string, stdin io.Writer, stdout io.Reader) error {
		// 客户端发送的命令形如: scp -qt "/remote/path" (-q 为 quiet)
		if !strings.HasPrefix(cmd, "scp -qt") {
			return fmt.Errorf("unexpected cmd: %s", cmd)
		}
		name, data, err := scpSinkHandler(stdin, stdout)
		if err == nil {
			receivedName = name
			receivedData = data
		}
		return err
	})
	defer stop()

	// 构造 scp 客户端
	client := &Client{
		Host: addr,
		ClientConfig: &ssh.ClientConfig{
			User:            "test",
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		},
		RemoteBinary: "scp",
	}
	err := client.Connect()
	require.NoError(t, err)
	defer client.Close()

	// 上传测试数据
	testData := []byte("Hello, SCP World! This is a test file content.")
	err = client.CopyFile(bytes.NewReader(testData), "/remote/test.txt", "0644")
	assert.NoError(t, err)
	assert.Equal(t, "test.txt", receivedName)
	assert.Equal(t, testData, receivedData)
}

// TestCopyFromRemote_DownloadFromMockServer 验证从 mock SSH server 下载
func TestCopyFromRemote_DownloadFromMockServer(t *testing.T) {
	wantName := "remote_file.txt"
	wantContent := []byte("Downloaded content from mock scp server.")

	addr, signer, stop := startMockSSHServer(t, func(cmd string, stdin io.Writer, stdout io.Reader) error {
		// 客户端发送的命令形如: scp -f "/remote/path"
		if !strings.HasPrefix(cmd, "scp -f") {
			return fmt.Errorf("unexpected cmd: %s", cmd)
		}
		return scpSourceHandler(wantName, wantContent, stdin, stdout)
	})
	defer stop()

	client := &Client{
		Host: addr,
		ClientConfig: &ssh.ClientConfig{
			User:            "test",
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		},
		RemoteBinary: "scp",
	}
	err := client.Connect()
	require.NoError(t, err)
	defer client.Close()

	// 下载到本地临时文件
	tmpFile := filepath.Join(t.TempDir(), "downloaded.txt")
	f, err := os.Create(tmpFile)
	require.NoError(t, err)
	defer f.Close()

	err = client.CopyFromRemote(f, "/remote/"+wantName)
	assert.NoError(t, err)

	// 验证下载内容
	data, err := os.ReadFile(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, wantContent, data)
}

// TestCopyFile_LargeFile 验证大文件上传 (>64KB, 跨多次 io.CopyN 调用)
func TestCopyFile_LargeFile(t *testing.T) {
	// 64KB + 1 字节, 强制多次循环
	wantSize := 64*1024 + 1
	wantData := make([]byte, wantSize)
	for i := range wantData {
		wantData[i] = byte(i % 256)
	}

	receivedSize := int64(0)

	addr, signer, stop := startMockSSHServer(t, func(cmd string, stdin io.Writer, stdout io.Reader) error {
		if !strings.HasPrefix(cmd, "scp -qt") {
			return fmt.Errorf("unexpected cmd: %s", cmd)
		}
		_, data, err := scpSinkHandler(stdin, stdout)
		if err == nil {
			receivedSize = int64(len(data))
		}
		return err
	})
	defer stop()

	client := &Client{
		Host: addr,
		ClientConfig: &ssh.ClientConfig{
			User:            "test",
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		},
		RemoteBinary: "scp",
	}
	err := client.Connect()
	require.NoError(t, err)
	defer client.Close()

	err = client.CopyFile(bytes.NewReader(wantData), "/remote/big.bin", "0644")
	assert.NoError(t, err)
	assert.Equal(t, int64(wantSize), receivedSize)
}

// TestConnect_Failure 验证连接到不存在的 server 失败
func TestConnect_Failure(t *testing.T) {
	client := &Client{
		Host: "127.0.0.1:1", // 不可能监听的端口
		ClientConfig: &ssh.ClientConfig{
			User:            "test",
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(testSigner(t))},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         1 * time.Second,
		},
	}
	err := client.Connect()
	assert.Error(t, err)
}

// testSigner 生成临时 signer (用于失败连接测试)
func testSigner(t *testing.T) ssh.Signer {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

// 防止 binary 导入被误删 (无依赖, 仅占位以避免删 import 时 lint 失败)
var _ = binary.BigEndian

