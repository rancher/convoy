package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/alecthomas/kingpin"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/utils"
	"os"
	"path/filepath"
)

var (
	flagApp   = kingpin.New("volmgr", "A volume manager capable of snapshot and delta backup")
	flagDebug = flagApp.Flag("debug", "Enable debug mode.").Default("true").Bool()
	flagLog   = flagApp.Flag("log", "specific output log file, otherwise output to stderr by default").String()
	flagRoot  = flagApp.Flag("root", "specific root directory of volmgr").Default("/var/lib/volmgr").String()
	flagInfo  = flagApp.Command("info", "information about volmgr")

	flagInitialize           = flagApp.Command("init", "initialize volmgr")
	flagInitializeDriver     = flagInitialize.Flag("driver", "Driver for volume manager, only support \"devicemapper\" currently").Default("devicemapper").String()
	flagInitializeDriverOpts = flagInitialize.Flag("driver-opts", "options for driver").Required().StringMap()

	flagVolume                = flagApp.Command("volume", "volume related operations")
	flagVolumeCreate          = flagVolume.Command("create", "create a new volume")
	flagVolumeCreateSize      = flagVolumeCreate.Flag("size", "size of volume").Required().Int64()
	flagVolumeDelete          = flagVolume.Command("delete", "delete a volume with all of it's snapshots")
	flagVolumeDeleteUUID      = flagVolumeDelete.Flag("uuid", "uuid of volume").Required().String()
	flagVolumeUpdate          = flagVolume.Command("update", "update info about volume")
	flagVolumeUpdateUUID      = flagVolumeUpdate.Flag("uuid", "uuid of volume").Required().String()
	flagVolumeUpdateSize      = flagVolumeUpdate.Flag("size", "size of volume").Required().Int64()
	flagVolumeMount           = flagVolume.Command("mount", "mount a volume to an specific path")
	flagVolumeMountUUID       = flagVolumeMount.Flag("uuid", "uuid of volume").Required().String()
	flagVolumeMountPoint      = flagVolumeMount.Flag("mountpoint", "mountpoint of volume").String()
	flagVolumeMountFS         = flagVolumeMount.Flag("fs", "filesystem of volume(supports ext4)").Default("ext4").String()
	flagVolumeMountFormat     = flagVolumeMount.Flag("format", "format or not").Bool()
	flagVolumeMountOptions    = flagVolumeMount.Flag("option", "mount options").String()
	flagVolumeMountSwitchNS   = flagVolumeMount.Flag("switch-ns", "switch to another mount namespace, need namespace file descriptor").String()
	flagVolumeUnmount         = flagVolume.Command("umount", "umount a volume")
	flagVolumeUnmountUUID     = flagVolumeUnmount.Flag("uuid", "uuid of volume").Required().String()
	flagVolumeUnmountSwitchNS = flagVolumeUnmount.Flag("switch-ns", "switch to another mount namespace, need namespace file descriptor").String()
	flagVolumeList            = flagVolume.Command("list", "list all managed volumes")
	flagVolumeListUUID        = flagVolumeList.Flag("uuid", "uuid of volume").String()

	flagSnapshot                 = flagApp.Command("snapshot", "snapshot related operations")
	flagSnapshotCreate           = flagSnapshot.Command("create", "create a snapshot")
	flagSnapshotCreateVolumeUUID = flagSnapshotCreate.Flag("volume-uuid", "uuid of volume for snapshot").Required().String()
	flagSnapshotDelete           = flagSnapshot.Command("delete", "delete a snapshot")
	flagSnapshotDeleteUUID       = flagSnapshotDelete.Flag("uuid", "uuid of snapshot").Required().String()
	flagSnapshotDeleteVolumeUUID = flagSnapshotDelete.Flag("volume-uuid", "uuid of volume for snapshot").Required().String()

	flagBlockStore                 = flagApp.Command("blockstore", "blockstore related operations")
	flagBlockStoreRegister         = flagBlockStore.Command("register", "register a blockstore, create it if it's not existed yet")
	flagBlockStoreRegisterKind     = flagBlockStoreRegister.Flag("kind", "kind of blockstore").Required().String()
	flagBlockStoreRegisterOpts     = flagBlockStoreRegister.Flag("opts", "options used to register blockstore").StringMap()
	flagBlockStoreDeregister       = flagBlockStore.Command("deregister", "delete a blockstore")
	flagBlockStoreDeregisterUUID   = flagBlockStoreDeregister.Flag("uuid", "uuid of blockstore").Required().String()
	flagBlockStoreAdd              = flagBlockStore.Command("add", "add a volume to blockstore, one volume can only associate with one block store")
	flagBlockStoreAddUUID          = flagBlockStoreAdd.Flag("uuid", "uuid of blockstore").Required().String()
	flagBlockStoreAddVolumeUUID    = flagBlockStoreAdd.Flag("volume-uuid", "uuid of volume").Required().String()
	flagBlockStoreRemove           = flagBlockStore.Command("remove", "remove a volume from blockstore, WARNING: ALL THE DATA ABOUT THE VOLUME IN THIS BLOCKSTORE WOULD BE REMOVED!")
	flagBlockStoreRemoveUUID       = flagBlockStoreRemove.Flag("uuid", "uuid of blockstore").Required().String()
	flagBlockStoreRemoveVolumeUUID = flagBlockStoreRemove.Flag("volume-uuid", "uuid of volume").Required().String()
	flagBlockStoreInfo             = flagBlockStore.Command("info", "info of blockstores")

	flagSnapshotBackup                  = flagSnapshot.Command("backup", "backup an snapshot to blockstore")
	flagSnapshotBackupUUID              = flagSnapshotBackup.Flag("uuid", "uuid of snapshot").Required().String()
	flagSnapshotBackupVolumeUUID        = flagSnapshotBackup.Flag("volume-uuid", "uuid of volume for snapshot").Required().String()
	flagSnapshotBackupBlockStoreUUID    = flagSnapshotBackup.Flag("blockstore-uuid", "uuid of blockstore").Required().String()
	flagSnapshotRestore                 = flagSnapshot.Command("restore", "restore an snapshot from blockstore")
	flagSnapshotRestoreUUID             = flagSnapshotRestore.Flag("uuid", "uuid of snapshot").Required().String()
	flagSnapshotRestoreOriginVolumeUUID = flagSnapshotRestore.Flag("origin-volume-uuid", "uuid of original volume of snapshot").Required().String()
	flagSnapshotRestoreTargetVolumeUUID = flagSnapshotRestore.Flag("target-volume-uuid", "uuid of target volume for snapshot restore").Required().String()
	flagSnapshotRestoreBlockStoreUUID   = flagSnapshotRestore.Flag("blockstore-uuid", "uuid of blockstore").Required().String()
	flagSnapshotRemove                  = flagSnapshot.Command("remove", "remove an snapshot in blockstore")
	flagSnapshotRemoveUUID              = flagSnapshotRemove.Flag("uuid", "uuid of snapshot").Required().String()
	flagSnapshotRemoveVolumeUUID        = flagSnapshotRemove.Flag("volume-uuid", "uuid of volume for snapshot").Required().String()
	flagSnapshotRemoveBlockStoreUUID    = flagSnapshotRemove.Flag("blockstore-uuid", "uuid of blockstore").Required().String()
)

