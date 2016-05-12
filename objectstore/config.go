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
	OBJECTSTORE_BASE       = "convoy-objectstore"
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

func volumeExists(volumeName string, driver ObjectStoreDriver) bool {
	volumeFile := getVolumeFilePath(volumeName)
	return driver.FileExists(volumeFile)
}

func getVolumePath(volumeName string) string {
	name := volumeName
	//Fix short volume name, add '!' at the end which cannot be used in valid name
	for l := len(volumeName); l < 4; l++ {
		name = name + "!"
	}
	volumeLayer1 := name[0:VOLUME_SEPARATE_LAYER1]
	volumeLayer2 := name[VOLUME_SEPARATE_LAYER1:VOLUME_SEPARATE_LAYER2]
	return filepath.Join(OBJECTSTORE_BASE, VOLUME_DIRECTORY, volumeLayer1, volumeLayer2, name)
}

func getVolumeFilePath(volumeName string) string {
	volumePath := getVolumePath(volumeName)
	volumeCfg := VOLUME_CONFIG_FILE
	return filepath.Join(volumePath, volumeCfg)
}

func getVolumeNames(driver ObjectStoreDriver) ([]string, error) {
	names := []string{}

	volumePathBase := filepath.Join(OBJECTSTORE_BASE, VOLUME_DIRECTORY)
	lv1Dirs, err := driver.List(volumePathBase)
	// Directory doesn't exist
	if err != nil {
		return names, nil
	}
	for _, lv1 := range lv1Dirs {
		lv1Path := filepath.Join(volumePathBase, lv1)
		lv2Dirs, err := driver.List(lv1Path)
		if err != nil {
			return nil, err
		}
		for _, lv2 := range lv2Dirs {
			lv2Path := filepath.Join(lv1Path, lv2)
			volumeNames, err := driver.List(lv2Path)
			if err != nil {
				return nil, err
			}
			names = append(names, volumeNames...)
		}
	}
	return names, nil
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

func getBackupNamesForVolume(volumeName string, driver ObjectStoreDriver) ([]string, error) {
	result := []string{}
	fileList, err := driver.List(getBackupPath(volumeName))
	if err != nil {
		// path doesn't exist
		return result, nil
	}
	return util.ExtractNames(fileList, BACKUP_CONFIG_PREFIX, CFG_SUFFIX)
}

func getBackupPath(volumeName string) string {
	return filepath.Join(getVolumePath(volumeName), BACKUP_DIRECTORY) + "/"
}

func getBackupConfigPath(backupName, volumeName string) string {
	path := getBackupPath(volumeName)
	fileName := getBackupConfigName(backupName)
	return filepath.Join(path, fileName)
}

func backupExists(backupName, volumeName string, bsDriver ObjectStoreDriver) bool {
	return bsDriver.FileExists(getBackupConfigPath(backupName, volumeName))
}

func loadBackup(backupName, volumeName string, bsDriver ObjectStoreDriver) (*Backup, error) {
	backup := &Backup{}
	if err := loadConfigInObjectStore(getBackupConfigPath(backupName, volumeName), bsDriver, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

func saveBackup(backup *Backup, bsDriver ObjectStoreDriver) error {
	filePath := getBackupConfigPath(backup.Name, backup.VolumeName)
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
	filePath := getBackupConfigPath(backup.Name, backup.VolumeName)
	if err := bsDriver.Remove(filePath); err != nil {
		return err
	}
	log.Debugf("Removed %v on objectstore", filePath)
	return nil
}
