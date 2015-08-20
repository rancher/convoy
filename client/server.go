package client

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/rancher/convoy/server"
	"io/ioutil"
)

var (
	serverCmd = cli.Command{
		Name:  "daemon",
		Usage: "start convoy daemon",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  "debug",
				Usage: "Debug log, enabled by default",
			},
			cli.StringFlag{
				Name:  "log",
				Usage: "specific output log file, otherwise output to stderr by default",
			},
			cli.StringFlag{
				Name:  "root",
				Value: "/var/lib/convoy",
				Usage: "specific root directory of convoy, if configure file exists, daemon specific options would be ignored",
			},
			cli.StringSliceFlag{
				Name:  "drivers",
				Value: &cli.StringSlice{},
				Usage: "Drivers to be enabled, first driver in the list would be treated as default driver",
			},
			cli.StringSliceFlag{
				Name:  "driver-opts",
				Value: &cli.StringSlice{},
				Usage: "options for driver",
			},
			cli.StringFlag{
				Name:  "mounts-dir",
				Value: "/var/lib/convoy/mounts",
				Usage: "default directory for mounting volume",
			},
		},
		Action: cmdStartServer,
	}

	infoCmd = cli.Command{
		Name:   "info",
		Usage:  "information about convoy",
		Action: cmdInfo,
	}
)

func cmdInfo(c *cli.Context) {
	if err := doInfo(c); err != nil {
		panic(err)
	}
}

func doInfo(c *cli.Context) error {
	rc, _, err := client.call("GET", "/info", nil, nil)
	if err != nil {
		return err
	}
	defer rc.Close()

	b, err := ioutil.ReadAll(rc)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func cmdStartServer(c *cli.Context) {
	if err := startServer(c); err != nil {
		panic(err)
	}
}

func startServer(c *cli.Context) error {
	return server.Start(client.addr, c)
}
