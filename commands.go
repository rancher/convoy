package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/yasker/volmgr/blockstores"
	"github.com/yasker/volmgr/drivers"
	"github.com/yasker/volmgr/utils"
	"os"
	"os/exec"
	"path/filepath"
)

func getDriverRoot(root, driverName string) string {
	return filepath.Join(root, driverName) + "/"
}

func doInitialize(root, driverName string, driverOpts map[string]string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		if err := os.MkdirAll(root, os.ModeDir|0700); err != nil {
			return err
		}
	}
	log.Debug("Config root is ", root)

	driverRoot := getDriverRoot(root, driverName)
	log.Debug("Driver root is ", driverRoot)
	if _, err := os.Stat(driverRoot); os.IsNotExist(err) {
		if err := os.MkdirAll(driverRoot, os.ModeDir|0700); err != nil {
			return err
		}
	}

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

func getBlockStoreConfigFilename(config *Config, kind, id string) string {
	return filepath.Join(filepath.Join(config.Root, BLOCKSTORE_PATH)+"/", kind+"-"+id+".cfg")
}

func doBlockStoreRegister(config *Config, kind string, opts map[string]string) error {
	id := uuid.New()
	configFile := getBlockStoreConfigFilename(config, kind, id)
	_, err := blockstores.GetBlockStore(kind, configFile, id, opts)
	return err
}

func doBlockStoreDeregister(config *Config, kind, id string) error {
	configFile := getBlockStoreConfigFilename(config, kind, id)
	err := exec.Command("rm", "-f", configFile).Run()
	return err
}
