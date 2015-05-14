package blockstore

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/rancherio/volmgr/utils"
	"path/filepath"
	"strings"
)

const (
	BLOCKSTORE_BASE        = "rancher-blockstore"
	BLOCKSTORE_CONFIG_FILE = "blockstore.cfg"
	VOLUME_DIRECTORY       = "volumes"
	VOLUME_CONFIG_FILE     = "volume.cfg"
	VOLUME_SEPARATE_LAYER1 = 2
	VOLUME_SEPARATE_LAYER2 = 4
	SNAPSHOTS_DIRECTORY    = "snapshots"
	SNAPSHOT_CONFIG_PREFIX = "snapshot_"
	BLOCKS_DIRECTORY       = "blocks"
	BLOCK_SEPARATE_LAYER1  = 2
	BLOCK_SEPARATE_LAYER2  = 4
	HASH_LEVEL             = 2
)

func getSnapshotConfigName(id string) string {
	return SNAPSHOT_CONFIG_PREFIX + id + ".cfg"
}

func getDriverCfgName(kind, id string) string {
	return "blockstore_" + id + "_" + kind + ".cfg"
}

func getCfgName(id string) string {
	return "blockstore_" + id + ".cfg"
}

func loadConfigInBlockStore(filePath string, driver BlockStoreDriver, v interface{}) error {
	size := driver.FileSize(filePath)
	if size < 0 {
		return fmt.Errorf("cannot find %v in blockstore", filePath)
	}
	data := make([]byte, size)
	if err := driver.Read(filePath, data); err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}

func saveConfigInBlockStore(filePath string, driver BlockStoreDriver, v interface{}) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := driver.Write(j, filePath); err != nil {
		return err
	}
	return nil
}

func loadVolumeConfig(volumeID string, driver BlockStoreDriver) (*Volume, error) {
	v := &Volume{}
	path := getVolumePath(volumeID)
	file := VOLUME_CONFIG_FILE
	if err := loadConfigInBlockStore(filepath.Join(path, file), driver, v); err != nil {
		return nil, err
	}
	return v, nil
}

func saveVolumeConfig(volumeID string, driver BlockStoreDriver, v *Volume) error {
	path := getVolumePath(volumeID)
	file := VOLUME_CONFIG_FILE
	if err := saveConfigInBlockStore(filepath.Join(path, file), driver, v); err != nil {
		return err
	}
	return nil
}

func loadRemoteBlockStoreConfig(driver BlockStoreDriver) (*BlockStore, error) {
	b := &BlockStore{}
	path := BLOCKSTORE_BASE
	file := BLOCKSTORE_CONFIG_FILE
	if err := loadConfigInBlockStore(filepath.Join(path, file), driver, b); err != nil {
		return nil, err
	}
	return b, nil
}

func saveRemoteBlockStoreConfig(driver BlockStoreDriver, b *BlockStore) error {
	path := BLOCKSTORE_BASE
	file := BLOCKSTORE_CONFIG_FILE
	if err := saveConfigInBlockStore(filepath.Join(path, file), driver, b); err != nil {
		return err
	}
	return nil
}

func removeDriverConfigFile(root, kind, id string) error {
	cfgName := getDriverCfgName(kind, id)
	if err := utils.RemoveConfig(root, cfgName); err != nil {
		return err
	}
	log.Debug("Removed ", cfgName)
	return nil
}

func removeConfigFile(root, id string) error {
	cfgName := getCfgName(id)
	if err := utils.RemoveConfig(root, cfgName); err != nil {
		return err
	}
	log.Debug("Removed ", cfgName)
	return nil
}

func snapshotExists(snapshotID, volumeID string, bsDriver BlockStoreDriver) bool {
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)
	return bsDriver.FileExists(filepath.Join(path, fileName))
}

func loadSnapshotMap(snapshotID, volumeID string, bsDriver BlockStoreDriver) (*SnapshotMap, error) {
	snapshotMap := SnapshotMap{}
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)

	if err := loadConfigInBlockStore(filepath.Join(path, fileName), bsDriver, &snapshotMap); err != nil {
		return nil, err
	}
	return &snapshotMap, nil
}

func saveSnapshotMap(snapshotID, volumeID string, bsDriver BlockStoreDriver, snapshotMap *SnapshotMap) error {
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)
	filePath := filepath.Join(path, fileName)
	if bsDriver.FileExists(filePath) {
		log.Warnf("Snapshot configuration file %v already exists, would remove it\n", filePath)
		if err := bsDriver.RemoveAll(filePath); err != nil {
			return err
		}
	}
	if err := saveConfigInBlockStore(filePath, bsDriver, snapshotMap); err != nil {
		return err
	}
	return nil
}

func getVolumePath(volumeID string) string {
	volumeLayer1 := volumeID[0:VOLUME_SEPARATE_LAYER1]
	volumeLayer2 := volumeID[VOLUME_SEPARATE_LAYER1:VOLUME_SEPARATE_LAYER2]
	return filepath.Join(BLOCKSTORE_BASE, VOLUME_DIRECTORY, volumeLayer1, volumeLayer2, volumeID)
}

func getSnapshotsPath(volumeID string) string {
	return filepath.Join(getVolumePath(volumeID), SNAPSHOTS_DIRECTORY)
}

func getBlocksPath(volumeID string) string {
	return filepath.Join(getVolumePath(volumeID), BLOCKS_DIRECTORY)
}

func getBlockFilePath(volumeID, checksum string) string {
	blockSubDirLayer1 := checksum[0:BLOCK_SEPARATE_LAYER1]
	blockSubDirLayer2 := checksum[BLOCK_SEPARATE_LAYER1:BLOCK_SEPARATE_LAYER2]
	path := filepath.Join(getBlocksPath(volumeID), blockSubDirLayer1, blockSubDirLayer2)
	fileName := checksum + ".blk"

	return filepath.Join(path, fileName)
}

// Used for cleanup remaining hashed directories
func removeAndCleanup(path string, driver BlockStoreDriver) error {
	if err := driver.RemoveAll(path); err != nil {
		return err
	}
	dir := path
	for i := 0; i < HASH_LEVEL; i++ {
		dir = filepath.Dir(dir)
		// If directory is not empty, then we don't need to continue
		if err := driver.Remove(dir); err != nil {
			break
		}
	}
	return nil
}

func getSnapshots(volumeID string, driver BlockStoreDriver) (map[string]bool, error) {
	fileList, err := driver.List(getSnapshotsPath(volumeID))
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, f := range fileList {
		parts := strings.Split(f, "_")
		if len(parts) != 2 {
			return nil, fmt.Errorf("incorrect filename format:", f)
		}
		parts = strings.Split(parts[1], ".")
		if len(parts) != 2 {
			return nil, fmt.Errorf("incorrect filename format:", f)
		}
		result[parts[0]] = true
	}
	return result, nil
}
