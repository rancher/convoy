package objectstore

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/drivers"
	"github.com/rancher/rancher-volume/util"
	"io"
	"net/url"
	"os"

	. "github.com/rancher/rancher-volume/logging"
)

const (
	DEFAULT_BLOCK_SIZE = 2097152
)

type InitFunc func(destURL string) (ObjectStoreDriver, error)

type ObjectStoreDriver interface {
	Kind() string
	FileExists(filePath string) bool
	FileSize(filePath string) int64
	Remove(names ...string) error
	Read(src string) (io.ReadCloser, error) // Caller needs to close
	Write(dst string, rs io.ReadSeeker) error
	List(path string) ([]string, error)
	Upload(src, dst string) error
	Download(src, dst string) error
}

type Volume struct {
	UUID           string
	Name           string
	Size           int64
	FileSystem     string
	CreatedTime    string
	LastBackupUUID string
}

type ObjectStore struct {
	UUID string
	Kind string
}

type BlockMapping struct {
	Offset        int64
	BlockChecksum string
}

type Backup struct {
	UUID         string
	VolumeUUID   string
	SnapshotUUID string
	Blocks       []BlockMapping
}

var (
	initializers map[string]InitFunc
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "objectstore"})
)

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("objectstore", fields, format, v)
}

func init() {
	initializers = make(map[string]InitFunc)
}

func RegisterDriver(kind string, initFunc InitFunc) error {
	if _, exists := initializers[kind]; exists {
		return fmt.Errorf("%s has already been registered", kind)
	}
	initializers[kind] = initFunc
	return nil
}

func getObjectStoreDriver(destURL string) (ObjectStoreDriver, error) {
	u, err := url.Parse(destURL)
	if err != nil {
		return nil, err
	}
	if _, exists := initializers[u.Scheme]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", u.Scheme)
	}
	return initializers[u.Scheme](destURL)
}

func VolumeExists(volumeUUID, destURL string) bool {
	driver, err := getObjectStoreDriver(destURL)
	if err != nil {
		return false
	}

	return driver.FileExists(getVolumeFilePath(volumeUUID))
}

func addVolume(volume *Volume, driver ObjectStoreDriver) error {
	if volumeExists(volume.UUID, driver) {
		return nil
	}

	if err := saveVolumeConfig(volume, driver); err != nil {
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

func BackupSnapshot(volumeDesc *Volume, snapshotUUID, destURL string, sDriver drivers.Driver) (string, error) {
	bsDriver, err := getObjectStoreDriver(destURL)
	if err != nil {
		return "", err
	}

	if err := addVolume(volumeDesc, bsDriver); err != nil {
		return "", err
	}

	volume, err := loadVolumeConfig(volumeDesc.UUID, bsDriver)
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
		if lastSnapshotUUID == snapshotUUID {
			//Generate full snapshot if the snapshot has been backed up last time
			lastSnapshotUUID = ""
			log.Debug("Would create full snapshot metadata")
		} else if !sDriver.HasSnapshot(lastSnapshotUUID, volume.UUID) {
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
		LOG_FIELD_SNAPSHOT:      snapshotUUID,
		LOG_FIELD_LAST_SNAPSHOT: lastSnapshotUUID,
	}).Debug("Generating snapshot changed blocks metadata")
	delta, err := sDriver.CompareSnapshot(snapshotUUID, lastSnapshotUUID, volume.UUID)
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
		LOG_FIELD_SNAPSHOT:      snapshotUUID,
		LOG_FIELD_LAST_SNAPSHOT: lastSnapshotUUID,
	}).Debug("Generated snapshot changed blocks metadata")

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotUUID,
	}).Debug("Creating backup")

	deltaBackup := &Backup{
		UUID:         uuid.New(),
		VolumeUUID:   volume.UUID,
		SnapshotUUID: snapshotUUID,
		Blocks:       []BlockMapping{},
	}
	if err := sDriver.OpenSnapshot(snapshotUUID, volume.UUID); err != nil {
		return "", err
	}
	defer sDriver.CloseSnapshot(snapshotUUID, volume.UUID)
	for _, d := range delta.Mappings {
		block := make([]byte, DEFAULT_BLOCK_SIZE)
		for i := int64(0); i < d.Size/delta.BlockSize; i++ {
			offset := d.Offset + i*delta.BlockSize
			err := sDriver.ReadSnapshot(snapshotUUID, volume.UUID, offset, block)
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
			log.Debugf("Creating new block file at %v", blkFile)
			if err := bsDriver.Write(blkFile, bytes.NewReader(block)); err != nil {
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
		LOG_FIELD_SNAPSHOT: snapshotUUID,
	}).Debug("Created snapshot changed blocks")
	backup := mergeSnapshotMap(deltaBackup, lastBackup)

	if err := saveBackup(backup, bsDriver); err != nil {
		return "", err
	}

	volume.LastBackupUUID = backup.UUID
	if err := saveVolumeConfig(volume, bsDriver); err != nil {
		return "", err
	}

	return encodeBackupURL(volume.UUID, backup.UUID, destURL), nil
}

