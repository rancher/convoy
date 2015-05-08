package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/utils"
	"path/filepath"
)

func getConfigFileName(root string) string {
	return filepath.Join(root, CONFIGFILE)
}

func cmdInitialize(c *cli.Context) {
	if err := doInitialize(c); err != nil {
		panic(err)
	}
}

func doInitialize(c *cli.Context) error {
	root := c.GlobalString("root")
	driverName := c.String("driver")
	driverOpts := utils.SliceToMap(c.StringSlice("driver-opts"))
	if root == "" || driverName == "" || driverOpts == nil {
		return fmt.Errorf("Missing or invalid parameters")
	}

	log.Debug("Config root is ", root)

	configFileName := getConfigFileName(root)
	if utils.ConfigExists(configFileName) {
		return fmt.Errorf("Configuration file %v existed. Don't need to initialize.", configFileName)
	}

	_, err := drivers.GetDriver(driverName, root, driverOpts)
	if err != nil {
		return err
	}

	config := Config{
		Root:    root,
		Driver:  driverName,
		Volumes: make(map[string]Volume),
	}
	err = utils.SaveConfig(configFileName, &config)
	return err
}

func loadGlobalConfig(c *cli.Context) (*Config, drivers.Driver, error) {
	config := Config{}
	root := c.GlobalString("root")
	if root == "" {
		return nil, nil, genRequiredMissingError("root")
	}
	configFileName := getConfigFileName(root)
	err := utils.LoadConfig(configFileName, &config)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to load config:", err.Error())
	}

	driver, err := drivers.GetDriver(config.Driver, config.Root, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to load driver:", err.Error())
	}
	return &config, driver, nil
}

func genRequiredMissingError(name string) error {
	return fmt.Errorf("Cannot find valid required parameter:", name)
}

func cmdInfo(c *cli.Context) {
	if err := doInfo(c); err != nil {
		panic(err)
	}
}

func doInfo(c *cli.Context) error {
	_, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	return driver.Info()
}
