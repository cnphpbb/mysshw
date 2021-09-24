package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path"
	"runtime"
	"strings"

	"mysshw"
	"mysshw/auth"
	"mysshw/scp"

	"github.com/manifoldco/promptui"
	"golang.org/x/crypto/ssh"
)

const prev = "-parent-"

var (
	Version   = "20210921"
	Build     = "devel"
	BuildTime string
	GoVersion string = runtime.Version()
	V                = flag.Bool("v", false, " show version ")
	H                = flag.Bool("h", false, " show help ")
	S                = flag.Bool("s", false, " use local ssh config '~/.ssh/config' ")
	U                = flag.Bool("u", false, " ✨ Updata sshw config '(~/|./).sshw(.yaml|.yml)' ")
	Z                = flag.Bool("z", false, " ✨ Download sshw config '(~/|./).sshw(.yaml|.yml)' ")

	log = mysshw.GetLogger()

	templates = &promptui.SelectTemplates{
		Label:    "✨ {{ . | green}}",
		Active:   "➤ {{ .Name | cyan  }}{{if .Alias}}({{.Alias | yellow}}){{end}} {{if .Host}}{{if .User}}{{.User | faint}}{{`@` | faint}}{{end}}{{.Host | faint}}{{end}}",
		Inactive: "  {{.Name | faint}}{{if .Alias}}({{.Alias | faint}}){{end}} {{if .Host}}{{if .User}}{{.User | faint}}{{`@` | faint}}{{end}}{{.Host | faint}}{{end}}",
	}
)

func findAlias(nodes []*mysshw.Node, nodeAlias string) *mysshw.Node {
	for _, node := range nodes {
		if node.Alias == nodeAlias {
			return node
		}
		if len(node.Children) > 0 {
			return findAlias(node.Children, nodeAlias)
		}
	}
	return nil
}

func newScpClient() (scp.Client, error) {
	scpNode := &mysshw.Node{
		User:     "root",
		Host:     "hwc.7y2.org",
		Port:     65535,
		Password: "Kevin@0412",
	}
	cfg, _ := auth.PasswordKey(scpNode.User, scpNode.Password, ssh.InsecureIgnoreHostKey())
	client := scp.NewClient(fmt.Sprintf("%s:%d", scpNode.Host, scpNode.Port), &cfg)
	err := client.Connect()
	if err != nil {
		log.Errorf("Couldn't establish a connection to the remote server: %s", err)
		return client, err
	}
	return client, nil
}

func main() {
	flag.Parse()
	if !flag.Parsed() {
		flag.Usage()
		return
	}

	if *H {
		flag.Usage()
		return
	}

	if *V {
		fmt.Println("sshw - ssh client wrapper for automatic login")
		fmt.Println("     Version::", Version)
		fmt.Println("     GitVersion::", Build)
		fmt.Println("     GoVersion ::", GoVersion)
		fmt.Println("     BuildTime ::", BuildTime)
		return
	}

	if *S {
		err := mysshw.LoadSshConfig()
		if err != nil {
			log.Error("load ssh config error", err)
			os.Exit(1)
		}
	} else {
		err := mysshw.LoadConfig()
		if err != nil {
			log.Error("load config error", err)
			os.Exit(1)
		}
	}
	// new function
	u, _ := user.Current()
	localPath := path.Join(u.HomeDir, ".sshw.yaml")
	remotePath := "/data/backup/mysshw/sshw.yaml"
	if *U {
		fmt.Println("mysshw:: Use Upload Local Config Remote Server!! Bigen... ")
		scp, _ := newScpClient()
		defer scp.Close()
		c, _ := mysshw.LoadConfigBytes(localPath)
		err := scp.CopyFile(bytes.NewReader(c), remotePath, "0644")
		if err != nil {
			log.Errorf("Error while copying file: %s", err)
			os.Exit(1)
		}
		fmt.Println("sshw:: Use Upload Local Config Remote Server!! End... ")
		return
	}

	if *Z {
		fmt.Println("sshw:: Use Remote Config Download Local!!  Bigen... ")
		scp, _ := newScpClient()
		defer scp.Close()
		f, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			log.Errorf("Couldn't open the output file: %s", err)
			os.Exit(1)
		}
		defer f.Close()

		err = scp.CopyFromRemote(f, remotePath)
		if err != nil {
			log.Errorf("Error Copy failed from remote: %s", err)
			os.Exit(1)
		}
		fmt.Println("sshw:: Use Remote Config Download Local!!  End... ")
		return
	}

	// login by alias
	if len(os.Args) > 1 {
		var nodeAlias = os.Args[1]
		var nodes = mysshw.GetConfig()
		var node = findAlias(nodes, nodeAlias)
		if node != nil {
			client := mysshw.NewClient(node)
			client.Login()
			return
		}
	}

	node := choose(nil, mysshw.GetConfig())
	if node == nil {
		return
	}

	client := mysshw.NewClient(node)
	client.Login()
}

func choose(parent, trees []*mysshw.Node) *mysshw.Node {
	prompt := promptui.Select{
		Label:        "select host",
		Items:        trees,
		Templates:    templates,
		Size:         20,
		HideSelected: true,
		Searcher: func(input string, index int) bool {
			node := trees[index]
			content := fmt.Sprintf("%s %s %s", node.Name, node.User, node.Host)
			if strings.Contains(input, " ") {
				for _, key := range strings.Split(input, " ") {
					key = strings.TrimSpace(key)
					if key != "" {
						if !strings.Contains(content, key) {
							return false
						}
					}
				}
				return true
			}
			if strings.Contains(content, input) {
				return true
			}
			return false
		},
	}
	index, _, err := prompt.Run()
	if err != nil {
		return nil
	}

	node := trees[index]
	if len(node.Children) > 0 {
		first := node.Children[0]
		if first.Name != prev {
			first = &mysshw.Node{Name: prev}
			node.Children = append(node.Children[:0], append([]*mysshw.Node{first}, node.Children...)...)
		}
		return choose(trees, node.Children)
	}

	if node.Name == prev {
		if parent == nil {
			return choose(nil, mysshw.GetConfig())
		}
		return choose(nil, parent)
	}

	return node
}
