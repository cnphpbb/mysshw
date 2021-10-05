
package main

import (
	"fmt"
	"os"

	"mysshw/cmd"
	"mysshw/config"
	"mysshw/ssh"

	"github.com/urfave/cli/v2"
)

func main() {
	config.CFG_PATH = "build/.mysshw.toml"
	config.LoadConfig()

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
	node := ssh.Choose(config.CFG)
	client := ssh.NewClient(node)
	client.Login()
}
