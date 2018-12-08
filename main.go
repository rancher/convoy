package main

import (
	"fmt"
	"os"

	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/client"
)

var (
	// version of Convoy
	VERSION = "v0.5.2"
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
