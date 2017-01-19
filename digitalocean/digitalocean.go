package digitalocean

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	. "github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/util"
)

const (
	DRIVER_NAME        = "digitalocean"
	DRIVER_CONFIG_FILE = "digitalocean.cfg"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_PREFIX        = DRIVER_NAME + "_"
	CFG_SUFFIX        = ".json"

	DO_DEFAULT_VOLUME_SIZE = "do.defaultvolumesize"
	DEFAULT_VOLUME_SIZE    = "10G"

	DO_DEVICE_FOLDER = "/dev/disk/by-id"
	DO_DEVICE_PREFIX = "scsi-0DO_Volume_"
	DO_VOLUME_FS     = "ext4"

	MOUNTS_DIR = "mounts"

	GB = 1073741824
)

// Driver is a convoy driver for DigitalOcean volumes
type Driver struct {
	mutex  *sync.RWMutex
	client *Client
	Device
}

type Device struct {
	Root              string
	DefaultVolumeSize int64
}

func (d *Device) ConfigFile() (string, error) {
	if d.Root == "" {
		return "", errors.New("BUG: invalid empty device config path")
	}
	return filepath.Join(d.Root, DRIVER_CONFIG_FILE), nil
}

type Volume struct {
	Name       string
	ID         string
	Device     string
	MountPoint string
	Size       int64
	configPath string
}

func (v *Volume) ConfigFile() (string, error) {
	if v.Name == "" {
		return "", errors.New("empty volume")
	}
	if v.configPath == "" {
		return "", errors.New("empty config path")
	}

	return filepath.Join(v.configPath, CFG_PREFIX+VOLUME_CFG_PREFIX+v.Name+CFG_SUFFIX), nil
}

func (v *Volume) GetDevice() (string, error) {
	return v.Device, nil
}

func (v *Volume) GetMountOpts() []string {
	return []string{}
}

func (v *Volume) GenerateDefaultMountPoint() string {
	return filepath.Join(v.configPath, MOUNTS_DIR, v.Name)
}

func (d *Driver) blankVolume(name string) *Volume {
	return &Volume{
		configPath: d.Root,
		Name:       name,
	}
}

func (d *Driver) remountVolumes() error {
	volumes, err := util.ListConfigIDs(d.Root, CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_SUFFIX)
	if err != nil {
		return err
	}

	for _, id := range volumes {
		vol := d.blankVolume(id)
		if err := util.ObjectLoad(vol); err != nil {
			return err
		}
		if vol.MountPoint == "" {
			continue
		}

		req := Request{
			Name:    id,
			Options: map[string]string{},
		}
		if _, err := d.MountVolume(req); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	if err := Register(DRIVER_NAME, Init); err != nil {
		panic(err)
	}
}

func Init(root string, config map[string]string) (ConvoyDriver, error) {
	dev := &Device{
		Root: root,
	}
	exists, err := util.ObjectExists(dev)
	if err != nil {
		return nil, err
	}

	if exists {
		if err := util.ObjectLoad(dev); err != nil {
			return nil, err
		}
	} else {
		if err := util.MkdirIfNotExists(root); err != nil {
			return nil, err
		}

		if config[DO_DEFAULT_VOLUME_SIZE] == "" {
			config[DO_DEFAULT_VOLUME_SIZE] = DEFAULT_VOLUME_SIZE
		}
		size, err := util.ParseSize(config[DO_DEFAULT_VOLUME_SIZE])
		if err != nil {
			return nil, err
		}

		dev = &Device{
			Root:              root,
			DefaultVolumeSize: size,
		}
		if err := util.ObjectSave(dev); err != nil {
			return nil, err
		}
	}
	client, err := NewClient()
	if err != nil {
		return nil, err
	}
	driver := &Driver{
		Device: *dev,
		client: client,
		mutex:  new(sync.RWMutex),
	}

	if err := driver.remountVolumes(); err != nil {
		return nil, err
	}
	return driver, nil
}

func (d *Driver) Name() string {
	return DRIVER_NAME
}

func (d *Driver) Info() (map[string]string, error) {
	ret := map[string]string{
		"DefaultVolumeSize": strconv.FormatInt(d.DefaultVolumeSize, 10),
	}
	return ret, nil
}

func (d *Driver) VolumeOps() (VolumeOperations, error) {
	return d, nil
}

func (d *Driver) CreateVolume(req Request) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	opt := req.Options
	vol := d.blankVolume(id)
	var (
		size   int64
		format bool
	)

	exists, err := util.ObjectExists(vol)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("volume %s already exists", id)
	}
	// DigitalOcean Volume ID
	vID := opt[OPT_VOLUME_DRIVER_ID]
	if vID != "" {
		doVol, err := d.client.GetVolume(vID)
		if err != nil {
			return err
		}

		size = doVol.SizeGigaBytes * GB
	} else {
		// Create new volume
		vSize, err := d.getSize(opt, d.DefaultVolumeSize)
		if err != nil {
			return err
		}

		vID, err = d.client.CreateVolume(id, vSize)
		if err != nil {
			return err
		}
		size = vSize
		format = true
	}

	if err := d.client.AttachVolume(vID); err != nil {
		return err
	}

	vol.Name = id
	vol.ID = vID
	vol.Device = filepath.Join(DO_DEVICE_FOLDER, DO_DEVICE_PREFIX+id)
	vol.Size = size

	if format {
		if err := formatDevice(vol.Device, DO_VOLUME_FS); err != nil {
			return err
		}
	}
	return util.ObjectSave(vol)
}

