package objectstore

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/metadata"
	"github.com/rancher/rancher-volume/storagedriver"
	"github.com/rancher/rancher-volume/util"
	"io"
	"net/url"
	"os"
	"strconv"

	. "github.com/rancher/rancher-volume/logging"
)

const (
	DEFAULT_BLOCK_SIZE = 2097152
)

type Volume struct {
	UUID           string
	Name           string
	Driver         string
	Size           int64
	FileSystem     string
	CreatedTime    string
	LastBackupUUID string
}

type Snapshot struct {
	UUID        string
	Name        string
	CreatedTime string
}

type BlockMapping struct {
	Offset        int64
	BlockChecksum string
}

type Backup struct {
	UUID              string
	Driver            string
	VolumeUUID        string
	SnapshotUUID      string
	SnapshotName      string
	SnapshotCreatedAt string
	CreatedTime       string
	Blocks            []BlockMapping
}

type DeltaBlockBackupOperations interface {
	GetVolumeDevice(id string) (string, error)
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
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

func CreateBackup(volumeDesc *Volume, snapshot *Snapshot, destURL string, sDriver storagedriver.StorageDriver) (string, error) {
	deltaOps, ok := sDriver.(DeltaBlockBackupOperations)
	if !ok {
		return "", fmt.Errorf("Driver %s doesn't implemented DeltaBlockBackupOperations interface", sDriver.Name())
	}

	bsDriver, err := GetObjectStoreDriver(destURL)
	if err != nil {
		return "", err
	}

	if err := addVolume(volumeDesc, bsDriver); err != nil {
		return "", err
	}

	volume, err := loadVolume(volumeDesc.UUID, bsDriver)
	if err != nil {
		return "", err
	}

	lastBackupUUID := volume.LastBackupUUID

	var lastSnapshotUUID string
	var lastBackup *Backup
	if lastBackupUUID != "" {
		lastBackup, err = loadBackup(lastBackupUUID, volume.UUID, bsDriver)
		if err != nil {
			return "", err
		}

		lastSnapshotUUID = lastBackup.SnapshotUUID
		if lastSnapshotUUID == snapshot.UUID {
			//Generate full snapshot if the snapshot has been backed up last time
			lastSnapshotUUID = ""
			log.Debug("Would create full snapshot metadata")
		} else if !deltaOps.HasSnapshot(lastSnapshotUUID, volume.UUID) {
			// It's possible that the snapshot in objectstore doesn't exist
			// in local storage
			lastSnapshotUUID = ""
			log.WithFields(logrus.Fields{
				LOG_FIELD_REASON:   LOG_REASON_FALLBACK,
				LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
				LOG_FIELD_SNAPSHOT: lastSnapshotUUID,
				LOG_FIELD_VOLUME:   volume.UUID,
			}).Debug("Cannot find last snapshot in local storage, would process with full backup")
		}
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:        LOG_REASON_START,
		LOG_FIELD_OBJECT:        LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_EVENT:         LOG_EVENT_COMPARE,
		LOG_FIELD_SNAPSHOT:      snapshot.UUID,
		LOG_FIELD_LAST_SNAPSHOT: lastSnapshotUUID,
	}).Debug("Generating snapshot changed blocks metadata")
	delta, err := deltaOps.CompareSnapshot(snapshot.UUID, lastSnapshotUUID, volume.UUID)
	if err != nil {
		return "", err
	}
	if delta.BlockSize != DEFAULT_BLOCK_SIZE {
		return "", fmt.Errorf("Currently doesn't support different block sizes driver other than %v", DEFAULT_BLOCK_SIZE)
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:        LOG_REASON_COMPLETE,
		LOG_FIELD_OBJECT:        LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_EVENT:         LOG_EVENT_COMPARE,
		LOG_FIELD_SNAPSHOT:      snapshot.UUID,
		LOG_FIELD_LAST_SNAPSHOT: lastSnapshotUUID,
	}).Debug("Generated snapshot changed blocks metadata")

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshot.UUID,
	}).Debug("Creating backup")

	deltaBackup := &Backup{
		UUID:         uuid.New(),
		VolumeUUID:   volume.UUID,
		SnapshotUUID: snapshot.UUID,
		Blocks:       []BlockMapping{},
	}
	if err := deltaOps.OpenSnapshot(snapshot.UUID, volume.UUID); err != nil {
		return "", err
	}
	defer deltaOps.CloseSnapshot(snapshot.UUID, volume.UUID)
	for _, d := range delta.Mappings {
		block := make([]byte, DEFAULT_BLOCK_SIZE)
		for i := int64(0); i < d.Size/delta.BlockSize; i++ {
			offset := d.Offset + i*delta.BlockSize
			err := deltaOps.ReadSnapshot(snapshot.UUID, volume.UUID, offset, block)
			if err != nil {
				return "", err
			}
			checksum := util.GetChecksum(block)
			blkFile := getBlockFilePath(volume.UUID, checksum)
			if bsDriver.FileSize(blkFile) >= 0 {
				blockMapping := BlockMapping{
					Offset:        offset,
					BlockChecksum: checksum,
				}
				deltaBackup.Blocks = append(deltaBackup.Blocks, blockMapping)
				log.Debugf("Found existed block match at %v", blkFile)
				continue
			}

			rs, err := util.CompressData(block)
			if err != nil {
				return "", err
			}

			log.Debugf("Creating new block file at %v", blkFile)
			if err := bsDriver.Write(blkFile, rs); err != nil {
				return "", err
			}
			log.Debugf("Created new block file at %v", blkFile)

			blockMapping := BlockMapping{
				Offset:        offset,
				BlockChecksum: checksum,
			}
			deltaBackup.Blocks = append(deltaBackup.Blocks, blockMapping)
		}
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshot.UUID,
	}).Debug("Created snapshot changed blocks")

	backup := mergeSnapshotMap(deltaBackup, lastBackup)
	backup.SnapshotName = snapshot.Name
	backup.SnapshotCreatedAt = snapshot.CreatedTime
	backup.CreatedTime = util.Now()

	if err := saveBackup(backup, bsDriver); err != nil {
		return "", err
	}

	volume.LastBackupUUID = backup.UUID
	if err := saveVolume(volume, bsDriver); err != nil {
		return "", err
	}

	return encodeBackupURL(backup.UUID, volume.UUID, destURL), nil
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

