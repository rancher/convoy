package glusterfs

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/util"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	DRIVER_NAME        = "glusterfs"
	DRIVER_CONFIG_FILE = "glusterfs.cfg"

	VOLUME_CFG_PREFIX = "volume_"
	DRIVER_CFG_PREFIX = DRIVER_NAME + "_"
	CFG_POSTFIX       = ".json"

	SNAPSHOT_PATH = "snapshots"

	MOUNTS_DIR = "mounts"

	GLUSTERFS_SERVERS             = "glusterfs.servers"
	GLUSTERFS_DEFAULT_VOLUME_POOL = "glusterfs.defaultvolumepool"
	GLUSTERFS_DEFAULT_VOLUME_SIZE = "glusterfs.defaultvolumesize"
	DEFAULT_VOLUME_SIZE           = "100G"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "glusterfs"})
)

type Driver struct {
	mutex    *sync.RWMutex
	gVolumes map[string]*GlusterFSVolume
	Device
}

func init() {
	convoydriver.Register(DRIVER_NAME, Init)
}

func (d *Driver) Name() string {
	return DRIVER_NAME
}

type Device struct {
	Root              string
	Servers           []string
	DefaultVolumePool string
	DefaultVolumeSize int64
}

func (dev *Device) ConfigFile() (string, error) {
	if dev.Root == "" {
		return "", fmt.Errorf("BUG: Invalid empty device config path")
	}
	return filepath.Join(dev.Root, DRIVER_CONFIG_FILE), nil
}

type Snapshot struct {
	UUID       string
	VolumeUUID string
}

type Volume struct {
	UUID         string
	Name         string
	Path         string
	MountPoint   string
	VolumePool   string
	Size         int64
	PrepareForVM bool

	configPath string
}

type GlusterFSVolume struct {
	UUID       string // volume name in fact
	MountPoint string
	Servers    []string

	configPath string
}

func (gv *GlusterFSVolume) GetDevice() (string, error) {
	l := len(gv.Servers)
	if gv.Servers == nil || len(gv.Servers) == 0 {
		return "", fmt.Errorf("No server IP provided for glusterfs")
	}
	ip := gv.Servers[rand.Intn(l)]
	return ip + ":/" + gv.UUID, nil
}

func (gv *GlusterFSVolume) GetMountOpts() []string {
	return []string{"-t", "glusterfs"}
}

func (gv *GlusterFSVolume) GenerateDefaultMountPoint() string {
	return filepath.Join(gv.configPath, MOUNTS_DIR, gv.UUID)
}

func (v *Volume) ConfigFile() (string, error) {
	if v.UUID == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume UUID")
	}
	if v.configPath == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, DRIVER_CFG_PREFIX+VOLUME_CFG_PREFIX+v.UUID+CFG_POSTFIX), nil
}

func (device *Device) listVolumeIDs() ([]string, error) {
	return util.ListConfigIDs(device.Root, DRIVER_CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_POSTFIX)
}

func Init(root string, config map[string]string) (convoydriver.ConvoyDriver, error) {
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

		serverList := config[GLUSTERFS_SERVERS]
		if serverList == "" {
			return nil, fmt.Errorf("Missing required parameter: %v", GLUSTERFS_SERVERS)
		}

		servers := strings.Split(serverList, ",")
		for _, server := range servers {
			if !util.ValidNetworkAddr(server) {
				return nil, fmt.Errorf("Invalid or unsolvable address: %v", server)
			}
		}

		defaultVolumePool := config[GLUSTERFS_DEFAULT_VOLUME_POOL]
		if defaultVolumePool == "" {
			return nil, fmt.Errorf("Missing required parameter: %v", GLUSTERFS_DEFAULT_VOLUME_POOL)
		}

		if _, exists := config[GLUSTERFS_DEFAULT_VOLUME_SIZE]; !exists {
			config[GLUSTERFS_DEFAULT_VOLUME_SIZE] = DEFAULT_VOLUME_SIZE
		}
		volumeSize, err := util.ParseSize(config[GLUSTERFS_DEFAULT_VOLUME_SIZE])
		if err != nil || volumeSize == 0 {
			return nil, fmt.Errorf("Illegal default volume size specified")
		}
		dev.DefaultVolumeSize = volumeSize

		dev = &Device{
			Root:              root,
			Servers:           servers,
			DefaultVolumePool: defaultVolumePool,
		}
	}

	// For upgrade case
	if dev.DefaultVolumeSize == 0 {
		dev.DefaultVolumeSize, err = util.ParseSize(DEFAULT_VOLUME_SIZE)
		if err != nil || dev.DefaultVolumeSize == 0 {
			return nil, fmt.Errorf("Illegal default volume size specified")
		}
	}

	d := &Driver{
		mutex:    &sync.RWMutex{},
		gVolumes: map[string]*GlusterFSVolume{},
		Device:   *dev,
	}
	gVolume := &GlusterFSVolume{
		UUID:       dev.DefaultVolumePool,
		Servers:    dev.Servers,
		configPath: d.Root,
	}
	// We would always mount the default volume pool
	// TODO: Also need to mount any existing volume's pool
	if _, err := util.VolumeMount(gVolume, "", true); err != nil {
		return nil, err
	}
	d.gVolumes[d.DefaultVolumePool] = gVolume

	if err := util.ObjectSave(dev); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Driver) Info() (map[string]string, error) {
	return map[string]string{
		"Root":              d.Root,
		"GlusterFSServers":  fmt.Sprintf("%v", d.Servers),
		"DefaultVolumePool": d.DefaultVolumePool,
		"DefaultVolumeSize": strconv.FormatInt(d.DefaultVolumeSize, 10),
	}, nil
}

