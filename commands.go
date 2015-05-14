package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/utils"
	"path/filepath"
	"strings"
)

func getConfigFileName(root string) string {
	return filepath.Join(root, CONFIGFILE)
}

func getCfgName() string {
	return CONFIGFILE
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
	imagesDir := c.String("images-dir")
	if root == "" || driverName == "" || driverOpts == nil || imagesDir == "" {
		return fmt.Errorf("Missing or invalid parameters")
	}

	log.Debug("Config root is ", root)

	if utils.ConfigExists(root, getCfgName()) {
		return fmt.Errorf("Configuration file already existed. Don't need to initialize.")
	}

	if err := utils.MkdirIfNotExists(imagesDir); err != nil {
		return err
	}
	log.Debug("Images would be stored at ", imagesDir)

	_, err := drivers.GetDriver(driverName, root, driverOpts)
	if err != nil {
		return err
	}

	config := Config{
		Root:      root,
		Driver:    driverName,
		ImagesDir: imagesDir,
	}
	err = utils.SaveConfig(root, getCfgName(), &config)
	return err
}

func loadGlobalConfig(c *cli.Context) (*Config, drivers.Driver, error) {
	config := Config{}
	root := c.GlobalString("root")
	if root == "" {
		return nil, nil, genRequiredMissingError("root")
	}
	err := utils.LoadConfig(root, getCfgName(), &config)
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

func getLowerCaseFlag(c *cli.Context, name string, required bool, err error) (string, error) {
	if err != nil {
		return "", err
	}
	result := strings.ToLower(c.String(name))
	if required && result == "" {
		err = genRequiredMissingError(name)
	}
	return result, err
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
