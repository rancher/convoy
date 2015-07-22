package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/api"
	"os"
)

const (
	VERSION = "0.2-dev"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "main"})

	sockFile string = "/var/run/rancher/volume.sock"
)

func cleanup() {
	if r := recover(); r != nil {
		api.ResponseLogAndError(r)
		os.Exit(1)
	}
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stderr)

	InitClient(sockFile)
	defer cleanup()

	cli := NewCli()
	err := cli.Run(os.Args)
	if err != nil {
		panic(fmt.Errorf("Error when executing command", err.Error()))
	}
}
