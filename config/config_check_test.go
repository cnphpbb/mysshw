package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// testBcryptHash 一个合法的 bcrypt 哈希长度的测试串 (60 字符)
const testBcryptHash = "$2a$10$abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQ"

// ============================================================
// 关键配置项检查 (CheckConfigItems)
// ============================================================

// findIssue 按 Item 前缀查找问题
func findIssue(issues []ConfigIssue, itemPrefix string) *ConfigIssue {
	for i := range issues {
		if len(issues[i].Item) >= len(itemPrefix) && issues[i].Item[:len(itemPrefix)] == itemPrefix {
			return &issues[i]
		}
	}
	return nil
}

func TestCheckConfigItems_NilConfig(t *testing.T) {
	issues := CheckConfigItems(nil)
	assert.NotEmpty(t, issues)
	assert.Equal(t, IssueError, issues[0].Level)
}

func TestCheckConfigItems_NoNodes(t *testing.T) {
	issues := CheckConfigItems(&Configs{})
	it := findIssue(issues, "nodes")
	assert.NotNil(t, it)
	assert.Equal(t, IssueWarn, it.Level)
}

func TestCheckConfigItems_NodeMissingCredential(t *testing.T) {
	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "10.0.0.1"}},
		}},
	}
	issues := CheckConfigItems(cfg)

	// 缺凭据 → warn (不阻塞: SSH agent 也可能提供认证)
	cred := findIssue(issues, "node \"web-1\".password")
	assert.NotNil(t, cred)
	assert.Equal(t, IssueWarn, cred.Level)

	// user / port 未配置 → info
	user := findIssue(issues, "node \"web-1\".user")
	assert.NotNil(t, user)
	assert.Equal(t, IssueInfo, user.Level)

	port := findIssue(issues, "node \"web-1\".port")
	assert.NotNil(t, port)
	assert.Equal(t, IssueInfo, port.Level)
}

func TestCheckConfigItems_NodePlaintextPassword(t *testing.T) {
	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "h", User: "u", Port: 22, Password: "plain-secret"}},
		}},
	}
	issues := CheckConfigItems(cfg)
	pw := findIssue(issues, "node \"web-1\".password")
	assert.NotNil(t, pw)
	assert.Equal(t, IssueWarn, pw.Level)
	assert.Contains(t, pw.Message, "plaintext")
}

func TestCheckConfigItems_NodeEnvRefPasswordNotFlagged(t *testing.T) {
	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "h", User: "u", Port: 22, Password: "${MYSSHW_PWD_w1}"}},
		}},
	}
	issues := CheckConfigItems(cfg)
	pw := findIssue(issues, "node \"web-1\".password")
	assert.Nil(t, pw, "env 引用不应被当成明文密码")
}

func TestCheckConfigItems_NodeBcryptPasswordNotFlagged(t *testing.T) {
	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "h", User: "u", Port: 22, Password: testBcryptHash}},
		}},
	}
	issues := CheckConfigItems(cfg)
	pw := findIssue(issues, "node \"web-1\".password")
	assert.Nil(t, pw, "bcrypt 哈希不应被当成明文密码")
}

func TestCheckConfigItems_NodeInvalidPort(t *testing.T) {
	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "h", User: "u", Port: 70000, Password: "x"}},
		}},
	}
	issues := CheckConfigItems(cfg)
	port := findIssue(issues, "node \"web-1\".port")
	assert.NotNil(t, port)
	assert.Equal(t, IssueError, port.Level)
}

func TestCheckConfigItems_NodeDuplicatedName(t *testing.T) {
	cfg := &Configs{
		Nodes: []Nodes{
			{Groups: "a", SSHNodes: []*SSHNode{{Name: "dup", Host: "h1", User: "u", Port: 22, Password: "x"}}},
			{Groups: "b", SSHNodes: []*SSHNode{{Name: "dup", Host: "h2", User: "u", Port: 22, Password: "x"}}},
		},
	}
	issues := CheckConfigItems(cfg)
	dup := findIssue(issues, "node \"dup\".name")
	assert.NotNil(t, dup)
	assert.Equal(t, IssueWarn, dup.Level)
}

