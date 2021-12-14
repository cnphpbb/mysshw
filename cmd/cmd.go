package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"mysshw/auth"
	"mysshw/config"
	"mysshw/scp"

	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
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
			//config.CFG_PATH = ctx.Path("cfg")
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
				Usage: " Update mysshw config",
			},
			&cli.BoolFlag{
				Name: "down",
				Aliases: []string{"z"},
				Usage: " Download mysshw config",
			},
		},
		Action: func(ctx *cli.Context) error {
			// 全局选项
			if ctx.IsSet("cfg") {
				config.CFG_PATH = ctx.Path("cfg")
			}

			syncCfg := config.CFG.SyncCfg
			//fmt.Println(syncCfg)
			cfg, _ := auth.PasswordKey(syncCfg.UserName, syncCfg.Password, ssh.InsecureIgnoreHostKey())
			client := scp.NewClient(syncCfg.RemoteUri, &cfg)
			err := client.Connect()
			if err != nil {
				fmt.Printf("Couldn't establish a connection to the remote server: %s \n", err)
			}
			defer client.Close()

			if ctx.Bool("upload") {
				fmt.Println("mysshw:: Use Upload Local Config Remote Server!! Begin... ")
				cfg, _ := config.LoadConfigBytes(config.CFG_PATH)
				err := client.CopyFile(bytes.NewReader(cfg), syncCfg.RemotePath, "0644")
				if err != nil {
					fmt.Printf("Error while copying file: %s", err)
					os.Exit(1)
				}
				fmt.Println("mysshw:: Use Upload Local Config Remote Server!! End... ")
				return nil
			}

			if ctx.Bool("down") {
				fmt.Println("mysshw:: Use Remote Config Download Local!!  Begin... ")
				localPath, _ := config.GetCfgPath(config.CFG_PATH)
				f, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE, 0644)
				if err != nil {
					fmt.Printf("Couldn't open the output file: %s  \n", err)
					os.Exit(1)
				}
				defer f.Close()
				err = client.CopyFromRemote(f, syncCfg.RemotePath)
				if err != nil {
					fmt.Printf("Error Copy failed from remote: %s \n", err)
					os.Exit(1)
				}
				fmt.Println("mysshw:: Use Remote Config Download Local!!  End...  ")
				return nil
			}

			return nil
		},
	}
)