func (d *Driver) DeleteVolume(req Request) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	opts := req.Options

	vol := d.blankVolume(id)
	if err := util.ObjectLoad(vol); err != nil {
		return err
	}

	refOnly, _ := strconv.ParseBool(opts[OPT_REFERENCE_ONLY])

	if err := d.client.DetachVolume(vol.ID); err != nil {
		return err
	}

	if !refOnly {
		if err := d.client.DeleteVolume(vol.ID); err != nil {
			return err
		}
	}
	return util.ObjectDelete(vol)
}

func (d *Driver) MountVolume(req Request) (string, error) {
	id := req.Name
	opts := req.Options

	vol := d.blankVolume(id)
	if err := util.ObjectLoad(vol); err != nil {
		return "", err
	}

	mountPoint, err := util.VolumeMount(vol, opts[OPT_MOUNT_POINT], false)
	if err != nil {
		return "", err
	}

	if err := util.ObjectSave(vol); err != nil {
		return "", err
	}

	return mountPoint, nil
}

func (d *Driver) UmountVolume(req Request) error {
	id := req.Name

	vol := d.blankVolume(id)
	if err := util.ObjectLoad(vol); err != nil {
		return err
	}

	if err := util.VolumeUmount(vol); err != nil {
		return err
	}

	return util.ObjectSave(vol)
}

func (d *Driver) MountPoint(req Request) (string, error) {
	id := req.Name

	vol := d.blankVolume(id)
	if err := util.ObjectLoad(vol); err != nil {
		return "", err
	}

	return vol.MountPoint, nil
}

func (d *Driver) GetVolumeInfo(name string) (map[string]string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	vol := d.blankVolume(name)
	if err := util.ObjectLoad(vol); err != nil {
		return nil, err
	}

	doVol, err := d.client.GetVolume(vol.ID)
	if err != nil {
		return nil, err
	}

	size := doVol.SizeGigaBytes * GB
	info := map[string]string{
		"Device":        vol.Device,
		"MountPoint":    vol.MountPoint,
		"ID":            vol.ID,
		OPT_VOLUME_NAME: name,
		"Size":          strconv.FormatInt(size, 10),
	}
	return info, nil
}

func (d *Driver) ListVolume(opts map[string]string) (map[string]map[string]string, error) {
	volumes, err := util.ListConfigIDs(d.Root, CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_SUFFIX)
	if err != nil {
		return nil, err
	}
	ret := make(map[string]map[string]string)
	for _, id := range volumes {
		ret[id], err = d.GetVolumeInfo(id)
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func (d *Driver) getSize(opts map[string]string, defaultVolumeSize int64) (int64, error) {
	size := opts[OPT_SIZE]
	if size == "" || size == "0" {
		size = strconv.FormatInt(defaultVolumeSize, 10)

	}
	return util.ParseSize(size)
}

func formatDevice(device, fs string) error {
	_, err := util.Execute("mkfs", []string{"-t", fs, device})
	return err
}

// These methods are not implemented currently at DigitalOcean
func (d *Driver) SnapshotOps() (SnapshotOperations, error) {
	return nil, errors.New("not implemented")
}

func (d *Driver) BackupOps() (BackupOperations, error) {
	return nil, errors.New("not implemented")
}
