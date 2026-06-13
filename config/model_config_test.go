package config

import (
	"testing"

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
