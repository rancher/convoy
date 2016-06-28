package objectstore

import (
	"fmt"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/util"

	. "github.com/rancher/convoy/logging"
)

const (
	BACKUP_FILES_DIRECTORY = "BackupFiles"
)

type BackupFile struct {
	FilePath string
}

func getSingleFileBackupFilePath(sfBackup *Backup) string {
	backupFileName := sfBackup.Name + ".bak"
	return filepath.Join(getVolumePath(sfBackup.VolumeName), BACKUP_FILES_DIRECTORY, backupFileName)
}

func CreateSingleFileBackup(volume *Volume, snapshot *Snapshot, filePath, destURL string) (string, error) {
	driver, err := GetObjectStoreDriver(destURL)
	if err != nil {
		return "", err
	}

	if err := addVolume(volume, driver); err != nil {
		return "", err
	}

	volume, err = loadVolume(volume.Name, driver)
	if err != nil {
		return "", err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshot.Name,
		LOG_FIELD_FILEPATH: filePath,
	}).Debug("Creating backup")

	backup := &Backup{
		Name:              util.GenerateName("backup"),
		VolumeName:        volume.Name,
		SnapshotName:      snapshot.Name,
		SnapshotCreatedAt: snapshot.CreatedTime,
	}
	backup.SingleFile.FilePath = getSingleFileBackupFilePath(backup)

	if err := driver.Upload(filePath, backup.SingleFile.FilePath); err != nil {
		return "", err
	}

	backup.CreatedTime = util.Now()
	if err := saveBackup(backup, driver); err != nil {
		return "", err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshot.Name,
	}).Debug("Created backup")

	return encodeBackupURL(backup.Name, volume.Name, destURL), nil
}

func RestoreSingleFileBackup(backupURL, path string) (string, error) {
	driver, err := GetObjectStoreDriver(backupURL)
	if err != nil {
		return "", err
	}

	srcBackupName, srcVolumeName, err := decodeBackupURL(backupURL)
	if err != nil {
		return "", err
	}

	if _, err := loadVolume(srcVolumeName, driver); err != nil {
		return "", generateError(logrus.Fields{
			LOG_FIELD_VOLUME:     srcVolumeName,
			LOG_FIELD_BACKUP_URL: backupURL,
		}, "Volume doesn't exist in objectstore: %v", err)
	}

	backup, err := loadBackup(srcBackupName, srcVolumeName, driver)
	if err != nil {
		return "", err
	}

	dstFile := filepath.Join(path, filepath.Base(backup.SingleFile.FilePath))
	if err := driver.Download(backup.SingleFile.FilePath, dstFile); err != nil {
		return "", err
	}

	return dstFile, nil
}

func DeleteSingleFileBackup(backupURL string) error {
	driver, err := GetObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	backupName, volumeName, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	_, err = loadVolume(volumeName, driver)
	if err != nil {
		return fmt.Errorf("Cannot find volume %v in objectstore", volumeName, err)
	}

	backup, err := loadBackup(backupName, volumeName, driver)
	if err != nil {
		return err
	}

	if err := driver.Remove(backup.SingleFile.FilePath); err != nil {
		return err
	}

	if err := removeBackup(backup, driver); err != nil {
		return err
	}

	return nil
}
