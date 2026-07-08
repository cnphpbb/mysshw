package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// validBaseSSHNode 返回一个最小的合法 SSH 节点
func validBaseSSHNode() *SSHNode {
	return &SSHNode{
		Name: "test-node",
		Host: "127.0.0.1",
		Port: 22,
		User: "root",
	}
}

// validBaseConfig 返回一个最小的合法 Configs
func validBaseConfig() *Configs {
	return &Configs{
		SyncCfg: SyncInfo{
			Type:       "scp",
			RemoteUri:  "127.0.0.1:22",
			RemotePath: "/tmp/backup",
			SCPConfig:  SCPConfig{Username: "root", Password: "test_pwd"},
		},
		Nodes: []Nodes{
			{
				Groups:   "test",
				SSHNodes: []*SSHNode{validBaseSSHNode()},
			},
		},
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	err := ValidateConfig(validBaseConfig())
	assert.NoError(t, err)
}

func TestValidateConfig_NoNodes(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Nodes = nil
	err := ValidateConfig(cfg)
	assert.NoError(t, err, "0 nodes is valid (e.g. after removing all nodes)")
}

func TestValidateNodeGroup_EmptyGroupName(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Nodes[0].Groups = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "group name")
}

func TestValidateNodeGroup_NoSSHNodes(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Nodes[0].SSHNodes = nil
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no SSH nodes")
}

func TestValidateSSHNode_EmptyName(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Nodes[0].SSHNodes[0].Name = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no name")
}

func TestValidateSSHNode_EmptyHost(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Nodes[0].SSHNodes[0].Host = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no host")
}

func TestValidateSSHNode_InvalidPort(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Nodes[0].SSHNodes[0].Port = 99999
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid port")
}

func TestValidateSSHNode_NegativePort(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Nodes[0].SSHNodes[0].Port = -1
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid port")
}

func TestValidateSSHNode_KeyFileNotFound(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Nodes[0].SSHNodes[0].KeyPath = "/nonexistent/path/id_rsa"
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key file not found")
}

func TestValidateSSHNode_KeyFileExists(t *testing.T) {
	// 创建临时 key 文件
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_rsa")
	err := os.WriteFile(keyPath, []byte("fake key"), 0600)
	assert.NoError(t, err)

	cfg := validBaseConfig()
	cfg.Nodes[0].SSHNodes[0].KeyPath = keyPath
	err = ValidateConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateSSHNode_KeyPathWindowsBackslash(t *testing.T) {
	// 验证 \\ 自动转为 /
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_rsa")
	err := os.WriteFile(keyPath, []byte("fake key"), 0600)
	assert.NoError(t, err)

	cfg := validBaseConfig()
	cfg.Nodes[0].SSHNodes[0].KeyPath = keyPath + ".bak" // 不存在
	err = ValidateConfig(cfg)
	assert.Error(t, err)

	// 即使传 Windows 风格路径也应能正确识别
	cfg.Nodes[0].SSHNodes[0].KeyPath = filepath.ToSlash(keyPath) + ".bak"
	err = ValidateConfig(cfg)
	assert.Error(t, err)
}

func TestValidateSyncConfig_EmptyType(t *testing.T) {
	// type 为空时跳过 sync 校验
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = ""
	err := ValidateConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateSyncConfig_UnsupportedType(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "github"
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported sync type")
}

func TestValidateSyncConfig_SCP_MissingRemoteURI(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "scp"
	cfg.SyncCfg.RemoteUri = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remote_uri")
}

func TestValidateSyncConfig_SCP_MissingRemotePath(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "scp"
	cfg.SyncCfg.RemotePath = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remote_path")
}

func TestValidateSyncConfig_SCP_MissingAllAuth(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "scp"
	cfg.SyncCfg.SCPConfig.Username = ""
	cfg.SyncCfg.SCPConfig.Password = ""
	cfg.SyncCfg.SCPConfig.KeyPath = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required for scp")
}

func TestValidateSyncConfig_SCP_UsernameOnly(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "scp"
	cfg.SyncCfg.SCPConfig.Username = "root"
	err := ValidateConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateSyncConfig_S3_MissingAccessKey(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "s3"
	cfg.SyncCfg.S3Config.AccessKey = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access_key")
}

func TestValidateSyncConfig_S3_MissingSecretKey(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "s3"
	cfg.SyncCfg.S3Config.AccessKey = "ak"
	cfg.SyncCfg.S3Config.SecretKey = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret_key")
}

func TestValidateSyncConfig_S3_MissingBucketName(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "s3"
	cfg.SyncCfg.S3Config.AccessKey = "ak"
	cfg.SyncCfg.S3Config.SecretKey = "sk"
	cfg.SyncCfg.S3Config.BucketName = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bucket_name")
}

func TestValidateSyncConfig_S3_MissingRemotePath(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "s3"
	cfg.SyncCfg.S3Config.AccessKey = "ak"
	cfg.SyncCfg.S3Config.SecretKey = "sk"
	cfg.SyncCfg.S3Config.BucketName = "bkt"
	cfg.SyncCfg.RemotePath = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remote_path")
}

func TestValidateSyncConfig_S3_MissingEndpointAndRemoteURI(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "s3"
	cfg.SyncCfg.S3Config.AccessKey = "ak"
	cfg.SyncCfg.S3Config.SecretKey = "sk"
	cfg.SyncCfg.S3Config.BucketName = "bkt"
	cfg.SyncCfg.RemotePath = "backup/"
	cfg.SyncCfg.S3Config.Endpoint = ""
	cfg.SyncCfg.RemoteUri = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint or remote_uri")
}

func TestValidateSyncConfig_S3_Valid(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "s3"
	cfg.SyncCfg.S3Config.AccessKey = "ak"
	cfg.SyncCfg.S3Config.SecretKey = "sk"
	cfg.SyncCfg.S3Config.BucketName = "bkt"
	cfg.SyncCfg.S3Config.Endpoint = "https://s3.amazonaws.com"
	err := ValidateConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateSyncConfig_WebDAV_MissingAllAuth(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "webdav"
	cfg.SyncCfg.WebDAVConfig.Username = ""
	cfg.SyncCfg.WebDAVConfig.Password = ""
	err := ValidateConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webdav")
}

func TestValidateSyncConfig_WebDAV_UsernameOnly(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "webdav"
	cfg.SyncCfg.WebDAVConfig.Username = "root"
	err := ValidateConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateSyncConfig_CaseInsensitiveType(t *testing.T) {
	// 大小写不敏感
	cfg := validBaseConfig()
	cfg.SyncCfg.Type = "SCP"
	err := ValidateConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateConfigFile_ValidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "valid.toml")
	content := `
[nodes]
groups = "test"
[[nodes.ssh]]
name = "n1"
host = "127.0.0.1"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	assert.NoError(t, err)

	err = ValidateConfigFile(cfgPath)
	assert.NoError(t, err)
}

func TestValidateConfigFile_InvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "invalid.toml")
	content := `this is not valid TOML = = =`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	assert.NoError(t, err)

	err = ValidateConfigFile(cfgPath)
	assert.Error(t, err)
}

func TestValidateConfigFile_NotExist(t *testing.T) {
	err := ValidateConfigFile("/nonexistent/path.toml")
	assert.Error(t, err)
}