const (
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

func main() {
	if len(os.Args) == 1 {
		fmt.Println("Use --help to see command list")
		os.Exit(-1)
	}

	command := kingpin.MustParse(flagApp.Parse(os.Args[1:]))

	if *flagDebug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	root := *flagRoot
	if root == "" {
		fmt.Println("Have to specific root directory")
		os.Exit(-1)
	}
	if err := utils.MkdirIfNotExists(root); err != nil {
		fmt.Println("Invalid root directory:", err)
		os.Exit(-1)
	}

	lock := filepath.Join(root, LOCKFILE)
	if err := utils.LockFile(lock); err != nil {
		api.ResponseError("Fail to lock the file", err)
		os.Exit(-1)
	}

	defer utils.UnlockFile(lock)

	if *flagLog != "" {
		f, err := os.OpenFile(*flagLog, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			api.ResponseLogAndError(err.Error())
			os.Exit(-1)
		}
		defer f.Close()
		log.SetFormatter(&log.JSONFormatter{})
		log.SetOutput(f)
	} else {
		log.SetOutput(os.Stderr)
	}

	configFile := filepath.Join(root, CONFIGFILE)

	if command == flagInitialize.FullCommand() {
		if _, err := os.Stat(configFile); err == nil {
			api.ResponseLogAndError("Configuration file %v existed. Don't need to initialize.", configFile)
			os.Exit(-1)
		}

		err := doInitialize(root, *flagInitializeDriver, *flagInitializeDriverOpts)
		if err != nil {
			api.ResponseLogAndError("Failed to initialize volmgr.", err)
			os.Exit(-1)
		}
		os.Exit(0)
	}

	config := Config{}
	err := utils.LoadConfig(configFile, &config)
	if err != nil {
		api.ResponseLogAndError("Failed to load config.", err)
		os.Exit(-1)
	}

	driver, err := drivers.GetDriver(config.Driver, getDriverRoot(config.Root, config.Driver), nil)
	if err != nil {
		api.ResponseLogAndError("Failed to load driver.", err)
		os.Exit(-1)
	}

	switch command {
	case flagInfo.FullCommand():
		err = doInfo(&config, driver)
	case flagVolumeCreate.FullCommand():
		err = doVolumeCreate(&config, driver, *flagVolumeCreateSize)
	case flagVolumeDelete.FullCommand():
		err = doVolumeDelete(&config, driver, *flagVolumeDeleteUUID)
	case flagVolumeUpdate.FullCommand():
		err = doVolumeUpdate(&config, driver, *flagVolumeUpdateUUID, *flagVolumeUpdateSize)
	case flagVolumeList.FullCommand():
		err = doVolumeList(&config, driver, *flagVolumeListUUID)
	case flagVolumeMount.FullCommand():
		err = doVolumeMount(&config, driver, *flagVolumeMountUUID, *flagVolumeMountPoint, *flagVolumeMountFS,
			*flagVolumeMountOptions, *flagVolumeMountFormat, *flagVolumeMountSwitchNS)
	case flagVolumeUnmount.FullCommand():
		err = doVolumeUnmount(&config, driver, *flagVolumeUnmountUUID, *flagVolumeUnmountSwitchNS)
	case flagSnapshotCreate.FullCommand():
		err = doSnapshotCreate(&config, driver, *flagSnapshotCreateVolumeUUID)
	case flagSnapshotDelete.FullCommand():
		err = doSnapshotDelete(&config, driver, *flagSnapshotDeleteUUID, *flagSnapshotDeleteVolumeUUID)
	case flagBlockStoreRegister.FullCommand():
		err = doBlockStoreRegister(&config, *flagBlockStoreRegisterKind, *flagBlockStoreRegisterOpts)
	case flagBlockStoreDeregister.FullCommand():
		err = doBlockStoreDeregister(&config, *flagBlockStoreDeregisterUUID)
	case flagBlockStoreAdd.FullCommand():
		err = doBlockStoreAdd(&config, *flagBlockStoreAddUUID, *flagBlockStoreAddVolumeUUID)
	case flagBlockStoreRemove.FullCommand():
		err = doBlockStoreRemove(&config, *flagBlockStoreRemoveUUID, *flagBlockStoreRemoveVolumeUUID)
	case flagSnapshotBackup.FullCommand():
		err = doSnapshotBackup(&config, driver, *flagSnapshotBackupUUID, *flagSnapshotBackupVolumeUUID,
			*flagSnapshotBackupBlockStoreUUID)
	case flagSnapshotRestore.FullCommand():
		err = doSnapshotRestore(&config, driver, *flagSnapshotRestoreUUID, *flagSnapshotRestoreOriginVolumeUUID,
			*flagSnapshotRestoreTargetVolumeUUID, *flagSnapshotRestoreBlockStoreUUID)
	case flagSnapshotRemove.FullCommand():
		err = doSnapshotRemove(&config, *flagSnapshotRemoveUUID, *flagSnapshotRemoveVolumeUUID, *flagSnapshotRemoveBlockStoreUUID)
	default:
		api.ResponseLogAndError("Unrecognized command", command)
		os.Exit(-1)
	}
	if err != nil {
		api.ResponseLogAndError("Failed to complete", command, err)
		os.Exit(-1)
	}
}
