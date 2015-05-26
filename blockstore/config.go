package blockstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/rancherio/volmgr/util"
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
	IMAGES_DIRECTORY       = "images"
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
	rc, err := driver.Read(filePath)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := json.NewDecoder(rc).Decode(v); err != nil {
		return err
	}
	return nil
}

func saveConfigInBlockStore(filePath string, driver BlockStoreDriver, v interface{}) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := driver.Write(filePath, bytes.NewReader(j)); err != nil {
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
	if err := util.RemoveConfig(root, cfgName); err != nil {
		return err
	}
	log.Debug("Removed ", cfgName)
	return nil
}

func removeConfigFile(root, id string) error {
	cfgName := getCfgName(id)
	if err := util.RemoveConfig(root, cfgName); err != nil {
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
		if err := bsDriver.Remove(filePath); err != nil {
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

func getSnapshots(volumeID string, driver BlockStoreDriver) (map[string]bool, error) {
	result := make(map[string]bool)
	fileList, err := driver.List(getSnapshotsPath(volumeID))
	if err != nil {
		// path doesn't exist
		return result, nil
	}

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

func GetImageLocalStorePath(imageDir, imageUUID string) string {
	return filepath.Join(imageDir, imageUUID+".img")
}

func getImageBlockStorePath(imageUUID string) string {
	return filepath.Join(BLOCKSTORE_BASE, IMAGES_DIRECTORY, imageUUID+".img.gz")
}

func getImageCfgBlockStorePath(imageUUID string) string {
	return filepath.Join(BLOCKSTORE_BASE, IMAGES_DIRECTORY, imageUUID+".json")
}

func saveImageConfig(imageUUID string, driver BlockStoreDriver, img *Image) error {
	file := getImageCfgBlockStorePath(imageUUID)
	if err := saveConfigInBlockStore(file, driver, img); err != nil {
		return err
	}
	return nil
}

func loadImageConfig(imageUUID string, driver BlockStoreDriver) (*Image, error) {
	img := &Image{}
	file := getImageCfgBlockStorePath(imageUUID)
	if err := loadConfigInBlockStore(file, driver, img); err != nil {
		return nil, err
	}
	return img, nil
}

func removeImageConfig(image *Image, driver BlockStoreDriver) error {
	file := getImageCfgBlockStorePath(image.UUID)
	if err := driver.Remove(file); err != nil {
		return err
	}
	return nil
}
