package objectstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/util"
	"path/filepath"

	. "github.com/rancher/rancher-volume/logging"
)

const (
	OBJECTSTORE_BASE       = "rancher-objectstore"
	VOLUME_SEPARATE_LAYER1 = 2
	VOLUME_SEPARATE_LAYER2 = 4

	VOLUME_DIRECTORY     = "volumes"
	VOLUME_CONFIG_FILE   = "volume.cfg"
	BACKUP_DIRECTORY     = "backups"
	BACKUP_CONFIG_PREFIX = "backup_"

	CFG_SUFFIX = ".cfg"
)

func getBackupConfigName(id string) string {
	return BACKUP_CONFIG_PREFIX + id + CFG_SUFFIX
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

func volumeExists(volumeUUID string, driver ObjectStoreDriver) bool {
	volumeFile := getVolumeFilePath(volumeUUID)
	return driver.FileExists(volumeFile)
}

func getVolumePath(volumeUUID string) string {
	volumeLayer1 := volumeUUID[0:VOLUME_SEPARATE_LAYER1]
	volumeLayer2 := volumeUUID[VOLUME_SEPARATE_LAYER1:VOLUME_SEPARATE_LAYER2]
	return filepath.Join(OBJECTSTORE_BASE, VOLUME_DIRECTORY, volumeLayer1, volumeLayer2, volumeUUID)
}

func getVolumeFilePath(volumeUUID string) string {
	volumePath := getVolumePath(volumeUUID)
	volumeCfg := VOLUME_CONFIG_FILE
	return filepath.Join(volumePath, volumeCfg)
}

func getVolumeUUIDs(driver ObjectStoreDriver) ([]string, error) {
	uuids := []string{}

	volumePathBase := filepath.Join(OBJECTSTORE_BASE, VOLUME_DIRECTORY)
	lv1Dirs, err := driver.List(volumePathBase)
	// Directory doesn't exist
	if err != nil {
		return uuids, nil
	}
	for _, lv1 := range lv1Dirs {
		lv1Path := filepath.Join(volumePathBase, lv1)
		lv2Dirs, err := driver.List(lv1Path)
		if err != nil {
			return nil, err
		}
		for _, lv2 := range lv2Dirs {
			lv2Path := filepath.Join(lv1Path, lv2)
			volumeUUIDs, err := driver.List(lv2Path)
			if err != nil {
				return nil, err
			}
			uuids = append(uuids, volumeUUIDs...)
		}
	}
	return uuids, nil
}

func loadVolume(volumeUUID string, driver ObjectStoreDriver) (*Volume, error) {
	v := &Volume{}
	file := getVolumeFilePath(volumeUUID)
	if err := loadConfigInObjectStore(file, driver, v); err != nil {
		return nil, err
	}
	return v, nil
}

func saveVolume(v *Volume, driver ObjectStoreDriver) error {
	file := getVolumeFilePath(v.UUID)
	if err := saveConfigInObjectStore(file, driver, v); err != nil {
		return err
	}
	return nil
}

func getBackupUUIDsForVolume(volumeUUID string, driver ObjectStoreDriver) ([]string, error) {
	result := []string{}
	fileList, err := driver.List(getBackupPath(volumeUUID))
	if err != nil {
		// path doesn't exist
		return result, nil
	}
	return util.ExtractUUIDs(fileList, BACKUP_CONFIG_PREFIX, CFG_SUFFIX)
}

func getBackupPath(volumeUUID string) string {
	return filepath.Join(getVolumePath(volumeUUID), BACKUP_DIRECTORY) + "/"
}

func getBackupConfigPath(backupUUID, volumeUUID string) string {
	path := getBackupPath(volumeUUID)
	fileName := getBackupConfigName(backupUUID)
	return filepath.Join(path, fileName)
}

func backupExists(backupUUID, volumeUUID string, bsDriver ObjectStoreDriver) bool {
	return bsDriver.FileExists(getBackupConfigPath(backupUUID, volumeUUID))
}

func loadBackup(backupUUID, volumeUUID string, bsDriver ObjectStoreDriver) (*Backup, error) {
	backup := &Backup{}
	if err := loadConfigInObjectStore(getBackupConfigPath(backupUUID, volumeUUID), bsDriver, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

func saveBackup(backup *Backup, bsDriver ObjectStoreDriver) error {
	filePath := getBackupConfigPath(backup.UUID, backup.VolumeUUID)
	if bsDriver.FileExists(filePath) {
		log.Warnf("Snapshot configuration file %v already exists, would remove it\n", filePath)
		if err := bsDriver.Remove(filePath); err != nil {
			return err
		}
	}
	if err := saveConfigInObjectStore(filePath, bsDriver, backup); err != nil {
		return err
	}
	return nil
}

func removeBackup(backup *Backup, bsDriver ObjectStoreDriver) error {
	filePath := getBackupConfigPath(backup.UUID, backup.VolumeUUID)
	if err := bsDriver.Remove(filePath); err != nil {
		return err
	}
	log.Debugf("Removed %v on objectstore", filePath)
	return nil
}