func TestCheckConfigItems_NodeUnknownProxy(t *testing.T) {
	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "h", User: "u", Port: 22, Password: "x", Proxy: "missing-node"}},
		}},
	}
	issues := CheckConfigItems(cfg)
	proxy := findIssue(issues, "node \"web-1\".proxy")
	assert.NotNil(t, proxy)
	assert.Equal(t, IssueWarn, proxy.Level)
}

func TestCheckConfigItems_NodeStrictHostKeyCheckingDisabled(t *testing.T) {
	off := false
	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "h", User: "u", Port: 22, Password: "x", StrictHostKeyChecking: &off}},
		}},
	}
	issues := CheckConfigItems(cfg)
	shk := findIssue(issues, "node \"web-1\".strict_host_key_checking")
	assert.NotNil(t, shk)
	assert.Equal(t, IssueWarn, shk.Level)
}

func TestCheckConfigItems_NodeKeyFileMissing(t *testing.T) {
	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "h", User: "u", Port: 22, KeyPath: "/no/such/key.pem"}},
		}},
	}
	issues := CheckConfigItems(cfg)
	kp := findIssue(issues, "node \"web-1\".keypath")
	assert.NotNil(t, kp)
	assert.Equal(t, IssueError, kp.Level)
}

func TestCheckConfigItems_NodeKeyFileExists(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "id_rsa")
	assert.NoError(t, os.WriteFile(key, []byte("key"), 0600))

	cfg := &Configs{
		Nodes: []Nodes{{
			Groups:   "prod",
			SSHNodes: []*SSHNode{{Name: "web-1", Host: "h", User: "u", Port: 22, KeyPath: key}},
		}},
	}
	issues := CheckConfigItems(cfg)
	assert.Nil(t, findIssue(issues, "node \"web-1\".keypath"))
}

// ============================================================
// sync 项检查
// ============================================================

func TestCheckConfigItems_SyncTypeMissing(t *testing.T) {
	issues := CheckConfigItems(&Configs{})
	it := findIssue(issues, "sync.type")
	assert.NotNil(t, it)
	assert.Equal(t, IssueInfo, it.Level)
}

func TestCheckConfigItems_SyncUnsupportedType(t *testing.T) {
	cfg := &Configs{SyncCfg: SyncInfo{Type: "ftp"}}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "sync.type")
	assert.NotNil(t, it)
	assert.Equal(t, IssueError, it.Level)
}

func TestCheckConfigItems_SyncScpMissingFields(t *testing.T) {
	cfg := &Configs{SyncCfg: SyncInfo{Type: "scp"}}
	issues := CheckConfigItems(cfg)
	assert.NotNil(t, findIssue(issues, "sync.remote_uri"))
	assert.NotNil(t, findIssue(issues, "sync.remote_path"))
}

func TestCheckConfigItems_SyncS3MissingFields(t *testing.T) {
	cfg := &Configs{SyncCfg: SyncInfo{Type: "s3"}}
	issues := CheckConfigItems(cfg)
	assert.NotNil(t, findIssue(issues, "sync.s3.access_key"))
	assert.NotNil(t, findIssue(issues, "sync.s3.secret_key"))
	assert.NotNil(t, findIssue(issues, "sync.s3.bucket_name"))
	assert.NotNil(t, findIssue(issues, "sync.s3.endpoint"))
}

func TestCheckConfigItems_SyncEncryptDisabled(t *testing.T) {
	off := false
	cfg := &Configs{SyncCfg: SyncInfo{Type: "scp", RemoteUri: "h", RemotePath: "p", Encrypt: &off}}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "sync.encrypt")
	assert.NotNil(t, it)
	assert.Equal(t, IssueWarn, it.Level)
}

