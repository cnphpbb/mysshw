package s3

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoinRemotePath(t *testing.T) {
	cases := []struct {
		prefix, name, expected string
	}{
		{"", "file", "file"},
		{"prefix", "file", "prefix/file"},
		{"a/b", "c", "a/b/c"},
		{"a/b/", "c", "a/b//c"}, // 不主动去重, 留给上层
	}
	for _, c := range cases {
		t.Run(c.prefix+"|"+c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, joinRemotePath(c.prefix, c.name))
		})
	}
}

func TestMinioFileInfo_Name(t *testing.T) {
	fi := &minioFileInfo{name: "test.txt", size: 100}
	assert.Equal(t, "test.txt", fi.Name())
	assert.Equal(t, int64(100), fi.Size())
	assert.False(t, fi.IsDir())
}

func TestMinioFileInfo_Mode(t *testing.T) {
	// Mode() 固定返回 0644
	fi := &minioFileInfo{}
	assert.Equal(t, "-rw-r--r--", fi.Mode().String())
}
