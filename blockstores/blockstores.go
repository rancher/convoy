package blockstores

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/yasker/volmgr/utils"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	BLOCKSTORE_BASE       = "rancher-blockstore"
	VOLUME_DIRECTORY      = "volume"
	SNAPSHOTS_DIRECTORY   = "snapshots"
	BLOCKS_DIRECTORY      = "blocks"
	BLOCK_SEPARATE_LAYER1 = 2
	BLOCK_SEPARATE_LAYER2 = 4
)

type InitFunc func(configFile, id string, config map[string]string) (BlockStoreDriver, error)

type BlockStoreDriver interface {
	Kind() string
	FileExists(fileName string) bool
	MkDirAll(dirName string) error
	RemoveAll(dirName string) error
	Read(srcPath, srcFileName string, data []byte) error
	Write(data []byte, dstPath, dstFileName string) error
	CopyToPath(srcFileName string, path string) error
}

type Volume struct {
	Blocks map[string]bool
}

type BlockStore struct {
	Kind    string
	Volumes map[string]Volume
}

var (
	initializers map[string]InitFunc
)

func init() {
	initializers = make(map[string]InitFunc)
}

func RegisterDriver(kind string, initFunc InitFunc) error {
	if _, exists := initializers[kind]; exists {
		return fmt.Errorf("%s has already been registered", kind)
	}
	initializers[kind] = initFunc
	return nil
}

func GetBlockStoreDriver(kind, configFile, id string, config map[string]string) (BlockStoreDriver, error) {
	if _, exists := initializers[kind]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", kind)
	}
	return initializers[kind](configFile, id, config)
}

func getDriverConfigFilename(root, kind, id string) string {
	return filepath.Join(root, id+"-"+kind+".cfg")
}

func getConfigFilename(root, id string) string {
	return filepath.Join(root, id+".cfg")
}

func Register(root, kind, id string, config map[string]string) error {
	configFile := getDriverConfigFilename(root, kind, id)
	if _, err := os.Stat(configFile); err == nil {
		return fmt.Errorf("BlockStore %v is already registered", id)
	}
	driver, err := GetBlockStoreDriver(kind, configFile, id, config)
	if err != nil {
		return err
	}
	log.Debug("Created ", configFile)

	basePath := filepath.Join(BLOCKSTORE_BASE, VOLUME_DIRECTORY)
	err = driver.MkDirAll(basePath)
	if err != nil {
		removeDriverConfigFile(root, kind, id)
		return err
	}
	log.Debug("Created base directory of blockstore at ", basePath)

	bs := &BlockStore{
		Kind:    kind,
		Volumes: make(map[string]Volume),
	}
	configFile = getConfigFilename(root, id)
	if err := utils.SaveConfig(configFile, bs); err != nil {
		return err
	}
	log.Debug("Created ", configFile)
	return nil
}

func removeDriverConfigFile(root, kind, id string) error {
	configFile := getDriverConfigFilename(root, kind, id)
	if err := exec.Command("rm", "-f", configFile).Run(); err != nil {
		return err
	}
	log.Debug("Removed ", configFile)
	return nil
}

func removeConfigFile(root, id string) error {
	configFile := getConfigFilename(root, id)
	if err := exec.Command("rm", "-f", configFile).Run(); err != nil {
		return err
	}
	log.Debug("Removed ", configFile)
	return nil
}

func Deregister(root, kind, id string) error {
	err := removeDriverConfigFile(root, kind, id)
	if err != nil {
		return err
	}
	err = removeConfigFile(root, id)
	if err != nil {
		return err
	}
	return nil
}

func AddVolume(root, id, volumeId string) error {
	configFile := getConfigFilename(root, id)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}
	driverConfigFile := getDriverConfigFilename(root, b.Kind, id)
	driver, err := GetBlockStoreDriver(b.Kind, driverConfigFile, id, nil)
	if err != nil {
		return err
	}

	volumeBase := filepath.Join(BLOCKSTORE_BASE, VOLUME_DIRECTORY, volumeId)
	err = driver.MkDirAll(volumeBase)
	if err != nil {
		return err
	}
	log.Debug("Created volume base: ", volumeBase)
	volume := b.Volumes[id]
	volume.Blocks = make(map[string]bool)
	if err = utils.SaveConfig(configFile, b); err != nil {
		return err
	}
	return nil
}

func RemoveVolume(root, id, volumeId string) error {
	configFile := getConfigFilename(root, id)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}
	driverConfigFile := getDriverConfigFilename(root, b.Kind, id)
	driver, err := GetBlockStoreDriver(b.Kind, driverConfigFile, id, nil)
	if err != nil {
		return err
	}

	volumeBase := filepath.Join(BLOCKSTORE_BASE, VOLUME_DIRECTORY, volumeId)
	err = driver.RemoveAll(volumeBase)
	if err != nil {
		return err
	}
	log.Debug("Removed volume base: ", volumeBase)
	delete(b.Volumes, id)

	if err = utils.SaveConfig(configFile, b); err != nil {
		return err
	}
	return nil
}
