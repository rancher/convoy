package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/utils"
	"os"
	"path/filepath"
)

const (
	VERSION    = "0.1.2"
	LOCKFILE   = "lock"
	CONFIGFILE = "volmgr.cfg"
)

type Volume struct {
	Base       string
	Size       int64
	MountPoint string
	FileSystem string
	Snapshots  map[string]bool
}

type Config struct {
	Root    string
	Driver  string
	Volumes map[string]Volume
}

var (
	lock    string
	logFile *os.File
)

func preAppRun(c *cli.Context) error {
	if c.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	root := c.String("root")
	if root == "" {
		return fmt.Errorf("Have to specific root directory")
	}
	if err := utils.MkdirIfNotExists(root); err != nil {
		return fmt.Errorf("Invalid root directory:", err)
	}

	lock = filepath.Join(root, LOCKFILE)
	if err := utils.LockFile(lock); err != nil {
		return fmt.Errorf("Failed to lock the file", err.Error())
	}

	logName := c.String("log")
	if logName != "" {
		logFile, err := os.OpenFile(logName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		log.SetFormatter(&log.JSONFormatter{})
		log.SetOutput(logFile)
	} else {
		log.SetOutput(os.Stderr)
	}

	return nil
}

func cleanup() {
	if lock != "" {
		utils.UnlockFile(lock)
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
		},
		Action: cmdInitialize,
	}

	volumeCreateCmd := cli.Command{
		Name:  "create",
		Usage: "create a new volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume",
			},
			cli.IntFlag{
				Name:  "size",
				Usage: "size of volume, in bytes",
			},
		},
		Action: cmdVolumeCreate,
	}

	volumeDeleteCmd := cli.Command{
		Name:  "delete",
		Usage: "delete a volume with all of it's snapshots",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume",
			},
		},
		Action: cmdVolumeDelete,
	}

	volumeMountCmd := cli.Command{
		Name:  "mount",
		Usage: "mount a volume to an specific path",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  "mountpoint",
				Usage: "mountpoint of volume",
			},
			cli.StringFlag{
				Name:  "fs",
				Value: "ext4",
				Usage: "filesystem of volume(supports ext4 only)",
			},
			cli.BoolFlag{
				Name:  "format",
				Usage: "format or not",
			},
			cli.StringFlag{
				Name:  "option",
				Usage: "mount options",
			},
			cli.StringFlag{
				Name:  "switch-ns",
				Usage: "switch to another mount namespace, need namespace file descriptor",
			},
		},
		Action: cmdVolumeMount,
	}

	volumeUmountCmd := cli.Command{
		Name:  "umount",
		Usage: "umount a volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  "switch-ns",
				Usage: "switch to another mount namespace, need namespace file descriptor",
			},
		},
		Action: cmdVolumeUmount,
	}

	volumeListCmd := cli.Command{
		Name:  "list",
		Usage: "list all managed volumes",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume, if not supplied, would list all volumes",
			},
		},
		Action: cmdVolumeList,
	}

	volumeCmd := cli.Command{
		Name:  "volume",
		Usage: "volume related operations",
		Subcommands: []cli.Command{
			volumeCreateCmd,
			volumeDeleteCmd,
			volumeMountCmd,
			volumeUmountCmd,
			volumeListCmd,
		},
	}

	snapshotCreateCmd := cli.Command{
		Name:  "create",
		Usage: "create a snapshot for certain volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  "volume-uuid",
				Usage: "uuid of volume for snapshot",
			},
		},
		Action: cmdSnapshotCreate,
	}

	snapshotDeleteCmd := cli.Command{
		Name:  "delete",
		Usage: "delete a snapshot of certain volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  "volume-uuid",
				Usage: "uuid of volume for snapshot",
			},
		},
		Action: cmdSnapshotDelete,
	}

	snapshotBackupCmd := cli.Command{
		Name:  "backup",
		Usage: "backup an snapshot to blockstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  "volume-uuid",
				Usage: "uuid of volume for snapshot",
			},
			cli.StringFlag{
				Name:  "blockstore-uuid",
				Usage: "uuid of blockstore",
			},
		},
		Action: cmdSnapshotBackup,
	}

	snapshotRestoreCmd := cli.Command{
		Name:  "restore",
		Usage: "restore an snapshot from blockstore to volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  "origin-volume-uuid",
				Usage: "uuid of origin volume for snapshot",
			},
			cli.StringFlag{
				Name:  "target-volume-uuid",
				Usage: "uuid of target volume",
			},
			cli.StringFlag{
				Name:  "blockstore-uuid",
				Usage: "uuid of blockstore",
			},
		},
		Action: cmdSnapshotRestore,
	}

	snapshotRemoveCmd := cli.Command{
		Name:  "remove",
		Usage: "remove an snapshot in blockstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  "volume-uuid",
				Usage: "uuid of volume for snapshot",
			},
			cli.StringFlag{
				Name:  "blockstore-uuid",
				Usage: "uuid of blockstore",
			},
		},
		Action: cmdSnapshotRemove,
	}

	snapshotCmd := cli.Command{
		Name:  "snapshot",
		Usage: "snapshot related operations",
		Subcommands: []cli.Command{
			snapshotCreateCmd,
			snapshotDeleteCmd,
			snapshotBackupCmd,
			snapshotRestoreCmd,
			snapshotRemoveCmd,
		},
	}

	blockstoreRegisterCmd := cli.Command{
		Name:  "register",
		Usage: "register a blockstore for current setup, create it if it's not existed yet",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "kind",
				Value: "vfs",
				Usage: "kind of blockstore, only support vfs now",
			},
			cli.StringSliceFlag{
				Name:  "opts",
				Value: &cli.StringSlice{},
				Usage: "options used to register blockstore",
			},
		},
		Action: cmdBlockStoreRegister,
	}

	blockstoreDeregisterCmd := cli.Command{
		Name:  "deregister",
		Usage: "deregister a blockstore from current setup(no data in it would be changed)",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of blockstore",
			},
		},
		Action: cmdBlockStoreDeregister,
	}

	blockstoreAddVolumeCmd := cli.Command{
		Name:  "add",
		Usage: "add a volume to blockstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  "volume-uuid",
				Usage: "uuid of volume",
			},
		},
		Action: cmdBlockStoreAdd,
	}

	blockstoreRemoveVolumeCmd := cli.Command{
		Name:  "remove",
		Usage: "remove a volume from blockstore. WARNING: ALL THE DATA ABOUT THE VOLUME IN THIS BLOCKSTORE WOULD BE REMOVED!",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  "volume-uuid",
				Usage: "uuid of volume",
			},
		},
		Action: cmdBlockStoreRemove,
	}

	blockstoreCmd := cli.Command{
		Name:  "blockstore",
		Usage: "blockstore related operations",
		Subcommands: []cli.Command{
			blockstoreRegisterCmd,
			blockstoreDeregisterCmd,
			blockstoreAddVolumeCmd,
			blockstoreRemoveVolumeCmd,
		},
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
