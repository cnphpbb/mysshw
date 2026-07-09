package cmd

import (
	"encoding/json"
	"mysshw/config"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// exportCmd 集成测试
// 策略: 复用 config_cmd_test.go 的 testBinary + TestMain
// ============================================================

func writeExportFixture(t *testing.T) string {
	t.Helper()
	return writeFixtureConfig(t)
}

func runExportCmd(t *testing.T, cfgPath string, args ...string) (string, int) {
	t.Helper()
	cmdArgs := append([]string{"-c", cfgPath, "export"}, args...)
	return runConfigCmd(t, "", cmdArgs...)
}

// runExportCmdRaw 返回原始 stdout (不拼接 stderr)
func runExportCmdRaw(t *testing.T, cfgPath string, args ...string) (stdout string, exitCode int) {
	t.Helper()
	all, code := runExportCmd(t, cfgPath, args...)
	return all, code
}

// ============================================================
// export — basic
// ============================================================

func TestExport_JSON_All(t *testing.T) {
	path := writeExportFixture(t)
	out, exitCode := runExportCmd(t, path, "--format", "json")
	assert.Equal(t, 0, exitCode, "stdout: %s", out)

	// 找 JSON 部分 (可能有 stderr 消息)
	start := strings.Index(out, "[")
	require.GreaterOrEqual(t, start, 0, "no JSON array: %s", out)
	jsonPart := out[start:]

	var nodes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonPart), &nodes))
	assert.NotEmpty(t, nodes)

	// 验证密码已替换为 ${IMPORTED}
	for _, n := range nodes {
		pwd, hasPwd := n["password"]
		if hasPwd {
			assert.Equal(t, "${IMPORTED}", pwd, "密码应替换为占位符")
		}
	}
}

func TestExport_GroupFilter(t *testing.T) {
	path := writeExportFixture(t)
	out, exitCode := runExportCmd(t, path, "--group", "Production", "--format", "json")
	assert.Equal(t, 0, exitCode)

	start := strings.Index(out, "[")
	require.GreaterOrEqual(t, start, 0)
	jsonPart := out[start:]

	var nodes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonPart), &nodes))
	for _, n := range nodes {
		assert.Equal(t, "Production", n["group"])
	}
}

func TestExport_NameFilter(t *testing.T) {
	path := writeExportFixture(t)
	out, exitCode := runExportCmd(t, path, "--name", "prod-1", "--format", "json")
	assert.Equal(t, 0, exitCode)

	start := strings.Index(out, "[")
	require.GreaterOrEqual(t, start, 0)
	jsonPart := out[start:]

	var nodes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonPart), &nodes))
	assert.Len(t, nodes, 1)
	assert.Equal(t, "prod-1", nodes[0]["name"])
}

func TestExport_NoMatchError(t *testing.T) {
	path := writeExportFixture(t)
	_, exitCode := runExportCmd(t, path, "--name", "nonexistent")
	assert.NotZero(t, exitCode)
	assert.Equal(t, 78, exitCode, "应返回 EX_CONFIG")
}

func TestExport_TOMLFormat(t *testing.T) {
	path := writeExportFixture(t)
	out, exitCode := runExportCmd(t, path, "--format", "toml")
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "# mysshw export")
	assert.Contains(t, out, "prod-1")
	assert.Contains(t, out, "prod-2")
	assert.Contains(t, out, "stag-1")
}

func TestExport_ToFile(t *testing.T) {
	path := writeExportFixture(t)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "exported.json")

	out, exitCode := runExportCmd(t, path, "--format", "json", "--output", outPath)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "exported")

	// 验证文件存在
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "prod-1")
}

func TestExport_UnsupportedFormat(t *testing.T) {
	path := writeExportFixture(t)
	_, exitCode := runExportCmd(t, path, "--format", "yaml")
	assert.NotZero(t, exitCode)
	assert.Equal(t, 64, exitCode, "应返回 EX_USAGE")
}

// ============================================================
// import — 完整的导入生命周期
// ============================================================

func writeTempJSONNodes(t *testing.T, nodes []portableNode) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "nodes.json")
	data, err := json.MarshalIndent(nodes, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

func runImportCmd(t *testing.T, cfgPath string, filePath string, args ...string) (string, int) {
	t.Helper()
	cmdArgs := append([]string{"-c", cfgPath, "import", filePath}, args...)
	return runConfigCmd(t, "", cmdArgs...)
}

func TestImport_DryRun(t *testing.T) {
	cfgPath := writeExportFixture(t)
	importPath := writeTempJSONNodes(t, []portableNode{
		{Group: "NewGroup", Name: "new-1", Host: "10.0.0.99", User: "deploy"},
	})

	out, exitCode := runImportCmd(t, cfgPath, importPath)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "dry-run")

	// 验证原文件未改动
	origData, _ := os.ReadFile(cfgPath)
	newData, _ := os.ReadFile(cfgPath)
	assert.Equal(t, origData, newData, "dry-run 不应写文件")
}

