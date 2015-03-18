package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/alecthomas/kingpin"
	"github.com/yasker/volmgr/drivers"
	"github.com/yasker/volmgr/utils"
	"os"
	"path/filepath"
)

var (
	flagApp   = kingpin.New("volmgr", "A volume manager capable of snapshot and delta backup")
	flagDebug = flagApp.Flag("debug", "Enable debug mode.").Default("true").Bool()
	flagInfo  = flagApp.Command("info", "information about volmgr")

	flagInitialize           = flagApp.Command("init", "initialize volmgr")
	flagInitializeDriver     = flagInitialize.Flag("driver", "Driver for volume manager, only support \"devicemapper\" currently").Default("devicemapper").String()
	flagInitializeDriverOpts = flagInitialize.Flag("driver-opts", "options for driver").Required().StringMap()

	flagVolume           = flagApp.Command("volume", "volume related operations")
	flagVolumeCreate     = flagVolume.Command("create", "create a new volume")
	flagVolumeCreateSize = flagVolumeCreate.Flag("size", "size of volume").Required().Uint64()
	flagVolumeDelete     = flagVolume.Command("delete", "delete a volume with all of it's snapshots")
	flagVolumeDeleteUUID = flagVolumeDelete.Flag("uuid", "uuid of volume").Required().String()
	flagVolumeUpdate     = flagVolume.Command("update", "update info about volume")
	flagVolumeUpdateUUID = flagVolumeUpdate.Flag("uuid", "uuid of volume").Required().String()
	flagVolumeUpdateSize = flagVolumeUpdate.Flag("size", "size of volume").Required().Uint64()
	flagVolumeList       = flagVolume.Command("list", "list all managed volumes")

	flagSnapshot                 = flagApp.Command("snapshot", "snapshot related operations")
	flagSnapshotCreate           = flagSnapshot.Command("create", "create a snapshot")
	flagSnapshotCreateVolumeUUID = flagSnapshotCreate.Flag("volume-uuid", "uuid of volume for snapshot").Required().String()
	flagSnapshotDelete           = flagSnapshot.Command("delete", "delete a snapshot")
	flagSnapshotDeleteUUID       = flagSnapshotDelete.Flag("uuid", "uuid of snapshot").Required().String()
	flagSnapshotDeleteVolumeUUID = flagSnapshotDelete.Flag("volume-uuid", "uuid of volume for snapshot").Required().String()
	flagSnapshotList             = flagSnapshot.Command("list", "list snapshots")
	flagSnapshotListVolumeUUID   = flagSnapshotList.Flag("volume-uuid", "uuid of volume for snapshot").String()

	flagBlockStore                 = flagApp.Command("blockstore", "blockstore related operations")
	flagBlockStoreRegister         = flagBlockStore.Command("register", "register a existed blockstore")
	flagBlockStoreRegisterKind     = flagBlockStoreRegister.Flag("kind", "kind of blockstore").Required().String()
	flagBlockStoreRegisterOpts     = flagBlockStoreRegister.Flag("opts", "options used to register blockstore").StringMap()
	flagBlockStoreDeregister       = flagBlockStore.Command("deregister", "delete a blockstore")
	flagBlockStoreDeregisterKind   = flagBlockStoreDeregister.Flag("kind", "kind of blockstore").Required().String()
	flagBlockStoreDeregisterUUID   = flagBlockStoreDeregister.Flag("uuid", "uuid of blockstore").Required().String()
	flagBlockStoreAdd              = flagBlockStore.Command("add", "add a volume to blockstore, one volume can only associate with one block store")
	flagBlockStoreAddUUID          = flagBlockStoreAdd.Flag("uuid", "uuid of blockstore").Required().String()
	flagBlockStoreAddVolumeUUID    = flagBlockStoreAdd.Flag("volume-uuid", "uuid of volume").Required().String()
	flagBlockStoreRemove           = flagBlockStore.Command("remove", "remove a volume from blockstore, WARNING: ALL THE DATA ABOUT THE VOLUME IN THIS BLOCKSTORE WOULD BE REMOVED!")
	flagBlockStoreRemoveUUID       = flagBlockStoreRemove.Flag("uuid", "uuid of blockstore").Required().String()
	flagBlockStoreRemoveVolumeUUID = flagBlockStoreRemove.Flag("volume-uuid", "uuid of volume").Required().String()
	flagBlockStoreInfo             = flagBlockStore.Command("info", "info of blockstores")

	flagSnapshotBackup                = flagSnapshot.Command("backup", "backup an snapshot to blockstore")
	flagSnapshotBackupUUID            = flagSnapshotBackup.Flag("uuid", "uuid of snapshot").Required().String()
	flagSnapshotBackupVolumeUUID      = flagSnapshotBackup.Flag("volume-uuid", "uuid of volume for snapshot").Required().String()
	flagSnapshotBackupBlockStoreUUID  = flagSnapshotBackup.Flag("blockstore-uuid", "uuid of blockstore").Required().String()
	flagSnapshotRestore               = flagSnapshot.Command("restore", "restore an snapshot from blockstore")
	flagSnapshotRestoreUUID           = flagSnapshotRestore.Flag("uuid", "uuid of snapshot").Required().String()
	flagSnapshotRestoreVolumeUUID     = flagSnapshotRestore.Flag("volume-uuid", "uuid of volume to be restored to").Required().String()
	flagSnapshotRestoreBlockStoreUUID = flagSnapshotRestore.Flag("blockstore-uuid", "uuid of blockstore").Required().String()
	flagSnapshotRemove                = flagSnapshot.Command("remove", "remove an snapshot in blockstore")
	flagSnapshotRemoveUUID            = flagSnapshotRemove.Flag("uuid", "uuid of snapshot").Required().String()
	flagSnapshotRemoveVolumeUUID      = flagSnapshotRemove.Flag("volume-uuid", "uuid of volume for snapshot").Required().String()
	flagSnapshotRemoveBlockStoreUUID  = flagSnapshotRemove.Flag("blockstore-uuid", "uuid of blockstore").Required().String()
)

