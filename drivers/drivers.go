package drivers

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/metadata"
)

type InitFunc func(root, cfgName string, config map[string]string) (Driver, error)

type Driver interface {
	Name() string
	CreateVolume(id string, size int64) error
	DeleteVolume(id string) error
	Mount(id, mountPoint string) error
	Umount(id, mountPoint string) error
	GetVolumeDevice(id string) (string, error)
	ListVolume(id string) ([]byte, error)
	CreateSnapshot(id, volumeID string) error
	DeleteSnapshot(id, volumeID string) error
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
	Info() ([]byte, error)
	Shutdown() error
	CheckEnvironment() error
}

var (
	initializers map[string]InitFunc
	log          = logrus.WithFields(logrus.Fields{"pkg": "drivers"})
)

const (
	MOUNT_BINARY  = "mount"
	UMOUNT_BINARY = "umount"
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

func getCfgName(name string) string {
	return "driver_" + name + ".cfg"
}

func GetDriver(name, root string, config map[string]string) (Driver, error) {
	if _, exists := initializers[name]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", name)
	}
	return initializers[name](root, getCfgName(name), config)
}

func CheckEnvironment(driver Driver) error {
	if err := driver.CheckEnvironment(); err != nil {
		return err
	}
	return nil
}
