package objectstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/util"
	"path/filepath"

	. "github.com/rancher/convoy/logging"
)

const (
	OBJECTSTORE_BASE     = "convoy-objectstore"
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

func volumeExists(volumeName string, driver ObjectStoreDriver) bool {
	volumeFile := getVolumeFilePath(volumeName)
	return driver.FileExists(volumeFile)
}

func getVolumePath(volumeName string) string {
	return filepath.Join(OBJECTSTORE_BASE, VOLUME_DIRECTORY, volumeName)
}

func getVolumeFilePath(volumeName string) string {
	volumePath := getVolumePath(volumeName)
	volumeCfg := VOLUME_CONFIG_FILE
	return filepath.Join(volumePath, volumeCfg)
}

func getVolumeNames(driver ObjectStoreDriver) ([]string, error) {
	uuids := []string{}

	volumePathBase := filepath.Join(OBJECTSTORE_BASE, VOLUME_DIRECTORY)
	volumeDirs, err := driver.List(volumePathBase)
	// Directory doesn't exist
	if err != nil {
		return uuids, nil
	}
	for _, volume := range volumeDirs {
		uuids = append(uuids, volume)
	}
	return uuids, nil
}

func loadVolume(volumeName string, driver ObjectStoreDriver) (*Volume, error) {
	v := &Volume{}
	file := getVolumeFilePath(volumeName)
	if err := loadConfigInObjectStore(file, driver, v); err != nil {
		return nil, err
	}
	return v, nil
}

func saveVolume(v *Volume, driver ObjectStoreDriver) error {
	file := getVolumeFilePath(v.Name)
	if err := saveConfigInObjectStore(file, driver, v); err != nil {
		return err
	}
	return nil
}

func getBackupUUIDsForVolume(volumeName string, driver ObjectStoreDriver) ([]string, error) {
	result := []string{}
	fileList, err := driver.List(getBackupPath(volumeName))
	if err != nil {
		// path doesn't exist
		return result, nil
	}
	return util.ExtractUUIDs(fileList, BACKUP_CONFIG_PREFIX, CFG_SUFFIX)
}

func getBackupPath(volumeName string) string {
	return filepath.Join(getVolumePath(volumeName), BACKUP_DIRECTORY) + "/"
}

func getBackupConfigPath(backupUUID, volumeName string) string {
	path := getBackupPath(volumeName)
	fileName := getBackupConfigName(backupUUID)
	return filepath.Join(path, fileName)
}

func backupExists(backupUUID, volumeName string, bsDriver ObjectStoreDriver) bool {
	return bsDriver.FileExists(getBackupConfigPath(backupUUID, volumeName))
}

func loadBackup(backupUUID, volumeName string, bsDriver ObjectStoreDriver) (*Backup, error) {
	backup := &Backup{}
	if err := loadConfigInObjectStore(getBackupConfigPath(backupUUID, volumeName), bsDriver, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

func saveBackup(backup *Backup, bsDriver ObjectStoreDriver) error {
	filePath := getBackupConfigPath(backup.UUID, backup.VolumeName)
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
	filePath := getBackupConfigPath(backup.UUID, backup.VolumeName)
	if err := bsDriver.Remove(filePath); err != nil {
		return err
	}
	log.Debugf("Removed %v on objectstore", filePath)
	return nil
}
