package objectstore

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/metadata"
	"github.com/rancher/convoy/util"
	"io"
	"os"
	"path/filepath"

	. "github.com/rancher/convoy/logging"
)

type BlockMapping struct {
	Offset        int64
	BlockChecksum string
}

type DeltaBlockBackupOperations interface {
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
}

const (
	DEFAULT_BLOCK_SIZE = 2097152

	BLOCKS_DIRECTORY      = "blocks"
	BLOCK_SEPARATE_LAYER1 = 2
	BLOCK_SEPARATE_LAYER2 = 4
)

func CreateDeltaBlockBackup(volume *Volume, snapshot *Snapshot, destURL string, deltaOps DeltaBlockBackupOperations) (string, error) {
	if deltaOps == nil {
		return "", fmt.Errorf("Missing DeltaBlockBackupOperations")
	}

	bsDriver, err := GetObjectStoreDriver(destURL)
	if err != nil {
		return "", err
	}

	if err := addVolume(volume, bsDriver); err != nil {
		return "", err
	}

	// Update volume from objectstore
	volume, err = loadVolume(volume.Name, bsDriver)
	if err != nil {
		return "", err
	}

	lastBackupName := volume.LastBackupName

	var lastSnapshotName string
	var lastBackup *Backup
	if lastBackupName != "" {
		lastBackup, err = loadBackup(lastBackupName, volume.Name, bsDriver)
		if err != nil {
			return "", err
		}

		lastSnapshotName = lastBackup.SnapshotName
		if lastSnapshotName == snapshot.Name {
			//Generate full snapshot if the snapshot has been backed up last time
			lastSnapshotName = ""
			log.Debug("Would create full snapshot metadata")
		} else if !deltaOps.HasSnapshot(lastSnapshotName, volume.Name) {
			// It's possible that the snapshot in objectstore doesn't exist
			// in local storage
			lastSnapshotName = ""
			log.WithFields(logrus.Fields{
				LOG_FIELD_REASON:   LOG_REASON_FALLBACK,
				LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
				LOG_FIELD_SNAPSHOT: lastSnapshotName,
				LOG_FIELD_VOLUME:   volume.Name,
			}).Debug("Cannot find last snapshot in local storage, would process with full backup")
		}
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:        LOG_REASON_START,
		LOG_FIELD_OBJECT:        LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_EVENT:         LOG_EVENT_COMPARE,
		LOG_FIELD_SNAPSHOT:      snapshot.Name,
		LOG_FIELD_LAST_SNAPSHOT: lastSnapshotName,
	}).Debug("Generating snapshot changed blocks metadata")

	if err := deltaOps.OpenSnapshot(snapshot.Name, volume.Name); err != nil {
		return "", err
	}
	defer deltaOps.CloseSnapshot(snapshot.Name, volume.Name)

	delta, err := deltaOps.CompareSnapshot(snapshot.Name, lastSnapshotName, volume.Name)
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
		LOG_FIELD_SNAPSHOT:      snapshot.Name,
		LOG_FIELD_LAST_SNAPSHOT: lastSnapshotName,
	}).Debug("Generated snapshot changed blocks metadata")

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshot.Name,
	}).Debug("Creating backup")

	deltaBackup := &Backup{
		Name:         util.GenerateName("backup"),
		VolumeName:   volume.Name,
		SnapshotName: snapshot.Name,
		Blocks:       []BlockMapping{},
	}
	mCounts := len(delta.Mappings)
	for m, d := range delta.Mappings {
		block := make([]byte, DEFAULT_BLOCK_SIZE)
		blkCounts := d.Size / delta.BlockSize
		for i := int64(0); i < blkCounts; i++ {
			offset := d.Offset + i*delta.BlockSize
			log.Debugf("Backup for %v: segment %v/%v, blocks %v/%v", snapshot.Name, m+1, mCounts, i+1, blkCounts)
			err := deltaOps.ReadSnapshot(snapshot.Name, volume.Name, offset, block)
			if err != nil {
				return "", err
			}
			checksum := util.GetChecksum(block)
			blkFile := getBlockFilePath(volume.Name, checksum)
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
		LOG_FIELD_SNAPSHOT: snapshot.Name,
	}).Debug("Created snapshot changed blocks")

	backup := mergeSnapshotMap(deltaBackup, lastBackup)
	backup.SnapshotName = snapshot.Name
	backup.SnapshotCreatedAt = snapshot.CreatedTime
	backup.CreatedTime = util.Now()

	if err := saveBackup(backup, bsDriver); err != nil {
		return "", err
	}

	volume.LastBackupName = backup.Name
	if err := saveVolume(volume, bsDriver); err != nil {
		return "", err
	}

	return encodeBackupURL(backup.Name, volume.Name, destURL), nil
}

