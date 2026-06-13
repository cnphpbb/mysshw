package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeTomlString(t *testing.T) {
	cases := []struct {
		input, expected string
	}{
		{"plain", "plain"},
		{"with'quote", "with\\'quote"},
		{"with\\backslash", "with\\\\backslash"},
		{"both\\and'", "both\\\\and\\'"},
		{"", ""},
		{"multi'quote's", "multi\\'quote\\'s"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			assert.Equal(t, c.expected, escapeTomlString(c.input))
		})
	}
}

func TestConvertYAMLToTOML_Empty(t *testing.T) {
	result, err := convertYAMLToTOML(nil)
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestConvertYAMLToTOML_SingleServerMinimal(t *testing.T) {
	groups := []YMLServerGroup{
		{
			Name: "production",
			Children: []YMLServer{
				{Name: "web1", Host: "192.168.1.10"},
			},
		},
	}
	result, err := convertYAMLToTOML(groups)
	assert.NoError(t, err)
	assert.Contains(t, result, "[[nodes]]")
	assert.Contains(t, result, `groups = "production"`)
	assert.Contains(t, result, "[[nodes.ssh]]")
	assert.Contains(t, result, "name = 'web1'")
	assert.Contains(t, result, "host = '192.168.1.10'")
	assert.Contains(t, result, "port = 22") // 默认端口
	// 最小配置不输出 password / keypath / passphrase
	assert.NotContains(t, result, "password")
	assert.NotContains(t, result, "keypath")
	assert.NotContains(t, result, "passphrase")
}

func TestConvertYAMLToTOML_FullServerFields(t *testing.T) {
	groups := []YMLServerGroup{
		{
			Name: "staging",
			Children: []YMLServer{
				{
					Alias:      "stg-web1",
					Host:       "10.0.0.5",
					Name:       "web1",
					User:       "deploy",
					Port:       2222,
					KeyPath:    "/home/deploy/.ssh/id_rsa",
					Passphrase: "secret123",
					Password:   "plain-pwd",
				},
			},
		},
	}
	result, err := convertYAMLToTOML(groups)
	assert.NoError(t, err)
	assert.Contains(t, result, "alias = 'stg-web1'")
	assert.Contains(t, result, "host = '10.0.0.5'")
	assert.Contains(t, result, "name = 'web1'")
	assert.Contains(t, result, "user = 'deploy'")
	assert.Contains(t, result, "port = 2222")
	assert.Contains(t, result, "keypath = '/home/deploy/.ssh/id_rsa'")
	assert.Contains(t, result, "passphrase = 'secret123'")
	assert.Contains(t, result, "password = 'plain-pwd'")
}

func TestConvertYAMLToTOML_MultipleGroupsAndServers(t *testing.T) {
	groups := []YMLServerGroup{
		{
			Name: "prod",
			Children: []YMLServer{
				{Name: "p1", Host: "10.0.0.1"},
				{Name: "p2", Host: "10.0.0.2"},
			},
		},
		{
			Name: "dev",
			Children: []YMLServer{
				{Name: "d1", Host: "10.0.0.100"},
			},
		},
	}
	result, err := convertYAMLToTOML(groups)
	assert.NoError(t, err)

	// 统计 [[nodes]] 数量: 应为 2
	assert.Equal(t, 2, strings.Count(result, "[[nodes]]\n"))
	// [[nodes.ssh]] 应为 3
	assert.Equal(t, 3, strings.Count(result, "[[nodes.ssh]]\n"))
	assert.Contains(t, result, `groups = "prod"`)
	assert.Contains(t, result, `groups = "dev"`)
}

func TestConvertYAMLToTOML_ServerWithSpecialChars(t *testing.T) {
	groups := []YMLServerGroup{
		{
			Name: "special",
			Children: []YMLServer{
				{
					Name:     "server's name",
					Host:     "host\\backslash",
					User:     "user'quote",
					Password: "p\\ass'word",
				},
			},
		},
	}
	result, err := convertYAMLToTOML(groups)
	assert.NoError(t, err)
	// 单引号和反斜杠应被转义
	assert.Contains(t, result, "name = 'server\\'s name'")
	assert.Contains(t, result, "host = 'host\\\\backslash'")
	assert.Contains(t, result, "user = 'user\\'quote'")
	assert.Contains(t, result, "password = 'p\\\\ass\\'word'")
}

func TestConvertYAMLToTOML_ZeroPort(t *testing.T) {
	groups := []YMLServerGroup{
		{
			Name: "p",
			Children: []YMLServer{
				{Name: "n", Host: "h", Port: 0}, // 0 触发默认 22
			},
		},
	}
	result, err := convertYAMLToTOML(groups)
	assert.NoError(t, err)
	assert.Contains(t, result, "port = 22")
}
