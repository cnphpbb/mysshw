package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// configCmd 集成测试
//
// 策略: 编译 mysshw 二进制, 通过 os/exec 调用
// 优点: 隔离测试, 真实环境, 无 cobra 全局状态污染
// 缺点: 编译稍慢 (~3s), 但只在 TestMain 跑一次
// ============================================================

var testBinary string

func TestMain(m *testing.M) {
	// Build mysshw binary for integration tests
	// 注意: go test ./cmd/ 时 os.Getwd() 返回 cmd/ 目录, 而主二进制在项目根目录
	// 使用 runtime.Caller(0) 获取本文件路径, 再上溯两层得到项目根目录
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to get caller info")
	}
	projectRoot := filepath.Dir(filepath.Dir(filename))

	tmp, err := os.MkdirTemp("", "mysshw-cfg-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "mysshw")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("build failed: " + err.Error() + "\n" + string(out))
	}
	testBinary = bin
	os.Exit(m.Run())
}

const fixtureTOML = `
[[nodes]]
groups = "Production"
[[nodes.ssh]]
name = "prod-1"
alias = "Web01"
host = "10.0.0.1"
user = "app"
port = 22
password = "${MYSSHW_PWD_prod_1}"

[[nodes.ssh]]
name = "prod-2"
host = "10.0.0.2"
user = "root"
port = 2222
password = "${MYSSHW_PWD_prod_2}"

[[nodes]]
groups = "Staging"
[[nodes.ssh]]
name = "stag-1"
host = "192.168.1.1"
user = "deploy"
`

func writeFixtureConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(path, []byte(fixtureTOML), 0600))
	return path
}

// runConfigCmd 跑 mysshw config <subargs>, 捕获输出 + 退出码
func runConfigCmd(t *testing.T, cfgPath string, subargs ...string) (string, int) {
	t.Helper()
	args := append([]string{"-c", cfgPath, "config"}, subargs...)
	return runMySshwCmd(t, args...)
}

// runMySshwCmd 跑 mysshw <args>, 捕获输出 + 退出码
//
// 用于测试顶层子命令 (export / import / list / exec 等不隶属 config 的命令)。
// 历史上 import_export_test.go 错误地把 export/import 走 runConfigCmd
// 导致被多加了 "config" 子命令前缀, 详见 import_export_test.go 顶部说明。
func runMySshwCmd(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(testBinary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	}
	return stdout.String() + stderr.String(), exitCode
}

// ============================================================
// list
// ============================================================

func TestConfigList_Plain(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "list")
	assert.Equal(t, 0, exitCode, "expected success, got: %s", out)
	assert.Contains(t, out, "prod-1")
	assert.Contains(t, out, "prod-2")
	assert.Contains(t, out, "stag-1")
}

func TestConfigList_JSON(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "list", "--format", "json")
	assert.Equal(t, 0, exitCode)

	start := strings.Index(out, "[")
	require.GreaterOrEqual(t, start, 0, "no JSON array in output: %s", out)
	jsonPart := out[start:]

	var nodes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonPart), &nodes))
	assert.NotEmpty(t, nodes)
	for _, n := range nodes {
		_, hasPwd := n["password"]
		assert.False(t, hasPwd, "JSON 不能含 password 字段")
	}
}

func TestConfigList_GroupFilter(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "list", "--group", "Production", "--format", "json")
	assert.Equal(t, 0, exitCode)

	start := strings.Index(out, "[")
	jsonPart := out[start:]
	var nodes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonPart), &nodes))
	for _, n := range nodes {
		assert.Equal(t, "Production", n["group"])
	}
}

func TestConfigList_EmptyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0600))
	out, exitCode := runConfigCmd(t, path, "list")
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "no nodes")
}

// ============================================================
// get
// ============================================================

func TestConfigGet_ByName(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "get", "prod-1")
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "Name:         prod-1")
	assert.Contains(t, out, "Host:         10.0.0.1")
	assert.Contains(t, out, "Alias:        Web01")
	assert.NotContains(t, out, "${MYSSHW_PWD_prod_1}")
}

func TestConfigGet_ByAlias(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "get", "Web01")
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "Name:         prod-1")
}

func TestConfigGet_NotFound(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "get", "nonexistent")
	assert.NotZero(t, exitCode)
	assert.Equal(t, 78, exitCode, "got: %s", out)
}

func TestConfigGet_MissingArg(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "get")
	assert.NotZero(t, exitCode)
	assert.Equal(t, 64, exitCode, "got: %s", out)
}

// ============================================================
// add
// ============================================================

func TestConfigAdd_DryRun(t *testing.T) {
	path := writeFixtureConfig(t)
	origData, _ := os.ReadFile(path)

	out, exitCode := runConfigCmd(t, path,
		"add",
		"--group", "NewGroup",
		"--name", "new-1",
		"--host", "10.0.0.99",
		"--user", "deploy",
	)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "dry-run")

	newData, _ := os.ReadFile(path)
	assert.Equal(t, origData, newData, "dry-run 不应写文件")
}

func TestConfigAdd_WithConfirm(t *testing.T) {
	path := writeFixtureConfig(t)

	_, exitCode := runConfigCmd(t, path,
		"add",
		"--group", "NewGroup",
		"--name", "new-1",
		"--host", "10.0.0.99",
		"--user", "deploy",
		"--confirm",
	)
	assert.Equal(t, 0, exitCode)

	cfg, _ := config.ReadConfigOrEmpty(path)
	found := false
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == "new-1" {
				found = true
				assert.Equal(t, "10.0.0.99", n.Host)
			}
		}
	}
	assert.True(t, found)
}

