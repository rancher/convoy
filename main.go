package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/gorilla/mux"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/drivers"
	"github.com/rancher/rancher-volume/util"
	"os"
	"sync"
)

const (
	VERSION     = "0.2-dev"
	API_VERSION = "1"
	LOCKFILE    = "lock"
	CONFIGFILE  = "rancher-volume.cfg"

	KEY_NAME          = "name"
	KEY_VOLUME_UUID   = "volume-uuid"
	KEY_SNAPSHOT_UUID = "snapshot-uuid"
	KEY_BACKUP_URL    = "backup"
	KEY_DEST_URL      = "dest"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_POSTFIX       = ".json"
)

type Volume struct {
	UUID        string
	Name        string
	Size        int64
	MountPoint  string
	FileSystem  string
	CreatedTime string
	Snapshots   map[string]Snapshot
}

type Snapshot struct {
	UUID        string
	VolumeUUID  string
	Name        string
	CreatedTime string
}

type Server struct {
	Router              *mux.Router
	StorageDriver       drivers.Driver
	GlobalLock          *sync.RWMutex
	NameUUIDIndex       *util.Index
	SnapshotVolumeIndex *util.Index
	UUIDIndex           *truncindex.TruncIndex
	Config
}

type Config struct {
	Root              string
	Driver            string
	MountsDir         string
	DefaultVolumeSize int64
}

var (
	lock    string
	logFile *os.File
	log     = logrus.WithFields(logrus.Fields{"pkg": "main"})

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
