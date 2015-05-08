package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/blockstores"
	"github.com/rancherio/volmgr/utils"
)

var (
	snapshotBackupCmd = cli.Command{
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

	snapshotRestoreCmd = cli.Command{
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

	snapshotRemoveCmd = cli.Command{
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

	blockstoreRegisterCmd = cli.Command{
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

	blockstoreDeregisterCmd = cli.Command{
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

	blockstoreAddVolumeCmd = cli.Command{
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

	blockstoreRemoveVolumeCmd = cli.Command{
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

	blockstoreListCmd = cli.Command{
		Name:  "list",
		Usage: "list volume and snapshots in blockstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  "volume-uuid",
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  "snapshot-uuid",
				Usage: "uuid of snapshot",
			},
		},
		Action: cmdBlockStoreList,
	}

	blockstoreCmd = cli.Command{
		Name:  "blockstore",
		Usage: "blockstore related operations",
		Subcommands: []cli.Command{
			blockstoreRegisterCmd,
			blockstoreDeregisterCmd,
			blockstoreAddVolumeCmd,
			blockstoreRemoveVolumeCmd,
			blockstoreListCmd,
		},
	}
)

const (
	BLOCKSTORE_PATH = "blockstore"
)

func cmdBlockStoreRegister(c *cli.Context) {
	if err := doBlockStoreRegister(c); err != nil {
		panic(err)
	}
}

func doBlockStoreRegister(c *cli.Context) error {
	config, _, err := loadGlobalConfig(c)
	if err != nil {
		return nil
	}
	kind := c.String("kind")
	if kind == "" {
		return genRequiredMissingError("kind")
	}
	opts := utils.SliceToMap(c.StringSlice("opts"))
	if opts == nil {
		return genRequiredMissingError("opts")
	}

	id, blockSize, err := blockstores.Register(config.Root, kind, opts)
	if err != nil {
		return err
	}

	api.ResponseOutput(api.BlockStoreResponse{
		UUID:      id,
		Kind:      kind,
		BlockSize: blockSize,
	})
	return nil
}

func cmdBlockStoreDeregister(c *cli.Context) {
	if err := doBlockStoreDeregister(c); err != nil {
		panic(err)
	}
}

func doBlockStoreDeregister(c *cli.Context) error {
	config, _, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	uuid := c.String("uuid")
	if uuid == "" {
		return genRequiredMissingError("uuid")
	}
	return blockstores.Deregister(config.Root, uuid)
}

func cmdBlockStoreAdd(c *cli.Context) {
	if err := doBlockStoreAdd(c); err != nil {
		panic(err)
	}
}

func doBlockStoreAdd(c *cli.Context) error {
	config, _, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	blockstoreUUID := c.String("uuid")
	if blockstoreUUID == "" {
		return genRequiredMissingError("uuid")
	}
	volumeUUID := c.String("volume-uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("volume-uuid")
	}

	volume := config.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	return blockstores.AddVolume(config.Root, blockstoreUUID, volumeUUID, volume.Base, volume.Size)
}

func cmdBlockStoreRemove(c *cli.Context) {
	if err := doBlockStoreRemove(c); err != nil {
		panic(err)
	}
}

func doBlockStoreRemove(c *cli.Context) error {
	config, _, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	blockstoreUUID := c.String("uuid")
	if blockstoreUUID == "" {
		return genRequiredMissingError("uuid")
	}
	volumeUUID := c.String("volume-uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("volume-uuid")
	}

	if config.loadVolume(volumeUUID) == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	return blockstores.RemoveVolume(config.Root, blockstoreUUID, volumeUUID)
}

func cmdBlockStoreList(c *cli.Context) {
	if err := doBlockStoreList(c); err != nil {
		panic(err)
	}
}

func doBlockStoreList(c *cli.Context) error {
	config, _, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	blockstoreUUID := c.String("uuid")
	if blockstoreUUID == "" {
		return genRequiredMissingError("uuid")
	}
	volumeUUID := c.String("volume-uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("volume-uuid")
	}
	snapshotUUID := c.String("snapshot-uuid")

	return blockstores.List(config.Root, blockstoreUUID, volumeUUID, snapshotUUID)
}

func cmdSnapshotBackup(c *cli.Context) {
	if err := doSnapshotBackup(c); err != nil {
		panic(err)
	}
}

func doSnapshotBackup(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	blockstoreUUID := c.String("blockstore-uuid")
	if blockstoreUUID == "" {
		return genRequiredMissingError("uuid")
	}
	volumeUUID := c.String("volume-uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("volume-uuid")
	}
	snapshotUUID := c.String("uuid")
	if snapshotUUID == "" {
		return genRequiredMissingError("uuid")
	}

	if !config.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	return blockstores.BackupSnapshot(config.Root, snapshotUUID, volumeUUID, blockstoreUUID, driver)
}

func cmdSnapshotRestore(c *cli.Context) {
	if err := doSnapshotRestore(c); err != nil {
		panic(err)
	}
}

func doSnapshotRestore(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	blockstoreUUID := c.String("blockstore-uuid")
	if blockstoreUUID == "" {
		return genRequiredMissingError("uuid")
	}
	originVolumeUUID := c.String("origin-volume-uuid")
	if originVolumeUUID == "" {
		return genRequiredMissingError("origin-volume-uuid")
	}
	targetVolumeUUID := c.String("target-volume-uuid")
	if targetVolumeUUID == "" {
		return genRequiredMissingError("target-volume-uuid")
	}
	snapshotUUID := c.String("uuid")
	if snapshotUUID == "" {
		return genRequiredMissingError("uuid")
	}

	originVol := config.loadVolume(originVolumeUUID)
	if originVol == nil {
		return fmt.Errorf("volume %v doesn't exist", originVolumeUUID)
	}
	if _, exists := originVol.Snapshots[snapshotUUID]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, originVolumeUUID)
	}
	targetVol := config.loadVolume(targetVolumeUUID)
	if targetVol == nil {
		return fmt.Errorf("volume %v doesn't exist", targetVolumeUUID)
	}
	if originVol.Size != targetVol.Size || originVol.Base != targetVol.Base {
		return fmt.Errorf("target volume %v doesn't match original volume %v's size or base",
			targetVolumeUUID, originVolumeUUID)
	}

	return blockstores.RestoreSnapshot(config.Root, snapshotUUID, originVolumeUUID,
		targetVolumeUUID, blockstoreUUID, driver)
}

func cmdSnapshotRemove(c *cli.Context) {
	if err := doSnapshotRemove(c); err != nil {
		panic(err)
	}
}

func doSnapshotRemove(c *cli.Context) error {
	config, _, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	blockstoreUUID := c.String("blockstore-uuid")
	if blockstoreUUID == "" {
		return genRequiredMissingError("uuid")
	}
	volumeUUID := c.String("volume-uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("volume-uuid")
	}
	snapshotUUID := c.String("uuid")
	if snapshotUUID == "" {
		return genRequiredMissingError("uuid")
	}

	if !config.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	return blockstores.RemoveSnapshot(config.Root, snapshotUUID, volumeUUID, blockstoreUUID)
}
