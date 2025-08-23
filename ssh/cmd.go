package ssh

import (
	"mysshw/config"

	"github.com/manifoldco/promptui"
)

var (
	templates = &promptui.SelectTemplates{
		Label:    " {{ . | green}}",
		Active:   "➤ {{ .Groups | cyan  }}",
		Inactive: "{{ .Groups | faint}}",
	}
	ctemplates = &promptui.SelectTemplates{
		Label:    " {{ . | green}}",
		Active:   "➤ {{ .Name | cyan  }}{{if .Alias}}({{.Alias | yellow}}){{end}} {{if .Host}}{{if .User}}{{.User | faint}}{{`@` | faint}}{{end}}{{.Host | faint}}{{end}}",
		Inactive: "  {{.Name | faint}}{{if .Alias}}({{.Alias | faint}}){{end}} {{if .Host}}{{if .User}}{{.User | faint}}{{`@` | faint}}{{end}}{{.Host | faint}}{{end}}",
	}
)

func Choose(trees *config.Configs) *config.SSHNode {
	prompt := promptui.Select{
		Label:        " select host",
		HideSelected: true,
		Size:         20,
		Items:        trees.Nodes,
		Templates:    templates,
	}

	index, _, err := prompt.Run()
	if err != nil {
		return nil
	}

	cTrees := trees.Nodes[index].SSHNodes
	if len(cTrees) > 0 {
		first := &config.SSHNode{Name: "-parent-"}
		cTrees = append(cTrees[:0], append([]*config.SSHNode{first}, cTrees...)...)
	}
	prompt.Items = cTrees
	prompt.Templates = ctemplates
	index, _, _ = prompt.Run()
	if cTrees[index].Name == "-parent-" {
		return Choose(trees)
	}
	return cTrees[index]
}