func mergeSnapshotMap(deltaBackup, lastBackup *Backup) *Backup {
	if lastBackup == nil {
		return deltaBackup
	}
	backup := &Backup{
		UUID:         deltaBackup.UUID,
		VolumeUUID:   deltaBackup.VolumeUUID,
		SnapshotUUID: deltaBackup.SnapshotUUID,
		Blocks:       []BlockMapping{},
	}
	var d, l int
	for d, l = 0, 0; d < len(deltaBackup.Blocks) && l < len(lastBackup.Blocks); {
		dB := deltaBackup.Blocks[d]
		lB := lastBackup.Blocks[l]
		if dB.Offset == lB.Offset {
			backup.Blocks = append(backup.Blocks, dB)
			d++
			l++
		} else if dB.Offset < lB.Offset {
			backup.Blocks = append(backup.Blocks, dB)
			d++
		} else {
			//dB.Offset > lB.offset
			backup.Blocks = append(backup.Blocks, lB)
			l++
		}
	}

	if d == len(deltaBackup.Blocks) {
		backup.Blocks = append(backup.Blocks, lastBackup.Blocks[l:]...)
	} else {
		backup.Blocks = append(backup.Blocks, deltaBackup.Blocks[d:]...)
	}

	return backup
}

func RestoreBackup(backupURL, dstVolumeUUID string, sDriver storagedriver.StorageDriver) error {
	deltaOps, ok := sDriver.(DeltaBlockBackupOperations)
	if !ok {
		return fmt.Errorf("Driver %s doesn't implemented DeltaBlockBackupOperations interface", sDriver.Name())
	}

	bsDriver, err := GetObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	srcBackupUUID, srcVolumeUUID, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	if _, err := loadVolume(srcVolumeUUID, bsDriver); err != nil {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME:     srcVolumeUUID,
			LOG_FIELD_BACKUP_URL: backupURL,
		}, "Volume doesn't exist in objectstore: %v", err)
	}

	volDevName, err := deltaOps.GetVolumeDevice(dstVolumeUUID)
	if err != nil {
		return err
	}
	volDev, err := os.Create(volDevName)
	if err != nil {
		return err
	}
	defer volDev.Close()

	backup, err := loadBackup(srcBackupUUID, srcVolumeUUID, bsDriver)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_START,
		LOG_FIELD_EVENT:       LOG_EVENT_RESTORE,
		LOG_FIELD_OBJECT:      LOG_FIELD_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    srcBackupUUID,
		LOG_FIELD_ORIN_VOLUME: srcVolumeUUID,
		LOG_FIELD_VOLUME:      dstVolumeUUID,
		LOG_FIELD_BACKUP_URL:  backupURL,
	}).Debug()
	for _, block := range backup.Blocks {
		blkFile := getBlockFilePath(srcVolumeUUID, block.BlockChecksum)
		rc, err := bsDriver.Read(blkFile)
		if err != nil {
			return err
		}
		r, err := util.DecompressAndVerify(rc, block.BlockChecksum)
		rc.Close()
		if err != nil {
			return err
		}
		if _, err := volDev.Seek(block.Offset, 0); err != nil {
			return err
		}
		if _, err := io.CopyN(volDev, r, DEFAULT_BLOCK_SIZE); err != nil {
			return err
		}
	}

	return nil
}