func encodeBackupURL(volumeUUID, backupUUID, destURL string) string {
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
		return "", "", fmt.Errorf("Invalid UUID parsed")
	}
	return volumeUUID, backupUUID, nil
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

func RestoreSnapshot(backupURL, dstVolumeID string, sDriver drivers.Driver) error {
	bsDriver, err := getObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	srcVolumeID, srcBackupUUID, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	if _, err := loadVolumeConfig(srcVolumeID, bsDriver); err != nil {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME:     srcVolumeID,
			LOG_FIELD_BACKUP_URL: backupURL,
		}, "Volume doesn't exist in objectstore: %v", err)
	}

	volDevName, err := sDriver.GetVolumeDevice(dstVolumeID)
	if err != nil {
		return err
	}
	volDev, err := os.Create(volDevName)
	if err != nil {
		return err
	}
	defer volDev.Close()

	backup, err := loadBackup(srcBackupUUID, srcVolumeID, bsDriver)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_START,
		LOG_FIELD_EVENT:       LOG_EVENT_RESTORE,
		LOG_FIELD_OBJECT:      LOG_FIELD_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    srcBackupUUID,
		LOG_FIELD_ORIN_VOLUME: srcVolumeID,
		LOG_FIELD_VOLUME:      dstVolumeID,
		//LOG_FIELD_OBJECTSTORE: objectstoreID,
	}).Debug()
	for _, block := range backup.Blocks {
		blkFile := getBlockFilePath(srcVolumeID, block.BlockChecksum)
		rc, err := bsDriver.Read(blkFile)
		if err != nil {
			return err
		}
		if _, err := volDev.Seek(block.Offset, 0); err != nil {
			rc.Close()
			return err
		}
		if _, err := io.CopyN(volDev, rc, DEFAULT_BLOCK_SIZE); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}

	return nil
}

func RemoveSnapshot(backupURL string) error {
	bsDriver, err := getObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	volumeUUID, backupUUID, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	v, err := loadVolumeConfig(volumeUUID, bsDriver)
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
		if err := saveVolumeConfig(v, bsDriver); err != nil {
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

func listVolume(volumeUUID, destURL string, driver ObjectStoreDriver) ([]byte, error) {
	resp := api.BackupsResponse{
		Backups: make(map[string]api.BackupResponse),
	}

	backupUUIDs, err := getBackupUUIDsForVolume(volumeUUID, driver)
	if err != nil {
		return nil, err
	}

	for _, backupUUID := range backupUUIDs {
		backup, err := loadBackup(backupUUID, volumeUUID, driver)
		if err != nil {
			return nil, err
		}
		backupResp := api.BackupResponse{
			URL:          encodeBackupURL(backupUUID, volumeUUID, destURL),
			VolumeUUID:   volumeUUID,
			SnapshotUUID: backup.SnapshotUUID,
		}
		resp.Backups[backupUUID] = backupResp
	}
	return api.ResponseOutput(resp)
}

func ListVolume(volumeUUID, destURL string) ([]byte, error) {
	bsDriver, err := getObjectStoreDriver(destURL)
	if err != nil {
		return nil, err
	}
	return listVolume(volumeUUID, destURL, bsDriver)
}
