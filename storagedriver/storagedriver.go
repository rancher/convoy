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
	Info() ([]byte, error)

	VolumeOps() (VolumeOperations, error)
	SnapshotOps() (SnapshotOperations, error)
}

type VolumeOperations interface {
	CreateVolume(id string, size int64) error
	DeleteVolume(id string) error
	MountVolume(id, mountPoint string) error
	UmountVolume(id, mountPoint string) error
	ListVolume(id string) ([]byte, error)
}

type SnapshotOperations interface {
	GetVolumeDevice(id string) (string, error)
	CreateSnapshot(id, volumeID string) error
	DeleteSnapshot(id, volumeID string) error
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
}

var (
	initializers map[string]InitFunc
	log          = logrus.WithFields(logrus.Fields{"pkg": "driver"})
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
