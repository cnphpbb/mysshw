package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"mysshw/config"
)

// TestPreferredHostKeyAlgorithms_Ed25519First 主机密钥算法必须 ed25519 优先
//
// 背景 (真实故障):
//
//	x/crypto/ssh 的默认算法列表把 `ssh-ed25519` 放在**最后**, 而 OpenSSH 客户端
//	放在**最前**。SSH 握手时服务端按**客户端**给的列表顺序挑算法, 于是:
//	  - OpenSSH 协商出 ed25519 host key
//	  - mysshw 协商出 ecdsa-sha2-nistp256
//	→ 两者写进 known_hosts 的指纹不同 → 用 OpenSSH 写的 known_hosts 校验
//	  mysshw 的连接时必然报 "主机指纹与 known_hosts 不匹配" (疑似 MITM 告警)
//
// 该测试锁死 ed25519 必须是首个算法。
func TestPreferredHostKeyAlgorithms_Ed25519First(t *testing.T) {
	require.NotEmpty(t, PreferredHostKeyAlgorithms)
	assert.Equal(t, "ssh-ed25519", PreferredHostKeyAlgorithms[0],
		"ed25519 必须是首选主机密钥算法 (与 OpenSSH 客户端一致)")
}

// TestPreferredHostKeyAlgorithms_Order 校验完整顺序与 OpenSSH 对齐
func TestPreferredHostKeyAlgorithms_Order(t *testing.T) {
	expected := []string{
		"ssh-ed25519",
		"ecdsa-sha2-nistp256",
		"ecdsa-sha2-nistp384",
		"ecdsa-sha2-nistp521",
		"rsa-sha2-512",
		"rsa-sha2-256",
		"ssh-rsa",
	}
	for i, want := range expected {
		require.Greater(t, len(PreferredHostKeyAlgorithms), i, "列表长度不足")
		assert.Equal(t, want, PreferredHostKeyAlgorithms[i], "第 %d 个算法应为 %s", i, want)
	}
}

// TestPreferredHostKeyAlgorithms_AllSupported 列表中的算法必须都被 x/crypto 支持
func TestPreferredHostKeyAlgorithms_AllSupported(t *testing.T) {
	supported := map[string]bool{
		gossh.KeyAlgoED25519:       true,
		gossh.KeyAlgoECDSA256:      true,
		gossh.KeyAlgoECDSA384:      true,
		gossh.KeyAlgoECDSA521:      true,
		gossh.KeyAlgoRSASHA256:     true,
		gossh.KeyAlgoRSASHA512:     true,
		gossh.KeyAlgoRSA:           true,
		gossh.CertAlgoED25519v01:   true,
		gossh.CertAlgoECDSA256v01:  true,
		gossh.CertAlgoECDSA384v01:  true,
		gossh.CertAlgoECDSA521v01:  true,
		gossh.CertAlgoRSASHA256v01: true,
		gossh.CertAlgoRSASHA512v01: true,
		gossh.CertAlgoRSAv01:       true,
	}
	for _, alg := range PreferredHostKeyAlgorithms {
		assert.True(t, supported[alg], "x/crypto/ssh 不支持算法 %s", alg)
	}
}

// TestPreferredHostKeyAlgorithms_ReturnsCopy 返回副本, 防止调用方污染全局
func TestPreferredHostKeyAlgorithms_ReturnsCopy(t *testing.T) {
	first := preferredHostKeyAlgorithms()
	require.NotEmpty(t, first)

	first[0] = "MUTATED"
	second := preferredHostKeyAlgorithms()

	assert.Equal(t, "ssh-ed25519", second[0], "包级变量被调用方污染了")
	assert.Equal(t, "ssh-ed25519", PreferredHostKeyAlgorithms[0])
}

// TestGenSSHConfig_UsesPreferredHostKeyAlgorithms 主 SSH 路径必须应用该偏好
func TestGenSSHConfig_UsesPreferredHostKeyAlgorithms(t *testing.T) {
	withMockHome(t)

	node := &config.SSHNode{
		Name:     "algo-test",
		Host:     "192.0.2.10",
		Port:     22,
		User:     "root",
		Password: "dummy",
	}

	c := NewClientNonInteractive(node)
	require.NotNil(t, c)

	dc, ok := c.(*defaultClient)
	require.True(t, ok)
	require.NotNil(t, dc.clientConfig)

	require.NotEmpty(t, dc.clientConfig.HostKeyAlgorithms)
	assert.Equal(t, "ssh-ed25519", dc.clientConfig.HostKeyAlgorithms[0],
		"主 SSH 路径必须使用 ed25519 优先的主机密钥算法")
}