func TestConfigAdd_DuplicateName(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path,
		"add",
		"--group", "Production",
		"--name", "prod-1", // 重复
		"--host", "10.0.0.99",
		"--confirm",
	)
	assert.NotZero(t, exitCode)
	assert.Equal(t, 78, exitCode, "got: %s", out)
}

func TestConfigAdd_DuplicateAlias(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path,
		"add",
		"--group", "NewGroup",
		"--name", "another",
		"--alias", "Web01", // 重复
		"--host", "10.0.0.99",
		"--confirm",
	)
	assert.NotZero(t, exitCode)
	assert.Equal(t, 78, exitCode, "got: %s", out)
}

func TestConfigAdd_MissingRequired(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path,
		"add",
		"--name", "x",
		"--host", "10.0.0.1",
		// 缺 --group
	)
	assert.NotZero(t, exitCode)
	assert.Equal(t, 64, exitCode, "got: %s", out)
}

// ============================================================
// remove
// ============================================================

func TestConfigRemove_DryRun(t *testing.T) {
	path := writeFixtureConfig(t)
	origData, _ := os.ReadFile(path)

	_, exitCode := runConfigCmd(t, path, "remove", "prod-1")
	assert.Equal(t, 0, exitCode)

	newData, _ := os.ReadFile(path)
	assert.Equal(t, origData, newData, "dry-run 不修改文件")
}

func TestConfigRemove_WithConfirm(t *testing.T) {
	path := writeFixtureConfig(t)

	_, exitCode := runConfigCmd(t, path, "remove", "prod-1", "--confirm")
	assert.Equal(t, 0, exitCode)

	cfg, _ := config.ReadConfigOrEmpty(path)
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			assert.NotEqual(t, "prod-1", n.Name)
		}
	}
}

func TestConfigRemove_NotFound(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "remove", "nope", "--confirm")
	assert.NotZero(t, exitCode)
	assert.Equal(t, 78, exitCode, "got: %s", out)
}

// ============================================================
// update
// ============================================================

func TestConfigUpdate_DryRun(t *testing.T) {
	path := writeFixtureConfig(t)
	origData, _ := os.ReadFile(path)

	_, exitCode := runConfigCmd(t, path, "update", "prod-1", "--port", "2222")
	assert.Equal(t, 0, exitCode)

	newData, _ := os.ReadFile(path)
	assert.Equal(t, origData, newData, "dry-run 不修改文件")
}

func TestConfigUpdate_Port(t *testing.T) {
	path := writeFixtureConfig(t)

	_, exitCode := runConfigCmd(t, path, "update", "prod-1", "--port", "2222", "--confirm")
	assert.Equal(t, 0, exitCode)

	cfg, _ := config.ReadConfigOrEmpty(path)
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == "prod-1" {
				assert.Equal(t, 2222, n.Port)
			}
		}
	}
}

func TestConfigUpdate_NoChanges(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "update", "prod-1", "--confirm")
	assert.NotZero(t, exitCode)
	assert.Equal(t, 64, exitCode, "got: %s", out)
}

// ============================================================
// diff
// ============================================================

func TestConfigDiff_SameNode(t *testing.T) {
	path := writeFixtureConfig(t)
	_, exitCode := runConfigCmd(t, path, "diff", "prod-1", "prod-2")
	assert.Equal(t, 0, exitCode)
}

func TestConfigDiff_NotFound(t *testing.T) {
	path := writeFixtureConfig(t)
	out, exitCode := runConfigCmd(t, path, "diff", "prod-1", "nope")
	assert.NotZero(t, exitCode)
	assert.Equal(t, 78, exitCode, "got: %s", out)
}

// ============================================================
// 集成: add → update → remove 完整 lifecycle
// ============================================================

func TestConfigCRUD_Lifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0600))

	// 1. add
	_, exitCode := runConfigCmd(t, path,
		"add",
		"--group", "Test",
		"--name", "lifecycle",
		"--host", "1.2.3.4",
		"--user", "u",
		"--password", "${MYSSHW_PWD_lifecycle}",
		"--confirm",
	)
	assert.Equal(t, 0, exitCode)

	cfg, _ := config.ReadConfigOrEmpty(path)
	require.Equal(t, 1, len(cfg.Nodes))
	assert.Equal(t, "lifecycle", cfg.Nodes[0].SSHNodes[0].Name)

	// 2. update
	_, exitCode = runConfigCmd(t, path, "update", "lifecycle",
		"--port", "9999",
		"--alias", "lc",
		"--confirm",
	)
	assert.Equal(t, 0, exitCode)

	cfg, _ = config.ReadConfigOrEmpty(path)
	assert.Equal(t, 9999, cfg.Nodes[0].SSHNodes[0].Port)
	assert.Equal(t, "lc", cfg.Nodes[0].SSHNodes[0].Alias)

	// 3. remove
	_, exitCode = runConfigCmd(t, path, "remove", "lifecycle", "--confirm")
	assert.Equal(t, 0, exitCode)

	cfg, _ = config.ReadConfigOrEmpty(path)
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			assert.NotEqual(t, "lifecycle", n.Name)
		}
	}
}
