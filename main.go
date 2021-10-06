
package main

import (
	"fmt"
	"os"
	"runtime"

	"mysshw/cmd"
	"mysshw/config"
	"mysshw/ssh"

	"github.com/urfave/cli/v2"
)

var (
	Version   = "v21.10.05"
	Build     = "master"
	BuildTime string
	GoVersion string = runtime.Version()
)
func main() {
	//config.CFG_PATH = "build/.mysshw.toml"
	if err := config.LoadViperConfig(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Println(" mysshw - a free and open source ssh cli client soft.")
		fmt.Println("    - Version::", Version)
		fmt.Println("    - GitVersion::", Build)
		fmt.Println("    - GoVersion ::", GoVersion)
		fmt.Println("    - BuildTime ::", BuildTime)
	}

	if len(os.Args) > 1 {
		app := &cli.App{
			Name: "mysshw",
			Usage: "a free and open source ssh cli client soft.",
			Version: Version,
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
