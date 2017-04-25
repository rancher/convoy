package profitbricks

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
	DRIVER_NAME        = "profitbricks"
	DRIVER_CONFIG_FILE = "profitbricks.cfg"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_PREFIX        = DRIVER_NAME + "_"
	CFG_SUFFIX        = ".json"

	PB_DEFAULT_VOLUME_SIZE = "profitbricks.defaultvolumesize"
	PB_DEFAULT_VOLUME_TYPE = "profitbricks.defaultvolumetype"
	DEFAULT_VOLUME_SIZE    = "5G"
	DEFAULT_VOLUME_TYPE    = "HDD"

	BASE_DEVICE_PATH = "/dev/vd"
	PB_VOLUME_FS     = "ext4"

	MOUNTS_DIR = "mounts"

	GB = int(1073741824)
)

type Driver struct {
	mutex  *sync.RWMutex
	client Client
	Device
}

func (d *Driver) Name() string {
	return DRIVER_NAME
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

type Device struct {
	Root              string
	DefaultVolumeSize int
	DefaultVolumeType string
}

func (d *Device) ConfigFile() (string, error) {
	if d.Root == "" {
		return "", errors.New("BUG: invalid empty device config path")
	}
	return filepath.Join(d.Root, DRIVER_CONFIG_FILE), nil
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

func init() {
	Register(DRIVER_NAME, Init)
}

func Init(root string, config map[string]string) (ConvoyDriver, error) {
	client, err := InitClient()
	if err != nil {
		return nil, err
	}

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

		if config[PB_DEFAULT_VOLUME_SIZE] == "" {
			config[PB_DEFAULT_VOLUME_SIZE] = DEFAULT_VOLUME_SIZE
		}
		size, err := util.ParseSize(config[PB_DEFAULT_VOLUME_SIZE])
		if err != nil {
			return nil, err
		}
		if config[PB_DEFAULT_VOLUME_TYPE] == "" {
			config[PB_DEFAULT_VOLUME_TYPE] = DEFAULT_VOLUME_TYPE
		}
		volumeType := config[PB_DEFAULT_VOLUME_TYPE]
		if err := checkVolumeType(volumeType); err != nil {
			return nil, err
		}

		dev = &Device{
			Root:              root,
			DefaultVolumeSize: int(size),
			DefaultVolumeType: volumeType,
		}
		if err := util.ObjectSave(dev); err != nil {
			return nil, err
		}
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

func checkVolumeType(volumeType string) error {
	validVolumeType := map[string]bool{
		"HDD": true,
		"SSD": true,
	}
	if !validVolumeType[volumeType] {
		return fmt.Errorf("Invalid volume type %v", volumeType)
	}
	return nil
}

func (d *Driver) Info() (map[string]string, error) {
	infos := make(map[string]string)
	infos["DefaultVolumeSize"] = strconv.FormatInt(int64(d.DefaultVolumeSize), 10)
	infos["DefaultVolumeType"] = d.DefaultVolumeType
	return infos, nil
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

	exists, err := util.ObjectExists(vol)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("volume %s already exists", id)
	}

	var (
		snapshotId   string
		volumeId     string
		size         int
		deviceSuffix string
		format       bool
	)
	snapshotId = opt[OPT_BACKUP_URL]
	volumeId = opt[OPT_VOLUME_DRIVER_ID]
	size, err = d.getSize(opt, d.DefaultVolumeSize)
	if err != nil {
		return err
	}

	if volumeId != "" {
		_, err := d.client.CreateVolume(CreateVolumeParams{
			Name:       id,
			Size:       (size / GB),
			Id:         volumeId,
			SnapshotId: snapshotId,
		})
		if err != nil {
			return err
		}

		volume, err := d.client.AttachVolume(volumeId)
		if err != nil {
			return err
		}
		size = (volume.Size * GB)
		deviceSuffix = d.client.GetDeviceSuffix(volume.DeviceNumber)
	} else {
		volumeType := opt[OPT_VOLUME_TYPE]
		if volumeType != "" {
			err = checkVolumeType(volumeType)
			if err != nil {
				volumeType = d.DefaultVolumeType
			}
		} else {
			volumeType = d.DefaultVolumeType
		}

		volume, err := d.client.CreateVolume(CreateVolumeParams{
			Name:       id,
			Size:       (size / GB),
			Type:       volumeType,
			SnapshotId: snapshotId,
		})
		if err != nil {
			return err
		}

		if volume, err = d.client.AttachVolume(volume.Id); err != nil {
			return err
		}
		volumeId = volume.Id
		size = (volume.Size * GB)
		deviceSuffix = d.client.GetDeviceSuffix(volume.DeviceNumber)
		format = true
	}

	vol.Name = id
	vol.Id = volumeId
	vol.Device = BASE_DEVICE_PATH + deviceSuffix
	vol.Size = size
	vol.Snapshots = make(map[string]Snapshot)

	if format {
		if _, err := util.Execute("mkfs", []string{"-t", PB_VOLUME_FS, vol.Device}); err != nil {
			return err
		}
	}
	return util.ObjectSave(vol)
}

func (d *Driver) getSize(opts map[string]string, defaultVolumeSize int) (int, error) {
	size := opts[OPT_SIZE]
	if size == "" || size == "0" {
		size = strconv.FormatInt(int64(defaultVolumeSize), 10)
	}
	parsedSize, err := util.ParseSize(size)
	if err != nil {
		return 0, err
	}
	return int(parsedSize), nil
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

	reference, _ := strconv.ParseBool(opts[OPT_REFERENCE_ONLY])
	if !reference {
		err := d.client.DeleteVolume(vol.Id)
		if err != nil {
			return err
		}
	}

	return util.ObjectDelete(vol)
}

func (d *Driver) GetVolumeInfo(name string) (map[string]string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	vol := d.blankVolume(name)
	if err := util.ObjectLoad(vol); err != nil {
		return nil, err
	}

	volume, err := d.client.GetVolume(vol.Id)
	if err != nil {
		return nil, err
	}
	size := strconv.FormatInt(int64((volume.Size * GB)), 10)

	info := map[string]string{
		"Device":                vol.Device,
		"MountPoint":            vol.MountPoint,
		"Id":                    vol.Id,
		OPT_VOLUME_NAME:         name,
		"Size":                  size,
		"Type":                  volume.Type,
		"State":                 volume.State,
		"AvailabilityZone":      volume.AvailabilityZone,
		OPT_VOLUME_CREATED_TIME: volume.CreationTime,
	}
	return info, nil
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

func (d *Driver) SnapshotOps() (SnapshotOperations, error) {
	return d, nil
}

func (d *Driver) CreateSnapshot(req Request) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	volumeName, err := util.GetFieldFromOpts(OPT_VOLUME_NAME, req.Options)
	if err != nil {
		return err
	}

	vol := d.blankVolume(volumeName)
	if err := util.ObjectLoad(vol); err != nil {
		return err
	}

	_, exists := vol.Snapshots[id]
	if exists {
		return fmt.Errorf("This volume already has a snapshot with UUID %s", id)
	}

	snapshot, err := d.client.CreateSnapshot(vol.Id, id)
	if err != nil {
		return err
	}

	snapshot = Snapshot{
		Name:        id,
		Id:          snapshot.Id,
		Description: snapshot.Description,
		Size:        snapshot.Size,
		State:       snapshot.State,
		Location:    snapshot.Location,
	}

	vol.Snapshots[id] = snapshot
	return util.ObjectSave(vol)
}

func (d *Driver) DeleteSnapshot(req Request) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	volumeName, err := util.GetFieldFromOpts(OPT_VOLUME_NAME, req.Options)
	if err != nil {
		return err
	}

	vol := d.blankVolume(volumeName)
	if err := util.ObjectLoad(vol); err != nil {
		return err
	}

	_, exists := vol.Snapshots[id]
	if !exists {
		return fmt.Errorf("Snapshot %s does not exist for volume %s.", id, volumeName)
	}

	delete(vol.Snapshots, id)
	return util.ObjectSave(vol)
}

func (d *Driver) GetSnapshotInfo(req Request) (map[string]string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	volumeName, err := util.GetFieldFromOpts(OPT_VOLUME_NAME, req.Options)
	if err != nil {
		return nil, err
	}

	vol := d.blankVolume(volumeName)
	if err := util.ObjectLoad(vol); err != nil {
		return nil, err
	}

	snapshot, exists := vol.Snapshots[id]
	if exists {
		snapshot, err = d.client.GetSnapshot(snapshot.Id)
		if err != nil {
			return nil, err
		}

		info := map[string]string{
			OPT_SNAPSHOT_NAME:         id,
			"Id":                      snapshot.Id,
			"Description":             snapshot.Description,
			OPT_SIZE:                  strconv.FormatInt(int64((snapshot.Size * GB)), 10),
			"State":                   snapshot.State,
			"Location":                snapshot.Location,
			OPT_SNAPSHOT_CREATED_TIME: snapshot.CreationTime,
		}
		return info, nil
	} else {
		return nil, fmt.Errorf("Snapshot %s does not exist for volume %s.", id, volumeName)
	}
}

func (d *Driver) ListSnapshot(opts map[string]string) (map[string]map[string]string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	var (
		volumeIDs []string
		err       error
	)
	snapshots := make(map[string]map[string]string)
	specifiedVolumeID, _ := util.GetFieldFromOpts(OPT_VOLUME_NAME, opts)
	if specifiedVolumeID != "" {
		volumeIDs = []string{
			specifiedVolumeID,
		}
	} else {
		volumeIDs, err = util.ListConfigIDs(d.Root, CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_SUFFIX)
		if err != nil {
			return nil, err
		}
	}
	for _, volumeID := range volumeIDs {
		volume := d.blankVolume(volumeID)
		if err := util.ObjectLoad(volume); err != nil {
			return nil, err
		}
		for snapshotID := range volume.Snapshots {
			snapshot, err := d.client.GetSnapshot(volume.Snapshots[snapshotID].Id)
			if err != nil {
				return nil, err
			}

			info := map[string]string{
				OPT_SNAPSHOT_NAME:         snapshot.Name,
				"VolumeName":              volumeID,
				"Id":                      snapshot.Id,
				"Description":             snapshot.Description,
				OPT_SIZE:                  strconv.FormatInt(int64((snapshot.Size * GB)), 10),
				"State":                   snapshot.State,
				"Location":                snapshot.Location,
				OPT_SNAPSHOT_CREATED_TIME: snapshot.CreationTime,
			}

			snapshots[snapshotID] = info
		}
	}
	return snapshots, nil
}

// There is no concept of "Backups" at ProfitBricks.
// Snapshots are available across all data centers in the same location.
// Any new volume can be restored from an existing snapshot.
func (d *Driver) BackupOps() (BackupOperations, error) {
	return nil, errors.New("Not implemented")
}