func mergeSnapshotMap(deltaBackup, lastBackup *Backup) *Backup {
	if lastBackup == nil {
		return deltaBackup
	}
	backup := &Backup{
		Name:         deltaBackup.Name,
		VolumeName:   deltaBackup.VolumeName,
		SnapshotName: deltaBackup.SnapshotName,
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

func RestoreDeltaBlockBackup(backupURL, volDevName string) error {
	bsDriver, err := GetObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	srcBackupName, srcVolumeName, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	if _, err := loadVolume(srcVolumeName, bsDriver); err != nil {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME:     srcVolumeName,
			LOG_FIELD_BACKUP_URL: backupURL,
		}, "Volume doesn't exist in objectstore: %v", err)
	}

	volDev, err := os.Create(volDevName)
	if err != nil {
		return err
	}
	defer volDev.Close()

	backup, err := loadBackup(srcBackupName, srcVolumeName, bsDriver)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_START,
		LOG_FIELD_EVENT:       LOG_EVENT_RESTORE,
		LOG_FIELD_OBJECT:      LOG_FIELD_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    srcBackupName,
		LOG_FIELD_ORIN_VOLUME: srcVolumeName,
		LOG_FIELD_VOLUME_DEV:  volDevName,
		LOG_FIELD_BACKUP_URL:  backupURL,
	}).Debug()
	blkCounts := len(backup.Blocks)
	for i, block := range backup.Blocks {
		log.Debugf("Restore for %v: block %v, %v/%v", volDevName, block.BlockChecksum, i+1, blkCounts)
		blkFile := getBlockFilePath(srcVolumeName, block.BlockChecksum)
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

func DeleteDeltaBlockBackup(backupURL string) error {
	bsDriver, err := GetObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	backupName, volumeName, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	v, err := loadVolume(volumeName, bsDriver)
	if err != nil {
		return fmt.Errorf("Cannot find volume %v in objectstore", volumeName, err)
	}

	backup, err := loadBackup(backupName, volumeName, bsDriver)
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

	if backup.Name == v.LastBackupName {
		v.LastBackupName = ""
		if err := saveVolume(v, bsDriver); err != nil {
			return err
		}
	}

	backupNames, err := getBackupNamesForVolume(volumeName, bsDriver)
	if err != nil {
		return err
	}
	if len(backupNames) == 0 {
		log.Debugf("No snapshot existed for the volume %v, removing volume", volumeName)
		if err := removeVolume(volumeName, bsDriver); err != nil {
			log.Warningf("Failed to remove volume %v due to: %v", volumeName, err.Error())
		}
		return nil
	}

	log.Debug("GC started")
	for _, backupName := range backupNames {
		backup, err := loadBackup(backupName, volumeName, bsDriver)
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
		blkFileList = append(blkFileList, getBlockFilePath(volumeName, blk))
		log.Debugf("Found unused blocks %v for volume %v", blk, volumeName)
	}
	if err := bsDriver.Remove(blkFileList...); err != nil {
		return err
	}
	log.Debug("Removed unused blocks for volume ", volumeName)

	log.Debug("GC completed")
	log.Debug("Removed objectstore backup ", backupName)

	return nil
}

func getBlockPath(volumeName string) string {
	return filepath.Join(getVolumePath(volumeName), BLOCKS_DIRECTORY) + "/"
}

func getBlockFilePath(volumeName, checksum string) string {
	blockSubDirLayer1 := checksum[0:BLOCK_SEPARATE_LAYER1]
	blockSubDirLayer2 := checksum[BLOCK_SEPARATE_LAYER1:BLOCK_SEPARATE_LAYER2]
	path := filepath.Join(getBlockPath(volumeName), blockSubDirLayer1, blockSubDirLayer2)
	fileName := checksum + ".blk"

	return filepath.Join(path, fileName)
}