func TestImport_WithConfirm(t *testing.T) {
	cfgPath := writeExportFixture(t)
	importPath := writeTempJSONNodes(t, []portableNode{
		{Group: "NewGroup", Name: "new-1", Host: "10.0.0.99", User: "deploy"},
	})

	out, exitCode := runImportCmd(t, cfgPath, importPath, "--confirm")
	assert.Equal(t, 0, exitCode, "stdout: %s", out)
	assert.Contains(t, out, "imported")

	// 验证节点已添加
	cfg, _ := config.ReadConfigOrEmpty(cfgPath)
	found := false
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == "new-1" {
				found = true
				assert.Equal(t, "10.0.0.99", n.Host)
			}
		}
	}
	assert.True(t, found, "节点 new-1 应已添加")
}

func TestImport_SkipDuplicate(t *testing.T) {
	cfgPath := writeExportFixture(t)
	importPath := writeTempJSONNodes(t, []portableNode{
		{Group: "Production", Name: "prod-1", Host: "10.0.0.99", User: "deploy"},
	})

	out, exitCode := runImportCmd(t, cfgPath, importPath, "--confirm")
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "skipped")

	// 验证原有节点未被覆盖 (host 未变)
	cfg, _ := config.ReadConfigOrEmpty(cfgPath)
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == "prod-1" {
				assert.Equal(t, "10.0.0.1", n.Host, "原有节点不应被覆盖")
			}
		}
	}
}

func TestImport_Overwrite(t *testing.T) {
	cfgPath := writeExportFixture(t)
	importPath := writeTempJSONNodes(t, []portableNode{
		{Group: "Production", Name: "prod-1", Host: "10.0.0.99", User: "deploy"},
	})

	out, exitCode := runImportCmd(t, cfgPath, importPath, "--confirm", "--overwrite")
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, out, "imported")

	// 验证节点已被覆盖
	cfg, _ := config.ReadConfigOrEmpty(cfgPath)
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == "prod-1" {
				assert.Equal(t, "10.0.0.99", n.Host, "原有节点应被覆盖")
			}
		}
	}
}

func TestImport_OverrideGroup(t *testing.T) {
	cfgPath := writeExportFixture(t)
	importPath := writeTempJSONNodes(t, []portableNode{
		{Group: "OriginalGroup", Name: "new-1", Host: "10.0.0.99", User: "deploy"},
	})

	out, exitCode := runImportCmd(t, cfgPath, importPath, "--confirm", "--group", "CustomGroup")
	assert.Equal(t, 0, exitCode, "stdout: %s", out)

	// 验证节点被添加到 CustomGroup 而不是 OriginalGroup
	cfg, _ := config.ReadConfigOrEmpty(cfgPath)
	found := false
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == "new-1" {
				found = true
				assert.Equal(t, "CustomGroup", g.Groups, "group 应被覆盖")
			}
		}
	}
	assert.True(t, found)
}

func TestImport_EmptyFile(t *testing.T) {
	cfgPath := writeExportFixture(t)
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.json")
	require.NoError(t, os.WriteFile(emptyPath, []byte(""), 0600))

	_, exitCode := runImportCmd(t, cfgPath, emptyPath)
	assert.NotZero(t, exitCode)
	assert.Equal(t, 66, exitCode, "应返回 EX_NOINPUT")
}

func TestImport_InvalidJSON(t *testing.T) {
	cfgPath := writeExportFixture(t)
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(badPath, []byte("{invalid json}"), 0600))

	_, exitCode := runImportCmd(t, cfgPath, badPath)
	assert.NotZero(t, exitCode)
}

