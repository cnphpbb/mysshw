package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
)

// 子命令分组
const (
	cmdGroupStart = "start a working area"
	cmdGroupWork  = "work on current change"
	// ...
)

var (
	Commands = []*cli.Command{
		syncCmd,
	}

	GlobalOptions = []cli.Flag {
		&cli.PathFlag{
			Name: "cfg",
			Aliases: []string{"c"},
			Usage: "config file (default is $HOME/.mysshw.toml)",
		},
	}
	// ErrPrintAndExit 表示遇到需要打印信息并提前退出的情形，不需要打印错误信息
	ErrPrintAndExit = errors.New("print and exit")

	LoadGlobalOptions = func(ctx *cli.Context) error {
		if ctx.IsSet("cfg") {
			fmt.Println("started path changed to", ctx.Path("cfg"))
		}
		return nil
	}

	syncCmd = &cli.Command{
		Name: "sync",
		Usage: "sync config file to remote",
		Category: cmdGroupWork,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "upload",
				Aliases: []string{"u"},
				Usage: "✨ Update sshw config",
			},
			&cli.BoolFlag{
				Name: "down",
				Aliases: []string{"z"},
				Usage: "✨ Download sshw config",
			},
		},
		Action: func(ctx *cli.Context) error {
			// 全局选项
			if ctx.IsSet("c") {
				// do something
			}



			if ctx.Bool("upload") {
				fmt.Println("mysshw:: Use Upload Local Config Remote Server!! Begin... ")
				fmt.Println("mysshw:: Use Upload Local Config Remote Server!! End... ")
				return nil
			}

			if ctx.Bool("down") {
				fmt.Println("mysshw:: Use Remote Config Download Local!!  Begin... ")
				fmt.Println("mysshw:: Use Remote Config Download Local!!  End...  ")
				return nil
			}

			return nil
		},
	}
)