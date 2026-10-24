package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mysshw/config"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// writeTestConfig 写一个测试配置文件并把 config.CFG_PATH 指向它
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mysshw.toml")
	assert.NoError(t, os.WriteFile(p, []byte(content), 0600))

	old := config.CFG_PATH
	t.Cleanup(func() { config.CFG_PATH = old })
	config.CFG_PATH = p
	return p
}

// execConfigCheck 以指定输出格式执行 config check, 返回 (stdout, error)
func execConfigCheck(t *testing.T, format string) (string, error) {
	t.Helper()
	var out bytes.Buffer

	c := &cobra.Command{Use: "check", RunE: runConfigCheck}
	// 与根命令一致: 注册 output flag (本地 flag, 保证 GetOutputFormat 可读)
	c.Flags().StringP(outputFlagName, "o", string(OutputPlain), "")
	c.SetOut(&out)
	c.SetErr(&out)
	if format != "" {
		assert.NoError(t, c.Flags().Set(outputFlagName, format))
	}

	err := runConfigCheck(c, nil)
	return out.String(), err
}

const testNodeTOML = `
[[nodes]]
groups = "prod"

[[nodes.ssh]]
name = "web-1"
host = "10.0.0.1"
user = "root"
port = 22
password = "plain"
`

func TestConfigCheckCmd_ReportsMissingSyncType(t *testing.T) {
	writeTestConfig(t, testNodeTOML)
	stdout, err := execConfigCheck(t, "plain")
	assert.NoError(t, err)
	assert.Contains(t, stdout, "config check")
	assert.Contains(t, stdout, "sync.type")
}

func TestConfigCheckCmd_PlaintextPasswordWarning(t *testing.T) {
	writeTestConfig(t, testNodeTOML)
	stdout, err := execConfigCheck(t, "plain")
	assert.NoError(t, err)
	assert.Contains(t, stdout, "plaintext")
}

func TestConfigCheckCmd_JSONOutput(t *testing.T) {
	writeTestConfig(t, testNodeTOML)
	stdout, err := execConfigCheck(t, "json")
	assert.NoError(t, err)

	var result configCheckResult
	assert.NoError(t, json.Unmarshal([]byte(stdout), &result))
	// 明文密码只是 warn 级别 → 不阻断 (OK 仍为 true)
	assert.True(t, result.OK)
	assert.NotEmpty(t, result.Issues)
	assert.Contains(t, result.Counts, "warn")
	assert.Greater(t, result.Counts["warn"], 0)
}

func TestConfigCheckCmd_CleanConfig(t *testing.T) {
	writeTestConfig(t, `
[[nodes]]
groups = "prod"

[[nodes.ssh]]
name = "web-1"
host = "10.0.0.1"
user = "root"
port = 22
password = "${MYSSHW_PWD_w1}"
`)
	stdout, err := execConfigCheck(t, "json")
	assert.NoError(t, err)

	var result configCheckResult
	assert.NoError(t, json.Unmarshal([]byte(stdout), &result))
	// 明文密码告警不应出现 → warn 计数为 0
	assert.Equal(t, 0, result.Counts["warn"], "env 引用密码不应触发明文告警")
}

func TestConfigCheckCmd_BrokenConfig(t *testing.T) {
	writeTestConfig(t, "this is not = = valid toml [[[")

	stdout, err := execConfigCheck(t, "json")
	assert.Error(t, err, "配置无法解析时应当返回错误")

	var result configCheckResult
	assert.NoError(t, json.Unmarshal([]byte(stdout), &result))
	assert.False(t, result.OK)
	assert.Equal(t, 1, result.Counts["error"])
}

func TestConfigCheckCmd_MissingFile(t *testing.T) {
	old := config.CFG_PATH
	t.Cleanup(func() { config.CFG_PATH = old })
	config.CFG_PATH = filepath.Join(t.TempDir(), "does-not-exist.toml")

	stdout, err := execConfigCheck(t, "json")
	assert.Error(t, err)

	var result configCheckResult
	assert.NoError(t, json.Unmarshal([]byte(stdout), &result))
	assert.False(t, result.OK)
	assert.Equal(t, 1, result.Counts["error"])
}
