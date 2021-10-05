
package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"os"

	"mysshw/cmd"
	"mysshw/config"
	"mysshw/ssh"
)

func main() {
	if len(os.Args) > 1 {
		app := &cli.App{
			Name: "mysshw",
			Usage: "a free and open source ssh cli client soft.",
			Version: "v0.0.1",
			UseShortOptionHandling: true,
			Flags: cmd.GlobalOptions,
			Before: cmd.LoadGlobalOptions,
			Commands: cmd.Commands,
		}
		err := app.Run(os.Args)
		if err != nil && err != cmd.ErrPrintAndExit {
			fmt.Println(err)
		}
	}else{
		SSHRUN()
	}
}

func SSHRUN() {
	config.CFG_PATH = "build/.mysshw.toml"
	config.LoadConfig()
	node := ssh.Choose(config.CFG)
	client := ssh.NewClient(node)
	client.Login()
}
