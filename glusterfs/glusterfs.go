package glusterfs

import (
	"fmt"
	"github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/rancher"
	"github.com/rancher/convoy/util"
	"math/rand"
	"path/filepath"
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

	GLUSTERFS_RANCHER_STACK           = "glusterfs.rancherstack"
	GLUSTERFS_RANCHER_GLUSTER_SERVICE = "glusterfs.rancherservice"
	GLUSTERFS_DEFAULT_VOLUME_POOL     = "glusterfs.defaultvolumepool"
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
	RancherURL        string
	RancherAccessKey  string
	RancherSecretKey  string
	RancherStack      string
	RancherService    string
	DefaultVolumePool string
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
	UUID       string
	MountPoint string
	Snapshots  map[string]Snapshot

	configPath string
}

type GlusterFSVolume struct {
	UUID       string // volume name in fact
	MountPoint string
	ServerIPs  []string

	configPath string
}

func (gv *GlusterFSVolume) GetDevice() (string, error) {
	l := len(gv.ServerIPs)
	if gv.ServerIPs == nil || len(gv.ServerIPs) == 0 {
		return "", fmt.Errorf("No server IP provided for glusterfs")
	}
	ip := gv.ServerIPs[rand.Intn(l)]
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

		stack := config[GLUSTERFS_RANCHER_STACK]
		if stack == "" {
			return nil, fmt.Errorf("Missing required parameter: %v", GLUSTERFS_RANCHER_STACK)
		}
		service := config[GLUSTERFS_RANCHER_GLUSTER_SERVICE]
		if service == "" {
			return nil, fmt.Errorf("Missing required parameter: %v", GLUSTERFS_RANCHER_GLUSTER_SERVICE)
		}
		defaultVolumePool := config[GLUSTERFS_DEFAULT_VOLUME_POOL]
		if defaultVolumePool == "" {
			return nil, fmt.Errorf("Missing required parameter: %v", GLUSTERFS_DEFAULT_VOLUME_POOL)
		}

		dev = &Device{
			Root:              root,
			RancherStack:      stack,
			RancherService:    service,
			DefaultVolumePool: defaultVolumePool,
		}
		if err := util.ObjectSave(dev); err != nil {
			return nil, err
		}
	}

	serverIPs, err := rancher.GetIPsForServiceInStack(dev.RancherService, dev.RancherStack)
	if err != nil {
		return nil, err
	}

	d := &Driver{
		mutex:    &sync.RWMutex{},
		gVolumes: map[string]*GlusterFSVolume{},
		Device:   *dev,
	}
	// We would always mount the default volume pool
	if err := d.mountVolumePool(serverIPs, dev.DefaultVolumePool); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Driver) mountVolumePool(serverIPs []string, volumePool string) error {
	gVolume := &GlusterFSVolume{
		UUID:       volumePool,
		ServerIPs:  serverIPs,
		configPath: d.Root,
	}
	if _, err := util.VolumeMount(gVolume, ""); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Info() (map[string]string, error) {
	return map[string]string{
		"Root":              d.Root,
		"RancherStack":      d.RancherStack,
		"RancherService":    d.RancherService,
		"DefaultVolumePool": d.DefaultVolumePool,
	}, nil
}

func (d *Driver) VolumeOps() (convoydriver.VolumeOperations, error) {
	return nil, fmt.Errorf("Hasn't implemented yet")
}

func (d *Driver) SnapshotOps() (convoydriver.SnapshotOperations, error) {
	return nil, fmt.Errorf("Doesn't support snapshot operations")
}

func (d *Driver) BackupOps() (convoydriver.BackupOperations, error) {
	return nil, fmt.Errorf("Doesn't support backup operations")
}

func (d *Driver) blankVolume(id string) *Volume {
	return &Volume{
		configPath: d.Root,
		UUID:       id,
	}
}
