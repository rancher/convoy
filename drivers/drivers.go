package drivers

import (
	"fmt"
)

type InitFunc func(root string, config map[string]string) (Driver, error)

type Driver interface {
	Name() string
	CreateVolume(id, baseId string) error
	DeleteVolume(id string) error
	CreateSnapshot(id, volumeId string) error
	DeleteSnapshot(id string) error
	ExportSnapshot(id, path string, blockSize uint32) error
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
