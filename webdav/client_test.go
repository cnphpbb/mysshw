package webdav

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
