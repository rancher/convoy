package vfs

import (
	"fmt"
	"github.com/rancher/rancher-volume/storagedriver"
	"github.com/rancher/rancher-volume/util"
	"path/filepath"
	"sync"
)

const (
	DRIVER_NAME        = "vfs"
	DRIVER_CONFIG_FILE = "vfs.cfg"

	VOLUME_CFG_PREFIX = "volume_"
	VFS_CFG_PREFIX    = DRIVER_NAME + "_"
	CFG_POSTFIX       = ".json"
)

type Driver struct {
	mutex *sync.RWMutex
	Device
}

type Device struct {
	Root string
	Path string
}

type Volume struct {
	UUID       string
	Path       string
	MountPoint string
}

func init() {
	storagedriver.Register(DRIVER_NAME, Init)
}

func (d *Driver) Name() string {
	return DRIVER_NAME
}

func (device *Device) getVolumeConfig(uuid string) (string, error) {
	if uuid == "" {
		return "", fmt.Errorf("Invalid volume UUID specified: %v", uuid)
	}
	return filepath.Join(device.Root, VFS_CFG_PREFIX+VOLUME_CFG_PREFIX+uuid+CFG_POSTFIX), nil
}

func (device *Device) loadVolume(uuid string) *Volume {
	config, err := device.getVolumeConfig(uuid)
	if err != nil {
		return nil
	}
	if !util.ConfigExists(config) {
		return nil
	}
	volume := &Volume{}
	if err := util.LoadConfig(config, volume); err != nil {
		log.Error("Failed to load volume json ", config)
		return nil
	}
	return volume
}

func (device *Device) checkLoadVolume(uuid string) (*Volume, error) {
	volume := device.loadVolume(uuid)
	if volume == nil {
		return nil, fmt.Errorf("Cannot find volume %v", uuid)
	}
	return volume, nil
}

func (device *Device) saveVolume(volume *Volume) error {
	config, err := device.getVolumeConfig(volume.UUID)
	if err != nil {
		return err
	}
	return util.SaveConfig(config, volume)
}

func (device *Device) deleteVolume(uuid string) error {
	config, err := device.getVolumeConfig(uuid)
	if err != nil {
		return err
	}
	return util.RemoveConfig(config)
}

func (device *Device) listVolumeIDs() ([]string, error) {
	return util.ListConfigIDs(device.Root, VFS_CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_POSTFIX)
}

func Init(root string, config map[string]string) (storagedriver.StorageDriver, error) {
	cfg := DRIVER_CONFIG_FILE
	if util.ConfigExists(filepath.Join(root, cfg)) {
		dev := Device{}
		if err := util.LoadConfig(filepath.Join(root, cfg), &dev); err != nil {
			return nil, err
		}
		d := &Driver{
			mutex:  &sync.RWMutex{},
			Device: dev,
		}
		return d, nil
	}

	if err := util.MkdirIfNotExists(root); err != nil {
		return nil, err
	}

	path := config[VFS_PATH]
	if path == "" {
		return nil, fmt.Errorf("VFS driver base path unspecified")
	}
	if err := util.MkdirIfNotExists(path); err != nil {
		return nil, err
	}
	dev := Device{
		Root: root,
		Path: path,
	}
	if err := util.SaveConfig(filepath.Join(root, cfg), &dev); err != nil {
		return nil, err
	}
	d := &Driver{
		mutex:  &sync.RWMutex{},
		Device: dev,
	}

	return d, nil
}

func (d *Driver) Info() (map[string]string, error) {
	return map[string]string{
		"Root": d.Root,
		"Path": d.Path,
	}, nil
}

func (d *Driver) VolumeOps() (storagedriver.VolumeOperations, error) {
	return d, nil
}

func (d *Driver) SnapshotOps() (storagedriver.SnapshotOperations, error) {
	return nil, fmt.Errorf("VFS driver doesn't support snapshot operations")
}

func (d *Driver) CreateVolume(id string, opts map[string]string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume := d.loadVolume(id)
	if volume != nil {
		return fmt.Errorf("volume %v already exists", id)
	}

	volumePath := filepath.Join(d.Path, id)
	if err := util.MkdirIfNotExists(volumePath); err != nil {
		return err
	}
	volume = &Volume{
		UUID: id,
		Path: volumePath,
	}
	return d.saveVolume(volume)
}

func (d *Driver) DeleteVolume(id string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return err
	}

	if volume.MountPoint != "" {
		return fmt.Errorf("Cannot delete volume %v. It is still mounted", id)
	}
	if out, err := util.Execute("rm", []string{"-rf", volume.Path}); err != nil {
		return fmt.Errorf("Fail to delete the volume, output: %v, error: %v", out, err.Error())
	}
	return d.deleteVolume(id)
}

func (d *Driver) MountVolume(id string, opts map[string]string) (string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return "", err
	}

	specifiedPoint := opts[storagedriver.OPTS_MOUNT_POINT]
	if specifiedPoint != "" {
		return "", fmt.Errorf("VFS doesn't support specified mount point")
	}
	if volume.MountPoint == "" {
		volume.MountPoint = volume.Path
	}
	if err := d.saveVolume(volume); err != nil {
		return "", err
	}
	return volume.MountPoint, nil
}

func (d *Driver) UmountVolume(id string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return err
	}

	if volume.MountPoint != "" {
		volume.MountPoint = ""
	}
	return d.saveVolume(volume)
}

func (d *Driver) ListVolume(opts map[string]string) (map[string]map[string]string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	volumeIDs, err := d.listVolumeIDs()
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]string{}
	for _, id := range volumeIDs {
		result[id], err = d.GetVolumeInfo(id)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (d *Driver) GetVolumeInfo(id string) (map[string]string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"Path": volume.Path,
		storagedriver.OPTS_MOUNT_POINT: volume.MountPoint,
	}, nil
}

func (d *Driver) MountPoint(id string) (string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return "", err
	}
	return volume.MountPoint, nil
}