func (d *Driver) VolumeOps() (convoydriver.VolumeOperations, error) {
	return d, nil
}

func (d *Driver) blankVolume(id string) *Volume {
	return &Volume{
		configPath: d.Root,
		UUID:       id,
	}
}

func (d *Driver) getSize(opts map[string]string, defaultVolumeSize int64) (int64, error) {
	size := opts[convoydriver.OPT_SIZE]
	if size == "" || size == "0" {
		size = strconv.FormatInt(defaultVolumeSize, 10)
	}
	return util.ParseSize(size)
}

func (d *Driver) CreateVolume(id string, opts map[string]string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volumeName := opts[convoydriver.OPT_VOLUME_NAME]
	if volumeName == "" {
		volumeName = "volume-" + id[:8]
	}

	volume := d.blankVolume(id)
	exists, err := util.ObjectExists(volume)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("volume %v already exists", id)
	}

	volume.PrepareForVM, err = strconv.ParseBool(opts[convoydriver.OPT_PREPARE_FOR_VM])
	if err != nil {
		return err
	}
	if volume.PrepareForVM {
		volume.Size, err = d.getSize(opts, d.DefaultVolumeSize)
		if err != nil {
			return err
		}
	}

	gVolume := d.gVolumes[d.DefaultVolumePool]
	volumePath := filepath.Join(gVolume.MountPoint, volumeName)
	if util.VolumeMountPointFileExists(gVolume, volumeName, util.FILE_TYPE_DIRECTORY) {
		log.Debugf("Found existing volume named %v, reuse it", volumeName)
	} else if err := util.VolumeMountPointDirectoryCreate(gVolume, volumeName); err != nil {
		return err
	}
	volume.Name = volumeName
	volume.Path = volumePath
	volume.VolumePool = gVolume.UUID

	return util.ObjectSave(volume)
}

func (d *Driver) DeleteVolume(id string, opts map[string]string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	if volume.MountPoint != "" {
		return fmt.Errorf("Cannot delete volume %v. It is still mounted", id)
	}
	referenceOnly, _ := strconv.ParseBool(opts[convoydriver.OPT_REFERENCE_ONLY])
	if !referenceOnly {
		log.Debugf("Cleaning up volume %v", id)
		gVolume := d.gVolumes[d.DefaultVolumePool]
		if err := util.VolumeMountPointDirectoryRemove(gVolume, volume.Name); err != nil {
			return err
		}
	}
	return util.ObjectDelete(volume)
}

func (d *Driver) MountVolume(id string, opts map[string]string) (string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return "", err
	}

	specifiedPoint := opts[convoydriver.OPT_MOUNT_POINT]
	if specifiedPoint != "" {
		return "", fmt.Errorf("GlusterFS doesn't support specified mount point")
	}
	if volume.MountPoint == "" {
		volume.MountPoint = volume.Path
	}
	if volume.PrepareForVM {
		if err := util.MountPointPrepareImageFile(volume.MountPoint, volume.Size); err != nil {
			return "", err
		}
	}
	if err := util.ObjectSave(volume); err != nil {
		return "", err
	}
	return volume.MountPoint, nil
}

func (d *Driver) UmountVolume(id string, opts map[string]string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	if volume.MountPoint != "" {
		volume.MountPoint = ""
	}
	return util.ObjectSave(volume)
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

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return nil, err
	}

	gVolume := d.gVolumes[volume.VolumePool]
	if gVolume == nil {
		return nil, fmt.Errorf("Cannot find volume pool %v", volume.VolumePool)
	}

	size := "-1"
	prepareForVM := strconv.FormatBool(volume.PrepareForVM)
	if volume.PrepareForVM {
		size = strconv.FormatInt(volume.Size, 10)
	}

	return map[string]string{
		"Name": volume.Name,
		"Path": volume.Path,
		convoydriver.OPT_MOUNT_POINT:    volume.MountPoint,
		convoydriver.OPT_SIZE:           size,
		convoydriver.OPT_PREPARE_FOR_VM: prepareForVM,
		"GlusterFSVolume":               volume.VolumePool,
		"GlusterFSServers":              fmt.Sprintf("%v", gVolume.Servers),
	}, nil
}

func (d *Driver) MountPoint(id string, opts map[string]string) (string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return "", err
	}
	return volume.MountPoint, nil
}

func (d *Driver) SnapshotOps() (convoydriver.SnapshotOperations, error) {
	return nil, fmt.Errorf("Doesn't support snapshot operations")
}

func (d *Driver) BackupOps() (convoydriver.BackupOperations, error) {
	return nil, fmt.Errorf("Doesn't support backup operations")
}
