package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	pelletiertoml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== SerializeConfigs ====================

func TestSerializeConfigs_Empty(t *testing.T) {
	data, err := SerializeConfigs(&Configs{})
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	// 至少应包含 cfg_dir (即使为空也有 "")
	assert.Contains(t, string(data), "cfg_dir")
}

func TestSerializeConfigs_Nodes(t *testing.T) {
	c := &Configs{
		CfgDir: "~/.mysshw.toml",
		Nodes: []Nodes{
			{Groups: "Production", SSHNodes: []*SSHNode{
				{Name: "node1", Host: "10.0.0.1", User: "root", Password: "${MY_VAR}"},
			}},
		},
	}
	data, err := SerializeConfigs(c)
	require.NoError(t, err)
	out := string(data)
	assert.Contains(t, out, "[[nodes]]")
	assert.Contains(t, out, "groups = 'Production'")
	assert.Contains(t, out, "[[nodes.ssh]]")
	assert.Contains(t, out, "name = 'node1'")
	assert.Contains(t, out, "host = '10.0.0.1'")
	assert.Contains(t, out, "user = 'root'")
	assert.Contains(t, out, "password = '${MY_VAR}'")
}

func TestSerializeConfigs_PreservesEnvReferences(t *testing.T) {
	c := &Configs{
		Nodes: []Nodes{
			{Groups: "T", SSHNodes: []*SSHNode{
				{Name: "n1", Host: "h", Password: "${MYSSHW_PWD_xxx}"},
			}},
		},
	}
	data, _ := SerializeConfigs(c)
	out := string(data)
	assert.Contains(t, out, "${MYSSHW_PWD_xxx}")
}

func TestSerializeConfigs_PreservesBcryptHash(t *testing.T) {
	hash, _ := HashPassword("test")
	c := &Configs{
		Nodes: []Nodes{
			{Groups: "T", SSHNodes: []*SSHNode{
				{Name: "n1", Host: "h", Password: hash},
			}},
		},
	}
	data, _ := SerializeConfigs(c)
	out := string(data)
	// bcrypt 哈希应以单引号字符串输出 (不转义 $ 字符)
	assert.Contains(t, out, "'"+hash+"'")
}

func TestSerializeConfigs_Nil(t *testing.T) {
	_, err := SerializeConfigs(nil)
	assert.Error(t, err)
}

// ==================== validateAndSerialize ====================

