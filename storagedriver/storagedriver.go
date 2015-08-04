package storagedriver

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/metadata"
	"path/filepath"
)

type InitFunc func(root string, config map[string]string) (StorageDriver, error)

type StorageDriver interface {
	Name() string
	Info() (map[string]string, error)

	VolumeOps() (VolumeOperations, error)
	SnapshotOps() (SnapshotOperations, error)
}

type VolumeOperations interface {
	Name() string
	CreateVolume(id string, opts map[string]string) error
	DeleteVolume(id string) error
	MountVolume(id string, opts map[string]string) (string, error)
	UmountVolume(id string) error
	MountPoint(id string) (string, error)
	GetVolumeInfo(id string) (map[string]string, error)
	ListVolume(opts map[string]string) (map[string]map[string]string, error)
}

type SnapshotOperations interface {
	Name() string
	CreateSnapshot(id, volumeID string) error
	DeleteSnapshot(id, volumeID string) error
	GetSnapshotInfo(id, volumeID string) (map[string]string, error)
	ListSnapshot(opts map[string]string) (map[string]map[string]string, error)

	GetVolumeDevice(id string) (string, error)
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
}

const (
	OPT_MOUNT_POINT = "MountPoint"
	OPT_SIZE        = "Size"
)

var (
	initializers map[string]InitFunc
	log          = logrus.WithFields(logrus.Fields{"pkg": "storagedriver"})
)

func init() {
	initializers = make(map[string]InitFunc)
}

func Register(name string, initFunc InitFunc) error {
	if _, exists := initializers[name]; exists {
		return fmt.Errorf("Driver %s has already been registered", name)
	}
	initializers[name] = initFunc
	return nil
}

func GetDriver(name, root string, config map[string]string) (StorageDriver, error) {
	if _, exists := initializers[name]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", name)
	}
	drvRoot := filepath.Join(root, name)
	return initializers[name](drvRoot, config)
}