func DeleteBackup(backupURL string) error {
	bsDriver, err := GetObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	backupUUID, volumeUUID, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	v, err := loadVolume(volumeUUID, bsDriver)
	if err != nil {
		return fmt.Errorf("Cannot find volume %v in objectstore", volumeUUID, err)
	}

	backup, err := loadBackup(backupUUID, volumeUUID, bsDriver)
	if err != nil {
		return err
	}
	discardBlockSet := make(map[string]bool)
	for _, blk := range backup.Blocks {
		discardBlockSet[blk.BlockChecksum] = true
	}
	discardBlockCounts := len(discardBlockSet)

	if err := removeBackup(backup, bsDriver); err != nil {
		return err
	}

	if backup.UUID == v.LastBackupUUID {
		v.LastBackupUUID = ""
		if err := saveVolume(v, bsDriver); err != nil {
			return err
		}
	}

	backupUUIDs, err := getBackupUUIDsForVolume(volumeUUID, bsDriver)
	if err != nil {
		return err
	}
	if len(backupUUIDs) == 0 {
		log.Debugf("No snapshot existed for the volume %v, removing volume", volumeUUID)
		if err := removeVolume(volumeUUID, bsDriver); err != nil {
			log.Warningf("Failed to remove volume %v due to: %v", volumeUUID, err.Error())
		}
		return nil
	}

	log.Debug("GC started")
	for _, backupUUID := range backupUUIDs {
		backup, err := loadBackup(backupUUID, volumeUUID, bsDriver)
		if err != nil {
			return err
		}
		for _, blk := range backup.Blocks {
			if _, exists := discardBlockSet[blk.BlockChecksum]; exists {
				delete(discardBlockSet, blk.BlockChecksum)
				discardBlockCounts--
				if discardBlockCounts == 0 {
					break
				}
			}
		}
		if discardBlockCounts == 0 {
			break
		}
	}

	var blkFileList []string
	for blk := range discardBlockSet {
		blkFileList = append(blkFileList, getBlockFilePath(volumeUUID, blk))
		log.Debugf("Found unused blocks %v for volume %v", blk, volumeUUID)
	}
	if err := bsDriver.Remove(blkFileList...); err != nil {
		return err
	}
	log.Debug("Removed unused blocks for volume ", volumeUUID)

	log.Debug("GC completed")
	log.Debug("Removed objectstore backup ", backupUUID)

	return nil
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