const (
	LOCKFILE   = "lock"
	CONFIGFILE = "volmgr.cfg"
	ROOTDIR    = "/var/lib/volmgr/"
)

type Config struct {
	Root   string
	Driver string
}

func main() {
	log.SetOutput(os.Stderr)

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
	configFile := filepath.Join(ROOTDIR, CONFIGFILE)

	if command == flagInitialize.FullCommand() {
		if _, err := os.Stat(configFile); err == nil {
			log.Errorf("Configuration file %v existed. Don't need to initialize.", configFile)
			os.Exit(-1)
		}

		err := doInitialize(ROOTDIR, *flagInitializeDriver, *flagInitializeDriverOpts)
		if err != nil {
			log.Errorln("Failed to initialize volmgr.", err)
			os.Exit(-1)
		}
		os.Exit(0)
	}

	config := Config{}
	err := utils.LoadConfig(configFile, &config)
	if err != nil {
		log.Errorln("Failed to load config.", err)
		os.Exit(-1)
	}

	driver, err := drivers.GetDriver(config.Driver, getDriverRoot(config.Root, config.Driver), nil)
	if err != nil {
		log.Errorln("Failed to load driver.", err)
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
		err = doVolumeList(&config, driver)
	case flagSnapshotCreate.FullCommand():
		err = doSnapshotCreate(&config, driver, *flagSnapshotCreateVolumeUUID)
	case flagSnapshotDelete.FullCommand():
		err = doSnapshotDelete(&config, driver, *flagSnapshotDeleteUUID, *flagSnapshotDeleteVolumeUUID)
	case flagSnapshotList.FullCommand():
		err = doSnapshotList(&config, driver, *flagSnapshotListVolumeUUID)
	case flagBlockStoreRegister.FullCommand():
		err = doBlockStoreRegister(&config, *flagBlockStoreRegisterKind, *flagBlockStoreRegisterOpts)
	case flagBlockStoreDeregister.FullCommand():
		err = doBlockStoreDeregister(&config, *flagBlockStoreDeregisterKind, *flagBlockStoreDeregisterUUID)
	case flagBlockStoreAdd.FullCommand():
		err = doBlockStoreAdd(&config, *flagBlockStoreAddUUID, *flagBlockStoreAddVolumeUUID)
	case flagBlockStoreRemove.FullCommand():
		err = doBlockStoreRemove(&config, *flagBlockStoreRemoveUUID, *flagBlockStoreRemoveVolumeUUID)
	default:
		log.Errorln("Unrecognized command")
		os.Exit(-1)
	}
	if err != nil {
		log.Errorln("Failed to complete", command, err)
		os.Exit(-1)
	}
}
