package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/server"
	"io/ioutil"
)

var (
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
