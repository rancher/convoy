package main

import (
	"fmt"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/client"
	"os"
)

const (
	// version of Convoy
	VERSION = "0.5.0-rancher"
)

func cleanup() {
	if r := recover(); r != nil {
		api.ResponseLogAndError(r)
		os.Exit(1)
	}
}

func main() {
	defer cleanup()

	cli := client.NewCli(VERSION)
	err := cli.Run(os.Args)
	if err != nil {
		panic(fmt.Errorf("Error when executing command: %v", err))
	}
}
