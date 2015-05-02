package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/blockstores"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/utils"
	"os"
	"path/filepath"
)

func getDriverRoot(root, driverName string) string {
	return filepath.Join(root, driverName)
}

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
	if _, err := os.Stat(configFileName); err == nil {
		return fmt.Errorf("Configuration file %v existed. Don't need to initialize.", configFileName)
	}

	driverRoot := getDriverRoot(root, driverName)
	utils.MkdirIfNotExists(driverRoot)
	log.Debug("Driver root is ", driverRoot)

	_, err := drivers.GetDriver(driverName, driverRoot, driverOpts)
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

	driver, err := drivers.GetDriver(config.Driver, getDriverRoot(config.Root, config.Driver), nil)
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

func cmdVolumeCreate(c *cli.Context) {
	if err := doVolumeCreate(c); err != nil {
		panic(err)
	}
}

func duplicateVolumeUUID(config *Config, uuid string) bool {
	_, exists := config.Volumes[uuid]
	return exists
}

func doVolumeCreate(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}

	size := int64(c.Int("size"))
	if size == 0 {
		return genRequiredMissingError("size")
	}
	volumeUUID := c.String("uuid")

	uuid := uuid.New()
	if volumeUUID != "" {
		if duplicateVolumeUUID(config, uuid) {
			return fmt.Errorf("Duplicate volume UUID detected!")
		}
		uuid = volumeUUID
	}
	base := "" //Doesn't support base for now
	if err := driver.CreateVolume(uuid, base, size); err != nil {
		return err
	}
	log.Debug("Created volume using ", config.Driver)
	config.Volumes[uuid] = Volume{
		Base:       base,
		Size:       size,
		MountPoint: "",
		FileSystem: "",
		Snapshots:  make(map[string]bool),
	}
	if err := utils.SaveConfig(getConfigFileName(config.Root), config); err != nil {
		return err
	}
	api.ResponseOutput(api.VolumeResponse{
		UUID: uuid,
		Base: base,
		Size: size,
	})
	return nil
}

func cmdVolumeDelete(c *cli.Context) {
	if err := doVolumeDelete(c); err != nil {
		panic(err)
	}
}

func doVolumeDelete(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	uuid := c.String("uuid")
	if uuid == "" {
		return genRequiredMissingError("uuid")
	}

	if err := driver.DeleteVolume(uuid); err != nil {
		return err
	}
	log.Debug("Deleted volume using ", config.Driver)
	delete(config.Volumes, uuid)
	return utils.SaveConfig(getConfigFileName(config.Root), config)
}

func cmdVolumeList(c *cli.Context) {
	if err := doVolumeList(c); err != nil {
		panic(err)
	}
}

func doVolumeList(c *cli.Context) error {
	_, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	uuid := c.String("uuid")
	return driver.ListVolume(uuid)
}

func cmdVolumeMount(c *cli.Context) {
	if err := doVolumeMount(c); err != nil {
		panic(err)
	}
}

func doVolumeMount(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	volumeUUID := c.String("uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("uuid")
	}
	mountPoint := c.String("mountpoint")
	if mountPoint == "" {
		return genRequiredMissingError("mountpoint")
	}
	fs := c.String("fs")
	if fs == "" {
		return genRequiredMissingError("fs")
	}

	option := c.String("option")
	needFormat := c.Bool("format")
	newNS := c.String("switch-ns")

	volume, exists := config.Volumes[volumeUUID]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if err := drivers.Mount(driver, volumeUUID, mountPoint, fs, option, needFormat, newNS); err != nil {
		return err
	}
	log.Debugf("Mount %v to %v", volumeUUID, mountPoint)
	volume.MountPoint = mountPoint
	volume.FileSystem = fs
	config.Volumes[volumeUUID] = volume
	return utils.SaveConfig(getConfigFileName(config.Root), config)
}

func cmdVolumeUmount(c *cli.Context) {
	if err := doVolumeUmount(c); err != nil {
		panic(err)
	}
}

func doVolumeUmount(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	volumeUUID := c.String("uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("uuid")
	}
	newNS := c.String("switch-ns")

	volume, exists := config.Volumes[volumeUUID]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if err := drivers.Unmount(driver, volume.MountPoint, newNS); err != nil {
		return err
	}
	log.Debugf("Unmount %v from %v", volumeUUID, volume.MountPoint)
	volume.MountPoint = ""
	config.Volumes[volumeUUID] = volume
	return utils.SaveConfig(getConfigFileName(config.Root), config)
}

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
	if err := utils.SaveConfig(getConfigFileName(config.Root), config); err != nil {
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
	return utils.SaveConfig(getConfigFileName(config.Root), config)
}

const (
	BLOCKSTORE_PATH = "blockstore"
)

func getBlockStoreRoot(root string) string {
	return filepath.Join(root, BLOCKSTORE_PATH) + "/"
}

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

	path := getBlockStoreRoot(config.Root)
	err = utils.MkdirIfNotExists(path)
	if err != nil {
		return err
	}
	id, blockSize, err := blockstores.Register(path, kind, opts)
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
	return blockstores.Deregister(getBlockStoreRoot(config.Root), uuid)
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

	volume, exists := config.Volumes[volumeUUID]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	return blockstores.AddVolume(getBlockStoreRoot(config.Root), blockstoreUUID, volumeUUID, volume.Base, volume.Size)
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

	if _, exists := config.Volumes[volumeUUID]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	return blockstores.RemoveVolume(getBlockStoreRoot(config.Root), blockstoreUUID, volumeUUID)
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

	if _, exists := config.Volumes[volumeUUID]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if _, exists := config.Volumes[volumeUUID].Snapshots[snapshotUUID]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	return blockstores.BackupSnapshot(getBlockStoreRoot(config.Root), snapshotUUID, volumeUUID, blockstoreUUID, driver)
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

	originVol, exists := config.Volumes[originVolumeUUID]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", originVolumeUUID)
	}
	if _, exists := config.Volumes[originVolumeUUID].Snapshots[snapshotUUID]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, originVolumeUUID)
	}
	targetVol, exists := config.Volumes[targetVolumeUUID]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", targetVolumeUUID)
	}
	if originVol.Size != targetVol.Size || originVol.Base != targetVol.Base {
		return fmt.Errorf("target volume %v doesn't match original volume %v's size or base",
			targetVolumeUUID, originVolumeUUID)
	}

	return blockstores.RestoreSnapshot(getBlockStoreRoot(config.Root), snapshotUUID, originVolumeUUID,
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

	if _, exists := config.Volumes[volumeUUID]; !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if _, exists := config.Volumes[volumeUUID].Snapshots[snapshotUUID]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	return blockstores.RemoveSnapshot(getBlockStoreRoot(config.Root), snapshotUUID, volumeUUID, blockstoreUUID)
}
