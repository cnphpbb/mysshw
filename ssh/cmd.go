package ssh

import (
	"fmt"
	"mysshw/config"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var (
	// 定义样式
	// 标题样式 #71BEF2
	//titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#71BEF2")).Bold(true)
	// greenStyle #A8CC8C
	//greenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#A8CC8C"))
	// 黄色样式 #DBAB79
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#DBAB79"))
	// 灰色样式
	//faintStyle = lipgloss.NewStyle().Faint(true)
	// 蓝色样式 #71BEF2
	blueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#71BEF2"))
	// 父节点样式 #585858
	//parentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	parentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#585858")).Italic(true)
)

// Choose 交互式选择SSH节点
// 参数 trees 是配置文件中的所有节点组
// 返回选中的SSH节点，用户取消操作时返回nil
func Choose(trees *config.Configs) *config.SSHNode {
	// 选择节点组
	groups := make([]string, len(trees.Nodes))
	for i, node := range trees.Nodes {
		groups[i] = node.Groups
	}

	// 转换为huh.Option[int]类型
	groupOptions := make([]huh.Option[int], len(groups))
	for i, group := range groups {
		groupOptions[i] = huh.NewOption(group, i)
	}

	var selectedGroupIndex int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title(MsgSelectNodeGroup).
				//Description(MsgSelectDesc).
				Options(groupOptions...).
				Value(&selectedGroupIndex),
		),
	)

	err := form.Run()
	if err != nil {
		// 处理用户取消操作
		if err.Error() == errFormRunError {
			fmt.Println(MsgPrintLnStr)
		}
		return nil
	}

	// 选择SSH节点
	cTrees := trees.Nodes[selectedGroupIndex].SSHNodes

	// 创建带返回上级选项的节点列表
	nodesWithParent := make([]*config.SSHNode, len(cTrees)+1)
	nodesWithParent[0] = &config.SSHNode{Name: NodeParentName, Alias: NodeParentAlias}
	copy(nodesWithParent[1:], cTrees)

	nodeOptions := make([]huh.Option[int], len(nodesWithParent))
	for i, node := range nodesWithParent {
		name := node.Name
		if i == 0 {
			// 特殊处理返回上级选项
			name = parentStyle.Render(name + " " + node.Alias)
		} else {
			if node.Alias != "" {
				name += yellowStyle.Render("(" + node.Alias + ")")
			}
			if node.Host != "" {
				userHost := ""
				if node.User != "" {
					userHost += blueStyle.Render(node.User + "@")
				}
				userHost += blueStyle.Render(node.Host)
				name += " " + userHost
			}
		}
		nodeOptions[i] = huh.NewOption(name, i)
	}

	var selectedNodeIndex int
	nodeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title(MsgSelectNode).
				//Description(MsgSelectDesc).
				Options(nodeOptions...).
				Value(&selectedNodeIndex),
		),
	)

	err = nodeForm.Run()
	if err != nil {
		// 处理用户取消操作
		if err.Error() == errFormRunError {
			fmt.Println(MsgPrintLnStr)
		}
		return nil
	}

	if nodesWithParent[selectedNodeIndex].Name == NodeParentName {
		return Choose(trees)
	}
	return nodesWithParent[selectedNodeIndex]
}