func TestValidateAndSerialize_Valid(t *testing.T) {
	c := &Configs{
		CfgDir: "~/.mysshw.toml",
		Nodes: []Nodes{
			{Groups: "Test", SSHNodes: []*SSHNode{
				{Name: "n1", Host: "10.0.0.1"},
			}},
		},
	}
	data, err := validateAndSerialize(c)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestValidateAndSerialize_Invalid(t *testing.T) {
	// 节点名缺失 → validator 拒绝
	c := &Configs{
		Nodes: []Nodes{
			{Groups: "T", SSHNodes: []*SSHNode{
				{Host: "10.0.0.1"}, // 缺 name
			}},
		},
	}
	_, err := validateAndSerialize(c)
	assert.Error(t, err, "缺失 name 字段应被 ValidateConfig 拒绝")
}

func TestValidateAndSerialize_Nil(t *testing.T) {
	_, err := validateAndSerialize(nil)
	assert.Error(t, err)
}

// ==================== ReadConfigOrEmpty ====================

func TestReadConfigOrEmpty_NotExist(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ReadConfigOrEmpty(filepath.Join(dir, "missing.toml"))
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Empty(t, cfg.Nodes)
}

func TestReadConfigOrEmpty_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	// 注: 必须用 [[nodes]] (array of tables), [nodes] (inline table) 在 pelletiertoml/v2 中不支持嵌套数组
	content := `[[nodes]]
groups = "T"
[[nodes.ssh]]
name = "test"
host = "1.2.3.4"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := ReadConfigOrEmpty(path)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.Nodes)
	assert.Equal(t, "T", cfg.Nodes[0].Groups)
	assert.Equal(t, "test", cfg.Nodes[0].SSHNodes[0].Name)
}

func TestReadConfigOrEmpty_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	require.NoError(t, os.WriteFile(path, []byte("not = valid = toml = syntax"), 0600))

	_, err := ReadConfigOrEmpty(path)
	assert.Error(t, err)
}

func TestReadConfigOrEmpty_InlineArrayFails(t *testing.T) {
	// 注: 旧版用户可能写过 [nodes]groups="T"ssh=[{...}] inline array 语法
	// 这在 pelletiertoml/v2 + 新 schema 下解析失败
	dir := t.TempDir()
	path := filepath.Join(dir, "old-format.toml")
	oldFormat := `[nodes]
groups = "T"
ssh = [{name="n1",host="1.2.3.4"}]
`
	require.NoError(t, os.WriteFile(path, []byte(oldFormat), 0600))

	_, err := ReadConfigOrEmpty(path)
	assert.Error(t, err, "旧版 [nodes] inline ssh=[] 不被支持")
}

// ==================== Round-Trip 闭环 ====================

func TestRoundTrip_PlainConfig(t *testing.T) {
	original := &Configs{
		CfgDir:              "~/.mysshw.toml",
		KnownHostsPath:      "/tmp/known_hosts",
		BackupKeep:          3,
		EncryptionAlgorithm: "bcrypt-pepper",
		SecretKey:           "${MYSSHW_SECRET_KEY}",
		Cipher:              "aes-256-gcm",
		Nodes: []Nodes{
			{Groups: "Production", SSHNodes: []*SSHNode{
				{Name: "prod-1", Alias: "Web01", Host: "10.0.0.1", User: "app", Port: 22, Password: "${PWD_1}"},
				{Name: "prod-2", Host: "10.0.0.2", User: "root", Proxy: "prod-1"},
			}},
			{Groups: "Staging", SSHNodes: []*SSHNode{
				{Name: "stag-1", Host: "192.168.1.1"},
			}},
		},
	}

	// 序列化
	data, err := validateAndSerialize(original)
	require.NoError(t, err)

	// 反序列化
	var parsed Configs
	require.NoError(t, readTOMLInto(data, &parsed))

	// 对比: 关键字段应一致
	assert.Equal(t, original.CfgDir, parsed.CfgDir)
	assert.Equal(t, original.KnownHostsPath, parsed.KnownHostsPath)
	assert.Equal(t, original.BackupKeep, parsed.BackupKeep)
	assert.Equal(t, original.EncryptionAlgorithm, parsed.EncryptionAlgorithm)
	assert.Equal(t, original.SecretKey, parsed.SecretKey)
	assert.Equal(t, original.Cipher, parsed.Cipher)
	assert.Equal(t, len(original.Nodes), len(parsed.Nodes))
	for i, g := range original.Nodes {
		assert.Equal(t, g.Groups, parsed.Nodes[i].Groups)
		assert.Equal(t, len(g.SSHNodes), len(parsed.Nodes[i].SSHNodes))
		for j, n := range g.SSHNodes {
			assert.Equal(t, n.Name, parsed.Nodes[i].SSHNodes[j].Name)
			assert.Equal(t, n.Host, parsed.Nodes[i].SSHNodes[j].Host)
			assert.Equal(t, n.User, parsed.Nodes[i].SSHNodes[j].User)
			assert.Equal(t, n.Port, parsed.Nodes[i].SSHNodes[j].Port)
			assert.Equal(t, n.Password, parsed.Nodes[i].SSHNodes[j].Password)
			assert.Equal(t, n.Proxy, parsed.Nodes[i].SSHNodes[j].Proxy)
		}
	}
}

// ==================== WriteConfigAtomic ====================

func TestWriteConfigAtomic_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	c := &Configs{
		CfgDir: "~/.mysshw.toml",
		Nodes: []Nodes{
			{Groups: "T", SSHNodes: []*SSHNode{
				{Name: "n1", Host: "10.0.0.1"},
			}},
		},
	}
	err := WriteConfigAtomic(path, c)
	require.NoError(t, err)

	// 验证文件存在且内容正确
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed Configs
	require.NoError(t, readTOMLInto(data, &parsed))
	assert.Len(t, parsed.Nodes, 1)
}

func TestWriteConfigAtomic_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	original := "[[nodes]]\ngroups = \"T1\"\n[[nodes.ssh]]\nname = \"n1\"\nhost = \"1.1.1.1\"\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0600))

	// 写新配置
	c := &Configs{
		CfgDir: "~/.mysshw.toml",
		Nodes: []Nodes{
			{Groups: "T2", SSHNodes: []*SSHNode{
				{Name: "n2", Host: "2.2.2.2"},
			}},
		},
	}
	err := WriteConfigAtomic(path, c)
	require.NoError(t, err)

	// 验证新内容
	data, _ := os.ReadFile(path)
	var parsed Configs
	require.NoError(t, readTOMLInto(data, &parsed))
	assert.Len(t, parsed.Nodes, 1)
	assert.Equal(t, "T2", parsed.Nodes[0].Groups)
	assert.Equal(t, "n2", parsed.Nodes[0].SSHNodes[0].Name)

	// 备份应存在
	backups, _ := filepath.Glob(path + ".*.bak")
	assert.NotEmpty(t, backups, "备份文件应生成")
}

func TestWriteConfigAtomic_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysshw.toml")
	c := &Configs{
		Nodes: []Nodes{
			{Groups: "T", SSHNodes: []*SSHNode{
				{Host: "10.0.0.1"}, // 缺 name
			}},
		},
	}
	err := WriteConfigAtomic(path, c)
	assert.Error(t, err, "无效配置应被拒绝")

	// 验证文件未被创建
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "无效配置不应写文件")
}

// ==================== helpers ====================

func TestHasBcryptOrEnv(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"", false},
		{"plain-pwd", false},
		{"$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true}, // bcrypt 60 字符
		{"${MYSSHW_PWD_x}", true},
		{"${}", true},
		{" ${MY_X}", true}, // trim 后是合法 env 引用
		{" \t\n ", false},  // 仅空白
	}
	for _, c := range cases {
		got := hasBcryptOrEnv(c.input)
		assert.Equal(t, c.want, got, "input=%q", c.input)
	}
}

// ==================== 内部辅助 ====================

// readTOMLInto 包装 pelletiertoml.Unmarshal, 便于测试直接调用
func readTOMLInto(data []byte, v interface{}) error {
	return pelletiertoml.Unmarshal(data, v)
}

var _ = strings.Contains
