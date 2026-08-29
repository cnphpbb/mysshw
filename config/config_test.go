package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCfgPath_TildeExpansion(t *testing.T) {
	// 期望: "~/.mysshw.toml" 应展开为 "$HOME/.mysshw.toml"
	// 历史 bug: 原 TestGetCfgPath 错误断言 "~/..." == "/home/...", 实际应展开
	got, err := GetCfgPath("~/.mysshw.toml")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(got, "/.mysshw.toml"),
		"应展开为带 .mysshw.toml 后缀的绝对路径, got: %s", got)
	assert.NotEqual(t, "~/.mysshw.toml", got, "应展开 ~ 不应原样返回")
}

func TestGetCfgPath_AbsolutePath(t *testing.T) {
	// 绝对路径应原样保留 (Unix 风格)
	got, err := GetCfgPath("/etc/mysshw.toml")
	require.NoError(t, err)
	assert.Equal(t, "/etc/mysshw.toml", got)
}

func TestGetCfgPath_RelativePath(t *testing.T) {
	// 相对路径应 join 当前工作目录
	got, err := GetCfgPath("./mysshw.toml")
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got), "相对路径应展开为绝对路径, got: %s", got)
	assert.True(t, strings.HasSuffix(got, "/mysshw.toml"))
}

func TestGetCfgPath_PureDoesNotMutateGlobal(t *testing.T) {
	// 设计: GetCfgPath 纯函数, 不应修改全局 CFG_PATH
	// P2.3 重构后: 调用 GetCfgPath 后 CFG_PATH 应保持调用前的值
	before := CFG_PATH
	_, err := GetCfgPath("~/.mysshw.toml")
	require.NoError(t, err)
	assert.Equal(t, before, CFG_PATH, "GetCfgPath 不应修改全局 CFG_PATH")
}
