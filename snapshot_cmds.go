package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/utils"
)

var (
	snapshotCreateCmd = cli.Command{
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

	snapshotDeleteCmd = cli.Command{
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

	snapshotCmd = cli.Command{
		Name:  "snapshot",
		Usage: "snapshot related operations",
		Subcommands: []cli.Command{
			snapshotCreateCmd,
			snapshotDeleteCmd,
			snapshotBackupCmd,  // in blockstore_cmds.go
			snapshotRestoreCmd, // in blockstore_cmds.go
			snapshotRemoveCmd,  // in blockstore_cmds.go
		},
	}
)

func duplicateSnapshotUUID(config *Config, volumeUUID, snapshotUUID string) bool {
	volume, exists := config.Volumes[volumeUUID]
	if !exists {
		return false
	}
	_, exists = volume.Snapshots[snapshotUUID]
	return exists
}

func cmdSnapshotCreate(c *cli.Context) {
	if err := doSnapshotCreate(c); err != nil {
		panic(err)
	}
}

func doSnapshotCreate(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	volumeUUID := c.String("volume-uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("volume-uuid")
	}
	snapshotUUID := c.String("uuid")

	if _, exists := config.Volumes[volumeUUID]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	uuid := uuid.New()
	if snapshotUUID != "" {
		if duplicateSnapshotUUID(config, volumeUUID, snapshotUUID) {
			return fmt.Errorf("Duplicate snapshot UUID for volume %v detected", volumeUUID)
		}
		uuid = snapshotUUID
	}
	if err := driver.CreateSnapshot(uuid, volumeUUID); err != nil {
		return err
	}
	log.Debugf("Created snapshot %v of volume %v using %v\n", uuid, volumeUUID, config.Driver)

	config.Volumes[volumeUUID].Snapshots[uuid] = true
	if err := utils.SaveConfig(config.Root, getCfgName(), config); err != nil {
		return err
	}
	api.ResponseOutput(api.SnapshotResponse{
		UUID:       uuid,
		VolumeUUID: volumeUUID,
	})
	return nil
}

func cmdSnapshotDelete(c *cli.Context) {
	if err := doSnapshotDelete(c); err != nil {
		panic(err)
	}
}

func doSnapshotDelete(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	uuid := c.String("uuid")
	if uuid == "" {
		return genRequiredMissingError("uuid")
	}
	volumeUUID := c.String("volume-uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("volume-uuid")
	}

	if _, exists := config.Volumes[volumeUUID]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if _, exists := config.Volumes[volumeUUID].Snapshots[uuid]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", uuid, volumeUUID)
	}
	if err := driver.DeleteSnapshot(uuid, volumeUUID); err != nil {
		return err
	}
	log.Debugf("Deleted snapshot %v of volume %v using %v\n", uuid, volumeUUID, config.Driver)

	delete(config.Volumes[volumeUUID].Snapshots, uuid)
	return utils.SaveConfig(config.Root, getCfgName(), config)
}
