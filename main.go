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
	Version   = "v21.10.06"
	Build     = "master"
	BuildTime string
	GoVersion string = runtime.Version()
)
func main() {

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
		app.Action = func(ctx *cli.Context) error {
			config.CFG_PATH = ctx.Path("cfg")
			fmt.Println("started path changed to", config.CFG_PATH)
			//run
			RunSSH()
			return nil
		}
		err := app.Run(os.Args)
		if err != nil && err != cmd.ErrPrintAndExit {
			fmt.Println(err)
		}
	}else{
		RunSSH()
	}
}

func RunSSH() {
	if err := config.LoadViperConfig(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	node := ssh.Choose(config.CFG)
	client := ssh.NewClient(node)
	client.Login()
}
