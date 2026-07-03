package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixturesConfigs() *config.Configs {
	return &config.Configs{
		Nodes: []config.Nodes{
			{
				Groups: "Prod",
				SSHNodes: []*config.SSHNode{
					{Name: "web-1", Alias: "Web01", Host: "10.0.0.1", User: "app", Port: 22},
					{Name: "db-1", Alias: "DB01", Host: "10.0.0.2", User: "pg", Port: 5432, Password: "${DB_PWD}"},
					{Name: "key-1", Host: "10.0.0.3", User: "ops", KeyPath: "~/.ssh/id_rsa"},
				},
			},
			{
				Groups: "Empty", SSHNodes: nil,
			},
		},
	}
}

func TestClassifyAuth(t *testing.T) {
	cases := []struct {
		name string
		node *config.SSHNode
		want string
	}{
		{"empty+key", &config.SSHNode{Password: "", KeyPath: "/k"}, "key"},
		{"empty", &config.SSHNode{Password: ""}, "empty"},
		{"bcrypt", &config.SSHNode{Password: "$2a$10$abcdefghijklmnopqrstuvABCDEFGHIJKLMNOPQRSTUVWXYZ12345"}, "bcrypt"},
		{"env", &config.SSHNode{Password: "${ENV_VAR}"}, "env"},
		{"plain", &config.SSHNode{Password: "secret"}, "password"},
		{"plain+key", &config.SSHNode{Password: "secret", KeyPath: "/k"}, "password+key"},
		{"env+key", &config.SSHNode{Password: "${E}", KeyPath: "/k"}, "env+key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyAuth(tc.node))
		})
	}
}

func TestBuildNodeSummaries_OmitsPassword(t *testing.T) {
	out := buildNodeSummaries(fixturesConfigs())
	// 序列化整个 slice 然后扫描确认 password 字段绝不在 JSON 中
	b, err := json.Marshal(out)
	require.NoError(t, err)
	s := string(b)
	assert.NotContains(t, s, "password", "JSON schema 中不应出现 password 字段")
	assert.NotContains(t, s, "${DB_PWD}", "JSON schema 中不应出现 env 变量名")
}

func TestBuildNodeSummaries_PopulatesAll(t *testing.T) {
	out := buildNodeSummaries(fixturesConfigs())
	require.NotEmpty(t, out)
	for _, s := range out {
		assert.NotEmpty(t, s.Name, "name 必填")
		assert.NotEmpty(t, s.Host, "host 必填")
		assert.GreaterOrEqual(t, s.Port, 1, "port 默认 22")
		assert.NotEmpty(t, s.Auth, "auth 必填")
	}
}

func TestFilterSummaries_Group(t *testing.T) {
	in := buildNodeSummaries(fixturesConfigs())
	out := filterSummaries(in, "Prod", "")
	require.Len(t, out, 3)
	for _, s := range out {
		assert.Equal(t, "Prod", s.Group)
	}
}

func TestFilterSummaries_GroupMissing(t *testing.T) {
	in := buildNodeSummaries(fixturesConfigs())
	out := filterSummaries(in, "Nope", "")
	assert.Empty(t, out)
}

func TestFilterSummaries_Search(t *testing.T) {
	in := buildNodeSummaries(fixturesConfigs())
	out := filterSummaries(in, "", "prod")
	assert.Empty(t, out, "group 名不应被 search 命中")
}

func TestFilterSummaries_SearchCaseInsensitive(t *testing.T) {
	in := buildNodeSummaries(fixturesConfigs())
	// 节点 user 含 "app", host 含 "10.0.0"
	out := filterSummaries(in, "", "APP")
	require.NotEmpty(t, out)
	assert.Equal(t, "web-1", out[0].Name, "大小写不敏感匹配 user")
}

func TestSortSummaries_Stable(t *testing.T) {
	in := []nodeSummary{
		{Group: "Z", Name: "a"},
		{Group: "A", Name: "b"},
		{Group: "A", Name: "a"},
	}
	sortSummaries(in)
	require.Len(t, in, 3)
	// 期望 (group ASC, name ASC) 顺序
	assert.Equal(t, "A", in[0].Group)
	assert.Equal(t, "a", in[0].Name, "Group A 内按 Name 排序: a 在前")
	assert.Equal(t, "A", in[1].Group)
	assert.Equal(t, "b", in[1].Name)
	assert.Equal(t, "Z", in[2].Group)
	assert.Equal(t, "a", in[2].Name)
}

func TestPrintNodeTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	err := printNodeTable(&buf, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no nodes match")
}

func TestPrintNodeTable_Basic(t *testing.T) {
	out := buildNodeSummaries(fixturesConfigs())
	sortSummaries(out)
	var buf bytes.Buffer
	err := printNodeTable(&buf, out)
	require.NoError(t, err)
	s := buf.String()
	for _, sub := range []string{"Prod", "web-1", "db-1", "key-1", "env", "key"} {
		assert.Contains(t, s, sub)
	}
	assert.Contains(t, s, "GROUP")
	assert.Contains(t, s, "NAME")
}

func TestGetOutputFormat_DefaultPlain(t *testing.T) {
	assert.Equal(t, "plain", string(OutputPlain))
	assert.Equal(t, "json", string(OutputJSON))
}
