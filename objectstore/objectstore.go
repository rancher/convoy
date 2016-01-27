package objectstore

import (
	"fmt"
	"github.com/rancher/convoy/util"
	"net/url"
	"strconv"
)

type Volume struct {
	UUID           string
	Name           string
	Driver         string
	Size           int64
	CreatedTime    string
	LastBackupUUID string
}

type Snapshot struct {
	UUID        string
	Name        string
	CreatedTime string
}

type Backup struct {
	UUID              string
	Driver            string
	VolumeUUID        string
	SnapshotUUID      string
	SnapshotName      string
	SnapshotCreatedAt string
	CreatedTime       string

	Blocks     []BlockMapping `json:",omitempty"`
	SingleFile BackupFile     `json:",omitempty"`
}

func addVolume(volume *Volume, driver ObjectStoreDriver) error {
	if volumeExists(volume.UUID, driver) {
		return nil
	}

	if err := saveVolume(volume, driver); err != nil {
		log.Error("Fail add volume ", volume.UUID)
		return err
	}
	log.Debug("Added objectstore volume ", volume.UUID)

	return nil
}

func removeVolume(volumeUUID string, driver ObjectStoreDriver) error {
	if !volumeExists(volumeUUID, driver) {
		return fmt.Errorf("Volume %v doesn't exist in objectstore", volumeUUID)
	}

	volumeDir := getVolumePath(volumeUUID)
	if err := driver.Remove(volumeDir); err != nil {
		return err
	}
	log.Debug("Removed volume directory in objectstore: ", volumeDir)
	log.Debug("Removed objectstore volume ", volumeUUID)

	return nil
}

func encodeBackupURL(backupUUID, volumeUUID, destURL string) string {
	v := url.Values{}
	v.Add("volume", volumeUUID)
	v.Add("backup", backupUUID)
	return destURL + "?" + v.Encode()
}

func decodeBackupURL(backupURL string) (string, string, error) {
	u, err := url.Parse(backupURL)
	if err != nil {
		return "", "", err
	}
	v := u.Query()
	volumeUUID := v.Get("volume")
	backupUUID := v.Get("backup")
	if !util.ValidateUUID(volumeUUID) || !util.ValidateUUID(backupUUID) {
		return "", "", fmt.Errorf("Invalid UUID parsed, got %v and %v", backupUUID, volumeUUID)
	}
	return backupUUID, volumeUUID, nil
}

func addListVolume(resp map[string]map[string]string, volumeUUID string, driver ObjectStoreDriver, storageDriverName string) error {
	if volumeUUID == "" {
		return fmt.Errorf("Invalid empty volume UUID")
	}

	backupUUIDs, err := getBackupUUIDsForVolume(volumeUUID, driver)
	if err != nil {
		return err
	}

	volume, err := loadVolume(volumeUUID, driver)
	if err != nil {
		return err
	}
	//Skip any volumes not owned by specified storage driver
	if volume.Driver != storageDriverName {
		return nil
	}

	for _, backupUUID := range backupUUIDs {
		backup, err := loadBackup(backupUUID, volumeUUID, driver)
		if err != nil {
			return err
		}
		r := fillBackupInfo(backup, volume, driver.GetURL())
		resp[r["BackupURL"]] = r
	}
	return nil
}

func List(volumeUUID, destURL, storageDriverName string) (map[string]map[string]string, error) {
	driver, err := GetObjectStoreDriver(destURL)
	if err != nil {
		return nil, err
	}
	resp := make(map[string]map[string]string)
	if volumeUUID != "" {
		if err = addListVolume(resp, volumeUUID, driver, storageDriverName); err != nil {
			return nil, err
		}
	} else {
		volumeUUIDs, err := getVolumeUUIDs(driver)
		if err != nil {
			return nil, err
		}
		for _, volumeUUID := range volumeUUIDs {
			if err := addListVolume(resp, volumeUUID, driver, storageDriverName); err != nil {
				return nil, err
			}
		}
	}
	return resp, nil
}

func fillBackupInfo(backup *Backup, volume *Volume, destURL string) map[string]string {
	return map[string]string{
		"BackupUUID":        backup.UUID,
		"BackupURL":         encodeBackupURL(backup.UUID, backup.VolumeUUID, destURL),
		"DriverName":        volume.Driver,
		"VolumeUUID":        backup.VolumeUUID,
		"VolumeName":        volume.Name,
		"VolumeSize":        strconv.FormatInt(volume.Size, 10),
		"VolumeCreatedAt":   volume.CreatedTime,
		"SnapshotUUID":      backup.SnapshotUUID,
		"SnapshotName":      backup.SnapshotName,
		"SnapshotCreatedAt": backup.SnapshotCreatedAt,
		"CreatedTime":       backup.CreatedTime,
	}
}

func GetBackupInfo(backupURL string) (map[string]string, error) {
	driver, err := GetObjectStoreDriver(backupURL)
	if err != nil {
		return nil, err
	}
	backupUUID, volumeUUID, err := decodeBackupURL(backupURL)
	if err != nil {
		return nil, err
	}

	volume, err := loadVolume(volumeUUID, driver)
	if err != nil {
		return nil, err
	}

	backup, err := loadBackup(backupUUID, volumeUUID, driver)
	if err != nil {
		return nil, err
	}
	return fillBackupInfo(backup, volume, driver.GetURL()), nil
}

func LoadVolume(backupURL string) (*Volume, error) {
	_, volumeUUID, err := decodeBackupURL(backupURL)
	if err != nil {
		return nil, err
	}
	driver, err := GetObjectStoreDriver(backupURL)
	if err != nil {
		return nil, err
	}
	return loadVolume(volumeUUID, driver)
}
