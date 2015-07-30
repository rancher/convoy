package client

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/server"
	"io/ioutil"
)

var (
	serverCmd = cli.Command{
		Name:  "server",
		Usage: "start rancher-volume server",
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
				Value: "/var/lib/rancher-volume",
				Usage: "specific root directory of rancher-volume, if configure file exists, daemon specific options would be ignored",
			},
			cli.StringFlag{
				Name:  "driver",
				Value: "devicemapper",
				Usage: "Driver for volume manager, only support \"devicemapper\" currently",
			},
			cli.StringSliceFlag{
				Name:  "driver-opts",
				Value: &cli.StringSlice{},
				Usage: "options for driver",
			},
			cli.StringFlag{
				Name:  "mounts-dir",
				Value: "/var/lib/rancher-volume/mounts",
				Usage: "default directory for mounting volume",
			},
			cli.StringFlag{
				Name:  "default-volume-size",
				Value: "100G",
				Usage: "default size for volume creation",
			},
		},
		Action: cmdStartServer,
	}

	infoCmd = cli.Command{
		Name:   "info",
		Usage:  "information about rancher-volume",
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
	return server.Start(sockFile, c)
}