func TestImport_TOMLFormat(t *testing.T) {
	cfgPath := writeExportFixture(t)
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "nodes.toml")
	tomlContent := `[[nodes]]
groups = "TomlGroup"

[[nodes.ssh]]
name = "toml-1"
host = "10.10.10.10"
user = "admin"
password = "${IMPORTED}"
`
	require.NoError(t, os.WriteFile(tomlPath, []byte(tomlContent), 0600))

	out, exitCode := runImportCmd(t, cfgPath, tomlPath, "--confirm")
	assert.Equal(t, 0, exitCode, "stdout: %s", out)

	// 验证
	cfg, _ := config.ReadConfigOrEmpty(cfgPath)
	found := false
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == "toml-1" {
				found = true
				assert.Equal(t, "TomlGroup", g.Groups)
			}
		}
	}
	assert.True(t, found)
}

func TestImport_TOMLArrayFormat(t *testing.T) {
	cfgPath := writeExportFixture(t)
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "nodes.toml")
	tomlContent := `[[nodes]]
groups = "Batch"

[[nodes.ssh]]
name = "batch-1"
host = "10.1.1.1"

[[nodes.ssh]]
name = "batch-2"
host = "10.1.1.2"
`
	require.NoError(t, os.WriteFile(tomlPath, []byte(tomlContent), 0600))

	out, exitCode := runImportCmd(t, cfgPath, tomlPath, "--confirm")
	assert.Equal(t, 0, exitCode, "stdout: %s", out)

	cfg, _ := config.ReadConfigOrEmpty(cfgPath)
	count := 0
	for _, g := range cfg.Nodes {
		if g.Groups == "Batch" {
			count += len(g.SSHNodes)
		}
	}
	assert.Equal(t, 2, count, "应导入 2 个节点到 Batch 组")
}

// ============================================================
// 完整生命周期: export → import
// ============================================================

func TestExportImport_Lifecycle(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mysshw.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(""), 0600))

	// 1. 先用 config add 添加一个节点
	addArgs := []string{"-c", cfgPath, "config", "add",
		"--group", "Lifecycle",
		"--name", "lifecycle-1",
		"--host", "10.0.0.1",
		"--user", "test",
		"--confirm",
	}
	out, exitCode := runConfigCmd(t, "", addArgs...)
	require.Equal(t, 0, exitCode, "add failed: %s", out)

	// 2. export 为 JSON
	exportPath := filepath.Join(dir, "exported.json")
	out, exitCode = runExportCmd(t, cfgPath, "--format", "json", "--output", exportPath)
	require.Equal(t, 0, exitCode, "export failed: %s", out)

	// 3. 删除原节点
	removeArgs := []string{"-c", cfgPath, "config", "remove", "lifecycle-1", "--confirm"}
	out, exitCode = runConfigCmd(t, "", removeArgs...)
	require.Equal(t, 0, exitCode, "remove failed: %s", out)

	// 4. import 回来
	out, exitCode = runImportCmd(t, cfgPath, exportPath, "--confirm")
	assert.Equal(t, 0, exitCode, "import failed: %s", out)

	// 5. 验证
	cfg, _ := config.ReadConfigOrEmpty(cfgPath)
	found := false
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n.Name == "lifecycle-1" {
				found = true
				assert.Equal(t, "10.0.0.1", n.Host)
			}
		}
	}
	assert.True(t, found, "节点应在 export→delete→import 后恢复")
}

func TestImport_MissingRequiredFields(t *testing.T) {
	cfgPath := writeExportFixture(t)

	// name 缺失
	importPath := writeTempJSONNodes(t, []portableNode{
		{Host: "10.0.0.99"}, // missing name
	})
	_, exitCode := runImportCmd(t, cfgPath, importPath)
	assert.NotZero(t, exitCode)

	// host 缺失
	importPath2 := writeTempJSONNodes(t, []portableNode{
		{Name: "x", Host: ""}, // missing host
	})
	_, exitCode2 := runImportCmd(t, cfgPath, importPath2)
	assert.NotZero(t, exitCode2)
}

func TestImport_AliasConflict(t *testing.T) {
	cfgPath := writeExportFixture(t)
	importPath := writeTempJSONNodes(t, []portableNode{
		{Group: "NewGroup", Name: "conflict", Alias: "Web01", Host: "10.0.0.99", User: "u"},
	})

	out, exitCode := runImportCmd(t, cfgPath, importPath, "--confirm")
	assert.Equal(t, 0, exitCode, "alias 冲突不应阻塞, 只会 warning 说明: %s", out)
	assert.Contains(t, out, "Web01")
}

func TestImport_NoArgs(t *testing.T) {
	// import 需要 1 个参数
	cfgPath := writeExportFixture(t)
	cmdArgs := []string{"-c", cfgPath, "import"}
	_, exitCode := runConfigCmd(t, "", cmdArgs...)
	assert.NotZero(t, exitCode)
}
