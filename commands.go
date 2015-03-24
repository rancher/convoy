package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/yasker/volmgr/blockstores"
	"github.com/yasker/volmgr/drivers"
	"github.com/yasker/volmgr/utils"
	"path/filepath"
)

func getDriverRoot(root, driverName string) string {
	return filepath.Join(root, driverName) + "/"
}

func getConfigFileName(root string) string {
	return filepath.Join(root, CONFIGFILE)
}

func doInitialize(root, driverName string, driverOpts map[string]string) error {
	utils.MkdirIfNotExists(root)
	log.Debug("Config root is ", root)

	driverRoot := getDriverRoot(root, driverName)
	utils.MkdirIfNotExists(driverRoot)
	log.Debug("Driver root is ", driverRoot)

	_, err := drivers.GetDriver(driverName, driverRoot, driverOpts)
	if err != nil {
		return err
	}

	configFileName := getConfigFileName(root)
	config := Config{
		Root:    root,
		Driver:  driverName,
		Volumes: make(map[string]Volume),
	}
	err = utils.SaveConfig(configFileName, &config)
	return err
}

func doInfo(config *Config, driver drivers.Driver) error {
	fmt.Println("Driver: " + config.Driver)
	err := driver.Info()
	return err
}

func doVolumeCreate(config *Config, driver drivers.Driver, size uint64) error {
	uuid := uuid.New()
	base := "" //Doesn't support base for now
	if err := driver.CreateVolume(uuid, base, size); err != nil {
		return err
	}
	log.Debug("Created volume using ", config.Driver)
	config.Volumes[uuid] = Volume{
		Base:      base,
		Size:      size,
		Snapshots: make(map[string]bool),
	}
	err := utils.SaveConfig(getConfigFileName(config.Root), config)
	return err
}

func doVolumeDelete(config *Config, driver drivers.Driver, uuid string) error {
	if err := driver.DeleteVolume(uuid); err != nil {
		return err
	}
	log.Debug("Deleted volume using ", config.Driver)
	delete(config.Volumes, uuid)
	err := utils.SaveConfig(getConfigFileName(config.Root), config)
	return err
}

func doVolumeUpdate(config *Config, driver drivers.Driver, uuid string, size uint64) error {
	return nil
}

func doVolumeList(config *Config, driver drivers.Driver) error {
	err := driver.ListVolumes()
	return err
}

func doSnapshotCreate(config *Config, driver drivers.Driver, volumeUUID string) error {
	if _, exists := config.Volumes[volumeUUID]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	uuid := uuid.New()
	if err := driver.CreateSnapshot(uuid, volumeUUID); err != nil {
		return err
	}
	log.Debugf("Created snapshot %v of volume %v using %v\n", uuid, volumeUUID, config.Driver)

	config.Volumes[volumeUUID].Snapshots[uuid] = true
	err := utils.SaveConfig(getConfigFileName(config.Root), config)
	return err
}

func doSnapshotDelete(config *Config, driver drivers.Driver, uuid, volumeUUID string) error {
	if _, exists := config.Volumes[volumeUUID]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if _, exists := config.Volumes[volumeUUID].Snapshots[uuid]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", uuid, volumeUUID)
	}
	err := driver.DeleteSnapshot(uuid, volumeUUID)
	log.Debugf("Deleted snapshot %v of volume %v using %v\n", uuid, volumeUUID, config.Driver)

	delete(config.Volumes[volumeUUID].Snapshots, uuid)
	err = utils.SaveConfig(getConfigFileName(config.Root), config)
	return err
}

func doSnapshotList(config *Config, driver drivers.Driver, volumeUUID string) error {
	err := driver.ListSnapshot(volumeUUID)
	return err
}

const (
	BLOCKSTORE_PATH = "blockstore"
)

func getBlockStoreRoot(root string) string {
	return filepath.Join(root, BLOCKSTORE_PATH) + "/"
}

func doBlockStoreRegister(config *Config, kind string, opts map[string]string) error {
	id := uuid.New()
	path := getBlockStoreRoot(config.Root)
	err := utils.MkdirIfNotExists(path)
	if err != nil {
		return err
	}
	return blockstores.Register(path, kind, id, opts)
}

func doBlockStoreDeregister(config *Config, kind, id string) error {
	return blockstores.Deregister(getBlockStoreRoot(config.Root), kind, id)
}

func doBlockStoreAdd(config *Config, blockstoreId, volumeId string) error {
	volume, exists := config.Volumes[volumeId]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeId)
	}

	return blockstores.AddVolume(getBlockStoreRoot(config.Root), blockstoreId, volumeId, volume.Base, volume.Size)
}

func doBlockStoreRemove(config *Config, blockstoreId, volumeId string) error {
	if _, exists := config.Volumes[volumeId]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeId)
	}

	return blockstores.RemoveVolume(getBlockStoreRoot(config.Root), blockstoreId, volumeId)
}

func doSnapshotBackup(config *Config, driver drivers.Driver, snapshotId, volumeId, blockstoreId string) error {
	if _, exists := config.Volumes[volumeId]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeId)
	}
	if _, exists := config.Volumes[volumeId].Snapshots[snapshotId]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotId, volumeId)
	}

	return blockstores.BackupSnapshot(getBlockStoreRoot(config.Root), snapshotId, volumeId, blockstoreId, driver)
}

func doSnapshotRestore(config *Config, driver drivers.Driver, snapshotId, originVolumeId, targetVolumeId, blockstoreId string) error {
	originVol, exists := config.Volumes[originVolumeId]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", originVolumeId)
	}
	if _, exists := config.Volumes[originVolumeId].Snapshots[snapshotId]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotId, originVolumeId)
	}
	targetVol, exists := config.Volumes[targetVolumeId]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", targetVolumeId)
	}
	if originVol.Size != targetVol.Size || originVol.Base != targetVol.Base {
		return fmt.Errorf("target volume %v doesn't match original volume %v's size or base",
			targetVolumeId, originVolumeId)
	}

	return blockstores.RestoreSnapshot(getBlockStoreRoot(config.Root), snapshotId, originVolumeId,
		targetVolumeId, blockstoreId, driver)
}
