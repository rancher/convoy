package objectstore

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/rancher/convoy/util"
)

type Volume struct {
	Name           string
	Driver         string
	Size           int64
	CreatedTime    string
	LastBackupName string
}

type Snapshot struct {
	Name        string
	CreatedTime string
}

type Backup struct {
	Name              string
	Driver            string
	VolumeName        string
	SnapshotName      string
	SnapshotCreatedAt string
	CreatedTime       string

	Blocks     []BlockMapping `json:",omitempty"`
	SingleFile BackupFile     `json:",omitempty"`
}

func addVolume(volume *Volume, driver ObjectStoreDriver) error {
	if volumeExists(volume.Name, driver) {
		return nil
	}

	if err := saveVolume(volume, driver); err != nil {
		log.Error("Fail add volume ", volume.Name)
		return err
	}
	log.Debug("Added objectstore volume ", volume.Name)

	return nil
}

func removeVolume(volumeName string, driver ObjectStoreDriver) error {
	if !volumeExists(volumeName, driver) {
		return fmt.Errorf("Volume %v doesn't exist in objectstore", volumeName)
	}

	volumeDir := getVolumePath(volumeName)
	if err := driver.Remove(volumeDir); err != nil {
		return err
	}
	log.Debug("Removed volume directory in objectstore: ", volumeDir)
	log.Debug("Removed objectstore volume ", volumeName)

	return nil
}

func encodeBackupURL(backupName, volumeName, destURL string) string {
	v := url.Values{}
	v.Add("volume", volumeName)
	v.Add("backup", backupName)
	return destURL + "?" + v.Encode()
}

func decodeBackupURL(backupURL string) (string, string, error) {
	u, err := url.Parse(backupURL)
	if err != nil {
		return "", "", err
	}
	v := u.Query()
	volumeName := v.Get("volume")
	backupName := v.Get("backup")
	if !util.ValidateName(volumeName) || !util.ValidateName(backupName) {
		return "", "", fmt.Errorf("Invalid name parsed, got %v and %v", backupName, volumeName)
	}
	return backupName, volumeName, nil
}

func addListVolume(resp map[string]map[string]string, volumeName string, driver ObjectStoreDriver, storageDriverName string) error {
	if volumeName == "" {
		return fmt.Errorf("Invalid empty volume Name")
	}

	backupNames, err := getBackupNamesForVolume(volumeName, driver)
	if err != nil {
		return err
	}

	volume, err := loadVolume(volumeName, driver)
	if err != nil {
		return err
	}
	//Skip any volumes not owned by specified storage driver
	if volume.Driver != storageDriverName {
		return nil
	}

	for _, backupName := range backupNames {
		backup, err := loadBackup(backupName, volumeName, driver)
		if err != nil {
			return err
		}
		r := fillBackupInfo(backup, volume, driver.GetURL())
		resp[r["BackupURL"]] = r
	}
	return nil
}

func List(volumeName, destURL, endpointURL, storageDriverName string,accesskey string, secretkey string) (map[string]map[string]string, error) {
	driver, err := GetObjectStoreDriver(destURL, endpointURL, accesskey, secretkey)
	if err != nil {
		return nil, err
	}
	resp := make(map[string]map[string]string)
	if volumeName != "" {
		if err = addListVolume(resp, volumeName, driver, storageDriverName); err != nil {
			return nil, err
		}
	} else {
		volumeNames, err := getVolumeNames(driver)
		if err != nil {
			return nil, err
		}
		for _, volumeName := range volumeNames {
			if err := addListVolume(resp, volumeName, driver, storageDriverName); err != nil {
				return nil, err
			}
		}
	}
	return resp, nil
}

func fillBackupInfo(backup *Backup, volume *Volume, destURL string) map[string]string {
	return map[string]string{
		"BackupName":        backup.Name,
		"BackupURL":         encodeBackupURL(backup.Name, backup.VolumeName, destURL),
		"DriverName":        volume.Driver,
		"VolumeName":        backup.VolumeName,
		"VolumeSize":        strconv.FormatInt(volume.Size, 10),
		"VolumeCreatedAt":   volume.CreatedTime,
		"SnapshotName":      backup.SnapshotName,
		"SnapshotCreatedAt": backup.SnapshotCreatedAt,
		"CreatedTime":       backup.CreatedTime,
	}
}

func GetBackupInfo(backupURL, endpointURL string,accesskey string, secretkey string) (map[string]string, error) {
	driver, err := GetObjectStoreDriver(backupURL, endpointURL, accesskey, secretkey)
	if err != nil {
		return nil, err
	}
	backupName, volumeName, err := decodeBackupURL(backupURL)
	if err != nil {
		return nil, err
	}

	volume, err := loadVolume(volumeName, driver)
	if err != nil {
		return nil, err
	}

	backup, err := loadBackup(backupName, volumeName, driver)
	if err != nil {
		return nil, err
	}
	return fillBackupInfo(backup, volume, driver.GetURL()), nil
}

func LoadVolume(backupURL, endpointURL string,accesskey string, secretkey string) (*Volume, error) {
	_, volumeName, err := decodeBackupURL(backupURL)
	if err != nil {
		return nil, err
	}
	driver, err := GetObjectStoreDriver(backupURL, endpointURL,accesskey, secretkey)
	if err != nil {
		return nil, err
	}
	return loadVolume(volumeName, driver)
}