func TestCheckConfigItems_SyncEncryptNotConfiguredIsInfo(t *testing.T) {
	cfg := &Configs{SyncCfg: SyncInfo{Type: "scp", RemoteUri: "h", RemotePath: "p"}}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "sync.encrypt")
	assert.NotNil(t, it)
	assert.Equal(t, IssueInfo, it.Level)
}

func TestCheckConfigItems_SyncPassphraseMissing(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)

	cfg := &Configs{SyncCfg: SyncInfo{Type: "scp", RemoteUri: "h", RemotePath: "p"}}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "sync.passphrase")
	assert.NotNil(t, it)
	assert.Equal(t, IssueWarn, it.Level)
}

func TestCheckConfigItems_SyncPassphraseEnvRefUnresolved(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)
	t.Setenv("MYSSHW_TEST_PW", "")
	os.Unsetenv("MYSSHW_TEST_PW")

	cfg := &Configs{SyncCfg: SyncInfo{
		Type: "scp", RemoteUri: "h", RemotePath: "p",
		Passphrase: "${MYSSHW_TEST_PW}",
	}}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "sync.passphrase")
	assert.NotNil(t, it, "env 引用未设置 → 应视为未配置")
	assert.Equal(t, IssueWarn, it.Level)
	assert.Contains(t, it.Message, "not set")
}

func TestCheckConfigItems_SyncPassphraseEnvRefResolved(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)
	t.Setenv("MYSSHW_TEST_PW", "value")

	cfg := &Configs{SyncCfg: SyncInfo{
		Type: "scp", RemoteUri: "h", RemotePath: "p",
		Passphrase: "${MYSSHW_TEST_PW}",
	}}
	issues := CheckConfigItems(cfg)
	assert.Nil(t, findIssue(issues, "sync.passphrase"), "env 引用可解析 → 不应报未配置")
}

func TestCheckConfigItems_SyncPassphrasePlaintextConfigured(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)

	cfg := &Configs{SyncCfg: SyncInfo{
		Type: "scp", RemoteUri: "h", RemotePath: "p",
		Passphrase: "literal-pass",
	}}
	issues := CheckConfigItems(cfg)
	assert.Nil(t, findIssue(issues, "sync.passphrase"))
}

// ============================================================
// 加密字段检查
// ============================================================

func TestCheckConfigItems_BcryptPepperRequiresSecretKey(t *testing.T) {
	cfg := &Configs{EncryptionAlgorithm: "bcrypt-pepper"}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "secret_key")
	assert.NotNil(t, it)
	assert.Equal(t, IssueError, it.Level)
}

func TestCheckConfigItems_BcryptPepperWithSecretKey(t *testing.T) {
	cfg := &Configs{EncryptionAlgorithm: "bcrypt-pepper", SecretKey: "pepper-value"}
	issues := CheckConfigItems(cfg)
	assert.Nil(t, findIssue(issues, "secret_key"))
}

func TestCheckConfigItems_SecretKeyWithoutAlgo(t *testing.T) {
	cfg := &Configs{SecretKey: "pepper-value"}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "secret_key")
	assert.NotNil(t, it)
	assert.Equal(t, IssueInfo, it.Level)
}

func TestCheckConfigItems_UnknownAlgorithm(t *testing.T) {
	cfg := &Configs{EncryptionAlgorithm: "rot13"}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "encryption_algorithm")
	assert.NotNil(t, it)
	assert.Equal(t, IssueWarn, it.Level)
}

func TestCheckConfigItems_UnknownCipher(t *testing.T) {
	cfg := &Configs{Cipher: "des"}
	issues := CheckConfigItems(cfg)
	it := findIssue(issues, "cipher")
	assert.NotNil(t, it)
	assert.Equal(t, IssueWarn, it.Level)
}

