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

	configFileName := filepath.Join(root, CONFIGFILE)
	config := Config{
		Root:   root,
		Driver: driverName,
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
	base := ""
	err := driver.CreateVolume(uuid, base, size)
	return err
}

func doVolumeDelete(config *Config, driver drivers.Driver, uuid string) error {
	log.Debug("Deleting volume using ", config.Driver)
	err := driver.DeleteVolume(uuid)
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
	uuid := uuid.New()
	err := driver.CreateSnapshot(uuid, volumeUUID)
	return err
}

func doSnapshotDelete(config *Config, driver drivers.Driver, uuid, volumeUUID string) error {
	err := driver.DeleteSnapshot(uuid, volumeUUID)
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
	return blockstores.AddVolume(getBlockStoreRoot(config.Root), blockstoreId, volumeId)
}

func doBlockStoreRemove(config *Config, blockstoreId, volumeId string) error {
	return blockstores.RemoveVolume(getBlockStoreRoot(config.Root), blockstoreId, volumeId)
}
