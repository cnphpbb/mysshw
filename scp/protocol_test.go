package scp

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseResponse_Ok 验证 Ok 响应 (单字节 0x00, 无 message)
func TestParseResponse_Ok(t *testing.T) {
	r := bytes.NewReader([]byte{0x00})
	resp, err := ParseResponse(r)
	assert.NoError(t, err)
	assert.Equal(t, Ok, resp.Type)
	assert.Empty(t, resp.Message)
	assert.True(t, resp.IsOk())
	assert.False(t, resp.IsFailure())
}

// TestParseResponse_Warning 验证 Warning 响应 (0x01 + 消息)
func TestParseResponse_Warning(t *testing.T) {
	r := bytes.NewReader([]byte{0x01, 'h', 'i', '\n'})
	resp, err := ParseResponse(r)
	assert.NoError(t, err)
	assert.Equal(t, Warning, resp.Type)
	assert.Equal(t, "hi\n", resp.Message)
	assert.True(t, resp.IsWarning())
	assert.True(t, resp.IsFailure())
	assert.False(t, resp.IsOk())
}

// TestParseResponse_Error 验证 Error 响应 (0x02 + 消息)
func TestParseResponse_Error(t *testing.T) {
	r := bytes.NewReader([]byte{0x02, 'f', 'a', 'i', 'l', '\n'})
	resp, err := ParseResponse(r)
	assert.NoError(t, err)
	assert.Equal(t, Error, resp.Type)
	assert.Equal(t, "fail\n", resp.Message)
	assert.True(t, resp.IsError())
	assert.True(t, resp.IsFailure())
}

// TestParseResponse_EOF 验证读到 EOF 时返回错误
func TestParseResponse_EOF(t *testing.T) {
	r := bytes.NewReader([]byte{})
	_, err := ParseResponse(r)
	assert.Error(t, err)
}

// TestAck 验证 Ack 写入 0x00 字节
func TestAck(t *testing.T) {
	var buf bytes.Buffer
	err := Ack(&buf)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x00}, buf.Bytes())
}

// TestParseFileInfos_Valid 验证标准 SCP 文件头解析
func TestParseFileInfos_Valid(t *testing.T) {
	resp := &Response{
		Type:    Ok,
		Message: "C0644 1234 myfile.txt\n",
	}
	fi, err := resp.ParseFileInfos()
	assert.NoError(t, err)
	assert.Equal(t, "C0644", fi.Permissions)
	assert.Equal(t, int64(1234), fi.Size)
	assert.Equal(t, "myfile.txt", fi.Filename)
}

// TestParseFileInfos_InvalidSize 验证 size 非数字时返回错误
func TestParseFileInfos_InvalidSize(t *testing.T) {
	resp := &Response{
		Type:    Ok,
		Message: "C0644 NaN myfile.txt\n",
	}
	_, err := resp.ParseFileInfos()
	assert.Error(t, err)
}

// TestParseFileInfos_TooFewParts 验证字段不足时返回错误
func TestParseFileInfos_TooFewParts(t *testing.T) {
	resp := &Response{
		Type:    Ok,
		Message: "C0644\n",
	}
	_, err := resp.ParseFileInfos()
	assert.Error(t, err)
}

// TestCopyN 验证 CopyN 复制正好 N 字节 (含短读重试)
func TestCopyN(t *testing.T) {
	src := strings.NewReader("Hello, World! This is a longer string to test CopyN.")
	var dst bytes.Buffer

	n, err := CopyN(&dst, src, 13)
	assert.NoError(t, err)
	assert.Equal(t, int64(13), n)
	assert.Equal(t, "Hello, World!", dst.String())
}

// TestCopyN_ZeroSize 0 字节
func TestCopyN_ZeroSize(t *testing.T) {
	src := strings.NewReader("data")
	var dst bytes.Buffer

	n, err := CopyN(&dst, src, 0)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Empty(t, dst.String())
}

// TestCopyN_ShortRead 验证源数据不足时正确返回 EOF 错误
func TestCopyN_ShortRead(t *testing.T) {
	src := strings.NewReader("abc")
	var dst bytes.Buffer

	_, err := CopyN(&dst, src, 100)
	assert.Error(t, err)
	assert.Equal(t, io.EOF, err)
}
