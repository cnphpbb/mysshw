package config

import (
	"testing"

	"github.com/GuanceCloud/toml"
	"github.com/stretchr/testify/assert"
)

// ptrBool 返回 b 的指针, 用于 *bool 字段的测试构造
func ptrBool(b bool) *bool {
	return &b
}

func TestSSHNode_IsStrictHostKeyChecking(t *testing.T) {
	testCases := []struct {
		name     string
		node     SSHNode
		expected bool
	}{
		{
			name:     "nil 默认为严格 (true)",
			node:     SSHNode{},
			expected: true,
		},
		{
			name:     "显式 true (启用严格)",
			node:     SSHNode{StrictHostKeyChecking: ptrBool(true)},
			expected: true,
		},
		{
			name:     "显式 false (跳过校验)",
			node:     SSHNode{StrictHostKeyChecking: ptrBool(false)},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.node.IsStrictHostKeyChecking())
		})
	}
}

// TestSCPConfigKeypathUnmarshal 验证 [sync.scp] 的 keypath (小写) tag 正确解析
// 修复前: tag 是 keyPath (大写 P), 与 SSHNode.keypath (全小写) 不一致
// 修复后: 统一为 keypath, 与其他字段保持一致
func TestSCPConfigKeypathUnmarshal(t *testing.T) {
	tomlData := `
[sync.scp]
username = "root"
password = "test_pwd"
keypath = "/tmp/ssh_key"
passphrase = "key_pass"
`
	var c Configs
	err := toml.Unmarshal([]byte(tomlData), &c)
	assert.NoError(t, err)
	assert.Equal(t, "root", c.SyncCfg.SCPConfig.Username)
	assert.Equal(t, "test_pwd", c.SyncCfg.SCPConfig.Password)
	assert.Equal(t, "/tmp/ssh_key", c.SyncCfg.SCPConfig.KeyPath)
	assert.Equal(t, "key_pass", c.SyncCfg.SCPConfig.Passphrase)
}

func TestSSHNodeKeypathUnmarshal(t *testing.T) {
	tomlData := `
[[nodes]]
groups = "test"

[[nodes.ssh]]
name = "vm1"
host = "127.0.0.1"
user = "root"
keypath = "/tmp/ssh_key"
passphrase = "key_pass"
`
	var c Configs
	err := toml.Unmarshal([]byte(tomlData), &c)
	assert.NoError(t, err)
	assert.Len(t, c.Nodes, 1)
	assert.Len(t, c.Nodes[0].SSHNodes, 1)
	assert.Equal(t, "/tmp/ssh_key", c.Nodes[0].SSHNodes[0].KeyPath)
	assert.Equal(t, "key_pass", c.Nodes[0].SSHNodes[0].Passphrase)
}
