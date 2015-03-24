package drivers

import (
	"fmt"
	"github.com/yasker/volmgr/metadata"
)

type InitFunc func(root string, config map[string]string) (Driver, error)

type Driver interface {
	Name() string
	CreateVolume(id, baseId string, size uint64) error
	DeleteVolume(id string) error
	GetVolumeDevice(id string) (string, error)
	ListVolumes() error
	CreateSnapshot(id, volumeId string) error
	DeleteSnapshot(id, volumeId string) error
	ListSnapshot(volumeId string) error
	CompareSnapshot(id, compareId, volumeId string, mapping *metadata.Mappings) error
	OpenSnapshot(id, volumeId string) error
	ReadSnapshot(id, volumeId string, start int64, data []byte) error
	CloseSnapshot(id, volumeId string) error
	Info() error
}

var (
	initializers map[string]InitFunc
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

func GetDriver(name, root string, config map[string]string) (Driver, error) {
	if _, exists := initializers[name]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", name)
	}
	return initializers[name](root, config)
}
