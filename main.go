package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/util"
	"os"
	"path/filepath"
)

const (
	VERSION    = "0.1.5"
	LOCKFILE   = "lock"
	CONFIGFILE = "volmgr.cfg"
)

type Volume struct {
	UUID       string
	Base       string
	Size       int64
	MountPoint string
	FileSystem string
	Snapshots  map[string]bool
}

type Config struct {
	Root      string
	Driver    string
	ImagesDir string
}

var (
	lock    string
	logFile *os.File
	log     = logrus.WithFields(logrus.Fields{"pkg": "main"})
)

func preAppRun(c *cli.Context) error {
	if c.Bool("debug") {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	root := c.String("root")
	if root == "" {
		return fmt.Errorf("Have to specific root directory")
	}
	if err := util.MkdirIfNotExists(root); err != nil {
		return fmt.Errorf("Invalid root directory:", err)
	}

	lock = filepath.Join(root, LOCKFILE)
	if err := util.LockFile(lock); err != nil {
		return fmt.Errorf("Failed to lock the file", err.Error())
	}

	logName := c.String("log")
	if logName != "" {
		logFile, err := os.OpenFile(logName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		logrus.SetFormatter(&logrus.JSONFormatter{})
		logrus.SetOutput(logFile)
	} else {
		logrus.SetOutput(os.Stderr)
	}

	return nil
}

func cleanup() {
	if lock != "" {
		util.UnlockFile(lock)
	}
	if logFile != nil {
		logFile.Close()
	}
	if r := recover(); r != nil {
		api.ResponseLogAndError(fmt.Sprint(r))
		os.Exit(1)
	}
}

func main() {
	app := cli.NewApp()
	app.Name = "volmgr"
	app.Version = VERSION
	app.Usage = "A volume manager capable of snapshot and delta backup"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable debug log.",
		},
		cli.StringFlag{
			Name:  "log",
			Usage: "specific output log file, otherwise output to stderr by default",
		},
		cli.StringFlag{
			Name:  "root",
			Value: "/var/lib/volmgr",
			Usage: "specific root directory of volmgr",
		},
	}
	app.Before = preAppRun
	app.CommandNotFound = cmdNotFound

	infoCmd := cli.Command{
		Name:   "info",
		Usage:  "information about volmgr",
		Action: cmdInfo,
	}

	initCmd := cli.Command{
		Name:  "init",
		Usage: "initialize volmgr",
		Flags: []cli.Flag{
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
				Name:  "images-dir",
				Value: "/opt/volmgr_images",
				Usage: "specific local directory would contains base images",
			},
		},
		Action: cmdInitialize,
	}

	app.Commands = []cli.Command{
		initCmd,
		infoCmd,
		volumeCmd,
		snapshotCmd,
		blockstoreCmd,
	}

	defer cleanup()
	err := app.Run(os.Args)
	if err != nil {
		panic(fmt.Errorf("Error when executing command", err.Error()))
	}
}

func cmdNotFound(c *cli.Context, command string) {
	panic(fmt.Errorf("Unrecognized command", command))
}