// ============================================================
// 文件权限检查
// ============================================================

func TestCheckConfigFilePerm_WorldReadable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mysshw.toml")
	assert.NoError(t, os.WriteFile(p, []byte("x"), 0644))

	issues := CheckConfigFilePerm(p)
	assert.NotEmpty(t, issues)
	assert.Equal(t, IssueWarn, issues[0].Level)
}

func TestCheckConfigFilePerm_Secure(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mysshw.toml")
	assert.NoError(t, os.WriteFile(p, []byte("x"), 0600))

	issues := CheckConfigFilePerm(p)
	assert.Empty(t, issues)
}

func TestCheckConfigFilePerm_NotExist(t *testing.T) {
	issues := CheckConfigFilePerm("/no/such/mysshw.toml")
	assert.NotEmpty(t, issues)
	assert.Equal(t, IssueError, issues[0].Level)
}

// ============================================================
// 辅助函数
// ============================================================

func TestCountIssuesByLevel(t *testing.T) {
	counts := CountIssuesByLevel([]ConfigIssue{
		{Level: IssueError}, {Level: IssueWarn}, {Level: IssueWarn}, {Level: IssueInfo},
	})
	assert.Equal(t, 1, counts["error"])
	assert.Equal(t, 2, counts["warn"])
	assert.Equal(t, 1, counts["info"])
}

func TestFormatConfigIssues(t *testing.T) {
	out := FormatConfigIssues(nil)
	assert.Contains(t, out, "all key configuration items are configured")

	out = FormatConfigIssues([]ConfigIssue{{
		Level: IssueError, Item: "sync.type", Message: "bad", Hint: "fix it",
	}})
	assert.Contains(t, out, "[error] sync.type: bad")
	assert.Contains(t, out, "hint: fix it")
}

func TestSyncPassphraseConfigured(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "")
	os.Unsetenv(DefaultSyncPassphraseEnv)

	assert.False(t, SyncPassphraseConfigured(""))
	assert.False(t, SyncPassphraseConfigured("   "))
	assert.True(t, SyncPassphraseConfigured("literal"))

	t.Setenv("MYSSHW_PW_TEST", "v")
	assert.True(t, SyncPassphraseConfigured("${MYSSHW_PW_TEST}"))

	os.Unsetenv("MYSSHW_PW_TEST")
	assert.False(t, SyncPassphraseConfigured("${MYSSHW_PW_TEST}"))
}

func TestSyncPassphraseConfigured_EnvVariable(t *testing.T) {
	t.Setenv(DefaultSyncPassphraseEnv, "from-env")
	// 环境变量命中 → 即使 [sync].passphrase 为空也算已配置
	assert.True(t, SyncPassphraseConfigured(""))
}

func TestResolveEnvRef(t *testing.T) {
	t.Setenv("MYSSHW_ENV_A", "AAA")
	assert.Equal(t, "AAA", ResolveEnvRef("${MYSSHW_ENV_A}"))
	assert.Equal(t, "fallback", ResolveEnvRef("${MYSSHW_ENV_MISSING_XYZ:-fallback}"))
	assert.Equal(t, "plain", ResolveEnvRef("plain"))
	assert.Equal(t, "pre-AAA-post", ResolveEnvRef("pre-${MYSSHW_ENV_A}-post"))
}

func TestIsEnvRefPlaintext(t *testing.T) {
	assert.False(t, IsEnvRefPlaintext(""))
	assert.False(t, IsEnvRefPlaintext("${MYSSHW_PWD}"))
	assert.False(t, IsEnvRefPlaintext(testBcryptHash))
	assert.True(t, IsEnvRefPlaintext("hunter2"))
}

func TestHasEnvRef(t *testing.T) {
	assert.False(t, HasEnvRef("plain"))
	assert.True(t, HasEnvRef("${A}"))
	assert.True(t, HasEnvRef("${A:-x}"))
}
