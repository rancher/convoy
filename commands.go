package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	_ "github.com/yasker/volmgr/devmapper"
	"github.com/yasker/volmgr/drivers"
	"github.com/yasker/volmgr/utils"
	"os"
	"path/filepath"
)

func doInitialize(root, driverName string, driverOpts map[string]string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		if err := os.MkdirAll(root, os.ModeDir|0700); err != nil {
			return err
		}
	}
	log.Debug("Config root is", root)

	driverRoot := filepath.Join(root, driverName) + "/"
	log.Debug("Driver root is", driverRoot)
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

func doInfo(config *Config) error {
	fmt.Println("Driver: " + config.Driver)
	driver, err := drivers.GetDriver(config.Driver, filepath.Join(config.Root, config.Driver)+"/", nil)
	if err != nil {
		return err
	}
	err = driver.Info()
	return err
}

func doVolumeCreate(config *Config, size uint64) error {
	return nil
}

func doVolumeDelete(config *Config, uuid string) error {
	return nil
}

func doVolumeUpdate(config *Config, uuid string, size uint64) error {
	return nil
}

func doVolumeList(config *Config) error {
	return nil
}
