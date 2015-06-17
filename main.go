package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/gorilla/mux"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	VERSION     = "0.1.5"
	API_VERSION = "1"
	LOCKFILE    = "lock"
	CONFIGFILE  = "volmgr.cfg"

	KEY_VOLUME      = "volume-uuid"
	KEY_SNAPSHOT    = "snapshot-uuid"
	KEY_BLOCKSTORE  = "objectstore-uuid"
	KEY_IMAGE       = "image-uuid"
	KEY_VOLUME_NAME = "volume-name"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_POSTFIX       = ".json"
)

type Volume struct {
	UUID       string
	Name       string
	Base       string
	Size       int64
	MountPoint string
	FileSystem string
	Snapshots  map[string]bool
}

type Server struct {
	Router        *mux.Router
	StorageDriver drivers.Driver
	NameVolumeMap map[string]string
	Config
}

type Config struct {
	Root              string
	Driver            string
	ImagesDir         string
	MountsDir         string
	DefaultVolumeSize int64
}

var (
	lock    string
	logFile *os.File
	log     = logrus.WithFields(logrus.Fields{"pkg": "main"})

	sockFile string = "/var/run/rancher/volume.sock"
	client   Client
)

type Client struct {
	addr      string
	scheme    string
	transport *http.Transport
}

func cleanup() {
	if r := recover(); r != nil {
		api.ResponseLogAndError(r)
		os.Exit(1)
	}
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)

	app := cli.NewApp()
	app.Name = "volmgr"
	app.Version = VERSION
	app.Usage = "A volume manager capable of snapshot and delta backup"
	app.CommandNotFound = cmdNotFound

	serverCmd := cli.Command{
		Name:  "server",
		Usage: "start rancher-volmgr server",
		Flags: []cli.Flag{
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
				Usage: "specific root directory of volmgr, if configure file exists, daemon specific options would be ignored",
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
				Name:  "images-dir",
				Value: "/opt/volmgr_images",
				Usage: "specific local directory would contains base images",
			},
			cli.StringFlag{
				Name:  "mounts-dir",
				Value: "/var/lib/volmgr/mounts",
				Usage: "default directory for mounting volume",
			},
			cli.StringFlag{
				Name:  "default-volume-size",
				Value: "10G",
				Usage: "default size for volume creation",
			},
		},
		Action: cmdStartServer,
	}

	app.Commands = []cli.Command{
		serverCmd,
		infoCmd,
		volumeCmd,
		snapshotCmd,
		objectstoreCmd,
	}

	client.addr = sockFile
	client.scheme = "http"
	client.transport = &http.Transport{
		DisableCompression: true,
		Dial: func(_, _ string) (net.Conn, error) {
			return net.DialTimeout("unix", sockFile, 10*time.Second)
		},
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
