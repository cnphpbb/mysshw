package ssh

import (
	"errors"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtures returns a small config with 3 groups / 7 nodes that covers all
// selector branches: by-name, by-alias, by-host, by-host+user, mass fan-out.
func fixtures() *config.Configs {
	return &config.Configs{
		Nodes: []config.Nodes{
			{
				Groups: "Production",
				SSHNodes: []*config.SSHNode{
					{Name: "prod-1", Alias: "Web01", Host: "10.0.0.1", User: "app", Port: 22},
					{Name: "prod-2", Alias: "Web02", Host: "10.0.0.2", User: "app", Port: 22},
					{Name: "prod-3", Alias: "DB01", Host: "10.0.0.3", User: "postgres", Port: 5432},
				},
			},
			{
				Groups: "Staging",
				SSHNodes: []*config.SSHNode{
					{Name: "stag-1", Alias: "", Host: "10.0.0.10"}, // user="" 默认
					{Name: "stag-2", Alias: "", Host: "10.0.0.10", User: "root", Port: 2222},
				},
			},
			{
				Groups: "Empty",
				SSHNodes: []*config.SSHNode{
					{Name: "default-only", Host: "10.0.0.99"},
				},
			},
		},
	}
}

func TestResolveNodes_ByName_Single(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Names: []string{"prod-1"}})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "prod-1", nodes[0].Name)
}

func TestResolveNodes_ByName_MatchesAlias(t *testing.T) {
	// prod-3 的 alias="DB01", --node DB01 也应能匹配
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Names: []string{"DB01"}})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "prod-3", nodes[0].Name)
	assert.Equal(t, "DB01", nodes[0].Alias)
}

func TestResolveNodes_ByName_MixedNameAndAlias(t *testing.T) {
	// 批量 --node 可以同时混用 name 和 alias
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Names: []string{"prod-1", "Web02"}})
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	assert.Equal(t, "prod-1", nodes[0].Name)
	assert.Equal(t, "prod-2", nodes[1].Name)
}

func TestResolveNodes_ByName_NamePrecedesAlias(t *testing.T) {
	// 验证 Name 匹配优先于 Alias
	// fixtures 中 "prod-3" 的 Name="prod-3" 且 Alias="DB01"
	// --node prod-3: Name 精确匹配 → 找到 prod-3
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Names: []string{"prod-3"}})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "prod-3", nodes[0].Name)
	assert.Equal(t, "DB01", nodes[0].Alias)
}

func TestResolveNodes_ByName_Batch_AllFound(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Names: []string{"prod-1", "prod-3"}})
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	assert.Equal(t, "prod-1", nodes[0].Name)
	assert.Equal(t, "prod-3", nodes[1].Name)
}

func TestResolveNodes_ByName_Batch_PartialMissing(t *testing.T) {
	// Names 批量: 部分找不到时返回能找到的 + 软错误
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Names: []string{"prod-1", "nonexistent", "prod-2"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
	require.Len(t, nodes, 2)
	assert.Equal(t, "prod-1", nodes[0].Name)
	assert.Equal(t, "prod-2", nodes[1].Name)
}

func TestResolveNodes_ByName_Batch_Dedup(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Names: []string{"prod-1", "prod-1", "prod-1"}})
	require.NoError(t, err)
	require.Len(t, nodes, 1, "重复 name 应去重")
}

func TestResolveNodes_ByName_AllMissing(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Names: []string{"ghost-1", "ghost-2"}})
	assert.Nil(t, nodes)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNodeNotFound))
}

func TestResolveNodes_ByAlias(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Alias: "DB01"})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "prod-3", nodes[0].Name)
}

func TestResolveNodes_ByAlias_NotFound(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Alias: "NopeAlias"})
	assert.Nil(t, nodes)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNodeNotFound))
}

func TestResolveNodes_ByHost_Only(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Host: "10.0.0.10"})
	// 10.0.0.10 上有两个节点 (stag-1, stag-2), 应都返回
	require.NoError(t, err)
	require.Len(t, nodes, 2)
}

func TestResolveNodes_ByHost_UserTightens(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Host: "10.0.0.10", User: "root"})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "stag-2", nodes[0].Name)
}

func TestResolveNodes_ByHost_PortTightens(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Host: "10.0.0.10", Port: 2222})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "stag-2", nodes[0].Name)
}

func TestResolveNodes_ByHost_UserFallback(t *testing.T) {
	// staging 上 stag-1 user="" 默认, stag-2 user="root"
	// 查 user="ops" 时, 没匹配; 应回退到 user="" 的节点 (stag-1)
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Host: "10.0.0.10", User: "ops"})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "stag-1", nodes[0].Name)
}

func TestResolveNodes_ByHost_NoMatch(t *testing.T) {
	nodes, err := ResolveNodes(fixtures(), NodeSelector{Host: "203.0.113.1"})
	assert.Nil(t, nodes)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNodeNotFound))
}

func TestResolveNodes_Empty_ReturnsAll(t *testing.T) {
	// 不指定任何 selector: 兜底返回全部 (mass fan-out)
	nodes, err := ResolveNodes(fixtures(), NodeSelector{})
	require.NoError(t, err)
	assert.Len(t, nodes, 6) // 3 + 2 + 1
}

func TestResolveNodes_NilConfig(t *testing.T) {
	nodes, err := ResolveNodes(nil, NodeSelector{Names: []string{"x"}})
	assert.Nil(t, nodes)
	require.Error(t, err)
}

func TestResolveNodes_EmptyConfig(t *testing.T) {
	nodes, err := ResolveNodes(&config.Configs{}, NodeSelector{Host: "x"})
	assert.Nil(t, nodes)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNodeNotFound))
}

func TestNodeSelector_IsEmpty(t *testing.T) {
	assert.True(t, NodeSelector{}.IsEmpty())
	assert.True(t, NodeSelector{Port: 0}.IsEmpty())
	assert.False(t, NodeSelector{Names: []string{"x"}}.IsEmpty())
	assert.False(t, NodeSelector{Host: "x"}.IsEmpty())
	assert.False(t, NodeSelector{User: "root"}.IsEmpty())
}

func TestSummaryLine(t *testing.T) {
	n := &config.SSHNode{Name: "prod", Alias: "Web", Host: "10.0.0.5", User: "app", Port: 2222}
	line := SummaryLine(n)
	assert.Contains(t, line, "name=prod")
	assert.Contains(t, line, "alias=Web")
	assert.Contains(t, line, "app@10.0.0.5:2222")
}

func TestSummaryLine_DefaultsFilled(t *testing.T) {
	n := &config.SSHNode{Name: "test", Host: "1.2.3.4"} // user+port 缺省
	line := SummaryLine(n)
	assert.Contains(t, line, "root@1.2.3.4:22")
}

func TestSummaryLine_NilSafe(t *testing.T) {
	assert.Equal(t, "<nil>", SummaryLine(nil))
}
