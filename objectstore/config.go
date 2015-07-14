package objectstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/util"
	"path/filepath"
	"strings"

	. "github.com/rancher/rancher-volume/logging"
)

const (
	OBJECTSTORE_BASE        = "rancher-objectstore"
	OBJECTSTORE_CONFIG_FILE = "objectstore.cfg"
	VOLUME_DIRECTORY        = "volumes"
	VOLUME_CONFIG_FILE      = "volume.cfg"
	VOLUME_SEPARATE_LAYER1  = 2
	VOLUME_SEPARATE_LAYER2  = 4
	SNAPSHOTS_DIRECTORY     = "snapshots"
	SNAPSHOT_CONFIG_PREFIX  = "snapshot_"
	BLOCKS_DIRECTORY        = "blocks"
	BLOCK_SEPARATE_LAYER1   = 2
	BLOCK_SEPARATE_LAYER2   = 4

	OBJECTSTORE_CFG_PREFIX = "objectstore_"
	CFG_POSTFIX            = ".cfg"
)

func getSnapshotConfigName(id string) string {
	return SNAPSHOT_CONFIG_PREFIX + id + ".cfg"
}

func getDriverCfgName(kind, id string) string {
	return OBJECTSTORE_CFG_PREFIX + id + "_" + kind + CFG_POSTFIX
}

func getCfgName(id string) string {
	return OBJECTSTORE_CFG_PREFIX + id + CFG_POSTFIX
}

func loadConfigInObjectStore(filePath string, driver ObjectStoreDriver, v interface{}) error {
	size := driver.FileSize(filePath)
	if size < 0 {
		return fmt.Errorf("cannot find %v in objectstore", filePath)
	}
	rc, err := driver.Read(filePath)
	if err != nil {
		return err
	}
	defer rc.Close()

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_OBJECT:   LOG_OBJECT_CONFIG,
		LOG_FIELD_KIND:     driver.Kind(),
		LOG_FIELD_FILEPATH: filePath,
	}).Debug()
	if err := json.NewDecoder(rc).Decode(v); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_CONFIG,
		LOG_FIELD_KIND:     driver.Kind(),
		LOG_FIELD_FILEPATH: filePath,
	}).Debug()
	return nil
}

func saveConfigInObjectStore(filePath string, driver ObjectStoreDriver, v interface{}) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_OBJECT:   LOG_OBJECT_CONFIG,
		LOG_FIELD_KIND:     driver.Kind(),
		LOG_FIELD_FILEPATH: filePath,
	}).Debug()
	if err := driver.Write(filePath, bytes.NewReader(j)); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_CONFIG,
		LOG_FIELD_KIND:     driver.Kind(),
		LOG_FIELD_FILEPATH: filePath,
	}).Debug()
	return nil
}

func loadVolumeConfig(volumeID string, driver ObjectStoreDriver) (*Volume, error) {
	v := &Volume{}
	file := getVolumeFilePath(volumeID)
	if err := loadConfigInObjectStore(file, driver, v); err != nil {
		return nil, err
	}
	return v, nil
}

func saveVolumeConfig(v *Volume, driver ObjectStoreDriver) error {
	file := getVolumeFilePath(v.UUID)
	if err := saveConfigInObjectStore(file, driver, v); err != nil {
		return err
	}
	return nil
}

func volumeExists(volumeUUID string, driver ObjectStoreDriver) bool {
	volumeFile := getVolumeFilePath(volumeUUID)
	return driver.FileExists(volumeFile)
}

func loadRemoteObjectStoreConfig(driver ObjectStoreDriver) (*ObjectStore, error) {
	b := &ObjectStore{}
	path := OBJECTSTORE_BASE
	file := OBJECTSTORE_CONFIG_FILE
	if err := loadConfigInObjectStore(filepath.Join(path, file), driver, b); err != nil {
		return nil, err
	}
	return b, nil
}

func saveRemoteObjectStoreConfig(driver ObjectStoreDriver, b *ObjectStore) error {
	path := OBJECTSTORE_BASE
	file := OBJECTSTORE_CONFIG_FILE
	if err := saveConfigInObjectStore(filepath.Join(path, file), driver, b); err != nil {
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

func snapshotExists(snapshotID, volumeID string, bsDriver ObjectStoreDriver) bool {
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)
	return bsDriver.FileExists(filepath.Join(path, fileName))
}

func loadSnapshotMap(snapshotID, volumeID string, bsDriver ObjectStoreDriver) (*SnapshotMap, error) {
	snapshotMap := SnapshotMap{}
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)

	if err := loadConfigInObjectStore(filepath.Join(path, fileName), bsDriver, &snapshotMap); err != nil {
		return nil, err
	}
	return &snapshotMap, nil
}

func saveSnapshotMap(snapshotID, volumeID string, bsDriver ObjectStoreDriver, snapshotMap *SnapshotMap) error {
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)
	filePath := filepath.Join(path, fileName)
	if bsDriver.FileExists(filePath) {
		log.Warnf("Snapshot configuration file %v already exists, would remove it\n", filePath)
		if err := bsDriver.Remove(filePath); err != nil {
			return err
		}
	}
	if err := saveConfigInObjectStore(filePath, bsDriver, snapshotMap); err != nil {
		return err
	}
	return nil
}

func getVolumePath(volumeID string) string {
	volumeLayer1 := volumeID[0:VOLUME_SEPARATE_LAYER1]
	volumeLayer2 := volumeID[VOLUME_SEPARATE_LAYER1:VOLUME_SEPARATE_LAYER2]
	return filepath.Join(OBJECTSTORE_BASE, VOLUME_DIRECTORY, volumeLayer1, volumeLayer2, volumeID)
}

func getVolumeFilePath(volumeID string) string {
	volumePath := getVolumePath(volumeID)
	volumeCfg := VOLUME_CONFIG_FILE
	return filepath.Join(volumePath, volumeCfg)
}

func getSnapshotsPath(volumeID string) string {
	return filepath.Join(getVolumePath(volumeID), SNAPSHOTS_DIRECTORY) + "/"
}

func getBlocksPath(volumeID string) string {
	return filepath.Join(getVolumePath(volumeID), BLOCKS_DIRECTORY) + "/"
}

func getBlockFilePath(volumeID, checksum string) string {
	blockSubDirLayer1 := checksum[0:BLOCK_SEPARATE_LAYER1]
	blockSubDirLayer2 := checksum[BLOCK_SEPARATE_LAYER1:BLOCK_SEPARATE_LAYER2]
	path := filepath.Join(getBlocksPath(volumeID), blockSubDirLayer1, blockSubDirLayer2)
	fileName := checksum + ".blk"

	return filepath.Join(path, fileName)
}

func getSnapshots(volumeID string, driver ObjectStoreDriver) (map[string]bool, error) {
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
